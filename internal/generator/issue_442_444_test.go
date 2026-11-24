package generator

// #442: the page at the site root carries the Organization whatever it is —
// the post listing or a front page that moved the listing to posts_page — and
// a moved listing is named after its address, not the domain.
// #444: data files beside a page (.json, .csv, .txt, .xml) are published like
// its images, when the page mentions them.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestFrontPageStructuredData: the Organization goes to a page at "/" only,
// and only when the marketing settings describe one.
func TestFrontPageStructuredData(t *testing.T) {
	g := &Generator{config: Config{Domain: "example.com"},
		siteData: &models.SiteData{Marketing: models.Marketing{OGSiteName: "Example"}}}
	root := models.Page{Title: "Home", Slug: "home", Type: "page", Link: "/"}

	got := g.frontPageStructuredData(root)
	if len(got) != 1 || got[0]["@type"] != "Organization" || got[0]["name"] != "Example" {
		t.Fatalf("root page blocks = %v", got)
	}
	if got := g.frontPageStructuredData(models.Page{Title: "About", Slug: "about", Type: "page", Link: "/about/"}); got != nil {
		t.Errorf("non-root page blocks = %v, want nil", got)
	}
	g.siteData.Marketing = models.Marketing{}
	if got := g.frontPageStructuredData(root); got != nil {
		t.Errorf("no marketing data: blocks = %v, want nil", got)
	}
}

// TestListingTitle: the root listing keeps the domain; a moved listing takes
// the title of the page at its address, else its address titleized.
func TestListingTitle(t *testing.T) {
	newGen := func(postsPage string, titles map[string]string) *Generator {
		return &Generator{config: Config{Domain: "example.com", PostsPage: postsPage},
			siteData: &models.SiteData{}, crumbURLs: titles}
	}
	titled := map[string]string{"/ports/": "Ports tree", "/pl/ports/": "Drzewo portów"}
	tests := []struct {
		name      string
		postsPage string
		titles    map[string]string
		link      string
		want      string
	}{
		{"no posts_page", "", titled, "/", "example.com"},
		{"no posts_page, paged", "", titled, "/page/2/", "example.com"},
		{"posts_page, root link", "ports", titled, "/", "example.com"},
		{"page at the address", "ports", titled, "/ports/", "Ports tree"},
		{"pager of the address", "ports", titled, "/ports/page/2/", "Ports tree"},
		{"slashes in posts_page", "/ports/", titled, "/ports/", "Ports tree"},
		{"no page at the address", "ports", map[string]string{}, "/ports/", "Ports"},
		{"nested posts_page", "news/ports", map[string]string{}, "/news/ports/", "Ports"},
		{"hyphenated posts_page", "port-tree", map[string]string{}, "/port-tree/", "Port Tree"},
		{"language prefix, titled", "ports", titled, "/pl/ports/", "Drzewo portów"},
		{"language prefix, untitled", "ports", map[string]string{}, "/pl/ports/page/3/", "Ports"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newGen(tt.postsPage, tt.titles).listingTitle(tt.link); got != tt.want {
				t.Errorf("listingTitle(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}

// buildOrganizationSite builds a seo site with the Organization in its marketing
// settings, a page at "/" when postsPage is set, and two posts.
func buildOrganizationSite(t *testing.T, postsPage string) Config {
	t.Helper()
	files := map[string]string{
		"posts/news/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: post\ndate: 2026-07-01\n---\n\nA.\n",
		"posts/news/b.md": "---\ntitle: B\nslug: b\nstatus: publish\ntype: post\ndate: 2026-07-02\n---\n\nB.\n",
	}
	if postsPage != "" {
		files["pages/home.md"] = "---\ntitle: Welcome\nslug: home\nstatus: publish\ntype: page\nlink: \"/\"\n---\n\nFront.\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, func(string) string {
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.SEO = true
	cfg.PostsPage = postsPage
	cfg.Marketing = models.Marketing{OGSiteName: "Example"}
	buildSiteFixture(t, cfg)
	return cfg
}

// countType counts the JSON-LD blocks of a type on a page.
func countType(blocks []map[string]interface{}, typ string) int {
	n := 0
	for _, b := range blocks {
		if b["@type"] == typ {
			n++
		}
	}
	return n
}

// TestFrontPageCarriesOrganization: with posts_page and a page at "/", the
// front page has the Organization and the moved listing does not, and the
// listing's WebPage is named after its address.
func TestFrontPageCarriesOrganization(t *testing.T) {
	cfg := buildOrganizationSite(t, "ports")
	home := mustRead(t, filepath.Join(cfg.OutputDir, "index.html"))
	if n := strings.Count(home, `"@type":"Organization"`); n != 1 {
		t.Fatalf("front page has %d Organization blocks, want 1:\n%s", n, home)
	}
	if org := ldOf(t, home); countType(org, "Organization") != 1 {
		t.Errorf("front page types = %s", typesOf(org))
	}
	listing := mustRead(t, filepath.Join(cfg.OutputDir, "ports", "index.html"))
	if strings.Contains(listing, `"@type":"Organization"`) {
		t.Errorf("the moved listing has an Organization:\n%s", listing)
	}
	blocks := ldOf(t, listing)
	if countType(blocks, "WebPage") != 1 {
		t.Fatalf("listing types = %s, want one WebPage", typesOf(blocks))
	}
	for _, b := range blocks {
		if b["@type"] == "WebPage" && b["name"] != "Ports" {
			t.Errorf("listing WebPage name = %v, want Ports (not the domain)", b["name"])
		}
	}
}

// TestRootListingCarriesOrganization: without posts_page the listing at "/"
// still has exactly one Organization.
func TestRootListingCarriesOrganization(t *testing.T) {
	cfg := buildOrganizationSite(t, "")
	home := mustRead(t, filepath.Join(cfg.OutputDir, "index.html"))
	if n := strings.Count(home, `"@type":"Organization"`); n != 1 {
		t.Fatalf("root listing has %d Organization blocks, want 1:\n%s", n, home)
	}
}

// TestCopyColocatedDataFiles (#444): a .json beside a page is published when
// the page mentions it, and stays out when it does not.
func TestCopyColocatedDataFiles(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "ports.json"), []byte(`{"ports":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{config: Config{Quiet: true}}

	mentioned := filepath.Join(t.TempDir(), "out")
	if err := gen.copyColocatedAssets(srcDir, mentioned, `<script src="data.js" data-src="ports.json"></script>`); err != nil {
		t.Fatalf("copyColocatedAssets: %v", err)
	}
	if got := mustRead(t, filepath.Join(mentioned, "ports.json")); got != `{"ports":[]}` {
		t.Errorf("ports.json = %q", got)
	}

	unmentioned := filepath.Join(t.TempDir(), "out")
	if err := gen.copyColocatedAssets(srcDir, unmentioned, "No data here."); err != nil {
		t.Fatalf("copyColocatedAssets: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unmentioned, "ports.json")); !os.IsNotExist(err) {
		t.Errorf("an unmentioned ports.json was published: %v", err)
	}
}
