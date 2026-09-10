package generator

// Content dimensions: relations, versions and outputs per page (GO-096).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/sitegraph"
)

// dimensionSite builds a small documentation site with two versions of one
// page and a relation between two others.
func dimensionSite(t *testing.T, apply func(cfg *Config)) Config {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/auth-v3.md": "---\ntitle: API Authentication\nslug: api-auth-v3\nstatus: publish\ntype: page\n" +
			"version: 3\nversion_of: api-auth\n---\n\nThe old way.\n",
		"pages/auth-v4.md": "---\ntitle: API Authentication\nslug: api-auth-v4\nstatus: publish\ntype: page\n" +
			"version: 4\nversion_of: api-auth\nrelations:\n  supersedes: [api-auth-v3]\n  see_also: [oauth-setup]\n---\n\nThe new way.\n",
		"pages/oauth.md": "---\ntitle: OAuth setup\nslug: oauth-setup\nstatus: publish\ntype: page\n---\n\nSetup.\n",
	}, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	if apply != nil {
		apply(&cfg)
	}
	return cfg
}

// TestRelationsResolveToPages: a slug an author wrote becomes a page a
// template can render.
func TestRelationsResolveToPages(t *testing.T) {
	cfg := dimensionSite(t, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	v4 := -1
	for i := range gen.siteData.Pages {
		if gen.siteData.Pages[i].Slug == "api-auth-v4" {
			v4 = i
		}
	}
	if v4 < 0 {
		t.Fatal("the page is missing")
	}
	page := gen.siteData.Pages[v4]
	if len(page.RelatedPages["supersedes"]) != 1 || page.RelatedPages["supersedes"][0].Slug != "api-auth-v3" {
		t.Errorf("supersedes = %+v", page.RelatedPages["supersedes"])
	}
	if len(page.RelatedPages["see_also"]) != 1 || page.RelatedPages["see_also"][0].Title != "OAuth setup" {
		t.Errorf("see_also = %+v", page.RelatedPages["see_also"])
	}
	// The inverse is the half an author cannot write down.
	inverse := gen.inverseRelations(*page.RelatedPages["supersedes"][0], "supersedes")
	if len(inverse) != 1 || inverse[0].Slug != "api-auth-v4" {
		t.Errorf("relatedBy = %+v", inverse)
	}
}

// TestRelationsStayInExtra: promoting a frontmatter key to a struct field
// would take it out of .Extra and break templates that already read it there.
func TestRelationsStayInExtra(t *testing.T) {
	cfg := dimensionSite(t, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	for i := range gen.siteData.Pages {
		p := gen.siteData.Pages[i]
		if p.Slug != "api-auth-v4" {
			continue
		}
		if p.Version != "4" || p.VersionOf != "api-auth" {
			t.Errorf("version fields = %q / %q", p.Version, p.VersionOf)
		}
		if p.Extra["version"] == nil || p.Extra["relations"] == nil {
			t.Errorf("the values must still reach .Extra: %v", p.Extra)
		}
	}
}

// TestVersionsCanonicaliseToTheLatest: without this, a document's own old
// revisions compete with it in search results.
func TestVersionsCanonicaliseToTheLatest(t *testing.T) {
	cfg := dimensionSite(t, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	for i := range gen.siteData.Pages {
		p := gen.siteData.Pages[i]
		switch p.Slug {
		case "api-auth-v4":
			if !p.IsLatest || p.Canonical != "" {
				t.Errorf("the latest version should be canonical to itself: %+v", p.Canonical)
			}
			if len(p.Versions) != 2 || p.Versions[0].Slug != "api-auth-v4" {
				t.Errorf("the chain should be newest first: %+v", p.Versions)
			}
		case "api-auth-v3":
			if p.IsLatest {
				t.Error("version 3 is not the latest")
			}
			if !strings.HasSuffix(p.Canonical, "/api-auth-v4/") {
				t.Errorf("canonical = %q", p.Canonical)
			}
			if p.Robots != "" {
				t.Errorf("noindex is opt-in, got %q", p.Robots)
			}
		}
	}
}

// TestVersionsNoindexIsOptIn, and takes the page out of the sitemap through
// the rule that already drops noindex pages.
func TestVersionsNoindexIsOptIn(t *testing.T) {
	cfg := dimensionSite(t, func(cfg *Config) {
		cfg.Versions = VersionsConfig{NoindexOld: true}
	})
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	for i := range gen.siteData.Pages {
		p := gen.siteData.Pages[i]
		if p.Slug == "api-auth-v3" && !strings.Contains(p.Robots, "noindex") {
			t.Errorf("robots = %q", p.Robots)
		}
		if p.Slug == "api-auth-v4" && p.Robots != "" {
			t.Errorf("the latest must stay indexable, got %q", p.Robots)
		}
	}
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	if strings.Contains(sitemap, "/api-auth-v3/") {
		t.Errorf("a noindexed version should leave the sitemap:\n%s", sitemap)
	}
	if !strings.Contains(sitemap, "/api-auth-v4/") {
		t.Errorf("the latest belongs in the sitemap:\n%s", sitemap)
	}
}

// TestVersionOrdering: numbers sort as numbers, so 10 comes after 2.
func TestVersionOrdering(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2", "10", -1},
		{"10", "2", 1},
		{"4", "4", 0},
		{"1.5", "1.10", 1}, // 1.5 > 1.1 as numbers, which is what a float means
		{"beta", "alpha", 1},
		{"alpha", "beta", -1},
		{"2", "beta", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestBrokenRelationFollowsCheckLinks: a relation naming nothing is a broken
// link with a name on it.
func TestBrokenRelationFollowsCheckLinks(t *testing.T) {
	files := map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\nrelations:\n  see_also: [nowhere]\n---\n\nA.\n",
	}
	quiet := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	if err := mustBuild(t, quiet); err != nil {
		t.Errorf("with link checking off it is silent: %v", err)
	}

	strict := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	strict.CheckLinks = "strict"
	err := mustBuild(t, strict)
	if err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("strict must fail and name the slug: %v", err)
	}
}

// TestOutputsPerPage: one reference page can publish JSON without the rest of
// the site doing so.
func TestOutputsPerPage(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/api.md":   "---\ntitle: API\nslug: api\nstatus: publish\ntype: page\noutputs: [html, json]\n---\n\nAPI.\n",
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, nil)
	buildSiteFixture(t, cfg)
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "api", "index.json")); err != nil {
		t.Errorf("the page that asked for json did not get it: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "about", "index.json")); err == nil {
		t.Error("a page that asked for nothing should not publish json")
	}

	// A page's own list overrides the site's, in both directions.
	site := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/api.md":   "---\ntitle: API\nslug: api\nstatus: publish\ntype: page\n---\n\nAPI.\n",
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\noutputs: [html]\n---\n\nAbout.\n",
	}, nil)
	site.Outputs = []string{"html", "json"}
	buildSiteFixture(t, site)
	if _, err := os.Stat(filepath.Join(site.OutputDir, "api", "index.json")); err != nil {
		t.Errorf("the site-wide list should still apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(site.OutputDir, "about", "index.json")); err == nil {
		t.Error("a page opting out of json should not publish it")
	}
}

