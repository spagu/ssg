package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustMkdir creates dir (and parents) or fails the test.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// mustWrite writes content to path (creating parents) or fails the test.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildSite runs the standard fixture build (content/ + templates/simple/ →
// output/) rooted at tmp and returns the generator and its output directory,
// failing the test on any construction or generation error. Shared by the
// archive/heading/tag tests to keep the boilerplate in one place.
func buildSite(t *testing.T, tmp string) (*Generator, string) {
	t.Helper()
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return gen, filepath.Join(tmp, "output")
}

// writeSimpleTemplates writes the five core theme templates with a static body.
func writeSimpleTemplates(t *testing.T, tmplDir string) {
	t.Helper()
	for _, name := range []string{"base.html", "index.html", "post.html", "page.html", "category.html"} {
		body := `{{define "` + name + `"}}<html><body><a href="/one/">x</a>` +
			`<link rel="stylesheet" href="/css/style.css"></body></html>{{end}}`
		mustWrite(t, filepath.Join(tmplDir, name), body)
	}
}

// TestGenerateFullFeatureSet drives Generate() with most optional post-processing
// features enabled at once (feed, search index, minify, fingerprint, bundle, json
// output, cloudflare, math, md-link rewrite, link check, hooks, clean) so the many
// *IfRequested branches, renderArchive, writeFeedEntry and refResolves are exercised.
func TestGenerateFullFeatureSet(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	postsDir := filepath.Join(contentDir, "posts", "news")

	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],"users":[{"id":1,"name":"Ed","slug":"ed"}]}`)
	post := func(name, slug string) string {
		return "---\ntitle: " + name + "\nslug: " + slug +
			"\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\nauthor: 1\n---\n\n" +
			"## Head\n\nBody with a [link](other.md) and `code`.\n\n```go\nfunc x(){}\n```\n"
	}
	mustWrite(t, filepath.Join(postsDir, "one.md"), post("One", "one"))
	mustWrite(t, filepath.Join(postsDir, "other.md"), post("Other", "other"))

	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "css", "style.css"), "body {  color:  red;  }")
	mustWrite(t, filepath.Join(tmplDir, "js", "app.js"), "function f() { return  1; }")

	cfg := Config{
		Source:          "site",
		Template:        "simple",
		Domain:          "example.com",
		ContentDir:      filepath.Join(tmp, "content"),
		TemplatesDir:    filepath.Join(tmp, "templates"),
		OutputDir:       filepath.Join(tmp, "output"),
		Clean:           true,
		Feed:            true,
		FeedFullContent: true,
		SearchIndex:     true,
		MinifyHTML:      true,
		MinifyCSS:       true,
		MinifyJS:        true,
		Fingerprint:     true,
		Highlight:       true,
		TOC:             true,
		TOCDepth:        3,
		Math:            true,
		RewriteMdLinks:  true,
		CheckLinks:      "warn",
		Outputs:         []string{"json"},
		Bundles:         map[string][]string{"css/bundle.css": {"css/style.css"}},
		Hooks:           map[string][]string{"pre_build": {"true"}, "post_build": {"true"}},
		Quiet:           true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "output", "feed.xml")); err != nil {
		t.Errorf("feed.xml missing: %v", err)
	}
	if !treeHasFile(t, filepath.Join(tmp, "output"), "index.json") {
		t.Error("no per-page index.json produced")
	}
	if !globHasMatch(t, filepath.Join(tmp, "output", "css", "bundle*.css")) {
		t.Error("no css/bundle*.css produced")
	}
}

// TestGenerateFeedSummaryPath covers writeFeedEntry's summary branch (FeedFullContent
// off) with an excerpt-less post so the HTML-stripped/truncated path runs.
func TestGenerateFeedSummaryPath(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")

	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[],"exported_at":"","media":[],"users":[]}`)
	body := "---\ntitle: P\nslug: p\nstatus: publish\ntype: post\ndate: 2024-03-04\n---\n\n" + strings.Repeat("word ", 100)
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "p.md"), body)

	tmplDir := filepath.Join(tmp, "templates", "simple")
	for _, name := range []string{"base.html", "index.html", "post.html", "page.html", "category.html"} {
		mustWrite(t, filepath.Join(tmplDir, name), `{{define "`+name+`"}}ok{{end}}`)
	}

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Feed:         true, Quiet: true,
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "output", "feed.xml"))
	if err != nil {
		t.Fatalf("feed.xml: %v", err)
	}
	if !strings.Contains(string(data), "<summary>") {
		t.Errorf("expected <summary> in summary-mode feed; got:\n%s", data)
	}
}

// globHasMatch reports whether pattern matches at least one existing path.
func globHasMatch(t *testing.T, pattern string) bool {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	return len(m) > 0
}

// treeHasFile reports whether any file named base exists under root.
func treeHasFile(t *testing.T, root, base string) bool {
	t.Helper()
	found := false
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && info.Name() == base {
			found = true
		}
		return nil
	})
	return found
}
