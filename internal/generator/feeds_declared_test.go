package generator

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// feedGen builds a generator with posts from two content roots, tagged so the
// selection criteria can be told apart.
func feedGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	g.config.Domain = "ex.com"
	g.siteData.Categories[2] = models.Category{ID: 2, Name: "Releases", Slug: "releases"}
	day := func(n int) time.Time { return time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC) }
	g.siteData.Posts = []models.Page{
		{Slug: "b1", Title: "Blog one", Type: "post", Status: "publish", SourceDir: "blog",
			Date: day(1), Tags: []string{"release", "blog"}, Excerpt: "First blog post.", Categories: []int{2}},
		{Slug: "b2", Title: "Blog two", Type: "post", Status: "publish", SourceDir: "blog/2026",
			Date: day(2), Tags: []string{"blog"}, Excerpt: "Second blog post."},
		{Slug: "d1", Title: "Guide one", Type: "post", Status: "publish", SourceDir: "docs",
			Date: day(3), Tags: []string{"docs"}, Excerpt: "A guide."},
	}
	return g
}

func feedSlugs(posts []models.Page) []string {
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		out = append(out, p.Slug)
	}
	return out
}

// TestSelectFeedPostsBySource covers the headline ask of #86: a feed scoped to a
// content folder, including posts in subdirectories of it.
func TestSelectFeedPostsBySource(t *testing.T) {
	g := feedGen(t)
	got := feedSlugs(g.selectFeedPosts(models.FeedSpec{Source: "blog"}))
	if len(got) != 2 {
		t.Fatalf("source=blog selected %v, want the two blog posts", got)
	}
	// A post in blog/2026 belongs to the blog feed.
	if !strings.Contains(strings.Join(got, ","), "b2") {
		t.Errorf("a post in a subdirectory of the source must be included: %v", got)
	}
	if got := feedSlugs(g.selectFeedPosts(models.FeedSpec{Source: "docs"})); len(got) != 1 || got[0] != "d1" {
		t.Errorf("source=docs selected %v, want [d1]", got)
	}
	// No criteria ⇒ everything.
	if got := g.selectFeedPosts(models.FeedSpec{}); len(got) != 3 {
		t.Errorf("an unfiltered spec must cover every post, got %d", len(got))
	}
}

// TestSelectFeedPostsByTagsAndCategories: several terms in one feed — the thing
// per-term feeds cannot express — and category matching by name or slug.
func TestSelectFeedPostsByTagsAndCategories(t *testing.T) {
	g := feedGen(t)
	if got := feedSlugs(g.selectFeedPosts(models.FeedSpec{Tags: []string{"release"}})); len(got) != 1 || got[0] != "b1" {
		t.Errorf("tags=[release] selected %v, want [b1]", got)
	}
	// Two tags in one feed: any-of.
	if got := g.selectFeedPosts(models.FeedSpec{Tags: []string{"release", "docs"}}); len(got) != 2 {
		t.Errorf("tags=[release,docs] should select both, got %v", feedSlugs(got))
	}
	// Category by slug and by name both work.
	for _, term := range []string{"releases", "Releases"} {
		if got := g.selectFeedPosts(models.FeedSpec{Categories: []string{term}}); len(got) != 1 {
			t.Errorf("categories=[%s] selected %v, want one", term, feedSlugs(got))
		}
	}
	// Criteria combine with AND: blog folder AND the docs tag matches nothing.
	if got := g.selectFeedPosts(models.FeedSpec{Source: "blog", Tags: []string{"docs"}}); len(got) != 0 {
		t.Errorf("AND of source and tags should be empty, got %v", feedSlugs(got))
	}
}

// TestDeclaredFeedFormats writes one feed per format and checks each parses as
// what it claims to be — a feed a reader rejects is worse than no feed.
func TestDeclaredFeedFormats(t *testing.T) {
	g := feedGen(t)
	g.config.Feeds = []models.FeedSpec{
		{Path: "/blog/feed.xml", Title: "Blog", Source: "blog"},
		{Path: "/blog/rss.xml", Title: "Blog", Source: "blog", Format: "rss"},
		{Path: "/all.json", Title: "All", Format: "json"},
	}
	if err := g.generateDeclaredFeeds(); err != nil {
		t.Fatalf("generateDeclaredFeeds: %v", err)
	}

	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(g.config.OutputDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
		return string(b)
	}

	atom := read("blog/feed.xml")
	if err := xml.Unmarshal([]byte(atom), new(struct{})); err != nil {
		t.Errorf("atom feed is not valid XML: %v", err)
	}
	if !strings.Contains(atom, `xmlns="http://www.w3.org/2005/Atom"`) {
		t.Error("atom feed missing its namespace")
	}
	if strings.Count(atom, "<entry>") != 2 {
		t.Errorf("atom feed should hold the two blog posts, got %d", strings.Count(atom, "<entry>"))
	}

	rss := read("blog/rss.xml")
	if err := xml.Unmarshal([]byte(rss), new(struct{})); err != nil {
		t.Errorf("rss feed is not valid XML: %v", err)
	}
	for _, want := range []string{`<rss version="2.0"`, "<channel>", "<pubDate>", "isPermaLink=\"false\""} {
		if !strings.Contains(rss, want) {
			t.Errorf("rss feed missing %q", want)
		}
	}
	// RSS dates are RFC 1123Z; an RFC 3339 timestamp is ignored by strict readers.
	if strings.Contains(rss, "<pubDate>2026-01") {
		t.Error("rss pubDate must be RFC 1123Z, not RFC 3339")
	}

	var doc struct {
		Version string `json:"version"`
		Items   []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(read("all.json")), &doc); err != nil {
		t.Fatalf("json feed is not valid JSON: %v", err)
	}
	if !strings.HasSuffix(doc.Version, "/1.1") {
		t.Errorf("json feed version = %q, want JSON Feed 1.1", doc.Version)
	}
	if len(doc.Items) != 3 {
		t.Errorf("json feed should hold every post, got %d", len(doc.Items))
	}
}

