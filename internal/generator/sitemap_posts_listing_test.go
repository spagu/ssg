package generator

// The post listing is a rendered, indexable document that belongs to neither
// Pages nor Posts, so generateSitemap never named it (#244). On the site root
// the front-page entry covered it by accident; moving the listing to /blog/ with
// posts_page exposed the gap — an Ahrefs crawl found the most-linked page on the
// site after the home page reported as "indexable page not in sitemap".

import (
	"path/filepath"
	"strings"
	"testing"
)

// buildListingSite builds a site whose listing lives at /blog/, paginated so the
// tail exists and can be checked for.
func buildListingSite(t *testing.T, tmpl func(string) string) (cfg Config, sitemap string) {
	t.Helper()
	files := map[string]string{
		"pages/home.md": "---\ntitle: Home\nslug: index\nstatus: publish\ntype: page\n---\n\nFront.\n",
	}
	for _, n := range []string{"one", "two", "three"} {
		files["posts/news/"+n+".md"] = "---\ntitle: " + n +
			"\nslug: " + n + "\nstatus: publish\ntype: post\ndate: 2024-01-02\n---\n\nBody.\n"
	}
	cfg = newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, tmpl)
	cfg.PostsPage = "blog"
	cfg.Paginate = 2
	buildSiteFixture(t, cfg)
	return cfg, mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// TestSitemapNamesThePostsListing: /blog/ is written, every post links back to
// it, and it is the entry point for the blog section — it belongs in the file
// the site uses to state its own structure.
func TestSitemapNamesThePostsListing(t *testing.T) {
	cfg, sitemap := buildListingSite(t, nil)

	if !strings.Contains(sitemap, "<loc>https://example.com/blog/</loc>") {
		t.Errorf("the post listing is missing from the sitemap:\n%s", sitemap)
	}
	// It is listed once, and at a priority above an ordinary page.
	if n := strings.Count(sitemap, "<loc>https://example.com/blog/</loc>"); n != 1 {
		t.Errorf("the listing appears %d times, want 1", n)
	}
	if _, err := filepath.Glob(filepath.Join(cfg.OutputDir, "blog", indexHTMLName)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sitemap, "<priority>0.9</priority>") {
		t.Error("the listing should outrank an ordinary page and sit under the home page")
	}
}

// TestSitemapOmitsTheListingTail: the paginated tail is left out deliberately —
// /blog/page/2/ is not a hub, it is a slice of one.
func TestSitemapOmitsTheListingTail(t *testing.T) {
	_, sitemap := buildListingSite(t, nil)

	if strings.Contains(sitemap, "/blog/page/") {
		t.Error("the paginated tail should stay out of the sitemap")
	}
}

// TestSitemapListingHonoursNoindex: a theme that marks the listing noindex keeps
// it out, the same rule every other document follows.
func TestSitemapListingHonoursNoindex(t *testing.T) {
	_, sitemap := buildListingSite(t, func(name string) string {
		if name == indexHTMLName {
			return `<html><head><meta name="robots" content="noindex"></head><body><p>x</p></body></html>`
		}
		return `<html><body><p>x</p></body></html>`
	})

	if strings.Contains(sitemap, "https://example.com/blog/") {
		t.Error("a listing that marks itself noindex must not be advertised")
	}
}

// TestSitemapRootListingIsNotDuplicated: without posts_page the listing IS the
// site root, which the front-page entry already names. Listing it twice would be
// a duplicate crawlers report.
func TestSitemapRootListingIsNotDuplicated(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\n---\n\nBody.\n",
		}, nil)
	buildSiteFixture(t, cfg)
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	if n := strings.Count(sitemap, "<loc>https://example.com/</loc>"); n != 1 {
		t.Errorf("the site root appears %d times, want 1:\n%s", n, sitemap)
	}
}
