package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestMarkdownPublishIntegration builds a real site with markdown_publish and
// clean_special_chars on, then checks the on-disk outputs: both .md locations,
// cleaned content, the llms.txt index and the <head> alternate.
func TestMarkdownPublishIntegration(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(postsDir, "hello.md"),
		"---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\n"+
			"He said “hi” — really… nice.\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Domain = "example.com"
	cfg.PostURLFormat = "slug"
	cfg.MarkdownPublish = true
	cfg.CleanSpecialChars = true
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")

	// index.md next to the post, title prepended, smart punctuation cleaned.
	md := mustRead(t, filepath.Join(out, "hello", "index.md"))
	if !strings.HasPrefix(md, "# Hello\n") {
		t.Fatalf("title heading missing:\n%s", md)
	}
	if strings.ContainsAny(md, "“”—…") {
		t.Fatalf("smart punctuation not cleaned:\n%s", md)
	}
	if !strings.Contains(md, `"hi"`) || !strings.Contains(md, "-- really... nice.") {
		t.Fatalf("cleaned content wrong:\n%s", md)
	}
	// Flat sibling /hello.md holds the same bytes.
	if flat := mustRead(t, filepath.Join(out, "hello.md")); flat != md {
		t.Fatalf("flat sibling differs from index.md")
	}
	// llms.txt indexes the post's Markdown URL.
	llms := mustRead(t, filepath.Join(out, "llms.txt"))
	if !strings.Contains(llms, "https://example.com/hello/index.md") {
		t.Fatalf("llms.txt missing the post:\n%s", llms)
	}
	// The rendered HTML advertises the Markdown alternate.
	html := mustRead(t, filepath.Join(out, "hello", "index.html"))
	if !strings.Contains(html, `type="text/markdown"`) {
		t.Fatalf("HTML missing the markdown alternate:\n%s", html[:min(len(html), 400)])
	}
}

// TestOutputEncodingIntegration builds with utf-16le and checks the written
// HTML and Markdown carry a little-endian BOM.
func TestOutputEncodingIntegration(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(postsDir, "unicode.md"),
		"---\ntitle: 世界\nslug: unicode\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\n你好、世界。\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.PostURLFormat = "slug"
	cfg.MarkdownPublish = true
	cfg.OutputEncoding = "utf-16le"
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	for _, f := range []string{
		filepath.Join(out, "unicode", "index.html"),
		filepath.Join(out, "unicode", "index.md"),
	} {
		b := []byte(mustRead(t, f))
		if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
			t.Fatalf("%s: missing utf-16le BOM: % x", f, b[:min(len(b), 4)])
		}
	}
}

func TestPageMarkdown_PrependsTitle(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true}}
	p := models.Page{Title: "Configuration", Content: "Body text here."}
	md := g.pageMarkdown(p)
	if !strings.HasPrefix(md, "# Configuration\n\nBody text here.") {
		t.Fatalf("unexpected markdown:\n%s", md)
	}
}

func TestPageMarkdown_KeepsExistingH1(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true}}
	p := models.Page{Title: "Config", Content: "# Real Heading\n\nBody."}
	md := g.pageMarkdown(p)
	if strings.Count(md, "# ") != 1 || !strings.HasPrefix(md, "# Real Heading") {
		t.Fatalf("should not double the H1:\n%s", md)
	}
}

func TestPageMarkdown_CleanChars(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true, CleanSpecialChars: true}}
	p := models.Page{Title: "T", Content: "“smart” — dash"}
	md := g.pageMarkdown(p)
	if strings.ContainsAny(md, "“”—") {
		t.Fatalf("special chars not cleaned:\n%s", md)
	}
}

func TestStartsWithH1(t *testing.T) {
	if !startsWithH1("\n\n# Title\ntext") {
		t.Fatal("should detect a leading H1 after blank lines")
	}
	if startsWithH1("## Sub") || startsWithH1("text") || startsWithH1("#notheading") {
		t.Fatal("false positive H1")
	}
}

func TestInjectMarkdownAlternate(t *testing.T) {
	out := injectMarkdownAlternate(`<head><title>x</title></head><body></body>`, "index.md")
	if !strings.Contains(out, `<link rel="alternate" type="text/markdown" href="index.md">`) {
		t.Fatalf("alternate not injected: %s", out)
	}
	// Idempotent.
	if injectMarkdownAlternate(out, "index.md") != out {
		t.Fatal("second injection should be a no-op")
	}
	// A flat page links to its own <slug>.md.
	flat := injectMarkdownAlternate(`<head></head>`, "recipe.md")
	if !strings.Contains(flat, `href="recipe.md"`) {
		t.Fatalf("flat href wrong: %s", flat)
	}
	// Empty href suppresses the link entirely (#116).
	if got := injectMarkdownAlternate(`<head></head>`, ""); strings.Contains(got, "text/markdown") {
		t.Fatalf("empty href must inject nothing: %s", got)
	}
}

