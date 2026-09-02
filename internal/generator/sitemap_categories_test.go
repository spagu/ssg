package generator

// The sitemap must advertise the category archives the build wrote — not the
// ones the metadata merely declares (#228).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildCategorySite builds a site whose metadata declares more categories than
// its posts use, plus one category served away from /category/ by its link.
func buildCategorySite(t *testing.T) (outDir, sitemap string) {
	t.Helper()
	// News has a post. Legal exists only in metadata — the schema-resume.org
	// case, where /category/legal/ was in the sitemap and 404ed. Projects has a
	// post but serves at /projects-archive/ via its link (#143), so the flat
	// /category/projects/ is a redirect, not the archive.
	cfg := newSiteFixture(t, `{
		"categories":[
			{"id":2,"name":"News","slug":"news"},
			{"id":3,"name":"Legal","slug":"legal"},
			{"id":4,"name":"Projects","slug":"projects","link":"https://example.com/projects-archive/"}
		],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md":     "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\n---\n\nBody.\n",
			"posts/projects/two.md": "---\ntitle: Two\nslug: two\nstatus: publish\ntype: post\ndate: 2024-01-03\ncategories: [Projects]\n---\n\nBody.\n",
		}, nil)
	buildSiteFixture(t, cfg)
	return cfg.OutputDir, mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// TestSitemapListsOnlyRenderedCategories: a category nobody posted in has no
// archive, and a sitemap entry for it is a guaranteed 404 handed to every
// crawler that trusts the file.
func TestSitemapListsOnlyRenderedCategories(t *testing.T) {
	outDir, sitemap := buildCategorySite(t)

	if !strings.Contains(sitemap, "https://example.com/category/news/") {
		t.Error("the category with a post is missing from the sitemap")
	}
	if strings.Contains(sitemap, "/category/legal/") {
		t.Error("a category with no posts is advertised — that URL 404s")
	}
	// The invariant behind both assertions: every category URL the sitemap
	// names must be a document the build wrote.
	for _, line := range strings.Split(sitemap, "\n") {
		if !strings.Contains(line, "<loc>") {
			continue
		}
		loc := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "<loc>"), "</loc>")
		rel := strings.TrimPrefix(loc, "https://example.com")
		if rel == "" || rel == "/" {
			continue
		}
		file := filepath.Join(outDir, filepath.FromSlash(strings.Trim(rel, "/")), "index.html")
		if _, err := os.Stat(file); err != nil {
			t.Errorf("sitemap names %s but the build wrote no %s", loc, file)
		}
	}
}

// TestSitemapNamesTheServedCategoryPath: a category whose link says it lives at
// /projects-archive/ renders there (#143), and the sitemap must say so too —
// the flat /category/projects/ is a redirect at best.
func TestSitemapNamesTheServedCategoryPath(t *testing.T) {
	_, sitemap := buildCategorySite(t)

	if !strings.Contains(sitemap, "https://example.com/projects-archive/") {
		t.Error("the sitemap does not name the path the archive is served at")
	}
	if strings.Contains(sitemap, "/category/projects/") {
		t.Error("the sitemap names the redirect instead of the archive")
	}
}
