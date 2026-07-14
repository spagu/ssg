package taxonomy

import (
	"strings"
	"testing"
)

func resolvedDefs(t *testing.T, user map[string]DefinitionConfig) (map[string]Definition, []string) {
	t.Helper()
	defs, names, err := Resolve(user, nil)
	if err != nil {
		t.Fatal(err)
	}
	return defs, names
}

func TestExtractAssignmentsPriorityAndMerge(t *testing.T) {
	defs, names := resolvedDefs(t, map[string]DefinitionConfig{"technology": {}})
	src := PageSources{
		TaxonomiesFM: map[string]interface{}{"technology": []interface{}{"Go", "Rust"}},
		Extra:        map[string]interface{}{"technology": []interface{}{"rust", "Zig"}},
		Tags:         []string{"cli"},
		Series:       "Basics",
	}
	out, err := ExtractAssignments(defs, names, src, "post.md")
	if err != nil {
		t.Fatal(err)
	}
	// Map + direct field merge, case-insensitive dedupe keeps first display form.
	if got := strings.Join(out["technology"], ","); got != "Go,Rust,Zig" {
		t.Fatalf("technology = %v", out["technology"])
	}
	if strings.Join(out["tag"], ",") != "cli" || strings.Join(out["series"], ",") != "Basics" {
		t.Fatalf("legacy = %v / %v", out["tag"], out["series"])
	}
	if _, ok := out["category"]; ok {
		t.Fatal("category should be absent without values")
	}
}

func TestExtractAssignmentsSingleValueViolation(t *testing.T) {
	defs, names := resolvedDefs(t, nil)
	src := PageSources{
		TaxonomiesFM: map[string]interface{}{"series": "One"},
		Series:       "Two",
	}
	_, err := ExtractAssignments(defs, names, src, "post.md")
	if err == nil || !strings.Contains(err.Error(), "single-value") {
		t.Fatalf("err = %v", err)
	}
	// The same value from both sources dedupes to one entry: no violation.
	src.Series = "One"
	out, err := ExtractAssignments(defs, names, src, "post.md")
	if err != nil || strings.Join(out["series"], ",") != "One" {
		t.Fatalf("out = %v err = %v", out, err)
	}
}

func TestExtractAssignmentsScalarErrors(t *testing.T) {
	defs, names := resolvedDefs(t, map[string]DefinitionConfig{"technology": {}})
	badMap := PageSources{TaxonomiesFM: map[string]interface{}{"technology": 42}}
	if _, err := ExtractAssignments(defs, names, badMap, "a.md"); err == nil ||
		!strings.Contains(err.Error(), "taxonomies.technology") {
		t.Fatalf("map err = %v", err)
	}
	badField := PageSources{Extra: map[string]interface{}{"technology": []interface{}{"ok", 7}}}
	if _, err := ExtractAssignments(defs, names, badField, "a.md"); err == nil ||
		!strings.Contains(err.Error(), `field "technology"`) {
		t.Fatalf("field err = %v", err)
	}
}

func TestCoerceValues(t *testing.T) {
	if v, err := coerceValues("Go"); err != nil || len(v) != 1 || v[0] != "Go" {
		t.Fatalf("string: %v %v", v, err)
	}
	if v, err := coerceValues(""); err != nil || v != nil {
		t.Fatalf("empty string: %v %v", v, err)
	}
	if v, err := coerceValues([]interface{}{"a", "", "b"}); err != nil || strings.Join(v, ",") != "a,b" {
		t.Fatalf("list: %v %v", v, err)
	}
	if v, err := coerceValues([]string{"x"}); err != nil || v[0] != "x" {
		t.Fatalf("[]string: %v %v", v, err)
	}
	if v, err := coerceValues(nil); err != nil || v != nil {
		t.Fatalf("nil: %v %v", v, err)
	}
	if _, err := coerceValues(3.14); err == nil {
		t.Fatal("float should error")
	}
	if _, err := coerceValues([]interface{}{true}); err == nil {
		t.Fatal("bool item should error")
	}
}

func TestLegacyValues(t *testing.T) {
	src := PageSources{CategoryNames: []string{"News"}, Tags: []string{"go"}, Series: "S"}
	if legacyValues("category", src)[0] != "News" || legacyValues("tag", src)[0] != "go" ||
		legacyValues("series", src)[0] != "S" || legacyValues("technology", src) != nil {
		t.Fatal("legacyValues")
	}
	if legacyValues("series", PageSources{}) != nil {
		t.Fatal("empty series should yield nil")
	}
}

func TestDedupeNormalizedCaseSensitive(t *testing.T) {
	got := dedupeNormalized([]string{"Go", "go", " GO ", ""}, true)
	if strings.Join(got, ",") != "Go,go,GO" {
		t.Fatalf("case-sensitive = %v", got)
	}
	got = dedupeNormalized([]string{"Go", "go", " GO "}, false)
	if strings.Join(got, ",") != "Go" {
		t.Fatalf("case-insensitive = %v", got)
	}
}
