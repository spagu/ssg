package generator

// Output-check gaps (#75/#76/#77/#87): the checks must never be the thing that
// breaks a build (unreadable files are skipped), yet a missing output tree is a
// real error; plus the small parsing guards the happy-path tests never touch.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/spagu/ssg/internal/models"
)

// TestWalkOutputHTMLSkipsUnreadable: a file the walker cannot open is skipped
// rather than failing the build — a check reports problems, it must not add one.
func TestWalkOutputHTMLSkipsUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything; the permission guard cannot trigger")
	}
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "locked.html"), "<html></html>")
	mustWrite(t, filepath.Join(out, "open.html"), "<html></html>")
	if err := os.Chmod(filepath.Join(out, "locked.html"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(out, "locked.html"), 0o644) }) // #nosec G302 -- restoring perms on a test temp file

	g := &Generator{config: Config{OutputDir: out, Quiet: true}}
	var visited []string
	if err := g.walkOutputHTML(func(rel string, _ *html.Node) { visited = append(visited, rel) }); err != nil {
		t.Fatalf("unreadable file must be skipped, not fatal: %v", err)
	}
	if len(visited) != 1 || visited[0] != "open.html" {
		t.Errorf("visited = %v, want only open.html", visited)
	}
}

// TestOutputChecksMissingOutputDir: with no output tree at all the checks fail
// — that is not "nothing to report", it is a build that never produced output.
func TestOutputChecksMissingOutputDir(t *testing.T) {
	g := &Generator{config: Config{OutputDir: filepath.Join(t.TempDir(), "never-built"),
		Quiet: true, CheckImages: "warn", CheckMeta: "warn", CheckOrphans: "warn",
		CheckRedirects: "warn", PrettyURLs: models.PrettyStripSlash}}
	for name, check := range map[string]func() error{
		"images":    g.checkImagesIfRequested,
		"meta":      g.checkMetaIfRequested,
		"orphans":   g.checkOrphansIfRequested,
		"redirects": g.checkRedirectsIfRequested,
	} {
		if err := check(); err == nil {
			t.Errorf("check %s: a missing output dir must error", name)
		}
	}
}

// TestCheckImagesNoSrc: an <img> with neither src nor alt is still reported —
// labelled "(no src)" so the finding names something.
func TestCheckImagesNoSrc(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"), `<html><body><img></body></html>`)
	g := &Generator{config: Config{OutputDir: out, Quiet: true, CheckImages: "strict"}}
	if err := g.checkImagesIfRequested(); err == nil {
		t.Fatal("an alt-less image must fail strict mode even without a src")
	}
}

// TestLengthFindingSkipsEmpty: emptiness is a hard finding elsewhere — the
// length advisory stays silent about it.
func TestLengthFindingSkipsEmpty(t *testing.T) {
	if got := lengthFinding("title", "   ", 10, 20); got != "" {
		t.Errorf("whitespace-only value must yield no length finding, got %q", got)
	}
}

// TestMetaAndTitleFirstWins: duplicate meta/title tags resolve to the first —
// which is what a browser and a crawler read.
func TestMetaAndTitleFirstWins(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<html><head>
		<title>First</title><title>Second</title>
		<meta name="description" content="one">
		<meta name="description" content="two">
	</head><body></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := elementText(doc, "title"); got != "First" {
		t.Errorf("title = %q, want the first one", got)
	}
	if got, ok := metaContent(doc, "description"); !ok || got != "one" {
		t.Errorf("description = %q/%v, want the first one", got, ok)
	}
}

// TestCheckOrphansIgnoresNonLinks: anchors without href and external links buy
// a page nothing — a page referenced only that way is still an orphan.
func TestCheckOrphansIgnoresNonLinks(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"),
		`<html><body><a name="top">anchor</a><a href="https://other.site/">ext</a></body></html>`)
	mustWrite(t, filepath.Join(out, "lonely", "index.html"), `<html><body>alone</body></html>`)
	g := &Generator{config: Config{OutputDir: out, Quiet: true, CheckOrphans: "strict"}}
	if err := g.checkOrphansIfRequested(); err == nil {
		t.Fatal("a page linked by nothing must fail strict orphan checking")
	}
}

// TestRenderedExcludesItselfNoPath: a page that emitted no output path cannot
// exclude itself — there is no file to consult.
func TestRenderedExcludesItselfNoPath(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir()}}
	if g.renderedExcludesItself(models.Page{}, "") {
		t.Fatal("a page without an output path must not be excluded")
	}
}

// TestCanonicalPointsElsewhereGuards: the first canonical wins, and a document
// without one contradicts nothing.
func TestCanonicalPointsElsewhereGuards(t *testing.T) {
	two, err := html.Parse(strings.NewReader(`<html><head>
		<link rel="canonical" href="https://ex.com/a/">
		<link rel="canonical" href="https://ex.com/b/">
	</head></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if canonicalPointsElsewhere(two, "https://ex.com/a/") {
		t.Error("the first canonical is the page's own URL — no contradiction")
	}
	none, err := html.Parse(strings.NewReader(`<html><head></head></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if canonicalPointsElsewhere(none, "https://ex.com/a/") {
		t.Error("no canonical at all must not exclude the page")
	}
}

// TestSameURLUnparseable: URLs the parser rejects fall back to string equality
// — never a panic, never a spurious match.
func TestSameURLUnparseable(t *testing.T) {
	if !sameURL(":not-a-url", ":not-a-url") {
		t.Error("identical unparseable strings must compare equal")
	}
	if sameURL(":not-a-url", "https://ex.com/") {
		t.Error("an unparseable URL must not match a real one")
	}
}

// TestFillMetaDescriptionNoHead: with nowhere to inject, the page is returned
// unchanged rather than corrupted.
func TestFillMetaDescriptionNoHead(t *testing.T) {
	if got := fillMetaDescription("<p>fragment</p>", "desc"); got != "<p>fragment</p>" {
		t.Errorf("head-less fragment must pass through, got %q", got)
	}
}

// TestMatchGlobMixedWildcards: `*` and `?` keep their single-segment meaning
// inside a pattern that also uses `**`.
func TestMatchGlobMixedWildcards(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"docs/**/x*.m?", "docs/a/b/xfile.md", true},
		{"docs/**/x*.m?", "docs/a/yfile.md", false},
		{"docs/**/f?.md", "docs/f1.md", true}, // **/ may match zero segments
		{"docs/**/x*.m?", "docs/a/xf.html", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.name); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// TestOutputDirExistsRelative: relative links are check_links' job — the
// redirect model only answers for root-relative ones.
func TestOutputDirExistsRelative(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir()}}
	if g.outputDirExists("docs/intro") {
		t.Fatal("a relative link must not be modelled as a served directory")
	}
}
