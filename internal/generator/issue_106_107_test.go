package generator

// Tests for #106 (comment stripping that respects string literals) and #107
// (rewrite_md_links scope and pretty_urls agreement).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// --- #106 ------------------------------------------------------------------

// TestStripCommentsKeepsCommentCharactersInLiterals is the reported bug: the
// regex this replaced could not tell a comment from the same characters inside
// a string, so it deleted from the `/*` in one literal to the `*/` in another —
// taking the closing quote with it. The example below still parses after the
// damage, which is what made it silent.
func TestStripCommentsKeepsCommentCharactersInLiterals(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			"the reported case",
			`function f() { return "/*" + "x" + "*/"; }`,
			`function f() { return "/*" + "x" + "*/"; }`,
		},
		{"single quotes", `var s = '/* not a comment */';`, `var s = '/* not a comment */';`},
		{"template literal", "var t = `/* keep */`;", "var t = `/* keep */`;"},
		{"escaped quote before", `var s = "a\"/*b*/";`, `var s = "a\"/*b*/";`},
		{"url is not a line comment", `var u = "http://example.com";`, `var u = "http://example.com";`},
		// stylis' CSS-comment parser is what surfaced this in a vendored bundle.
		{"comment parser shape", `if (c === "/*") { return "*/"; }`, `if (c === "/*") { return "*/"; }`},
	} {
		if got := stripComments(tc.in, styleJS, false); got != tc.want {
			t.Errorf("%s:\n  got  %s\n  want %s", tc.name, got, tc.want)
		}
	}
}

