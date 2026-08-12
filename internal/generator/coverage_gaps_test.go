package generator

// Targeted tests for helpers with thin coverage (project-wide raise, 1.8.27).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"golang.org/x/net/html"
)

// TestTruncateRunes: rune-safe truncation never emits invalid UTF-8 (GO-021).
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("short string changed: %q", got)
	}
	if got := truncateRunes("hello", 2); got != "he" {
		t.Fatalf("ascii cut = %q", got)
	}
	// Multibyte: cutting bytes would split "ł"; runes must not.
	if got := truncateRunes("złoty", 2); got != "zł" {
		t.Fatalf("rune cut = %q", got)
	}
	if got := truncateRunes("你好世界", 2); got != "你好" {
		t.Fatalf("CJK cut = %q", got)
	}
}

// TestGeneratedTranslationKey: language suffixes are stripped from the source
// filename; empty results fall back to the slug.
func TestGeneratedTranslationKey(t *testing.T) {
	cases := []struct {
		file, slug, want string
	}{
		{"about.en.md", "about-en", "about"},
		{"about.pl.md", "x", "about"},
		{"guide_en-US.md", "x", "guide"},
		{"plain.md", "x", "plain"},
		{"UPPER.MD", "x", "upper"},
		{".md", "fallback", "fallback"}, // nothing left → slug
	}
	for _, c := range cases {
		p := models.Page{SourceFile: c.file, Slug: c.slug}
		if got := generatedTranslationKey(p); got != c.want {
			t.Errorf("generatedTranslationKey(%q) = %q, want %q", c.file, got, c.want)
		}
	}
}

