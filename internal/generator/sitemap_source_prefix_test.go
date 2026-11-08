package generator

// Splitting a generated site's sitemap by external source and by URL prefix
// (#386): pages built from external_sources had no source to select on, and
// every port, compare and vuln page was one undifferentiated block of `pages`.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/externalsource"
	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// TestSitemapSplitsByExternalSourceAndPrefix builds a site whose pages come
// from an external source and routes them by system prefix, then by source.
func TestSitemapSplitsByExternalSourceAndPrefix(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[
		{"origin":"openbsd/curl","slug":"o-curl","maintainerTerm":"a@example.com"},
		{"origin":"netbsd/curl","slug":"n-curl","maintainerTerm":"a@example.com"},
		{"origin":"net/curl","slug":"f-curl","maintainerTerm":"a@example.com"}
	]`)
	cfg.Sitemaps = []models.SitemapSpec{
		{Path: "/sitemap-openbsd.xml", Prefix: models.PathList{"/openbsd/"}},
		{Path: "/sitemap-net.xml", Prefix: models.PathList{"/net/"}},
		{Path: "/sitemap-ports.xml", Source: "ports"},
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string { return mustRead(t, filepath.Join(cfg.OutputDir, name)) }

	if s := read("sitemap-openbsd.xml"); !strings.Contains(s, "/openbsd/curl/") || strings.Contains(s, "/netbsd/") {
		t.Errorf("openbsd sitemap:\n%s", s)
	}
	if s := read("sitemap-net.xml"); !strings.Contains(s, "/net/curl/") || strings.Contains(s, "/netbsd/") {
		t.Errorf("/net/ must not claim /netbsd/:\n%s", s)
	}
	ports := read("sitemap-ports.xml")
	if !strings.Contains(ports, "/netbsd/curl/") || strings.Contains(ports, "/openbsd/") || strings.Contains(ports, "/news/") {
		t.Errorf("ports sitemap holds what is left of the source, and only that:\n%s", ports)
	}
	main := read("sitemap-main.xml")
	if strings.Contains(main, "curl") || !strings.Contains(main, "<loc>https://example.com/</loc>") {
		t.Errorf("the remainder:\n%s", main)
	}
	if idx := read("sitemap.xml"); !strings.Contains(idx, "<sitemapindex") || !strings.Contains(idx, "sitemap-ports.xml") {
		t.Errorf("index:\n%s", idx)
	}
}

// TestSitemapUnknownSourceFailsTheBuild: a name that is neither a folder nor
// an external source stops the build instead of writing nothing.
func TestSitemapUnknownSourceFailsTheBuild(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[{"origin":"x/a","slug":"x-a","maintainerTerm":"a@example.com"}]`)
	cfg.Sitemaps = []models.SitemapSpec{{Path: "/sitemap-p.xml", Source: "prots"}}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), `"prots"`) || !strings.Contains(err.Error(), "external_sources") {
		t.Fatalf("err = %v", err)
	}
}

