package generator

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/externalsource"
)

// externalTestConfig builds a Generate-ready config with three file sources.
func externalTestConfig(tmp string) Config {
	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Quiet:        true,
		ExternalSources: externalsource.Config{
			Enabled: true,
			Sources: map[string]externalsource.SourceConfig{
				"products": {Type: "file", Path: filepath.Join(tmp, "ext", "products.json"),
					Transform: externalsource.TransformConfig{Select: "data.items"}},
				"rates":  {Type: "file", Path: filepath.Join(tmp, "ext", "rates.csv")},
				"legacy": {Type: "file", Path: filepath.Join(tmp, "ext", "feed.xml")},
			},
		},
	}
	return cfg
}

func writeExternalFixtures(t *testing.T, tmp string) {
	t.Helper()
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "one", "One", "")
	mustWrite(t, filepath.Join(tmp, "ext", "products.json"),
		`{"data":{"items":[{"name":"Widget","price":"9.99"},{"name":"Gadget","price":"19.99"}]}}`)
	mustWrite(t, filepath.Join(tmp, "ext", "rates.csv"), "code,rate\nPLN,4.30\n")
	mustWrite(t, filepath.Join(tmp, "ext", "feed.xml"), `<feed><title>Legacy</title></feed>`)
}

// TestExternalSourcesFullBuild drives Generate() with external sources and a
// template consuming .ExternalData, .ExternalDataMeta and the helpers.
func TestExternalSourcesFullBuild(t *testing.T) {
	tmp := t.TempDir()
	writeExternalFixtures(t, tmp)
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "index.html"),
		`{{define "index.html"}}<html><body>`+
			`{{range .ExternalData.products}}<p>{{.name}}: {{.price}}</p>{{end}}`+
			`|meta:{{(index .ExternalDataMeta "products").RecordCount}}/{{(index .ExternalDataMeta "products").ContentType}}`+
			`|helper:{{range getExternal "rates"}}{{.code}}={{.rate}}{{end}}`+
			`|xml:{{(getExternal "legacy").feed.title}}`+
			`|hmeta:{{(getExternalMeta "rates").SourceType}}`+
			`</body></html>{{end}}`)

	gen, err := New(externalTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	idx := mustRead(t, filepath.Join(tmp, "output", "index.html"))
	wantContains(t, "index", idx,
		"<p>Widget: 9.99</p>", "<p>Gadget: 19.99</p>",
		"|meta:2/json", "|helper:PLN=4.30", "|xml:Legacy", "|hmeta:file")
}

// TestExternalSourcesRequiredFailure: a required source aborts the build with
// the unified error model.
func TestExternalSourcesRequiredFailure(t *testing.T) {
	tmp := t.TempDir()
	writeExternalFixtures(t, tmp)
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := externalTestConfig(tmp)
	cfg.ExternalSources.Sources["gone"] = externalsource.SourceConfig{
		Type: "file", Path: filepath.Join(tmp, "ext", "missing.yaml")}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), `external source "gone" (file) failed at read`) {
		t.Fatalf("err = %v", err)
	}
}

// newWordPressFixture creates a minimal WordPress sqlite database.
func newWordPressFixture(t *testing.T, tmp string) string {
	t.Helper()
	path := filepath.Join(tmp, "wp.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range []string{
		`CREATE TABLE wp_users (ID INTEGER, display_name TEXT, user_nicename TEXT)`,
		`INSERT INTO wp_users VALUES (7, 'Imported Author', 'imported-author')`,
		`CREATE TABLE wp_term_taxonomy (term_taxonomy_id INTEGER, term_id INTEGER, taxonomy TEXT)`,
		`CREATE TABLE wp_terms (term_id INTEGER, name TEXT, slug TEXT)`,
		`CREATE TABLE wp_term_relationships (object_id INTEGER, term_taxonomy_id INTEGER)`,
		`CREATE TABLE wp_postmeta (post_id INTEGER, meta_key TEXT, meta_value TEXT)`,
		`CREATE TABLE wp_posts (ID INTEGER, post_title TEXT, post_name TEXT, post_content TEXT,
		 post_excerpt TEXT, post_date TEXT, post_modified TEXT, post_status TEXT, post_type TEXT,
		 post_author INTEGER, guid TEXT, post_mime_type TEXT)`,
		`INSERT INTO wp_posts VALUES (101, 'Imported from WordPress', 'imported-from-wordpress',
		 '<p>Legacy body.</p>', 'Legacy excerpt', '2026-04-01 09:00:00', '2026-04-01 09:00:00',
		 'publish', 'post', 7, '', '')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestExternalSourcesCMSContentMerge: a WordPress source in content mode joins
// the site — the imported post renders, lands on the index and in the sitemap,
// and the import stays queryable under .ExternalData.
func TestExternalSourcesCMSContentMerge(t *testing.T) {
	tmp := t.TempDir()
	writeExternalFixtures(t, tmp)
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := externalTestConfig(tmp)
	cfg.ExternalSources.Sources["legacy_blog"] = externalsource.SourceConfig{
		Type: "cms", Adapter: "wordpress", Driver: "sqlite",
		Database: newWordPressFixture(t, tmp)}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	wantContains(t, "imported post", mustRead(t,
		filepath.Join(out, "2026", "04", "01", "imported-from-wordpress", "index.html")), "x")
	wantContains(t, "sitemap", mustRead(t, filepath.Join(out, "sitemap.xml")),
		"/2026/04/01/imported-from-wordpress/")
	if gen.siteData.Authors[7].Name != "Imported Author" {
		t.Fatalf("authors = %+v", gen.siteData.Authors)
	}
	if gen.externalData["legacy_blog"] == nil {
		t.Fatal("cms source must still expose .ExternalData")
	}
}

// TestExternalSourcesOptionalWarns: optional failures skip the source and keep
// the build green; .Data stays untouched by the new namespace.
func TestExternalSourcesOptionalWarns(t *testing.T) {
	tmp := t.TempDir()
	writeExternalFixtures(t, tmp)
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "index.html"),
		`{{define "index.html"}}<html><body>absent:{{if not (getExternal "gone")}}yes{{end}}`+
			`|data-intact:{{if not .Data}}yes{{end}}</body></html>{{end}}`)
	cfg := externalTestConfig(tmp)
	off := false
	cfg.ExternalSources.Sources["gone"] = externalsource.SourceConfig{
		Type: "file", Path: filepath.Join(tmp, "ext", "missing.yaml"), Required: &off}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("optional failure must not abort: %v", err)
	}
	wantContains(t, "index", mustRead(t, filepath.Join(tmp, "output", "index.html")),
		"absent:yes", "data-intact:yes")
}
