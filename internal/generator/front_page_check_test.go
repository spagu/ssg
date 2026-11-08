package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestHiddenFrontPageStopsTheBuild reproduces #394: a WordPress leftover at
// the site root, still marked noindex, would have become the live home page.
func TestHiddenFrontPageStopsTheBuild(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{
		{Slug: "home-old", Link: "https://example.com/", Robots: "noindex, follow", SourceFile: "pages/homepage.md"},
	}
	err := g.checkHiddenFrontPage()
	if err == nil {
		t.Fatal("a noindex page took the home page")
	}
	for _, want := range []string{"pages/homepage.md", "home page at /", `"noindex, follow"`, "remove its link:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// TestFrontPagesThatPass: an indexable front page, the documented sitemap:
// "no" workaround, a redirect at the root, and noindex away from the root.
func TestFrontPagesThatPass(t *testing.T) {
	for name, p := range map[string]models.Page{
		"indexable":    {Slug: "home", Link: "/"},
		"sitemap no":   {Slug: "home", Link: "/", Sitemap: "no"},
		"redirect":     {Slug: "home", Link: "/", Robots: "noindex", Layout: "redirect"},
		"not the root": {Slug: "old", Link: "/old/", Robots: "noindex"},
	} {
		g := newTestGen(t, "")
		g.siteData.Pages = []models.Page{p}
		if err := g.checkHiddenFrontPage(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestTheBuildRefusesAHiddenFrontPage: the check runs while content loads, so
// nothing is written over the theme's home page.
func TestTheBuildRefusesAHiddenFrontPage(t *testing.T) {
	build := func() error {
		cfg := mappedTaxonomySite(t, `[]`)
		mustWrite(t, filepath.Join(cfg.ContentDir, "site", "pages", "homepage.md"),
			"---\ntitle: Old home\nslug: home-old\nstatus: publish\nlink: \"https://example.com/\"\nrobots: \"noindex, follow\"\n---\n\nOld.\n")
		gen, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return gen.Generate()
	}
	if err := build(); err == nil || !strings.Contains(err.Error(), "#394") {
		t.Fatalf("err = %v", err)
	}
}
