package generator

// Category id 1 is not a reserved value (#243). Three places treated it as
// WordPress's "Uncategorized" and skipped it unconditionally, so an export that
// numbers its categories from 1 lost a real, populated, linked archive from
// sitemap.xml and got no feed for it — silently, with a green build.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildCatchAllSite reproduces the migration from the report: id 1 is
// "Air Conditioning" with posts, id 4 is another real category, and the actual
// catch-all term sits at an id nothing special.
func buildCatchAllSite(t *testing.T) (outDir, sitemap string) {
	t.Helper()
	cfg := newSiteFixture(t, `{
		"categories":[
			{"id":1,"name":"Air Conditioning","slug":"air-conditioning"},
			{"id":4,"name":"Gas Safety","slug":"gas-safety"},
			{"id":7,"name":"Uncategorized","slug":"uncategorized"}
		],"media":[],"users":[]}`,
		map[string]string{
			"posts/air-conditioning/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [Air Conditioning]\n---\n\nBody.\n",
			"posts/air-conditioning/two.md": "---\ntitle: Two\nslug: two\nstatus: publish\ntype: post\ndate: 2024-01-03\ncategories: [Air Conditioning]\n---\n\nBody.\n",
			"posts/gas-safety/three.md":     "---\ntitle: Three\nslug: three\nstatus: publish\ntype: post\ndate: 2024-01-04\ncategories: [Gas Safety]\n---\n\nBody.\n",
			"posts/uncategorized/four.md":   "---\ntitle: Four\nslug: four\nstatus: publish\ntype: post\ndate: 2024-01-05\ncategories: [Uncategorized]\n---\n\nBody.\n",
		}, nil)
	cfg.Feed = true
	buildSiteFixture(t, cfg)
	return cfg.OutputDir, mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// TestSitemapKeepsARealCategoryNumberedOne: the archive was written and every
// post in it links there, so leaving it out of the sitemap points crawlers at a
// page the site never declares.
func TestSitemapKeepsARealCategoryNumberedOne(t *testing.T) {
	_, sitemap := buildCatchAllSite(t)

	if !strings.Contains(sitemap, "https://example.com/category/air-conditioning/") {
		t.Error("a real category that happens to be id 1 is missing from the sitemap")
	}
	if !strings.Contains(sitemap, "https://example.com/category/gas-safety/") {
		t.Error("the sitemap lost a category it always listed")
	}
}

// TestSitemapSkipsTheCatchAllTermByName: the term is recognised by what it is,
// not by the number the export gave it.
func TestSitemapSkipsTheCatchAllTermByName(t *testing.T) {
	outDir, sitemap := buildCatchAllSite(t)

	if strings.Contains(sitemap, "/category/uncategorized/") {
		t.Error("the catch-all archive is advertised in the sitemap")
	}
	// It is still rendered — this is a sitemap rule, not a build rule.
	if _, err := os.Stat(filepath.Join(outDir, "category", "uncategorized", indexHTMLName)); err != nil {
		t.Errorf("the catch-all archive should still be written: %v", err)
	}
}

// TestFeedForARealCategoryNumberedOne: the same rule decides the per-category
// Atom feeds, so the sitemap and the feeds cannot disagree about which terms
// are real.
func TestFeedForARealCategoryNumberedOne(t *testing.T) {
	outDir, _ := buildCatchAllSite(t)

	feed := filepath.Join(outDir, "category", "air-conditioning", feedFileName)
	if _, err := os.Stat(feed); err != nil {
		t.Errorf("a real category numbered 1 got no feed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "category", "uncategorized", feedFileName)); err == nil {
		t.Error("the catch-all term should not get a feed")
	}
}