// TestKnownSource: each of the ways a source name can exist.
func TestKnownSource(t *testing.T) {
	g := newTestGen(t, "")
	tmp := t.TempDir()
	g.config.ContentDir, g.config.Source = tmp, "site"
	if err := os.MkdirAll(filepath.Join(tmp, "site", "drafts-only"), 0o750); err != nil {
		t.Fatal(err)
	}
	g.config.ExternalSources.Sources = map[string]externalsource.SourceConfig{"ports": {}}
	g.siteData.Pages = []models.Page{{Section: "guides"}}
	g.siteData.Posts = []models.Page{{SourceDir: filepath.Join(tmp, "elsewhere", "blog")}}
	for name, want := range map[string]bool{
		"ports": true, "/ports/": true, "drafts-only": true, "guides": true, "blog": true,
		"nothing": false,
	} {
		if got := g.knownSource(name); got != want {
			t.Errorf("knownSource(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestSitemapSpecMatchesSourcesAndPrefix: the new narrowings, alone and ANDed.
func TestSitemapSpecMatchesSourcesAndPrefix(t *testing.T) {
	port := sitemapEntry{kind: kindPage, external: "ports"}
	doc := sitemapEntry{kind: kindPage, section: "guides"}
	for _, c := range []struct {
		spec  models.SitemapSpec
		entry sitemapEntry
		path  string
		want  bool
	}{
		{models.SitemapSpec{Source: "ports"}, port, "", true},
		{models.SitemapSpec{Source: "compare"}, port, "", false},
		{models.SitemapSpec{Source: "guides"}, doc, "", true},
		{models.SitemapSpec{Prefix: models.PathList{"/openbsd/"}}, port, "/openbsd/x/", true},
		{models.SitemapSpec{Prefix: models.PathList{"/openbsd/"}}, port, "/netbsd/x/", false},
		{models.SitemapSpec{Source: "ports", Prefix: models.PathList{"/openbsd/"}}, doc, "/openbsd/x/", false},
		{models.SitemapSpec{Include: []string{"tags"}, Prefix: models.PathList{"/tag/"}}, sitemapEntry{kind: kindTag}, "/tag/go/", true},
	} {
		if got := sitemapSpecMatches(c.spec, c.entry, c.path); got != c.want {
			t.Errorf("%+v / %+v / %q = %v", c.spec, c.entry, c.path, got)
		}
	}
}

// TestSitePath: the path after the domain (with its base path) and after a
// language prefix, as `prefix:` compares it.
func TestSitePath(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "example.com/site"
	for loc, want := range map[string]string{
		"https://example.com/site/openbsd/x/":    "/openbsd/x/",
		"https://example.com/site":               "",
		"https://example.com/site/a/?p=2#top":    "/a/",
		"https://example.com/sitemap/":           "", // not under the base path
		"https://elsewhere.example.org/openbsd/": "",
	} {
		if got := g.sitePath(loc); got != want {
			t.Errorf("sitePath(%q) = %q, want %q", loc, got, want)
		}
	}

	g.config.Domain = "example.com"
	g.config.DefaultLanguage = "pl"
	g.config.I18n = ssgi18n.Config{Enabled: true}
	g.siteData.Languages = []ssgi18n.LanguageConfig{{Code: "pl"}, {Code: "en"}}
	for loc, want := range map[string]string{
		"https://example.com/en/openbsd/x/": "/openbsd/x/",
		"https://example.com/en":            "/",
		"https://example.com/openbsd/x/":    "/openbsd/x/",
		"https://example.com/engine/x/":     "/engine/x/",
	} {
		if got := g.sitePath(loc); got != want {
			t.Errorf("i18n sitePath(%q) = %q, want %q", loc, got, want)
		}
	}
}

// TestFeedSelectsBySourceNameAndPrefix: `feeds:` shares the narrowings.
func TestFeedSelectsBySourceNameAndPrefix(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{
		{Title: "a", Slug: "a", Link: "/vuln/a/", ExternalSource: "vuln"},
		{Title: "b", Slug: "b", Link: "/compare/b/", ExternalSource: "compare"},
		{Title: "c", Slug: "c", Link: "/compare/c/", SourceDir: "content/site/posts/blog"},
	}
	titles := func(ps []models.Page) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Title)
		}
		return strings.Join(out, ",")
	}
	if got := titles(g.selectFeedPosts(models.FeedSpec{Source: "vuln"})); got != "a" {
		t.Errorf("by source name: %s", got)
	}
	if got := titles(g.selectFeedPosts(models.FeedSpec{Prefix: models.PathList{"/compare/"}})); got != "b,c" {
		t.Errorf("by prefix: %s", got)
	}
	if got := titles(g.selectFeedPosts(models.FeedSpec{Source: "blog", Prefix: models.PathList{"/compare/"}})); got != "c" {
		t.Errorf("AND: %s", got)
	}
}

// TestStampSource: merged imports carry their source name, and a page that
// already has one keeps it.
func TestStampSource(t *testing.T) {
	pages := stampSource([]models.Page{{Title: "a"}, {Title: "b", ExternalSource: "kept"}}, "ports")
	if pages[0].ExternalSource != "ports" || pages[1].ExternalSource != "kept" {
		t.Errorf("%+v", pages)
	}
}

// TestASplitIsReported: a declared file over the ceiling is split, indexed
// and said so — the same as before #386, now for a prefix-selected file.
func TestASplitIsReported(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SitemapMaxURLs = 1
	f := sitemapFile{path: "sitemap-freebsd.xml", entries: []sitemapEntry{{loc: "https://example.com/a/"}, {loc: "https://example.com/b/"}}}
	var parts []sitemapFile
	out, _ := capture(t, func() error { parts = g.splitSitemap(f); return nil })
	if len(parts) != 2 || parts[0].path != "sitemap-freebsd-1.xml" || !strings.Contains(out, "was split into 2 files") {
		t.Errorf("parts %v, output %q", parts, out)
	}
}

// TestSplitSitemapsKeepLastmod: a split moves URLs between files, never their
// dates — each <url> keeps its record's modified (or date), and the index
// gives every file the newest date among its entries.
func TestSplitSitemapsKeepLastmod(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[
		{"origin":"openbsd/curl","slug":"o-curl","maintainerTerm":"a@example.com","updated":"2026-10-01","created":"2026-01-01"},
		{"origin":"net/curl","slug":"f-curl","maintainerTerm":"a@example.com","created":"2026-09-15"}
	]`)
	src := cfg.ExternalSources.Sources["ports"]
	src.ContentMap.Date, src.ContentMap.Modified = "created", "updated"
	cfg.ExternalSources.Sources["ports"] = src
	cfg.Sitemaps = []models.SitemapSpec{
		{Path: "/sitemap-openbsd.xml", Prefix: models.PathList{"/openbsd/"}},
		{Path: "/sitemap-ports.xml", Source: "ports"},
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string { return mustRead(t, filepath.Join(cfg.OutputDir, name)) }

	if s := read("sitemap-openbsd.xml"); !strings.Contains(s, "<lastmod>2026-10-01") {
		t.Errorf("modified wins over date:\n%s", s)
	}
	if s := read("sitemap-ports.xml"); !strings.Contains(s, "<lastmod>2026-09-15") {
		t.Errorf("date when nothing was modified:\n%s", s)
	}
	idx := read("sitemap.xml")
	for _, want := range []string{
		"sitemap-openbsd.xml</loc>\n    <lastmod>2026-10-01",
		"sitemap-ports.xml</loc>\n    <lastmod>2026-09-15",
	} {
		if !strings.Contains(idx, want) {
			t.Errorf("index lacks %q:\n%s", want, idx)
		}
	}
}
