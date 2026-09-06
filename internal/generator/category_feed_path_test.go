package generator

// A category feed belongs beside the archive it describes (#246). Deriving its
// path from the slug put it at /category/<slug>/ even when the archive was
// served somewhere else — a feed file in a directory holding no archive, whose
// <link> named a URL the archive does not live at.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFeedPathSite renders the two archive shapes whose path is not
// /category/<slug>/: a category with its own link (#143) and a nested one
// (#138), alongside an ordinary one that has not moved.
func buildFeedPathSite(t *testing.T) Config {
	t.Helper()
	cfg := newSiteFixture(t, `{
		"categories":[
			{"id":2,"name":"News","slug":"news"},
			{"id":3,"name":"Rooms","slug":"rooms"},
			{"id":4,"name":"Kitchens","slug":"kitchens","parent":3},
			{"id":5,"name":"Projects","slug":"projects","link":"https://example.com/projects-archive/"},
			{"id":6,"name":"Legal","slug":"legal"}
		],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md":       "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\n---\n\nBody.\n",
			"posts/kitchens/two.md":   "---\ntitle: Two\nslug: two\nstatus: publish\ntype: post\ndate: 2024-01-03\ncategories: [Kitchens]\n---\n\nBody.\n",
			"posts/projects/three.md": "---\ntitle: Three\nslug: three\nstatus: publish\ntype: post\ndate: 2024-01-04\ncategories: [Projects]\n---\n\nBody.\n",
		}, nil)
	cfg.Feed = true
	buildSiteFixture(t, cfg)
	return cfg
}

// TestCategoryFeedSitsBesideItsArchive: wherever the archive was written, that
// is where its feed goes.
func TestCategoryFeedSitsBesideItsArchive(t *testing.T) {
	cfg := buildFeedPathSite(t)

	for _, archive := range []string{"category/news", "category/rooms/kitchens", "projects-archive"} {
		dir := filepath.Join(cfg.OutputDir, filepath.FromSlash(archive))
		if _, err := os.Stat(filepath.Join(dir, indexHTMLName)); err != nil {
			t.Fatalf("fixture did not render /%s/: %v", archive, err)
		}
		if _, err := os.Stat(filepath.Join(dir, feedFileName)); err != nil {
			t.Errorf("/%s/ has an archive but no feed beside it: %v", archive, err)
		}
	}
	// And nowhere else: the derived paths these archives do NOT live at.
	for _, stray := range []string{"category/kitchens", "category/projects"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, filepath.FromSlash(stray), feedFileName)); err == nil {
			t.Errorf("a feed was written at /%s/, where no archive lives", stray)
		}
	}
}

// TestCategoryFeedNamesTheServedArchive: the feed's own <link> has to name the
// archive, or a reader following it lands on nothing.
func TestCategoryFeedNamesTheServedArchive(t *testing.T) {
	cfg := buildFeedPathSite(t)

	feed := mustRead(t, filepath.Join(cfg.OutputDir, "projects-archive", feedFileName))
	if !strings.Contains(feed, `href="https://example.com/projects-archive/"`) {
		t.Errorf("the feed does not link the archive it describes:\n%s", feed)
	}
	if strings.Contains(feed, "/category/projects/") {
		t.Error("the feed links the redirect instead of the archive")
	}
	nested := mustRead(t, filepath.Join(cfg.OutputDir, "category", "rooms", "kitchens", feedFileName))
	if !strings.Contains(nested, `href="https://example.com/category/rooms/kitchens/"`) {
		t.Errorf("the nested feed names the wrong archive:\n%s", nested)
	}
}

// TestNoFeedWithoutAnArchive: a term nobody posted in has no archive, and the
// invariant #228 set for the sitemap holds for the feeds too.
func TestNoFeedWithoutAnArchive(t *testing.T) {
	cfg := buildFeedPathSite(t)

	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "category", "legal")); err == nil {
		t.Error("a category with no posts must not get a feed directory")
	}
}