func TestSanitizeWorkerName(t *testing.T) {
	cases := map[string]string{
		"comments":    "comments",
		"my_worker-2": "my_worker-2",
		"a/b\\c":      "a-b-c",
		"../../etc":   "------etc",
		"":            "worker",
		"///":         "worker", // maps to ---, trims to empty → fallback
		"żółć":        "worker", // all non-ASCII → dashes → fallback
	}
	for in, want := range cases {
		if got := sanitizeWorkerName(in); got != want {
			t.Errorf("sanitizeWorkerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToFloat(t *testing.T) {
	cases := []struct {
		in    interface{}
		want  float64
		isInt bool
		ok    bool
	}{
		{int(3), 3, true, true},
		{int8(3), 3, true, true},
		{int16(3), 3, true, true},
		{int32(3), 3, true, true},
		{int64(3), 3, true, true},
		{uint(4), 4, true, true},
		{float32(1.5), 1.5, false, true},
		{float64(2.5), 2.5, false, true},
		{"nope", 0, false, false},
	}
	for _, c := range cases {
		got, isInt, ok := toFloat(c.in)
		if got != c.want || isInt != c.isInt || ok != c.ok {
			t.Errorf("toFloat(%v) = %v %v %v; want %v %v %v", c.in, got, isInt, ok, c.want, c.isInt, c.ok)
		}
	}
}

// assertFeedPagination: page 1 links forward, page 2 links back (Atom/RSS per
// RFC 5005); JSON Feed 1.1 has only next_url — no prev in the spec.
func assertFeedPagination(t *testing.T, format, first, second string) {
	t.Helper()
	if format == "json" {
		if !strings.Contains(first, "next_url") {
			t.Errorf("json: page 1 without next_url:\n%s", first)
		}
		return
	}
	if !strings.Contains(first, `"next"`) && !strings.Contains(first, "rel=\"next\"") {
		t.Errorf("%s: page 1 without a next link:\n%s", format, first)
	}
	if !strings.Contains(second, "prev") {
		t.Errorf("%s: page 2 without a prev link:\n%s", format, second)
	}
}

// TestPaginatedDeclaredFeeds: paginate: 1 over three posts yields three pages
// linked with RFC 5005 first/last/prev/next, in both Atom and RSS.
func TestPaginatedDeclaredFeeds(t *testing.T) {
	for _, format := range []string{"atom", "rss", "json"} {
		g := feedGen(t)
		spec := models.FeedSpec{Path: "/arch.xml", Format: format, Paginate: 1}
		if err := g.writeDeclaredFeed(spec); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		first, err := os.ReadFile(filepath.Join(g.config.OutputDir, "arch.xml"))
		if err != nil {
			t.Fatalf("%s: page 1 missing: %v", format, err)
		}
		second, err := os.ReadFile(filepath.Join(g.config.OutputDir, "arch-2.xml"))
		if err != nil {
			t.Fatalf("%s: page 2 missing: %v", format, err)
		}
		assertFeedPagination(t, format, string(first), string(second))
		if _, err := os.Stat(filepath.Join(g.config.OutputDir, "arch-3.xml")); err != nil {
			t.Errorf("%s: page 3 missing (3 items / 1 per page)", format)
		}
	}
}

// TestCheckSchemaIfRequested: incomplete JSON-LD in the output is reported;
// warn passes the build, strict fails it, off skips the walk.
func TestCheckSchemaIfRequested(t *testing.T) {
	out := t.TempDir()
	page := `<html><head><script type="application/ld+json">
		{"@context":"https://schema.org","@type":"Article"}
	</script></head><body></body></html>` // Article without its required headline
	if err := os.MkdirAll(filepath.Join(out, "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "post", "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	// Off → no error, no walk.
	g := &Generator{config: Config{OutputDir: out, Quiet: true}}
	if err := g.checkSchemaIfRequested(); err != nil {
		t.Fatalf("off mode: %v", err)
	}
	// warn → reports (Article misses datePublished/author…) but passes.
	g = &Generator{config: Config{OutputDir: out, Quiet: true, CheckSchema: "warn"}}
	if err := g.checkSchemaIfRequested(); err != nil {
		t.Fatalf("warn mode must pass: %v", err)
	}
	// strict → the same findings fail the build.
	g = &Generator{config: Config{OutputDir: out, Quiet: true, CheckSchema: "strict"}}
	if err := g.checkSchemaIfRequested(); err == nil {
		t.Fatal("strict mode must fail on incomplete JSON-LD")
	}
}

// TestScanRegex: a `/` inside a character class does not close the literal; a
// newline means it was division, not a regex.
func TestScanRegex(t *testing.T) {
	// s[i] is the opening slash. /ab/ → index past the closing slash.
	if got := scanRegex("/ab/x", 0); got != 4 {
		t.Fatalf("plain regex end = %d", got)
	}
	// Character class holding a slash: /[/*]/ closes at the LAST slash, and the
	// trailing 'y' is a regex FLAG (dgimsuvy) — consumed too.
	if got := scanRegex("/[/*]/y", 0); got != 7 {
		t.Fatalf("class regex end = %d", got)
	}
	// Flags run: /ab/gi → past both flags.
	if got := scanRegex("/ab/gi;", 0); got != 6 {
		t.Fatalf("flags end = %d", got)
	}
	// Escaped slash does not close.
	if got := scanRegex(`/a\/b/z`, 0); got != 6 {
		t.Fatalf("escaped slash end = %d", got)
	}
	// Newline before closing → it was division; return one past the slash.
	if got := scanRegex("/ab\ncd/", 0); got != 1 {
		t.Fatalf("division end = %d", got)
	}
}

// TestScanTemplate: JS template-literal scanning honours escapes, closing
// backticks and ${ interpolations (minify safety).
func TestScanTemplate(t *testing.T) {
	// s[i] is the opening backtick.
	end, interp := scanTemplate("`abc`x", 0)
	if end != 5 || interp {
		t.Fatalf("plain literal: end=%d interp=%v", end, interp)
	}
	end, interp = scanTemplate("`a${b}`", 0)
	if end != 4 || !interp {
		t.Fatalf("interpolation: end=%d interp=%v", end, interp)
	}
	end, interp = scanTemplate("`a\\`b`", 0)
	if end != 6 || interp {
		t.Fatalf("escaped backtick: end=%d interp=%v", end, interp)
	}
	end, interp = scanTemplate("`never closed", 0)
	if end != 13 || interp {
		t.Fatalf("unterminated: end=%d interp=%v", end, interp)
	}
}

func TestParseAITimeout(t *testing.T) {
	if parseAITimeout("") != 0 || parseAITimeout("bogus") != 0 {
		t.Fatal("empty/bad duration → 0 (client default)")
	}
	if parseAITimeout("10s").Seconds() != 10 {
		t.Fatal("10s should parse")
	}
}

// TestWarnPrettyURLMismatch: the strip-slash+Cloudflare pairing warns; every
// other combination stays silent.
func TestWarnPrettyURLMismatch(t *testing.T) {
	// Not strip-slash → silent path.
	(&Generator{config: Config{PrettyURLs: models.PrettyStrip, Deploy: "cloudflare"}}).warnPrettyURLMismatch()
	// strip-slash but not Cloudflare → silent path.
	(&Generator{config: Config{PrettyURLs: models.PrettyStripSlash, Deploy: "netlify"}}).warnPrettyURLMismatch()
	// The mismatch itself → prints the warning (smoke: no panic).
	(&Generator{config: Config{PrettyURLs: models.PrettyStripSlash, Deploy: "cloudflare"}}).warnPrettyURLMismatch()
}

func TestJSONLDBlocks(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<html><head>
		<script type="application/ld+json">{"@type":"WebSite"}</script>
		<script type="APPLICATION/LD+JSON"> {"@type":"Article"} </script>
		<script type="text/javascript">ignore()</script>
		<script type="application/ld+json"></script>
	</head><body></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := jsonLDBlocks(doc)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (case-insensitive type, empty skipped): %q", len(blocks), blocks)
	}
}

func TestStrSlice(t *testing.T) {
	if got := strSlice([]string{"a", "b"}); len(got) != 2 || got[1] != "b" {
		t.Fatalf("[]string passthrough = %v", got)
	}
	if got := strSlice([]interface{}{"a", 7, "c"}); len(got) != 2 || got[1] != "c" {
		t.Fatalf("mixed []interface{} = %v", got)
	}
	if got := strSlice(42); got != nil {
		t.Fatalf("unsupported type = %v", got)
	}
}

// TestImageHelperAdapters: the thin facade adapters route to the processor and
// surface its validation errors (bad options → error, not panic).
func TestImageHelperAdapters(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir()}}
	if _, err := g.tmplImageCrop("missing.png", map[string]any{"width": 10, "height": 10}); err == nil {
		t.Fatal("crop of a missing source must error")
	}
	if _, err := g.tmplImageProcess("missing.png", []any{map[string]any{"op": "resize", "width": 10}}); err == nil {
		t.Fatal("process of a missing source must error")
	}
	if _, err := g.tmplImageFilter("missing.png", []any{"grayscale"}, map[string]any{}); err == nil {
		t.Fatal("filter of a missing source must error")
	}
	if _, err := g.tmplImagePicture("missing.png", map[string]any{"widths": []any{100}}); err == nil {
		t.Fatal("picture of a missing source must error")
	}
}
