package generator

// Declared-feed coverage gaps (#86): full-content rendering per format, the
// partial last pagination page, write failures surfacing as build errors, and
// the selection/autodiscovery guard branches.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
)

// TestDeclaredFeedFullContentPerFormat: a per-feed full_content override beats
// the site default, and every format carries the rendered HTML body — a reader
// showing an empty article is worse than one showing a summary.
func TestDeclaredFeedFullContentPerFormat(t *testing.T) {
	on := true
	cases := []struct {
		format string
		want   string // the marker proving full content reached the output
	}{
		{"atom", "&lt;p&gt;Full &lt;em&gt;body&lt;/em&gt;"},
		{"rss", "&lt;p&gt;Full &lt;em&gt;body&lt;/em&gt;"},
		{"json", `"content_html"`},
	}
	for _, c := range cases {
		g := feedGen(t)
		g.siteData.Posts[0].Content = "Full *body* text."
		// One post with no excerpt: the summary falls back to stripped content.
		g.siteData.Posts[1].Excerpt = ""
		g.config.Feeds = []models.FeedSpec{{Path: "/f." + c.format, Format: c.format, FullContent: &on}}
		if err := g.generateDeclaredFeeds(); err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		raw, err := os.ReadFile(filepath.Join(g.config.OutputDir, "f."+c.format))
		if err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if !strings.Contains(string(raw), c.want) {
			t.Errorf("%s feed missing full content marker %q:\n%s", c.format, c.want, raw)
		}
	}
}

// TestDeclaredFeedPartialLastPage: 3 items at 2 per page yield a second page
// with the 1 leftover item — not a phantom fourth slot.
func TestDeclaredFeedPartialLastPage(t *testing.T) {
	g := feedGen(t)
	if err := g.writeDeclaredFeed(models.FeedSpec{Path: "/p.xml", Paginate: 2}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(g.config.OutputDir, "p-2.xml"))
	if err != nil {
		t.Fatalf("page 2 missing: %v", err)
	}
	if n := strings.Count(string(second), "<entry>"); n != 1 {
		t.Errorf("partial last page has %d entries, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "p-3.xml")); err == nil {
		t.Error("no third page expected for 3 items at 2 per page")
	}
}

// TestWriteFeedFileErrors: a feed the filesystem refuses is a build error, not
// a silently missing subscription URL.
func TestWriteFeedFileErrors(t *testing.T) {
	g := feedGen(t)
	// Escaping the output root is rejected before any I/O.
	if err := g.writeFeedFile("../evil.xml", "x", 0); err == nil {
		t.Error("a path escaping the output dir must error")
	}
	// A file where the parent directory should be blocks MkdirAll.
	mustWrite(t, filepath.Join(g.config.OutputDir, "sub"), "not a dir")
	if err := g.writeFeedFile("sub/feed.xml", "x", 0); err == nil {
		t.Error("a blocked parent directory must error")
	}
	// A directory sitting at the feed path blocks WriteFile.
	mustMkdir(t, filepath.Join(g.config.OutputDir, "f.xml"))
	if err := g.writeFeedFile("f.xml", "x", 0); err == nil {
		t.Error("a directory at the feed path must error")
	}
	// The same MkdirAll failure inside the pagination loop propagates out.
	if err := g.writeDeclaredFeed(models.FeedSpec{Path: "/sub/feed.xml", Paginate: 1}); err == nil {
		t.Error("a paginated feed hitting a blocked directory must error")
	}
}

// TestSelectFeedPostsGuards: type=page switches the pool, a page with no
// source dir never matches a source filter, and category matching accepts the
// page's direct frontmatter category string.
func TestSelectFeedPostsGuards(t *testing.T) {
	g := feedGen(t)
	g.siteData.Pages = []models.Page{{Slug: "about", Title: "About", Type: "page"}}
	got := g.selectFeedPosts(models.FeedSpec{Type: "page"})
	if len(got) != 1 || got[0].Slug != "about" {
		t.Errorf("type=page pool = %v", feedSlugs(got))
	}
	if pageInSource(models.Page{SourceDir: ""}, "blog") {
		t.Error("a page without a source dir cannot match a source filter")
	}
	if !g.pageInCategories(models.Page{Category: "Releases"}, map[string]bool{"releases": true}) {
		t.Error("the direct frontmatter category string must match")
	}
}

// TestFeedRendererItemDetails: an aggregated item's author lands in Atom and
// JSON, and newestItemUpdate falls back to Published when Updated is unset.
func TestFeedRendererItemDetails(t *testing.T) {
	g := feedGen(t)
	pub := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	page := feedPage{title: "T", selfURL: "https://ex.com/f", altURL: "https://ex.com/",
		full: true,
		items: []externalsource.FeedItem{{
			ID: "id1", URL: "https://ex.com/a/", Title: "A", Author: "Jane",
			ContentHTML: "<p>hi</p>", Published: pub,
		}}}
	atom := renderAtomFeed(g, page)
	if !strings.Contains(atom, "<author><name>Jane</name></author>") {
		t.Errorf("atom entry missing author:\n%s", atom)
	}
	jsonOut := renderJSONFeed(g, page)
	if !strings.Contains(jsonOut, `"name": "Jane"`) || !strings.Contains(jsonOut, `"content_html"`) {
		t.Errorf("json feed missing author/full content:\n%s", jsonOut)
	}
	if got := newestItemUpdate(page.items); !got.Equal(pub) {
		t.Errorf("newestItemUpdate fell back to %v, want the Published date %v", got, pub)
	}
}

// TestFeedAutodiscoverySkipsUnusable: a feed with no path or an unknown format
// must not be advertised — a link with a wrong MIME type or a dead href is
// worse than no link. With nothing usable and no built-in feed, injection is a
// no-op even though feeds are configured.
func TestFeedAutodiscoverySkipsUnusable(t *testing.T) {
	g := feedGen(t)
	g.config.Feed = false
	g.config.Feeds = []models.FeedSpec{
		{Title: "no path"},
		{Path: "/x.xml", Format: "sitemap"},
	}
	if links := g.feedAutodiscoveryLinks(); links != "" {
		t.Errorf("unusable feeds must not be advertised, got:\n%s", links)
	}
	page := `<html><head><title>H</title></head><body>x</body></html>`
	if got := g.injectFeedLinks(page); got != page {
		t.Errorf("empty link set must inject nothing, got:\n%s", got)
	}
}