// TestDeclaredFeedItemCapAndUnknownFormat: per-feed item cap overrides the site
// default, and an unsupported format is a build error naming the alternatives
// rather than a silently empty file.
func TestDeclaredFeedItemCapAndUnknownFormat(t *testing.T) {
	g := feedGen(t)
	one := 1
	g.config.Feeds = []models.FeedSpec{{Path: "/x.xml", Items: &one}}
	if err := g.generateDeclaredFeeds(); err != nil {
		t.Fatalf("generateDeclaredFeeds: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(g.config.OutputDir, "x.xml"))
	if n := strings.Count(string(b), "<entry>"); n != 1 {
		t.Errorf("items: 1 should cap the feed at one entry, got %d", n)
	}

	g.config.Feeds = []models.FeedSpec{{Path: "/bad.xml", Format: "sitemap"}}
	err := g.generateDeclaredFeeds()
	if err == nil || !strings.Contains(err.Error(), "atom, rss, json") {
		t.Errorf("an unknown format must error and name the supported ones, got %v", err)
	}

	// A missing path is an error too, not a file written to the output root.
	g.config.Feeds = []models.FeedSpec{{Title: "no path"}}
	if err := g.generateDeclaredFeeds(); err == nil {
		t.Error("a feed without a path must error")
	}
}

// TestFeedAutodiscoveryLinks covers the other half of #86: a reader offering a
// choice reads exactly these links, so every published feed needs its own, with
// the right MIME type.
func TestFeedAutodiscoveryLinks(t *testing.T) {
	g := feedGen(t)
	g.config.Feed = true
	g.config.Feeds = []models.FeedSpec{
		{Path: "/blog/feed.xml", Title: "Blog"},
		{Path: "/blog/rss.xml", Title: "Blog", Format: "rss"},
		{Path: "/all.json", Title: "All", Format: "json"},
	}
	links := g.feedAutodiscoveryLinks()
	for _, want := range []string{
		`type="application/atom+xml" title="ex.com" href="/feed.xml"`,
		`type="application/atom+xml" title="Blog" href="/blog/feed.xml"`,
		`type="application/rss+xml" title="Blog" href="/blog/rss.xml"`,
		`type="application/feed+json" title="All" href="/all.json"`,
	} {
		if !strings.Contains(links, want) {
			t.Errorf("autodiscovery missing %s\ngot:\n%s", want, links)
		}
	}

	// Injection lands in <head> on any page, including one with no page context —
	// the homepage, which the SEO block never covered.
	page := `<html><head><title>Home</title></head><body>x</body></html>`
	got := g.injectFeedLinks(page)
	if strings.Count(got, `rel="alternate"`) != 4 {
		t.Errorf("expected four feed links in the head, got:\n%s", got)
	}
	if !strings.Contains(got, `href="/all.json"></head>`) && !strings.Contains(got, "</head>") {
		t.Error("links must be injected before </head>")
	}

	// A theme that advertises its own feed is left alone.
	own := `<html><head><link rel="alternate" type="application/rss+xml" href="/mine.xml"></head></html>`
	if g.injectFeedLinks(own) != own {
		t.Error("a theme's own feed link must not be duplicated")
	}

	// Nothing configured ⇒ nothing injected.
	g.config.Feed, g.config.Feeds = false, nil
	if got := g.injectFeedLinks(page); got != page {
		t.Errorf("no feeds configured must inject nothing, got:\n%s", got)
	}
}

// TestFeedAltPath: a section feed points readers at that section, not the root.
func TestFeedAltPath(t *testing.T) {
	cases := map[string]string{
		"/blog/feed.xml": "/blog/",
		"/feed.xml":      "/",
		"feed.json":      "/",
		"/a/b/feed.xml":  "/a/b/",
	}
	for in, want := range cases {
		if got := feedAltPath(models.FeedSpec{Path: in}); got != want {
			t.Errorf("feedAltPath(%q) = %q, want %q", in, got, want)
		}
	}
}
