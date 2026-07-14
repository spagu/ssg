package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestColocatedAssetsOnlyWhereReferenced verifies PERF-007: with two posts in
// one category directory, a co-located image lands only in the output dir of
// the post that references it — not duplicated into every sibling post.
func TestColocatedAssetsOnlyWhereReferenced(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],"users":[]}`)
	mustWrite(t, filepath.Join(postsDir, "with-image.md"),
		"---\ntitle: With\nslug: with-image\nstatus: publish\ntype: post\ndate: 2026-01-02\n---\n\n![hero](hero.png)\n")
	mustWrite(t, filepath.Join(postsDir, "without-image.md"),
		"---\ntitle: Without\nslug: without-image\nstatus: publish\ntype: post\ndate: 2026-01-03\n---\n\nNo images here.\n")
	mustWrite(t, filepath.Join(postsDir, "hero.png"), "fake-png-bytes")

	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:    filepath.Join(tmp, "content"),
		TemplatesDir:  filepath.Join(tmp, "templates"),
		OutputDir:     filepath.Join(tmp, "output"),
		PostURLFormat: "slug",
		Quiet:         true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "output", "with-image", "hero.png")); err != nil {
		t.Errorf("hero.png missing from the referencing post: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "output", "without-image", "hero.png")); !os.IsNotExist(err) {
		t.Error("hero.png must NOT be duplicated into the non-referencing post (PERF-007)")
	}
}

// TestMarkdownConvertedOncePerContent verifies PERF-004: with feeds, search
// index, JSON output and TOC all enabled, each unique markdown body is run
// through goldmark exactly once per build (the memo serves every other consumer)
// and repeated builds of the same Generator stay cached.
func TestMarkdownConvertedOncePerContent(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],"users":[]}`)
	post := func(title, slug, body string) string {
		return "---\ntitle: " + title + "\nslug: " + slug +
			"\nstatus: publish\ntype: post\ndate: 2026-01-02\ncategories: [News]\n---\n\n" + body + "\n"
	}
	mustWrite(t, filepath.Join(postsDir, "a.md"), post("A", "a", "## Head A\n\nBody A"))
	mustWrite(t, filepath.Join(postsDir, "b.md"), post("B", "b", "## Head B\n\nBody B"))

	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Feed:         true, FeedFullContent: true,
		SearchIndex: true,
		Outputs:     []string{"json"},
		TOC:         true, TOCDepth: 3,
		PostURLFormat: "slug",
		Quiet:         true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Two unique bodies → exactly two real conversions; feeds/search/json served
	// from the memo. (Anything >2 means a consumer bypassed the cache.)
	if gen.mdConversions != 2 {
		t.Errorf("goldmark conversions = %d, want 2 (one per unique content)", gen.mdConversions)
	}

	// The memo also holds across a second Generate on the same instance.
	if err := gen.Generate(); err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if gen.mdConversions != 2 {
		t.Errorf("conversions after rebuild = %d, want still 2", gen.mdConversions)
	}
}

// TestRenderTimeTransformsSingleWrite verifies PERF-005: SEO block, KaTeX
// assets, relative links and HTML minification are all present in the final
// output even though no post-render tree-walk runs — they are applied in
// memory at render time in a single write.
func TestRenderTimeTransformsSingleWrite(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],"users":[]}`)
	mustWrite(t, filepath.Join(postsDir, "m.md"),
		"---\ntitle: Math Post\nslug: math-post\nstatus: publish\ntype: post\ndate: 2026-01-02\n---\n\n"+
			"Formula: $$x^2$$ and a [link](https://example.com/other/).\n")

	tmplDir := filepath.Join(tmp, "templates", "simple")
	static := `<html><head></head>

<body>   <a href="https://example.com/about/">about</a>
`
	for _, name := range []string{"base.html", "index.html", "category.html"} {
		mustWrite(t, filepath.Join(tmplDir, name), `{{define "`+name+`"}}`+static+`</body></html>{{end}}`)
	}
	for _, name := range []string{"post.html", "page.html"} {
		mustWrite(t, filepath.Join(tmplDir, name),
			`{{define "`+name+`"}}`+static+"{{.Content}}\n</body></html>{{end}}")
	}

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:    filepath.Join(tmp, "content"),
		TemplatesDir:  filepath.Join(tmp, "templates"),
		OutputDir:     filepath.Join(tmp, "output"),
		PostURLFormat: "slug",
		SEO:           true,
		Math:          true,
		RelativeLinks: true,
		MinifyHTML:    true,
		Quiet:         true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(tmp, "output", "math-post", "index.html"))
	if err != nil {
		t.Fatalf("post output: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "og:title") {
		t.Error("SEO block missing (render-time injectSEO)")
	}
	if !strings.Contains(s, "katex.min.css") {
		t.Error("KaTeX assets missing (render-time math injection)")
	}
	if strings.Contains(s, `href="https://example.com/about/"`) || !strings.Contains(s, `href="/about/"`) {
		t.Error("relative-links transform not applied at render time")
	}
	if strings.Contains(s, ">\n\n<") || strings.Contains(s, "body>   <a") {
		t.Error("HTML not minified at render time")
	}
}
