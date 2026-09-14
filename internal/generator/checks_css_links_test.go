package generator

// check_links follows url() and @import in stylesheets (#286).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCSSRefs pins what counts as a reference, and the line it is reported on.
func TestCSSRefs(t *testing.T) {
	css := "@import \"base.css\";\n" +
		"/* url(/commented-out.png) spans\n two lines */\n" +
		"@font-face { src: url('/fonts/inter.woff2') format('woff2'); }\n" +
		".a { background: URL( ../img/bg.png ) }\n" +
		"@import url(\"print.css\") print;\n" +
		".b { mask: url() }\n"
	got := cssRefs(css)
	want := []cssRef{
		{"base.css", 1},
		{"/fonts/inter.woff2", 4},
		{"../img/bg.png", 5},
		{"print.css", 6},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ref %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// cssSite builds a site whose stylesheet is css, returning the generator, the
// build output and the build error.
func cssSite(t *testing.T, strict bool, css string) (*Generator, string, error) {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\n---\n\nAbout.\n",
	}, func(string) string {
		return `<html><head><link rel="stylesheet" href="/css/site.css"></head><body><p>x</p></body></html>`
	})
	cfg.CheckLinks = "warn"
	if strict {
		cfg.CheckLinks = "strict"
	}
	static := filepath.Join(filepath.Dir(cfg.ContentDir), "static")
	cfg.StaticDir = static
	mustWrite(t, filepath.Join(static, "css", "site.css"), css)
	mustWrite(t, filepath.Join(static, "css", "img", "ok.png"), "png")
	mustWrite(t, filepath.Join(static, "fonts", "inter.woff2"), "font")
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var buildErr error
	out := captureGeneratorStdout(t, func() { buildErr = gen.Generate() })
	return gen, out, buildErr
}

// TestARenamedFontFailsStrictLinkChecking: the reported case. The page's own
// <link> resolves; the file the stylesheet names does not.
func TestARenamedFontFailsStrictLinkChecking(t *testing.T) {
	_, _, err := cssSite(t, true, "@font-face { src: url('/fonts/inter-renamed.woff2'); }\n")
	if err == nil || !strings.Contains(err.Error(), "broken internal link") {
		t.Fatalf("a missing @font-face source must fail a strict build, got %v", err)
	}
}

// TestCSSReferencesResolveAgainstTheStylesheet: relative targets resolve from
// the stylesheet's directory, as a browser resolves them, and what exists,
// what is external and what is inline data is not reported.
func TestCSSReferencesResolveAgainstTheStylesheet(t *testing.T) {
	gen, out, err := cssSite(t, false,
		".a{background:url(img/ok.png)}\n"+
			"@font-face{src:url(\"../fonts/inter.woff2?v=2#x\")}\n"+
			".b{background:url(https://cdn.example.net/x.png)}\n"+
			".c{background:url(data:image/png;base64,AAAA)}\n"+
			".d{filter:url(#blur)}\n"+
			".e{background:url(https://example.com/fonts/inter.woff2)}\n"+
			".f{background:url(img/missing.png)}\n")
	if err != nil {
		t.Fatalf("warn mode must not fail: %v", err)
	}
	broken, err := gen.checkCSSLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || broken[0].from != "css/site.css:7" || broken[0].href != "img/missing.png" {
		t.Errorf("want only css/site.css:7 → img/missing.png, got %+v", broken)
	}
	if !strings.Contains(out, "broken link in css/site.css:7 → img/missing.png") {
		t.Errorf("the report must name the stylesheet and line:\n%s", out)
	}
}

// TestCheckCSSLinksReportsAnUnreadableTree: a walk error is the build's error,
// not a silent pass.
func TestCheckCSSLinksReportsAnUnreadableTree(t *testing.T) {
	g := newTestGen(t, "")
	g.config.OutputDir = filepath.Join(t.TempDir(), "absent")
	if _, err := g.checkCSSLinks(); err == nil {
		t.Error("a missing output directory must be reported")
	}
	dir := t.TempDir()
	g.config.OutputDir = dir
	css := filepath.Join(dir, "broken.css")
	if err := os.Mkdir(css, 0o755); err != nil {
		t.Fatal(err)
	}
	if broken, err := g.checkCSSLinks(); err != nil || len(broken) != 0 {
		t.Errorf("a directory named like a stylesheet is not one: %v %v", broken, err)
	}
}
