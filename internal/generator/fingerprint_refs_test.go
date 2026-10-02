package generator

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFingerprintRewritesOnlyReferences is #316: an asset name is rewritten
// where the page references the site's own file, and nowhere else.
func TestFingerprintRewritesOnlyReferences(t *testing.T) {
	rw := newAssetRefRewriter(map[string]string{"app.js": "app.c9c8999c.js", "site.css": "site.0a1b2c3d.css"},
		siteHosts("https://www.example.com/docs"))
	in := `<html><head><link rel="stylesheet" href="/css/site.css">` +
		`<style>@import "site.css"; body{background:url(/img/x.png)}</style>` +
		`<script type="module">import "./js/app.js"; const cdn = "https://cdn.example.org/pkg/app.js";</script></head>` +
		`<body><script src="/js/app.js"></script><script src="https://example.com/js/app.js"></script>` +
		`<script src="https://cdn.example.org/npm/pkg/dist/app.js"></script><script src='//cdn.example.org/app.js'></script>` +
		`<a href=js/app.js>own, unquoted</a><div style="background:url('/css/site.css')"></div>` +
		`<img srcset="/x.png 1x" data-src="/js/app.js" alt="dist/app.js">` +
		`<p>The bundle (dist/app.js) is small.</p>` +
		`<pre><code>&lt;script src="https://cdn.example.org/npm/pkg/dist/app.js"&gt;&lt;/script&gt;</code></pre>` +
		`<!-- see /js/app.js --></body></html>`
	want := `<html><head><link rel="stylesheet" href="/css/site.0a1b2c3d.css">` +
		`<style>@import "site.0a1b2c3d.css"; body{background:url(/img/x.png)}</style>` +
		`<script type="module">import "./js/app.c9c8999c.js"; const cdn = "https://cdn.example.org/pkg/app.js";</script></head>` +
		`<body><script src="/js/app.c9c8999c.js"></script><script src="https://example.com/js/app.c9c8999c.js"></script>` +
		`<script src="https://cdn.example.org/npm/pkg/dist/app.js"></script><script src='//cdn.example.org/app.js'></script>` +
		`<a href=js/app.c9c8999c.js>own, unquoted</a><div style="background:url('/css/site.0a1b2c3d.css')"></div>` +
		`<img srcset="/x.png 1x" data-src="/js/app.c9c8999c.js" alt="dist/app.js">` +
		`<p>The bundle (dist/app.js) is small.</p>` +
		`<pre><code>&lt;script src="https://cdn.example.org/npm/pkg/dist/app.js"&gt;&lt;/script&gt;</code></pre>` +
		`<!-- see /js/app.js --></body></html>`
	if got := rw.rewriteHTML(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// TestFingerprintAssetFilesKeepForeignURLs: inside CSS and JS the same host
// rule holds — a CDN URL keeps its name, a local reference is rewritten.
func TestFingerprintAssetFilesKeepForeignURLs(t *testing.T) {
	by := map[string]string{"app.js": "app.c9c8999c.js", "font.css": "font.11112222.css"}
	css := `@import url("https://fonts.example.net/font.css"); @import "font.css";`
	if got := rewriteAssetRefs(css, by, siteHosts("example.com")); got != `@import url("https://fonts.example.net/font.css"); @import "font.11112222.css";` {
		t.Errorf("css = %s", got)
	}
	js := `import a from "./app.js"; import b from "https://esm.example.net/app.js";`
	if got := rewriteAssetRefs(js, by, nil); got != `import a from "./app.c9c8999c.js"; import b from "https://esm.example.net/app.js";` {
		t.Errorf("js = %s", got)
	}
}

func TestSiteHosts(t *testing.T) {
	for domain, want := range map[string]string{
		"example.com":                  "example.com,www.example.com",
		"https://WWW.Example.com/path": "example.com,www.example.com",
		"example.com:8080":             "example.com,www.example.com",
		"":                             "",
	} {
		if got := strings.Join(siteHosts(domain), ","); got != want {
			t.Errorf("siteHosts(%q) = %q, want %q", domain, got, want)
		}
	}
	rw := &assetRefRewriter{}
	if !rw.foreignURLAt("x https://%zz/app.js", len("x https://%zz/")) {
		t.Error("an unparseable absolute URL is not provably the site's own")
	}
}

// TestFingerprintBuildLeavesPageText runs the whole pass over an output tree.
func TestFingerprintBuildLeavesPageText(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "js", "app.js"), "console.log(1)")
	mustWrite(t, filepath.Join(out, "index.html"),
		`<script src="/js/app.js"></script><p>Copy <code>https://cdn.example.org/app.js</code> or (dist/app.js).</p>`)
	g := &Generator{config: Config{OutputDir: out, Domain: "example.com", Fingerprint: true, Quiet: true}}
	if err := g.fingerprintAssets(); err != nil {
		t.Fatal(err)
	}
	html := readFileString(t, filepath.Join(out, "index.html"))
	if strings.Contains(html, `src="/js/app.js"`) || !strings.Contains(html, "<code>https://cdn.example.org/app.js</code>") ||
		!strings.Contains(html, "(dist/app.js)") {
		t.Errorf("page = %s", html)
	}
}
