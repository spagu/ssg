package generator

// Tests for the 2026-07-11 audit round fixes:
// SEC-014/SEC-015, GO-021/022/023/037, PERF-001/002/003/006/009.

import (
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spagu/ssg/internal/models"
)

// ─── SEC-014: sanitizer must hold on every render path ─────────────────────

func TestSafeHTMLSanitizesScript(t *testing.T) {
	g := newTestGen(t, "")
	g.sanitizer = newSanitizer(true)
	out := string(g.tmplSafeHTML(nil, nil)(`hello <script>alert(1)</script> world`))
	if strings.Contains(out, "<script>") {
		t.Errorf("sanitizer must strip <script>, got %q", out)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Errorf("sanitizer must keep the text, got %q", out)
	}
}

func TestPrepAltDataSanitizesContent(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	g.sanitizer = newSanitizer(true)
	// template.HTML form (sanitizer off in context) and plain-string form
	// (sanitizer on, SEC-014) must both come out sanitized for alt engines.
	for _, data := range []interface{}{
		map[string]interface{}{"Content": template.HTML(`x <script>alert(1)</script>`)},
		map[string]interface{}{"Content": `x <script>alert(1)</script>`},
	} {
		out, ok := g.prepAltData(data).(map[string]interface{})
		if !ok {
			t.Fatalf("prepAltData did not return a map")
		}
		s, _ := out["Content"].(string)
		if strings.Contains(s, "<script>") {
			t.Errorf("alt-engine Content must be sanitized, got %q", s)
		}
	}
}

func TestFeedFullContentSanitized(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	g.sanitizer = newSanitizer(true)
	g.config.FeedFullContent = true
	var sb strings.Builder
	g.writeFeedEntry(&sb, models.Page{Title: "t", Slug: "s", Content: `x <script>alert(1)</script>`})
	if strings.Contains(sb.String(), "&lt;script&gt;") {
		t.Errorf("feed full content must be sanitized before XML-escaping: %q", sb.String())
	}
}

func TestContentContextValueGatedBySanitizer(t *testing.T) {
	g := newTestGen(t, "")
	if _, ok := g.contentContextValue("x").(string); ok {
		t.Errorf("without sanitizer Content must stay template.HTML (backward compat)")
	}
	g.sanitizer = newSanitizer(true)
	if _, ok := g.contentContextValue("x").(string); !ok {
		t.Errorf("with sanitizer Content must be a plain (auto-escaped) string")
	}
}

// ─── GO-037: trusted shortcode output survives the sanitizer ────────────────

func TestSanitizerKeepsWPVideoEmbeds(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	g.sanitizer = newSanitizer(true)
	out := string(g.tmplSafeHTML(nil, nil)("[youtube]https://youtu.be/abc123[/youtube]\n\n<iframe src=\"https://evil.example\"></iframe>"))
	if !strings.Contains(out, "youtube.com/embed/abc123") {
		t.Errorf("shortcode iframe must survive sanitization, got %q", out)
	}
	if strings.Contains(out, "evil.example") {
		t.Errorf("author-supplied iframe must be stripped, got %q", out)
	}
}

func TestSanitizerKeepsCustomShortcodeOutput(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	g.sanitizer = newSanitizer(true)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "promo.html"), []byte(`<iframe src="https://trusted.example"></iframe>`), 0644); err != nil {
		t.Fatal(err)
	}
	g.config.TemplatesDir = dir
	g.config.Template = "."
	g.shortcodeMap = map[string]Shortcode{"promo": {Name: "promo", Template: "promo.html"}}
	out := string(g.tmplSafeHTML(nil, nil)("before {{promo}} after"))
	if !strings.Contains(out, "trusted.example") {
		t.Errorf("custom shortcode iframe must survive sanitization, got %q", out)
	}
}

// ─── SEC-015: OpenGraph attribute escaping ──────────────────────────────────

func TestBuildOpenGraphEscapesAttributes(t *testing.T) {
	g := newTestGen(t, "")
	og := g.buildOpenGraph(models.Page{
		Title:       `Nice " onpointerover="alert(1)`,
		Description: `a<b>"c`,
		Slug:        "x",
	}, true)
	if strings.Contains(og, `onpointerover="alert`) {
		t.Errorf("attribute injection through title: %s", og)
	}
	if strings.Contains(og, `content="Nice " `) {
		t.Errorf("quote must not terminate the attribute: %s", og)
	}
	if !strings.Contains(og, "&#34;") && !strings.Contains(og, "&quot;") {
		t.Errorf("quotes must be HTML-escaped: %s", og)
	}
}

// ─── GO-021: feed summary truncation is rune-safe ───────────────────────────

func TestFeedSummaryTruncationRuneSafe(t *testing.T) {
	g := newTestGen(t, "")
	g.md = buildMarkdown(g.config)
	var sb strings.Builder
	g.writeFeedEntry(&sb, models.Page{Title: "t", Slug: "s", Content: strings.Repeat("ż", 400)})
	out := sb.String()
	if !utf8.ValidString(out) {
		t.Fatalf("feed entry contains invalid UTF-8")
	}
	if got := strings.Count(out, "ż"); got != 300 {
		t.Errorf("summary must be truncated to 300 runes, got %d", got)
	}
}

