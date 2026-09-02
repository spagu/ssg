package generator

// One small-site fixture for every e2e test that needs a real build. Each test
// used to lay out its own content/templates/output triple, and SonarCloud was
// right to call the copies what they were.

import (
	"path/filepath"
	"testing"
)

// fixtureTemplates is every template name ssg executes directly by file name.
var fixtureTemplates = []string{"base.html", "index.html", "post.html", "page.html",
	"category.html", "tag.html", "taxonomy.html"}

// newSiteFixture lays out one buildable site and returns its Config. files are
// content-relative ("pages/home.md"); template returns each template's body —
// pass nil for an inert `<html><body>x</body></html>` everywhere.
func newSiteFixture(t *testing.T, metadata string, files map[string]string, template func(name string) string) Config {
	t.Helper()
	if template == nil {
		template = func(string) string { return `<html><body><p>x</p></body></html>` }
	}
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content")
	tmplDir := filepath.Join(tmp, "templates")
	mustWrite(t, filepath.Join(contentDir, "site", "metadata.json"), metadata)
	for rel, body := range files {
		mustWrite(t, filepath.Join(contentDir, "site", filepath.FromSlash(rel)), body)
	}
	for _, name := range fixtureTemplates {
		mustWrite(t, filepath.Join(tmplDir, "simple", name),
			`{{define "`+name+`"}}`+template(name)+`{{end}}`)
	}
	return Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: contentDir, TemplatesDir: tmplDir,
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	}
}

// buildSiteFixture builds it, failing the test on any error.
func buildSiteFixture(t *testing.T, cfg Config) {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}
