package externalsource

import (
	"strings"
	"testing"
)

// sqlContentConfig is a SQL source over the fixture with a content_map, the
// shape issue #376 reported as producing one page instead of one per row.
func sqlContentConfig(t *testing.T, queries map[string]QueryConfig, transform TransformConfig) Config {
	t.Helper()
	return Config{Enabled: true, CacheDir: t.TempDir(), Sources: map[string]SourceConfig{
		"catalog": {Type: "sql", Driver: "sqlite", Database: newSQLiteFixture(t), Queries: queries,
			Transform: transform, Mode: "content",
			ContentMap: ContentMap{Title: "name", Slug: "slug", Content: "name", Type: "=product"}},
	}}
}

var publishedProducts = map[string]QueryConfig{
	"products": {SQL: "SELECT name, slug FROM products WHERE published = 1 ORDER BY id"},
}

// TestSQLContentMapOnePagePerRow: one query + content_map gives one page per
// row, while the data view keeps the per-query shape templates rely on.
func TestSQLContentMapOnePagePerRow(t *testing.T) {
	reg, warns, err := Load(sqlContentConfig(t, publishedProducts, TransformConfig{}))
	if err != nil || len(warns) != 0 {
		t.Fatalf("load: %v %v", err, warns)
	}
	imports := reg.CMSImports()
	if len(imports) != 1 || len(imports[0].Posts) != 2 {
		t.Fatalf("imports = %+v, want 2 posts (one per row)", imports)
	}
	if imports[0].Posts[0].Slug != "widget" || imports[0].Posts[1].Slug != "gadget" {
		t.Errorf("slugs = %q, %q", imports[0].Posts[0].Slug, imports[0].Posts[1].Slug)
	}
	if _, ok := reg.Data()["catalog"].(map[string]interface{})["products"]; !ok {
		t.Error("data view should stay keyed by query name")
	}
}

// TestSQLTransformSelect: transform.select runs for SQL sources too — it picks
// the query whose rows become pages (and the data view), as for file/http.
func TestSQLTransformSelect(t *testing.T) {
	queries := map[string]QueryConfig{
		"products": publishedProducts["products"],
		"count":    {SQL: "SELECT COUNT(*) AS n FROM products"},
	}
	reg, _, err := Load(sqlContentConfig(t, queries, TransformConfig{Select: "products"}))
	if err != nil {
		t.Fatal(err)
	}
	if posts := reg.CMSImports()[0].Posts; len(posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(posts))
	}
	if rows, ok := reg.Data()["catalog"].([]interface{}); !ok || len(rows) != 2 {
		t.Errorf("data = %#v, want the selected rows", reg.Data()["catalog"])
	}

	// A data-mode source honours select as well.
	cfg := sqlContentConfig(t, queries, TransformConfig{Select: "count"})
	sc := cfg.Sources["catalog"]
	sc.Mode, sc.ContentMap = "", ContentMap{}
	cfg.Sources["catalog"] = sc
	reg, _, err = Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rows, ok := reg.Data()["catalog"].([]interface{}); !ok || len(rows) != 1 {
		t.Errorf("data-mode select = %#v", reg.Data()["catalog"])
	}

	// A select naming no query fails the transform stage.
	_, _, err = Load(sqlContentConfig(t, queries, TransformConfig{Select: "missing"}))
	if err == nil || !strings.Contains(err.Error(), "transform") {
		t.Errorf("bad select err = %v", err)
	}
}

// TestSQLContentMapNeedsSelectWithSeveralQueries: with more than one query
// and no select, which rows are the pages is ambiguous — refused at config.
func TestSQLContentMapNeedsSelectWithSeveralQueries(t *testing.T) {
	queries := map[string]QueryConfig{
		"products": publishedProducts["products"],
		"count":    {SQL: "SELECT COUNT(*) AS n FROM products"},
	}
	_, _, err := Load(sqlContentConfig(t, queries, TransformConfig{}))
	if err == nil || !strings.Contains(err.Error(), "transform.select") ||
		!strings.Contains(err.Error(), "count, products") {
		t.Fatalf("err = %v, want a transform.select hint listing the queries", err)
	}
}

// TestUnsupportedFormatListsEveryFormat: the error lists the whole accepted
// set, derived from supportedFormats, including changelog and feed (#376).
func TestUnsupportedFormatListsEveryFormat(t *testing.T) {
	_, err := Resolve(Config{Sources: map[string]SourceConfig{
		"x": {Type: "file", Path: "a.bin", Format: "bin"},
	}})
	if err == nil {
		t.Fatal("want an error")
	}
	for f := range supportedFormats {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error %q misses format %q", err, f)
		}
	}
	if !strings.Contains(err.Error(), "changelog, csv, feed, json, toml, xml, yaml") {
		t.Errorf("error %q, want a sorted list", err)
	}
}

// TestSQLContentDataPassThrough: only an unselected single-query SQL map is
// unwrapped; every other shape reaches MapRecords as it is.
func TestSQLContentDataPassThrough(t *testing.T) {
	one := map[string]Query{"q": {}}
	rows := []interface{}{map[string]interface{}{"a": 1}}
	cases := map[string]struct {
		src  Source
		data interface{}
		want int
	}{
		"file source":   {Source{Type: "file", Queries: one}, "x", -1},
		"selected":      {Source{Type: "sql", Queries: one, Transform: TransformConfig{Select: "q"}}, rows, 1},
		"not a map":     {Source{Type: "sql", Queries: one}, rows, 1},
		"single query":  {Source{Type: "sql", Queries: one}, map[string]interface{}{"q": rows}, 1},
		"several query": {Source{Type: "sql", Queries: map[string]Query{"a": {}, "b": {}}}, "x", -1},
	}
	for label, tc := range cases {
		got := sqlContentData(tc.src, tc.data)
		if tc.want < 0 {
			if got != tc.data {
				t.Errorf("%s: got %#v", label, got)
			}
			continue
		}
		if l, ok := got.([]interface{}); !ok || len(l) != tc.want {
			t.Errorf("%s: got %#v", label, got)
		}
	}
}
