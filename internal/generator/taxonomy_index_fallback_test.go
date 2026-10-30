package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/taxonomy"
)

// buildTechnologySite builds a two-post site with a custom `technology`
// taxonomy on the named theme, paginated at paginate posts per page. When tmplFiles is nil the theme is the embedded
// one the generator extracts on demand; otherwise those files are the theme.
func buildTechnologySite(t *testing.T, theme string, tmplFiles map[string]string, paginate int) string {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	post := func(title, slug, tech string) string {
		return "---\ntitle: " + title + "\nslug: " + slug + "\nstatus: publish\ntype: post\ndate: 2024-01-02\n" +
			"technology: [" + tech + "]\n---\n\nBody of " + title + ".\n"
	}
	mustWrite(t, filepath.Join(content, "posts", "news", "one.md"), post("One", "one", "Go"))
	mustWrite(t, filepath.Join(content, "posts", "news", "two.md"), post("Two", "two", "Go, Rust"))
	for name, body := range tmplFiles {
		mustWrite(t, filepath.Join(tmp, "templates", theme, name), body)
	}
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: theme, Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: out, Quiet: true, Paginate: paginate,
		Taxonomies: map[string]taxonomy.DefinitionConfig{"technology": {}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

// TestTaxonomyIndexOnSimpleTheme is #377: a custom taxonomy on the embedded
// simple theme writes its index, listing every term, and its term archives
// with the term's own canonical rather than /category/<slug>/.
func TestTaxonomyIndexOnSimpleTheme(t *testing.T) {
	out := buildTechnologySite(t, "simple", nil, 1)
	index := readFileString(t, filepath.Join(out, "technology", indexHTMLName))
	for _, want := range []string{`href="/technology/go/"`, `href="/technology/rust/"`,
		`<link rel="canonical" href="https://example.com/technology/">`, "Technology", "2 posts", "1 post<"} {
		if !strings.Contains(index, want) {
			t.Errorf("index lacks %s:\n%s", want, index)
		}
	}
	term := readFileString(t, filepath.Join(out, "technology", "go", indexHTMLName))
	for _, want := range []string{`<link rel="canonical" href="https://example.com/technology/go/">`,
		`href="/technology/"`, `href="/technology/go/page/2/" rel="next"`} {
		if !strings.Contains(term, want) {
			t.Errorf("term archive lacks %s:\n%s", want, term)
		}
	}
	if strings.Contains(term, "/category/go/") {
		t.Errorf("term archive borrows the category canonical:\n%s", term)
	}
	page2 := readFileString(t, filepath.Join(out, "technology", "go", "page", "2", indexHTMLName))
	if !strings.Contains(page2, `href="/technology/go/" rel="prev"`) {
		t.Errorf("page 2 lacks the way back:\n%s", page2)
	}
	for _, post := range []string{`href="/one/"`, `href="/two/"`} {
		if strings.Count(term+page2, post) == 0 {
			t.Errorf("no page of the Go term lists %s", post)
		}
	}
}

// TestTaxonomyIndexFallsBackToCategory is the other half of #377: a theme with
// only category.html (no taxonomy.html, no archive.html) still renders the
// index, because the index view carries the archive-shaped fields a category
// template reads.
func TestTaxonomyIndexFallsBackToCategory(t *testing.T) {
	cat := `{{define "category.html"}}<h1>{{.Category.Name}}|{{.Category.Slug}}|{{.Kind}}|{{.Name}}|` +
		`{{len .Posts}}|{{.Pager.Current}}/{{.Pager.Total}}</h1>{{end}}`
	files := map[string]string{
		"index.html":    `{{define "index.html"}}home{{end}}`,
		"post.html":     `{{define "post.html"}}{{.Page.Title}}{{end}}`,
		"page.html":     `{{define "page.html"}}{{.Page.Title}}{{end}}`,
		"category.html": cat,
	}
	out := buildTechnologySite(t, "catonly", files, 0)
	index := readFileString(t, filepath.Join(out, "technology", indexHTMLName))
	if want := "<h1>Technology|technology|technology|Technology|0|1/1</h1>"; index != want {
		t.Errorf("index = %q, want %q", index, want)
	}
	if _, err := os.Stat(filepath.Join(out, "technology", "go", indexHTMLName)); err != nil {
		t.Errorf("term archive missing: %v", err)
	}
}
