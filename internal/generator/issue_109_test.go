package generator

// #109: the home page received no structured data at all.

import (
	"path/filepath"
	"testing"
)

// TestIndexPageContextTypesTheHomePage: derivedLD types the site root as
// WebSite, but the branch was unreachable — renderIndexPage passed no page, so
// the SEO block was skipped and the only page that selects WebSite never
// arrived at the function.
func TestIndexPageContextTypesTheHomePage(t *testing.T) {
	g := &Generator{config: Config{OutputDir: "/out", Domain: "ex.com", Quiet: true}}

	page := g.indexPageContext(filepath.Join("/out", indexHTMLName))
	if page == nil {
		t.Fatal("no page context for the home page")
	}
	if got := page.GetURL(); got != "/" {
		t.Errorf("home URL: got %q, want /", got)
	}

	ld := g.derivedLD(*page, false, g.servedCanonical(*page))
	if ld["@type"] != "WebSite" {
		t.Errorf("home page typed %v, want WebSite", ld["@type"])
	}
	if ld["url"] != "https://ex.com/" {
		t.Errorf("url: got %v", ld["url"])
	}
}

// TestIndexPageContextGivesPaginationItsOwnURL: a paginated index must declare
// itself, not be canonicalised onto page one.
func TestIndexPageContextGivesPaginationItsOwnURL(t *testing.T) {
	g := &Generator{config: Config{OutputDir: "/out", Domain: "ex.com", Quiet: true}}

	page := g.indexPageContext(filepath.Join("/out", "page", "2", indexHTMLName))
	if got := page.GetURL(); got != "/page/2/" {
		t.Errorf("paginated URL: got %q, want /page/2/", got)
	}
	ld := g.derivedLD(*page, false, g.servedCanonical(*page))
	if ld["@type"] == "WebSite" {
		t.Error("a paginated index must not claim to be the WebSite")
	}
	if ld["url"] != "https://ex.com/page/2/" {
		t.Errorf("url: got %v", ld["url"])
	}
}

// TestIndexPageContextSurvivesAnUnrelatedOutputPath: filepath.Rel fails when the
// path is outside OutputDir, and a nil page here would silently restore the bug.
func TestIndexPageContextSurvivesAnUnrelatedOutputPath(t *testing.T) {
	g := &Generator{config: Config{OutputDir: "/out", Domain: "ex.com", Quiet: true}}
	if page := g.indexPageContext("relative/index.html"); page == nil {
		t.Fatal("no page context")
	} else if page.Title != "ex.com" {
		t.Errorf("title: got %q", page.Title)
	}
}
