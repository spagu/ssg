package generator

// The branches of the files 1.9.0 touched that no build reached: empty inputs,
// a value JSON cannot encode, paths outside the output, a missing tree.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/sitegraph"
	"github.com/spagu/ssg/internal/taxonomy"
)

// unencodable is a date encoding/json refuses: RFC 3339 has four-digit years.
var unencodable = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)

func TestMarkdownPublishEdges(t *testing.T) {
	if startsWithH1("\n  \n") {
		t.Error("blank text has no heading")
	}
	if markdownURLFor(models.Page{Link: "/feed.xml"}, "example.com") != "" {
		t.Error("a URL with no Markdown shape has no copy")
	}
	var b strings.Builder
	writeSection(&b, "Docs", []sitegraph.Page{{Slug: "untitled", Outputs: sitegraph.Outputs{Markdown: "https://example.com/u/index.md"}}})
	if !strings.Contains(b.String(), "- [untitled](https://example.com/u/index.md)") {
		t.Errorf("an untitled page is named by its slug:\n%s", b.String())
	}
}

func TestPostsPageOwnerSkipsOtherLanguages(t *testing.T) {
	g := newTestGen(t, "")
	g.config.PostsPage = "blog"
	pages := []models.Page{{Slug: "blog", Lang: "pl"}, {Slug: "blog", Lang: "en"}}
	if p := g.postsPageOwner(pages, "en"); p == nil || p.Lang != "en" {
		t.Errorf("owner = %+v", p)
	}
}

func TestOutputEdges(t *testing.T) {
	g := newTestGen(t, "")
	if _, err := builtinFormats()[FormatJSON].Render(g, models.Page{Slug: "a", Date: unencodable}); err == nil {
		t.Error("an unencodable record was written")
	}
	// A directory where the file should go: the write fails and says so.
	dir := filepath.Join(g.config.OutputDir, "taken")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	out := captureGeneratorStdout(t, func() {
		if g.writeOutputFile(dir, []byte("x"), "output json") {
			t.Error("writing over a directory succeeded")
		}
	})
	if !strings.Contains(out, "output json") {
		t.Errorf("no warning: %q", out)
	}
	// The root has no flat sibling, and a path outside the output gets none.
	g.writeFlatMarkdownSibling(filepath.Join(g.config.OutputDir, indexHTMLName), []byte("x"))
	outside := filepath.Join(t.TempDir(), "escape", indexHTMLName)
	g.writeFlatMarkdownSibling(outside, []byte("x"))
	if _, err := os.Stat(filepath.Dir(outside) + ".md"); err == nil {
		t.Error("a flat sibling was written outside the output")
	}
}

func TestRenderTransformEdges(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SEO = true
	page := models.Page{Slug: "a", Title: "A"}
	// A theme that wrote everything: nothing is added.
	full := `<html><head><meta property="og:title" content="A"><script type="application/ld+json">{"@type":"BreadcrumbList"}</script>` +
		`<link rel="alternate" hreflang="en" href="/"><meta name="description" content="d"></head></html>`
	if got := g.seoHTMLString(full, page, false); got != full {
		t.Errorf("a complete head was changed:\n%s", got)
	}
	// No </head>: the tags go first.
	if got := g.seoHTMLString("<p>x</p>", page, false); !strings.HasSuffix(got, "<p>x</p>") || !strings.Contains(got, "og:title") {
		t.Errorf("headless page:\n%s", got)
	}
	g.config.OutputDir = "relative-out"
	if got := g.profileOutputPath("/abs/page.html"); got != "/abs/page.html" {
		t.Errorf("an unrelatable path is reported as given: %q", got)
	}
}

func TestSiteGraphEdges(t *testing.T) {
	g := newTestGen(t, "")
	g.taxonomies = &taxonomy.Registry{
		Names:       []string{"mood"},
		Definitions: map[string]taxonomy.Definition{"mood": {Name: "mood"}},
	}
	if got := g.graphTaxonomies(); len(got) != 0 {
		t.Errorf("a taxonomy without an archive is listed: %+v", got)
	}
	// A redirect loop: the host would never resolve it, the graph lists none.
	g.config.Redirects = []RedirectRule{{From: "/a/", To: "/b/"}, {From: "/b/", To: "/a/"}}
	if got := g.buildSiteGraph(false); got.Redirects == nil || len(got.Redirects) != 0 {
		t.Errorf("redirects = %+v", got.Redirects)
	}
	g.config.Redirects = nil
	// No output tree: no links, and a graph all the same.
	g.config.OutputDir = filepath.Join(t.TempDir(), "missing")
	if links := g.graphLinks(); len(links) != 0 {
		t.Errorf("links = %+v", links)
	}
	// A page in the graph whose date JSON cannot encode fails the stamp.
	g.config.SiteGraph, g.config.Quiet = true, true
	g.siteData.Posts = []models.Page{{Slug: "far", Title: "Far", Date: unencodable}}
	if err := g.writeSiteGraph(); err == nil || !strings.Contains(err.Error(), "site graph") {
		t.Errorf("err = %v", err)
	}
}

func TestClassifyRefSkipsFragmentsAndSchemes(t *testing.T) {
	g := newTestGen(t, "")
	for _, ref := range []string{"", "#top", "mailto:a@example.com", "tel:1", "data:x", "javascript:void(0)"} {
		if _, kind := g.classifyRef("/", ref); kind != "" {
			t.Errorf("%q is a link of kind %q", ref, kind)
		}
	}
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"), `<a href="#top">x</a><a href="/about/">y</a><a href="/about/">z</a>`)
	// One unreadable directory is skipped, not fatal.
	locked := filepath.Join(out, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) }) // #nosec G302 -- lets TempDir clean up
	g.config.OutputDir = out
	links := g.graphLinks()
	if len(links) != 1 || links[0].To != "/about/" {
		t.Errorf("links = %+v", links)
	}
}
