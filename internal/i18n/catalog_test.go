package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogNestedAndInterpolate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pl.yaml"), []byte("post:\n  read_more: 'Czytaj {{count}}'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"post":{"read_more":"Read more"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCatalog(dir, []LanguageConfig{{Code: "pl"}, {Code: "en"}})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := c.Lookup("pl", "post.read_more")
	if !ok {
		t.Fatal("nested lookup failed")
	}
	if got := Interpolate(v.(string), map[string]any{"count": 3}); got != "Czytaj 3" {
		t.Fatalf("interpolation = %q", got)
	}
}

func TestCatalogJSONMissingAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "de.json"), []byte(`{"nav":{"home":"Startseite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	langs := []LanguageConfig{{Code: "de"}, {Code: "fr"}} // fr has no file
	c, err := LoadCatalog(dir, langs)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if v, ok := c.Lookup("de", "nav.home"); !ok || v != "Startseite" {
		t.Errorf("json lookup = %v, %v", v, ok)
	}
	if _, ok := c.Lookup("fr", "nav.home"); ok {
		t.Error("missing catalog must resolve to empty map, not entries")
	}
	// Parse errors: yaml and json.
	if err := os.WriteFile(filepath.Join(dir, "pl.yaml"), []byte(":\tbroken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(dir, []LanguageConfig{{Code: "pl"}}); err == nil {
		t.Error("invalid yaml must error")
	}
	if err := os.WriteFile(filepath.Join(dir, "it.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(dir, []LanguageConfig{{Code: "it"}}); err == nil {
		t.Error("invalid json must error")
	}
	// Read error other than not-exist (permission), skipped as root.
	if os.Getuid() != 0 {
		if err := os.WriteFile(filepath.Join(dir, "es.yaml"), []byte("a: b"), 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCatalog(dir, []LanguageConfig{{Code: "es"}}); err == nil {
			t.Error("unreadable catalog must error")
		}
	}
}

func TestLookupAndInterpolateEdges(t *testing.T) {
	var nilCat *Catalog
	if _, ok := nilCat.Lookup("pl", "x"); ok {
		t.Error("nil catalog must miss")
	}
	c := &Catalog{Messages: map[string]map[string]any{"pl": {"a": map[string]any{"b": "v"}, "s": "leaf"}}}
	if _, ok := c.Lookup("pl", "s.deeper"); ok {
		t.Error("descending through a leaf must miss")
	}
	if _, ok := c.Lookup("pl", "a.missing"); ok {
		t.Error("missing part must miss")
	}
	if got := Interpolate("Hi {{name}}, {{unknown}}!", map[string]any{"name": "Jan"}); got != "Hi Jan, {{unknown}}!" {
		t.Errorf("interpolate = %q", got)
	}
	if got := Interpolate("{{ count }} items", map[string]any{"count": 3}); got != "3 items" {
		t.Errorf("spaced placeholder = %q", got)
	}
}
