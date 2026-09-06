package generator

// Archives had no canonical of their own (#245). A theme reaching for the name
// pages and posts already carry got a missing map key, which Go templates
// resolve to empty rather than failing, so every archive on the site shipped
// <link rel="canonical" href=""/> with a green build and a clean link check.

import (
	"path/filepath"
	"strings"
	"testing"
)

// canonicalTemplate renders the archive canonical into every archive view.
func canonicalTemplate(name string) string {
	if name == "category.html" || name == "tag.html" || name == "taxonomy.html" {
		return `<html><head><link rel="canonical" href="{{.CanonicalURL}}"/></head><body><p>x</p></body></html>`
	}
	return `<html><body><p>x</p></body></html>`
}

// buildCanonicalSite lays out the two shapes a template cannot reconstruct from
// .Category.Slug: a category served away from /category/ by its own link (#143),
// and a nested category (#138). Paginated, so page 2 exists.
func buildCanonicalSite(t *testing.T) Config {
	t.Helper()
	files := map[string]string{}
	for _, n := range []string{"one", "two", "three"} {
		files["posts/projects/"+n+".md"] = "---\ntitle: " + n + "\nslug: " + n +
			"\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [Projects]\ntags: [Go]\n---\n\nBody.\n"
	}
	files["posts/kitchens/four.md"] = "---\ntitle: Four\nslug: four\nstatus: publish\ntype: post\n" +
		"date: 2024-01-06\ncategories: [Kitchens]\n---\n\nBody.\n"
	cfg := newSiteFixture(t, `{
		"categories":[
			{"id":2,"name":"Projects","slug":"projects","link":"https://example.com/projects-archive/"},
			{"id":3,"name":"Rooms","slug":"rooms"},
			{"id":4,"name":"Kitchens","slug":"kitchens","parent":3}
		],"media":[],"users":[]}`, files, canonicalTemplate)
	cfg.Paginate = 2
	buildSiteFixture(t, cfg)
	return cfg
}

// canonicalOf reads the canonical href out of one rendered archive.
func canonicalOf(t *testing.T, cfg Config, rel string) string {
	t.Helper()
	html := mustRead(t, filepath.Join(cfg.OutputDir, filepath.FromSlash(rel), indexHTMLName))
	const marker = `<link rel="canonical" href="`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("%s has no canonical: %s", rel, html)
	}
	rest := html[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

// TestArchiveCanonicalIsNeverEmpty: the failure this fixes was silent, so the
// invariant worth asserting is the blunt one.
func TestArchiveCanonicalIsNeverEmpty(t *testing.T) {
	cfg := buildCanonicalSite(t)

	for _, rel := range []string{"projects-archive", "category/rooms/kitchens", "tag/go"} {
		if got := canonicalOf(t, cfg, rel); got == "" {
			t.Errorf("/%s/ shipped an empty canonical", rel)
		}
	}
}

// TestArchiveCanonicalNamesTheServedPath: a category with its own link is served
// away from /category/, and the canonical must say where it actually lives —
// the same class of error #228 fixed on the sitemap side.
func TestArchiveCanonicalNamesTheServedPath(t *testing.T) {
	cfg := buildCanonicalSite(t)

	if got, want := canonicalOf(t, cfg, "projects-archive"), "https://example.com/projects-archive/"; got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
	// A nested category serves at /category/<parent>/<child>/, which the slug
	// alone cannot express.
	if got, want := canonicalOf(t, cfg, "category/rooms/kitchens"), "https://example.com/category/rooms/kitchens/"; got != want {
		t.Errorf("nested canonical = %q, want %q", got, want)
	}
	if got, want := canonicalOf(t, cfg, "tag/go"), "https://example.com/tag/go/"; got != want {
		t.Errorf("tag canonical = %q, want %q", got, want)
	}
}

// TestArchiveCanonicalPageTwoNamesItself: a paginated archive whose every page
// claims to be page 1 asks search engines to drop the tail.
func TestArchiveCanonicalPageTwoNamesItself(t *testing.T) {
	cfg := buildCanonicalSite(t)

	got := canonicalOf(t, cfg, "projects-archive/page/2")
	if want := "https://example.com/projects-archive/page/2/"; got != want {
		t.Errorf("page 2 canonical = %q, want %q", got, want)
	}
}

// TestArchiveCanonicalFollowsPrettyURLs: the canonical is the URL the host
// serves, exactly as it is for a page (#103).
func TestArchiveCanonicalFollowsPrettyURLs(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"

	if got, want := g.archiveCanonical("tag/go", singlePagePager(1)), "https://example.com/tag/go/"; got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
	// Leading and trailing slashes are the caller's habit, not a difference.
	if got := g.archiveCanonical("/tag/go/", singlePagePager(1)); got != "https://example.com/tag/go/" {
		t.Errorf("canonical = %q, want the same URL", got)
	}
	// An archive with no path has no canonical to claim; "" is what a template
	// reading a missing key would have seen anyway.
	if got := g.archiveCanonical("", singlePagePager(1)); got != "" {
		t.Errorf("canonical = %q, want empty", got)
	}
}
