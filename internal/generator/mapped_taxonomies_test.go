package generator

// Taxonomies of pages made from records (#344). A content_map record keeps
// every field in Extra for its template (1.8.73), but its terms come from
// content_map.taxonomies alone, as before 1.8.73.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// mappedTaxonomySite builds a site whose ports come from a JSON source, with
// the maintainer taxonomy mapped to a normalised field.
func mappedTaxonomySite(t *testing.T, records string) Config {
	t.Helper()
	tmp := t.TempDir()
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "one", "One", "")
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeTaxonomyTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "post.html"),
		`{{define "post.html"}}<html><body><h1>{{.Title}}</h1>raw:{{with .Post}}{{.Extra.maintainer}}{{end}}`+
			`|terms:{{range pageTerms "maintainer" .Post}}{{.Name}};{{end}}</body></html>{{end}}`)
	mustWrite(t, filepath.Join(tmp, "ext", "ports.json"), records)
	return Config{
		Source: "site", Template: "simple", Domain: "example.com", Quiet: true,
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Taxonomies: map[string]taxonomy.DefinitionConfig{
			"maintainer": {Label: "Maintainers", Singular: "Maintainer"},
			"technology": {},
		},
		ExternalSources: externalsource.Config{
			Enabled: true,
			Sources: map[string]externalsource.SourceConfig{
				"ports": {Type: "file", Path: filepath.Join(tmp, "ext", "ports.json"), Mode: "content",
					ContentMap: externalsource.ContentMap{
						Title: "origin", Slug: "port-{{.slug}}", Link: "/{{.origin}}/", Type: "=port",
						Taxonomies: map[string]string{"maintainer": "maintainerTerm"},
					}},
			},
		},
	}
}

// TestMappedTaxonomyReplacesTheDefaultField reproduces #344: the raw
// "maintainer" differs from the normalised "maintainerTerm" only in a way
// slugify erases, and adding both failed the build on the slug collision.
func TestMappedTaxonomyReplacesTheDefaultField(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[
		{"origin":"x/a","slug":"x-a","maintainer":"ports@MidnightBSD..org","maintainerTerm":"ports@MidnightBSD.org","technology":"Go"},
		{"origin":"x/b","slug":"x-b","maintainer":"someone@example.com","maintainerTerm":""}
	]`)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v (the raw field must not add a second term)", err)
	}
	out := cfg.OutputDir
	a := mustRead(t, filepath.Join(out, "x", "a", "index.html"))
	if !strings.Contains(a, "raw:ports@MidnightBSD..org") {
		t.Errorf("the template still sees the raw record field:\n%s", a)
	}
	if !strings.Contains(a, "|terms:ports@MidnightBSD.org;<") {
		t.Errorf("the mapped field is the only source of terms:\n%s", a)
	}
	b := mustRead(t, filepath.Join(out, "x", "b", "index.html"))
	if !strings.Contains(b, "|terms:<") {
		t.Errorf("an empty mapped field leaves the record without a term, on purpose:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(out, "maintainer", "someone-example-com")); err == nil {
		t.Error("the raw field of a record with an empty mapping made a term page")
	}
	// A taxonomy the mapping does not name gets nothing from a record field
	// of the same name — as before records carried their fields (1.8.73).
	if _, err := os.Stat(filepath.Join(out, "technology", "go")); err == nil {
		t.Error("an unmapped taxonomy picked up the record's raw field")
	}
}

// TestFrontmatterStillMergesTheDirectField: a page written by hand keeps the
// documented merge of the taxonomies map and the direct field.
func TestFrontmatterStillMergesTheDirectField(t *testing.T) {
	hand := &models.Page{Extra: map[string]interface{}{"maintainer": "a@example.com"}}
	if got := taxonomyExtra(hand); got["maintainer"] != "a@example.com" {
		t.Errorf("custom frontmatter must still feed taxonomies: %v", got)
	}
	rec := &models.Page{FromRecord: true, Extra: map[string]interface{}{"maintainer": "a@example.com"}}
	if got := taxonomyExtra(rec); got != nil {
		t.Errorf("a record's fields must not feed taxonomies: %v", got)
	}
}
