package generator

// The site graph and the pages a site keeps out of search (#433): the 404 page
// was listed in site-graph.json with a canonical URL that does not exist,
// although sitemap.xml and llms.txt already left it out. The artifact now
// leaves out exactly what the sitemap leaves out; routes.json, a contract of
// every route the build emits, still lists them.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/sitegraph"
)

// TestSiteGraphLeavesOutWhatTheSitemapLeavesOut builds a public page, the 404
// page, a front-matter noindex page, a sitemap: no page and a page whose theme
// alone marks it noindex — the last one is only visible in the rendered HTML,
// which is what the sitemap reads too.
func TestSiteGraphLeavesOutWhatTheSitemapLeavesOut(t *testing.T) {
	page := func(slug, extra string) string {
		return "---\ntitle: " + slug + "\nslug: " + slug + "\nstatus: publish\ntype: page\n" + extra + "---\n\nBody of " + slug + ".\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md":   page("about", ""),
		"pages/404.md":     page("404", ""),
		"pages/archive.md": page("archive", "robots: \"noindex, follow\"\n"),
		"pages/thanks.md":  page("thanks", "sitemap: \"no\"\n"),
		"pages/themed.md":  page("themed", ""),
	}, func(name string) string {
		if name == "page.html" {
			return `<html><head><title>{{ .Title }}</title>` +
				`{{ if eq .Title "themed" }}<meta name="robots" content="noindex">{{ end }}` +
				`</head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.SiteGraph = true
	cfg.RouteManifest = true
	cfg.MarkdownPublish = true
	buildSiteFixture(t, cfg)

	g, err := sitegraph.Load(cfg.OutputDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if findPage(g, "/about/") == nil {
		t.Fatalf("the public page is missing: %+v", g.Pages)
	}
	for _, p := range g.Pages {
		for _, hidden := range []string{"404", "archive", "thanks", "themed"} {
			if p.Slug == hidden {
				t.Errorf("site-graph.json lists %s (%s, canonical %s)", hidden, p.URL, p.Canonical)
			}
		}
	}
	sitemap := mustRead(t, cfg.OutputDir+"/sitemap.xml")
	llms := mustRead(t, cfg.OutputDir+"/llms.txt")
	for _, hidden := range []string{"/404", "/archive/", "/thanks/", "/themed/"} {
		if strings.Contains(sitemap, hidden) {
			t.Errorf("sitemap.xml lists %s", hidden)
		}
		if strings.Contains(llms, hidden) {
			t.Errorf("llms.txt lists %s:\n%s", hidden, llms)
		}
	}
	// routes.json is the route contract, not a search index: the routes exist.
	routes := mustRead(t, cfg.OutputDir+"/routes.json")
	for _, slug := range []string{"404", "archive", "thanks", "themed"} {
		if !strings.Contains(routes, `"title": "`+slug+`"`) {
			t.Errorf("routes.json lost %s:\n%s", slug, routes)
		}
	}
}