// TestRelationsReachTheSiteGraph, which is where an agent looks for them.
func TestRelationsReachTheSiteGraph(t *testing.T) {
	cfg := dimensionSite(t, func(cfg *Config) { cfg.SiteGraph = true })
	buildSiteFixture(t, cfg)
	graph, err := sitegraph.Load(cfg.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range graph.Pages {
		if !strings.Contains(p.URL, "api-auth-v4") {
			continue
		}
		for _, r := range p.Relations {
			if r.Name == "supersedes" && strings.Contains(r.URL, "api-auth-v3") {
				found = true
			}
		}
	}
	if !found {
		t.Error("the declared relation is not in the graph")
	}
}

// TestNoDimensionsChangesNothing: frontmatter without the new keys behaves
// exactly as it did.
func TestNoDimensionsChangesNothing(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
	}, nil)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	p := gen.siteData.Pages[0]
	if p.Version != "" || p.VersionOf != "" || p.IsLatest || len(p.Relations) != 0 || len(p.Outputs) != 0 {
		t.Errorf("a page that declared nothing carries something: %+v", p)
	}
	if p.Canonical != "" || p.Robots != "" {
		t.Errorf("nothing should have been invented: canonical=%q robots=%q", p.Canonical, p.Robots)
	}
}

// TestRelationHelpersRenderInATemplate: both directions, from a theme.
func TestRelationHelpersRenderInATemplate(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/auth-v3.md": "---\ntitle: API Authentication\nslug: api-auth-v3\nstatus: publish\ntype: page\n" +
			"version: 3\nversion_of: api-auth\n---\n\nThe old way.\n",
		"pages/auth-v4.md": "---\ntitle: API Authentication\nslug: api-auth-v4\nstatus: publish\ntype: page\n" +
			"version: 4\nversion_of: api-auth\nrelations:\n  supersedes: [api-auth-v3]\n  see_also: [oauth-setup]\n---\n\nThe new way.\n",
		"pages/oauth.md": "---\ntitle: OAuth setup\nslug: oauth-setup\nstatus: publish\ntype: page\n---\n\nSetup.\n",
	}, func(name string) string {
		if name == "page.html" {
			return `<html><head><title>x</title></head><body>` +
				`{{ range relationsOf .Page "see_also" }}<a class="see" href="{{ .GetURL }}">{{ .Title }}</a>{{ end }}` +
				`{{ range relatedBy .Page "supersedes" }}<a class="by" href="{{ .GetURL }}">{{ .Title }}</a>{{ end }}` +
				`</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	buildSiteFixture(t, cfg)
	v4 := mustRead(t, filepath.Join(cfg.OutputDir, "api-auth-v4", "index.html"))
	if !strings.Contains(v4, `class="see" href="/oauth-setup/"`) {
		t.Errorf("relationsOf did not render:\n%s", v4)
	}
	v3 := mustRead(t, filepath.Join(cfg.OutputDir, "api-auth-v3", "index.html"))
	if !strings.Contains(v3, `class="by" href="/api-auth-v4/"`) {
		t.Errorf("relatedBy did not render the inverse:\n%s", v3)
	}
}

// TestRelationLookupAcrossLanguages: a relation names a slug in the page's own
// language, and falls back to the default bucket for a page not translated yet.
func TestRelationLookupAcrossLanguages(t *testing.T) {
	en := &models.Page{Slug: "pricing"}
	pl := &models.Page{Slug: "pricing", Lang: "pl"}
	index := map[string]map[string]*models.Page{
		"":   {"pricing": en, "only-en": {Slug: "only-en"}},
		"pl": {"pricing": pl},
	}
	if lookupRelated(index, "pl", "pricing") != pl {
		t.Error("a relation should prefer the page's own language")
	}
	if lookupRelated(index, "pl", "only-en") == nil {
		t.Error("a page with no translation should still be reachable")
	}
	if got := lookupRelated(index, "pl", "nowhere"); got != nil {
		t.Errorf("got %+v", got)
	}
	// Slashes and spaces around a slug are the author being human.
	if lookupRelated(index, "", " /pricing/ ") != en {
		t.Error("a slug written with slashes should still resolve")
	}
}