// ─── GO-022: minification must not corrupt <pre>/<code> ─────────────────────

func TestMinifyHTMLPreservesPreBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i.html")
	pre := "<pre><code>line1\n  indented\n\nline3</code></pre>"
	html := "<html>\n  <body>\n    " + pre + "\n  <script>\nvar a = 1;\n</script>\n</body>\n</html>"
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		t.Fatal(err)
	}
	if err := minifyHTMLFile(path); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), pre) {
		t.Errorf("pre block must survive minification unchanged:\n%s", out)
	}
	if !strings.Contains(string(out), "var a = 1;") {
		t.Errorf("script content must survive minification:\n%s", out)
	}
	if strings.Contains(string(out), "<body>\n") {
		t.Errorf("outside pre, whitespace should still be minified:\n%s", out)
	}
}

// ─── GO-023: post with an empty output path must not overwrite the homepage ─

func TestGeneratePostEmptyOutputPathGuard(t *testing.T) {
	g := newTestGen(t, `{{define "post.html"}}post{{end}}`)
	index := filepath.Join(g.config.OutputDir, "index.html")
	if err := os.WriteFile(index, []byte("homepage"), 0644); err != nil {
		t.Fatal(err)
	}
	// link with no path → GetOutputPath() == ""
	post := models.Page{Title: "ext", Slug: "", Type: "post", Link: "https://example.com"}
	if err := g.generatePost(post); err != nil {
		t.Fatalf("generatePost: %v", err)
	}
	data, _ := os.ReadFile(index)
	if string(data) != "homepage" {
		t.Errorf("homepage overwritten by post with empty output path: %q", data)
	}
}

// ─── PERF-001: git lastmod comes from a single batched scan ─────────────────

func TestGitLastModBatchScan(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_AUTHOR_DATE=2026-01-02T03:04:05Z",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_COMMITTER_DATE=2026-01-02T03:04:05Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "commit.gpgsign", "false")
	sub := filepath.Join(repo, "content")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "c1")

	oldWD, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	g := newTestGen(t, "")
	g.config.LastmodFromGit = true
	got := g.lastModFor(models.Page{SourceDir: "content", SourceFile: "a.md"})
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("lastModFor = %v, want %v", got, want)
	}
	// Untracked file → fallback to page dates.
	got = g.lastModFor(models.Page{SourceDir: "content", SourceFile: "missing.md", Date: want.AddDate(0, 1, 0)})
	if !got.Equal(want.AddDate(0, 1, 0)) {
		t.Errorf("untracked file must fall back to page date, got %v", got)
	}
}

// ─── PERF-002: shortcode templates are parsed once and cached ───────────────

func TestRenderShortcodeCachesTemplate(t *testing.T) {
	g := newTestGen(t, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "sc.html")
	if err := os.WriteFile(path, []byte("hi {{.Name}}"), 0644); err != nil {
		t.Fatal(err)
	}
	g.config.TemplatesDir = dir
	g.config.Template = "."
	sc := Shortcode{Name: "sc", Template: "sc.html"}
	if out := g.renderShortcode(sc); out != "hi sc" {
		t.Fatalf("first render = %q", out)
	}
	// Deleting the file proves the second render comes from the cache.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if out := g.renderShortcode(sc); out != "hi sc" {
		t.Errorf("second render must hit the cache, got %q", out)
	}
}

// ─── PERF-009: link-checker target memoization ──────────────────────────────

func TestRefResolvesMemoized(t *testing.T) {
	g := newTestGen(t, "")
	g.refCache = make(map[string]bool)
	css := filepath.Join(g.config.OutputDir, "style.css")
	if err := os.WriteFile(css, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !g.refResolves("/style.css", g.config.OutputDir) {
		t.Fatalf("existing target must resolve")
	}
	// Removing the file proves the second lookup is served from the memo.
	if err := os.Remove(css); err != nil {
		t.Fatal(err)
	}
	if !g.refResolves("/style.css", g.config.OutputDir) {
		t.Errorf("second lookup must be memoized")
	}
}

// ─── PERF-006: single-pass WP media URL rewrite stays correct ────────────────

func TestFixMediaPathsSinglePass(t *testing.T) {
	var item models.MediaItem
	item.ID = 1048
	item.MediaDetails.File = "2020/03/IMG_0316.jpg"
	media := map[int]models.MediaItem{1048: item}
	in := `<img class="wp-image-1048" src="http://old.example/wp/IMG_0316-300x225.jpg"> <img src="https://cdn.example/other.png">`
	out := fixMediaPaths(in, media)
	if !strings.Contains(out, `src="/media/1048_IMG_0316.jpg"`) {
		t.Errorf("wp-image URL must be rewritten locally: %s", out)
	}
	if !strings.Contains(out, `https://cdn.example/other.png`) {
		t.Errorf("unrelated CDN URL must stay untouched: %s", out)
	}
}
