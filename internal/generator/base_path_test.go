package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithBasePath(t *testing.T) {
	for in, want := range map[string]string{
		"/docs/":               "/site/docs/",
		"/":                    "/site/",
		"/site/docs/":          "/site/docs/",
		"/site":                "/site",
		"/site?x":              "/site?x",
		"/sitemap.xml":         "/site/sitemap.xml",
		"//cdn.example/x":      "//cdn.example/x",
		"https://example.com/": "https://example.com/",
		"docs/":                "docs/",
		"#top":                 "#top",
		"":                     "",
	} {
		if got := withBasePath(in, "/site"); got != want {
			t.Errorf("withBasePath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := withBasePath("/docs/", ""); got != "/docs/" {
		t.Errorf("no base must leave the URL alone, got %q", got)
	}
}

func TestCutBasePath(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"/site/docs/", "/docs/", true},
		{"/site", "/", true},
		{"/site#x", "/#x", true},
		{"/docs/", "/docs/", false},
		{"/sitemap.xml", "/sitemap.xml", false},
	} {
		got, ok := cutBasePath(tc.in, "/site")
		if got != tc.want || ok != tc.ok {
			t.Errorf("cutBasePath(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if got, ok := cutBasePath("/docs/", ""); got != "/docs/" || !ok {
		t.Errorf("no base: %q, %v", got, ok)
	}
}

// TestBasePathHTML: every URL-valued place in a page, and nothing else.
func TestBasePathHTML(t *testing.T) {
	in := `<a href="/docs/">d</a><img src=/i/a.png srcset="/i/a.png 1x, /i/b.png 2x">` +
		`<meta http-equiv="refresh" content="0; url=/new/">` +
		`<div style="background:url('/i/bg.png')"></div>` +
		`<style>body{background:url(/i/s.png)}</style>` +
		`<a href="https://example.com/x">e</a><a href="rel/">r</a><a href="/site/kept/">k</a>` +
		`<pre><code>make src=/usr/bin &lt;a href=&quot;/x&quot;&gt;</code></pre>`
	want := `<a href="/site/docs/">d</a><img src=/site/i/a.png srcset="/site/i/a.png 1x, /site/i/b.png 2x">` +
		`<meta http-equiv="refresh" content="0; url=/site/new/">` +
		`<div style="background:url('/site/i/bg.png')"></div>` +
		`<style>body{background:url(/site/i/s.png)}</style>` +
		`<a href="https://example.com/x">e</a><a href="rel/">r</a><a href="/site/kept/">k</a>` +
		`<pre><code>make src=/usr/bin &lt;a href=&quot;/x&quot;&gt;</code></pre>`
	if got := basePathHTML(in, "/site"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := basePathHTML(want, "/site"); got != want {
		t.Error("the pass must be idempotent")
	}
}

// TestBasePathBuild is #306 end to end: a site served under /site builds with
// every internal link prefixed, check_links passes in strict mode, and the
// search index carries the served URL. With host_files: false no _headers or
// _redirects are written (#308); with autolinking off, a matching list item
// stays text (#305).
func TestBasePathBuild(t *testing.T) {
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "pages", "docs.md"),
		"---\ntitle: Documentation\nslug: docs\nstatus: publish\ntype: page\n---\n\nSee [home](/) and [about](/about/).\n")
	mustWrite(t, filepath.Join(content, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\n- Documentation\n")
	tmplDir := filepath.Join(tmp, "templates", "simple")
	for _, name := range []string{"base.html", "index.html", "post.html", "category.html"} {
		mustWrite(t, filepath.Join(tmplDir, name), `{{define "`+name+`"}}<html><head><link rel="stylesheet" href="/css/style.css"></head><body><a href="/docs/">docs</a></body></html>{{end}}`)
	}
	mustWrite(t, filepath.Join(tmplDir, "page.html"), `{{define "page.html"}}<html><head><link rel="stylesheet" href="/css/style.css"></head><body>{{ .Page.Content | safeHTML }}</body></html>{{end}}`)
	mustWrite(t, filepath.Join(tmplDir, "css", "style.css"), `body{background:url(/images/bg.png)}`)
	mustWrite(t, filepath.Join(tmplDir, "images", "bg.png"), "png")

	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "user.github.io/site",
		BasePath: "/site", NoHostFiles: true, NoAutolinkLists: true, SearchIndex: true,
		CheckLinks: "strict", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: out, Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate with check_links strict: %v", err)
	}
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	docs := read("docs/index.html")
	for _, want := range []string{`href="/site/css/style.css"`, `href="/site/"`, `href="/site/about/"`} {
		if !strings.Contains(docs, want) {
			t.Errorf("docs page lacks %s:\n%s", want, docs)
		}
	}
	if css := read("css/style.css"); !strings.Contains(css, "url(/site/images/bg.png)") {
		t.Errorf("stylesheet url() not prefixed: %s", css)
	}
	if idx := read("search-index.json"); !strings.Contains(idx, `"url":"/site/docs/"`) {
		t.Errorf("search index URL not prefixed: %s", idx)
	}
	if about := read("about/index.html"); strings.Contains(about, "[Documentation]") || strings.Contains(about, `href="/site/docs/">Documentation`) {
		t.Errorf("autolinking was off, the list item must stay text: %s", about)
	}
	for _, name := range []string{"_headers", "_redirects"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Errorf("host_files: false must not write %s (err=%v)", name, err)
		}
	}
}

// TestCheckLinksOutsideBasePath: on a site under /site, a root-relative link
// to /elsewhere/ leaves the site, and the checker says so even when the file
// exists at the output root.
func TestCheckLinksOutsideBasePath(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"), `<a href="/elsewhere/">x</a><a href="/site/">home</a>`)
	mustWrite(t, filepath.Join(out, "elsewhere", "index.html"), `<p>x</p>`)
	g := &Generator{config: Config{OutputDir: out, BasePath: "/site", Quiet: true}}
	broken, err := g.checkLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || broken[0].href != "/elsewhere/" {
		t.Errorf("broken = %+v, want only /elsewhere/", broken)
	}
}

func TestApplyBasePathOff(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"), `<a href="/x/">x</a>`)
	g := &Generator{config: Config{OutputDir: out}}
	if err := g.applyBasePath(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "index.html")); string(b) != `<a href="/x/">x</a>` {
		t.Errorf("no base path must leave the output alone: %s", b)
	}
	g.config.BasePath = "/site"
	g.config.OutputDir = filepath.Join(out, "missing")
	if err := g.applyBasePath(); err == nil {
		t.Error("a missing output directory must be reported")
	}
}
