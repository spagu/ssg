package generator

// One model of the site, built from what the build already knows (GO-095).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/sitegraph"
)

// graphSite builds a small site with a post, a page, a tag, a category, an
// alias redirect and a link between pages, with the graph turned on.
func graphSite(t *testing.T, apply func(cfg *Config)) (Config, sitegraph.Graph) {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`,
		map[string]string{
			"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\n" +
				"categories: [News]\ntags: [alpha]\naliases: [/old-one/]\n---\n\nSee [about](/about/) and [ext](https://example.org/x).\n",
			"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\ndescription: Who we are\n---\n\n![logo](/img/logo.png)\n",
		}, func(name string) string {
			if name == "post.html" || name == "page.html" {
				return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
			}
			return `<html><head><title>x</title></head><body><p>x</p></body></html>`
		})
	cfg.SiteGraph = true
	cfg.Version = "test-build"
	if apply != nil {
		apply(&cfg)
	}
	buildSiteFixture(t, cfg)
	g, err := sitegraph.Load(cfg.OutputDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg, g
}

func findPage(g sitegraph.Graph, url string) *sitegraph.Page {
	for i := range g.Pages {
		if g.Pages[i].URL == url {
			return &g.Pages[i]
		}
	}
	return nil
}

// TestSiteGraphIsWrittenWhenAsked: the artifact exists, is stamped, and holds
// every kind of node from what the build already knew.
func TestSiteGraphIsWrittenWhenAsked(t *testing.T) {
	_, g := graphSite(t, nil)

	if g.Schema != sitegraph.Schema || g.Build.Version != "test-build" || g.Build.Hash == "" || g.Build.Time.IsZero() {
		t.Errorf("build stamp wrong: %+v", g.Build)
	}
	post := findPage(g, "/2024/01/02/one/")
	if post == nil {
		t.Fatalf("the post is missing: %v", g.Pages)
	}
	if post.Type != "post" || post.Canonical != "https://example.com/2024/01/02/one/" || len(post.Tags) != 1 ||
		len(post.Categories) != 1 || post.Categories[0] != "News" || post.Date == nil {
		t.Errorf("post node wrong: %+v", *post)
	}
	if about := findPage(g, "/about/"); about == nil || about.Type != "page" || about.Description != "Who we are" {
		t.Errorf("page node wrong: %+v", about)
	}
}

// TestSiteGraphDescribesTheArchivesTheBuildWrote, so an agent asking what
// sections exist gets the ones a reader can actually open.
func TestSiteGraphDescribesTheArchivesTheBuildWrote(t *testing.T) {
	_, g := graphSite(t, nil)
	kinds := map[string]bool{}
	for _, s := range g.Sections {
		kinds[s.Kind] = true
	}
	if !kinds["category"] || !kinds["tag"] {
		t.Errorf("sections lack the archives: %v", g.Sections)
	}
}

// TestSiteGraphCarriesTaxonomyTermsWithTheirArchives.
func TestSiteGraphCarriesTaxonomyTermsWithTheirArchives(t *testing.T) {
	_, g := graphSite(t, nil)
	var tagTax *sitegraph.Taxonomy
	for i := range g.Taxonomies {
		if g.Taxonomies[i].Name == "tag" {
			tagTax = &g.Taxonomies[i]
		}
	}
	if tagTax == nil || len(tagTax.Terms) != 1 || tagTax.Terms[0].URL != "/tag/alpha/" || tagTax.Terms[0].Count != 1 {
		t.Errorf("tag taxonomy wrong: %+v", tagTax)
	}
}

// TestSiteGraphReportsRedirectsAsTheHostWillSeeThem.
//
// An alias in frontmatter becomes a rule, and the redirect engine normalises a
// trailing slash away — so the graph reports the normalised form, which is what
// the host applies.
func TestSiteGraphReportsRedirectsAsTheHostWillSeeThem(t *testing.T) {
	_, g := graphSite(t, nil)
	for _, r := range g.Redirects {
		if strings.TrimSuffix(r.From, "/") == "/old-one" && r.Status == 301 && r.To == "/2024/01/02/one/" {
			return
		}
	}
	t.Errorf("the alias redirect is missing: %v", g.Redirects)
}

// TestSiteGraphLinksAreClassified: a page link, an asset and an external URL,
// each resolved from the page they appear on.
func TestSiteGraphLinksAreClassified(t *testing.T) {
	_, g := graphSite(t, nil)

	want := map[string]string{"/about/": "page", "https://example.org/x": "external"}
	got := map[string]string{}
	for _, l := range g.Links {
		if l.From == "/2024/01/02/one/" {
			got[l.To] = l.Kind
		}
	}
	for to, kind := range want {
		if got[to] != kind {
			t.Errorf("link to %s: kind %q, want %q (all: %v)", to, got[to], kind, got)
		}
	}
	var asset bool
	for _, l := range g.Links {
		if l.From == "/about/" && l.To == "/img/logo.png" && l.Kind == "asset" {
			asset = true
		}
	}
	if !asset {
		t.Errorf("the image was not recorded as an asset link: %v", g.Links)
	}
}

// TestClassifyRef: the resolution rules on their own.
func TestClassifyRef(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	cases := []struct{ from, ref, to, kind string }{
		{"/a/b/", "../x/", "/a/x/", "page"},
		{"/a/b/", "c", "/a/b/c/", "page"},
		{"/a/", "/p/?q=1#frag", "/p/", "page"},
		{"/a/", "https://example.com/p/", "/p/", "page"}, // own domain is internal
		{"/a/", "//cdn.example.org/x.js", "https://cdn.example.org/x.js", "external"},
		{"/a/", "/css/style.css", "/css/style.css", "asset"},
		{"/a/", "/page.html", "/page.html", "page"},
		{"/a/", "mailto:x@example.com", "", ""},
		{"/a/", "#top", "", ""},
		{"/a/", "javascript:void(0)", "", ""},
		{"/a/", "?only=query", "", ""},
	}
	for _, c := range cases {
		to, kind := g.classifyRef(c.from, c.ref)
		if to != c.to || kind != c.kind {
			t.Errorf("classifyRef(%q, %q) = %q,%q; want %q,%q", c.from, c.ref, to, kind, c.to, c.kind)
		}
	}
}

// TestOutputIsParsedOnce: the link checker and the graph share one walk of the
// output — the checker no longer discards what the graph needs.
func TestOutputIsParsedOnce(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\n[b](/b/)\n",
		"pages/b.md": "---\ntitle: B\nslug: b\nstatus: publish\ntype: page\n---\n\nB.\n",
	}, nil)
	cfg.SiteGraph = true
	cfg.CheckLinks = "warn"
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if gen.outputRefsParses != 1 {
		t.Errorf("output parsed %d times, want 1", gen.outputRefsParses)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, sitegraph.FileName)); err != nil {
		t.Error("graph not written alongside the link check")
	}
}

// TestSiteGraphOffWritesNothing: opt-in, like markdown_publish — and the
// derived manifests still come from the in-memory graph either way.
func TestSiteGraphOffWritesNothing(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n",
	}, nil)
	cfg.RouteManifest = true
	buildSiteFixture(t, cfg)
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, sitegraph.FileName)); err == nil {
		t.Error("site-graph.json written without site_graph: true")
	}
	routes := mustRead(t, filepath.Join(cfg.OutputDir, "routes.json"))
	if !strings.Contains(routes, `"path": "/a/"`) || !strings.Contains(routes, `"source": "a.md"`) {
		t.Errorf("routes.json is no longer derived correctly:\n%s", routes)
	}
	if strings.Contains(routes, "listing") {
		t.Error("routes.json must not gain the listing section it never had")
	}
}

// TestGraphPageOutputs: markdown_publish and outputs: [json] show up as the
// document's other addresses; translations come from the i18n pairing.
func TestGraphPageOutputs(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	g.config.MarkdownPublish = true
	g.config.Outputs = []string{"json"}
	g.translations = map[string][]Translation{"about": {{Lang: "en", URL: "/about/", IsCurrent: true}, {Lang: "pl", URL: "/pl/o-nas/"}}}
	node := g.graphPage(models.Page{Slug: "about", Title: "About", Type: "page", Category: "Misc"}, "page")

	if node.Outputs.Markdown == "" || node.Outputs.JSON != "/about/index.json" {
		t.Errorf("outputs = %+v", node.Outputs)
	}
	if len(node.Translations) != 1 || node.Translations[0].Lang != "pl" {
		t.Errorf("translations = %+v (current must be excluded)", node.Translations)
	}
	if len(node.Categories) != 1 || node.Categories[0] != "Misc" {
		t.Errorf("free-form category not used as fallback: %v", node.Categories)
	}
}

// TestWriteSiteGraphReportsAnUnwritableOutput: the artifact failing to land is
// a build error, not a warning — a site that asked for its graph and did not
// get one has a broken build.
func TestWriteSiteGraphReportsAnUnwritableOutput(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SiteGraph = true
	g.config.Quiet = true
	g.config.OutputDir = filepath.Join(t.TempDir(), "missing")
	if err := g.writeSiteGraph(); err == nil || !strings.Contains(err.Error(), "site graph") {
		t.Errorf("missing output dir: got %v", err)
	}
	// And the chatty path, which prints the summary line.
	g.config.Quiet = false
	g.config.OutputDir = t.TempDir()
	if err := g.writeSiteGraph(); err != nil {
		t.Errorf("writable output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, sitegraph.FileName)); err != nil {
		t.Error("artifact not written")
	}
}

// TestGraphSectionsListsPostListings: every non-root post listing is a section
// of kind "listing"; the root one is the home page, not a section.
func TestGraphSectionsListsPostListings(t *testing.T) {
	g := newTestGen(t, "")
	g.postsListings = map[string]bool{"": true, "pl/blog": true, "blog": true}
	got := g.graphSections()
	if len(got) != 2 || got[0].Path != "/blog/" || got[1].Path != "/pl/blog/" || got[0].Kind != "listing" {
		t.Errorf("sections = %+v", got)
	}
	if empty := (&Generator{}).graphSections(); len(empty) != 0 {
		t.Errorf("bare generator: %+v", empty)
	}
}
