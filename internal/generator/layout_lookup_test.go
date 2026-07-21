package generator

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GO-058: `layout: blog` resolved to the template name "layouts/blog.html",
// but ParseGlob registers a template under its BASE filename ("blog.html").
// The lookup therefore never matched, and the page silently fell back to
// page.html — the documented layout feature could not work at all.

func TestLayoutTemplateName(t *testing.T) {
	tests := []struct {
		name    string
		defines []string // template names present in the parsed set
		layout  string
		want    string
	}{
		{
			"base name, as ParseGlob registers it",
			[]string{"page.html", "blog.html"},
			"blog",
			"blog.html",
		},
		{
			"path form still wins when a theme defines it",
			[]string{"page.html", "layouts/blog.html", "blog.html"},
			"blog",
			"layouts/blog.html",
		},
		{
			"only the path form present",
			[]string{"page.html", "layouts/landing.html"},
			"landing",
			"layouts/landing.html",
		},
		{
			"unknown layout keeps the path form, so the caller falls back",
			[]string{"page.html"},
			"nope",
			"layouts/nope.html",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := template.New("")
			for _, name := range tc.defines {
				if _, err := tmpl.New(name).Parse("x"); err != nil {
					t.Fatalf("parsing %q: %v", name, err)
				}
			}
			g := &Generator{tmpl: tmpl}
			if got := g.layoutTemplateName(tc.layout); got != tc.want {
				t.Errorf("layoutTemplateName(%q) = %q, want %q", tc.layout, got, tc.want)
			}
		})
	}

	// A generator with no parsed templates must not panic.
	g := &Generator{}
	if got := g.layoutTemplateName("blog"); got != "layouts/blog.html" {
		t.Errorf("layoutTemplateName with no templates = %q", got)
	}
}

// TestLayoutRendersFromLayoutsDir is the end-to-end version: a theme with
// layouts/blog.html and a page declaring `layout: blog` must render through
// that layout rather than page.html.
func TestLayoutRendersFromLayoutsDir(t *testing.T) {
	tmp := t.TempDir()
	themeDir := filepath.Join(tmp, "templates", "test")
	if err := os.MkdirAll(filepath.Join(themeDir, "layouts"), 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(themeDir, "page.html"), `{{define "page.html"}}<html><body>PAGE {{ .Page.Title }}</body></html>{{end}}`)
	write(filepath.Join(themeDir, "index.html"), `{{define "index.html"}}<html><body>INDEX</body></html>{{end}}`)
	write(filepath.Join(themeDir, "post.html"), `{{define "post.html"}}<html><body>POST</body></html>{{end}}`)
	write(filepath.Join(themeDir, "category.html"), `{{define "category.html"}}<html><body>CATEGORY</body></html>{{end}}`)
	write(filepath.Join(themeDir, "layouts", "blog.html"), `{{define "blog.html"}}<html><body>LAYOUT {{ .Page.Title }}</body></html>{{end}}`)

	pagesDir := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pagesDir, 0o750); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(pagesDir, "blog.md"),
		"---\ntitle: Blog\nslug: blog\nstatus: publish\ntype: page\nlayout: blog\n---\n\nIndex body.\n")

	out := filepath.Join(tmp, "out")
	g, err := New(Config{
		Domain:         "example.com",
		TemplatesDir:   filepath.Join(tmp, "templates"),
		Template:       "test",
		OutputDir:      out,
		ContentSources: []ContentSource{{Path: pagesDir, Type: "page"}},
		Quiet:          true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	rendered, err := os.ReadFile(filepath.Join(out, "blog", "index.html"))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(rendered), "LAYOUT Blog") {
		t.Errorf("page rendered as %q, want the layouts/blog.html output", string(rendered))
	}
}