// TestStripCommentsStillRemovesRealComments: the fix must not simply stop
// working.
func TestStripCommentsStillRemovesRealComments(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"block", "a();/* gone */b();", "a();b();"},
		{"line", "a(); // gone\nb();", "a(); \nb();"},
		{"block spanning lines", "a();/* one\ntwo */b();", "a();b();"},
	} {
		if got := stripComments(tc.in, styleJS, false); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestStripCommentsRespectsRegexLiterals: a regex holding comment characters is
// code, and `/[/*]/` is the case that would otherwise open a block comment.
func TestStripCommentsRespectsRegexLiterals(t *testing.T) {
	for _, in := range []string{
		`e.replace(/\/\*/g, "");`,
		`x = /[/*]/.test(s);`,
	} {
		if got := stripComments(in, styleJS, false); got != in {
			t.Errorf("regex mangled:\n  got  %s\n  want %s", got, in)
		}
	}
	// Division is not a regex: the comment after it is still removed.
	if got := stripComments(`var r = a / b; /* gone */`, styleJS, false); got != `var r = a / b; ` {
		t.Errorf("division: got %q", got)
	}
}

// TestStripCommentsLeavesUnterminatedBlockAlone: truncating the rest of a file
// because a comment was never closed would turn a typo into data loss.
func TestStripCommentsLeavesUnterminatedBlockAlone(t *testing.T) {
	in := "a();/* oops"
	if got := stripComments(in, styleJS, false); got != in {
		t.Errorf("got %q, want the input unchanged", got)
	}
}

// TestStripCommentsCSSStrings: `content: "/*"` is legal CSS and had the same
// defect.
func TestStripCommentsCSSStrings(t *testing.T) {
	if got := stripComments(`a{content:"/*"}/* gone */`, styleCSS, false); got != `a{content:"/*"}` {
		t.Errorf("got %q", got)
	}
	// A // in CSS is not a comment, so it must survive.
	if got := stripComments(`a{background:url(http://x/y.png)}`, styleCSS, false); !strings.Contains(got, "http://x/y.png") {
		t.Errorf("url mangled: %q", got)
	}
}

// TestMinifyJSFileKeepsStringLiterals exercises the real entry point, not just
// the scanner, so a future rewiring cannot reintroduce the bug unnoticed.
func TestMinifyJSFileKeepsStringLiterals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.js")
	mustWrite(t, path, "function f() { return \"/*\" + \"x\" + \"*/\"; }\n/* really a comment */\n")
	if err := minifyJSFile(path); err != nil {
		t.Fatalf("minifyJSFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"/*" + "x" + "*/"`) {
		t.Errorf("string literal was eaten:\n%s", got)
	}
	if strings.Contains(string(got), "really a comment") {
		t.Errorf("real comment survived:\n%s", got)
	}
}

// --- #107 ------------------------------------------------------------------

// mdRewriteGen builds a generator whose md-link map resolves "contributing.md".
func mdRewriteGen(mode models.PrettyURLMode) *Generator {
	return &Generator{
		config:   Config{Domain: "example.com", PrettyURLs: mode, RewriteMdLinks: true, Quiet: true},
		siteData: &models.SiteData{},
	}
}

// TestRewriteMdLinksSkipsExternalURLs is the reported bug: a link to the page's
// own history on GitHub ends in .md, so it was matched on its basename and
// rewritten to the very page containing it. check_links passes — the target
// exists — so nothing reported it.
func TestRewriteMdLinksSkipsExternalURLs(t *testing.T) {
	g := mdRewriteGen(models.PrettyOff)
	m := map[string]map[string]string{"privacy.md": {"": "/privacy.html"}}

	external := `<a href="https://github.com/tradik/schema-resume/commits/main/content/site/pages/privacy.md">repository</a>`
	if got := g.rewriteMdLinks(external, m); got != external {
		t.Errorf("external URL rewritten:\n  got  %s\n  want %s", got, external)
	}

	for _, href := range []string{
		`href="http://example.org/a/privacy.md"`,
		`href="//cdn.example.org/privacy.md"`,
	} {
		in := "<a " + href + ">x</a>"
		if got := g.rewriteMdLinks(in, m); got != in {
			t.Errorf("rewrote %s → %s", href, got)
		}
	}
}

// TestRewriteMdLinksStillRewritesRelative: scoping the feature must not disable
// it.
func TestRewriteMdLinksStillRewritesRelative(t *testing.T) {
	g := mdRewriteGen(models.PrettyOff)
	m := map[string]map[string]string{"privacy.md": {"": "/privacy.html"}}
	got := g.rewriteMdLinks(`<a href="./privacy.md">p</a>`, m)
	if !strings.Contains(got, `href="/privacy.html"`) {
		t.Errorf("relative link not rewritten: %s", got)
	}
}

// TestRewriteMdLinksFollowsPrettyURLs: the rewriter emitted ".html" while
// check_redirects — correctly — reported that same link as one the host
// redirects. The two disagreed about one link, and the author could not fix it
// in the Markdown without abandoning the feature.
func TestRewriteMdLinksFollowsPrettyURLs(t *testing.T) {
	m := map[string]map[string]string{"contributing.md": {"": "/contributing.html"}}
	for _, tc := range []struct {
		mode models.PrettyURLMode
		want string
	}{
		{models.PrettyOff, `href="/contributing.html"`},
		{models.PrettyStrip, `href="/contributing"`},
		{models.PrettyStripSlash, `href="/contributing/"`},
	} {
		got := mdRewriteGen(tc.mode).rewriteMdLinks(`<a href="./contributing.md">c</a>`, m)
		if !strings.Contains(got, tc.want) {
			t.Errorf("mode %q: got %s, want %s", tc.mode, got, tc.want)
		}
	}
}

// TestLinkTargetResolvesExtensionlessToFlatFile: with pretty_urls a nav links
// "/validator", which is served by validator.html — there is no directory.
// Comparing the pre-normalisation path reported every such page as an orphan
// while check_links resolved the very same links.
func TestLinkTargetResolvesExtensionlessToFlatFile(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "validator.html"), "x")
	mustWrite(t, filepath.Join(out, "docs", "index.html"), "x")

	g := &Generator{config: Config{OutputDir: out, PrettyURLs: models.PrettyStrip, Quiet: true}}
	if got := g.linkTarget("/validator", indexHTMLName); got != "validator.html" {
		t.Errorf("flat page: got %q, want validator.html", got)
	}
	// A real directory still resolves to its index, which is the common case.
	if got := g.linkTarget("/docs", indexHTMLName); got != "docs/index.html" {
		t.Errorf("directory: got %q, want docs/index.html", got)
	}

	// Without pretty_urls the host would not serve /validator at all, so the
	// historical resolution stands.
	off := &Generator{config: Config{OutputDir: out, PrettyURLs: models.PrettyOff, Quiet: true}}
	if got := off.linkTarget("/validator", indexHTMLName); got != "validator/index.html" {
		t.Errorf("pretty off: got %q, want validator/index.html", got)
	}
}