func TestMarkdownLeaf(t *testing.T) {
	cases := map[string]string{
		"/configuration/":   "index.md",
		"/":                 "index.md",
		"/recipe-name.html": "recipe-name.md",
		"/a/b/post.html":    "post.md",
		"":                  "",
		"/weird":            "",
	}
	for u, want := range cases {
		if got := markdownLeaf(u); got != want {
			t.Errorf("markdownLeaf(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestEffectiveHomeLimit(t *testing.T) {
	cases := []struct{ cfg, total, want int }{
		{0, 20, 6},   // default 6
		{0, 3, 3},    // fewer than 6 → all
		{-1, 20, 20}, // negative → no limit
		{4, 20, 4},   // explicit
		{50, 20, 20}, // capped at total
	}
	for _, c := range cases {
		if got := effectiveHomeLimit(c.cfg, c.total); got != c.want {
			t.Errorf("effectiveHomeLimit(%d,%d) = %d, want %d", c.cfg, c.total, got, c.want)
		}
	}
}

func TestGenerateLLMsTxt_Sections(t *testing.T) {
	tmp := t.TempDir()
	g := &Generator{config: Config{MarkdownPublish: true, Domain: "example.com", OutputDir: tmp}}
	g.siteData = &models.SiteData{
		Pages: []models.Page{{Title: "Guide", Link: "/guide/", Description: "A guide."}},
		Posts: []models.Page{{Title: "Update", Link: "/blog/update/"}},
	}
	if err := g.generateLLMsTxt(); err != nil {
		t.Fatalf("generateLLMsTxt: %v", err)
	}
	txt := mustRead(t, filepath.Join(tmp, "llms.txt"))
	for _, want := range []string{
		"# example.com",
		"## Documentation",
		"- [Guide](https://example.com/guide/index.md): A guide.",
		"## Posts",
		"- [Update](https://example.com/blog/update/index.md)",
	} {
		if !strings.Contains(txt, want) {
			t.Fatalf("llms.txt missing %q:\n%s", want, txt)
		}
	}
}

func TestGenerateLLMsTxt_DisabledIsNoop(t *testing.T) {
	tmp := t.TempDir()
	g := &Generator{config: Config{MarkdownPublish: false, OutputDir: tmp}}
	g.siteData = &models.SiteData{}
	if err := g.generateLLMsTxt(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "llms.txt")); err == nil {
		t.Fatal("llms.txt should not be written when markdown_publish is off")
	}
}

func TestMarkdownURLFor(t *testing.T) {
	p := models.Page{Link: "/configuration/"}
	if got := markdownURLFor(p, "example.com"); got != "https://example.com/configuration/index.md" {
		t.Fatalf("md url = %q", got)
	}
	// A flat WordPress-style URL maps to /slug.md (#116).
	flat := models.Page{Link: "/recipe-name.html"}
	if got := markdownURLFor(flat, "example.com"); got != "https://example.com/recipe-name.md" {
		t.Fatalf("flat page md url = %q, want …/recipe-name.md", got)
	}
}

// TestMarkdownPublish_FlatURLPost is the #116 regression: a post with a flat
// link publishes /slug.md, links it from <head>, and appears in llms.txt — and
// crucially the alternate points at a file that exists (no self-broken link).
func TestMarkdownPublish_FlatURLPost(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(postsDir, "cake.md"),
		"---\ntitle: Lemon Cake\nslug: cake\nlink: \"/lemon-cake.html\"\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\nA moist lemon cake.\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Domain = "example.com"
	cfg.MarkdownPublish = true
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	// The flat Markdown twin exists.
	md := mustRead(t, filepath.Join(out, "lemon-cake.md"))
	if !strings.Contains(md, "# Lemon Cake") {
		t.Fatalf("flat .md missing/thin:\n%s", md)
	}
	// The HTML advertises the flat .md, not a bogus index.md.
	html := mustRead(t, filepath.Join(out, "lemon-cake.html"))
	if !strings.Contains(html, `href="lemon-cake.md"`) {
		t.Fatalf("alternate should point at lemon-cake.md:\n%s", html)
	}
	if strings.Contains(html, `href="index.md"`) {
		t.Fatal("flat page must not ship the broken index.md alternate (#116)")
	}
	// llms.txt lists the flat post.
	llms := mustRead(t, filepath.Join(out, "llms.txt"))
	if !strings.Contains(llms, "https://example.com/lemon-cake.md") {
		t.Fatalf("llms.txt missing the flat post:\n%s", llms)
	}
}
