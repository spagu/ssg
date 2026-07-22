package taxonomy

import (
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func TestResolvePerTaxonomyPaginate(t *testing.T) {
	defs, _, err := Resolve(map[string]DefinitionConfig{
		"technology": {Paginate: intPtr(24)},
		"tag":        {Paginate: intPtr(50)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if defs["technology"].Paginate != 24 {
		t.Fatalf("custom paginate = %d, want 24", defs["technology"].Paginate)
	}
	if defs["tag"].Paginate != 50 {
		t.Fatalf("legacy override paginate = %d, want 50", defs["tag"].Paginate)
	}
	// Unset paginate stays 0 (falls back to the global value at render time).
	if defs["category"].Paginate != 0 {
		t.Fatalf("default paginate = %d, want 0", defs["category"].Paginate)
	}
}

func TestResolvePaginateNegative(t *testing.T) {
	if _, _, err := Resolve(map[string]DefinitionConfig{"technology": {Paginate: intPtr(-1)}}, nil); err == nil {
		t.Fatal("expected a negative-paginate validation error")
	}
}

func TestResolveLegacyDefaults(t *testing.T) {
	defs, names, err := Resolve(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "category" || names[1] != "tag" || names[2] != "series" {
		t.Fatalf("names = %v", names)
	}
	cat := defs["category"]
	if !cat.Legacy || cat.Field != "categories" || cat.Path != "category" || !cat.Multiple ||
		!cat.Archive || !cat.Feed || !cat.Sitemap || cat.Template != "category.html" || cat.Sort != "name" || !cat.Slugify {
		t.Fatalf("category defaults = %+v", cat)
	}
	tag := defs["tag"]
	if !tag.Legacy || tag.Field != "tags" || !tag.Multiple || !tag.Feed || tag.Template != "tag.html" {
		t.Fatalf("tag defaults = %+v", tag)
	}
	ser := defs["series"]
	if !ser.Legacy || ser.Multiple || ser.Feed || ser.Path != "series" || ser.Singular != "Series" {
		t.Fatalf("series defaults = %+v", ser)
	}
}

func TestResolveCustomDefaultsAndOrdering(t *testing.T) {
	defs, names, err := Resolve(map[string]DefinitionConfig{"technology": {}, "difficulty": {}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"category", "tag", "series", "difficulty", "technology"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v", names)
	}
	tech := defs["technology"]
	if tech.Legacy || tech.Field != "technology" || tech.Label != "Technology" || tech.Singular != "Technology" ||
		tech.Path != "technology" || !tech.Multiple || !tech.Archive || tech.Feed || !tech.Sitemap ||
		tech.Template != "" || tech.Sort != "name" || !tech.Slugify || tech.CaseSensitive || tech.GenerateEmpty {
		t.Fatalf("custom defaults = %+v", tech)
	}
}

func TestResolveOverrides(t *testing.T) {
	defs, _, err := Resolve(map[string]DefinitionConfig{
		"technology": {
			Label: "Technologie", Singular: "Technologia", Path: "tech", Field: "techs",
			Template: "custom.html", TermTemplate: "custom-term.html", Sort: "count",
			Multiple: boolPtr(false), Archive: boolPtr(true), Feed: boolPtr(true),
			Sitemap: boolPtr(false), CaseSensitive: boolPtr(true), Slugify: boolPtr(false),
			GenerateEmpty: boolPtr(true),
		},
		"tag": {Feed: boolPtr(false), Label: "Etykiety"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tech := defs["technology"]
	if tech.Label != "Technologie" || tech.Singular != "Technologia" || tech.Path != "tech" ||
		tech.Field != "techs" || tech.Template != "custom.html" || tech.TermTemplate != "custom-term.html" ||
		tech.Sort != "count" || tech.Multiple || !tech.Feed || tech.Sitemap ||
		!tech.CaseSensitive || tech.Slugify || !tech.GenerateEmpty {
		t.Fatalf("overrides = %+v", tech)
	}
	tag := defs["tag"]
	if !tag.Legacy || tag.Feed || tag.Label != "Etykiety" || tag.Path != "tag" {
		t.Fatalf("legacy override = %+v", tag)
	}
}

func TestResolveValidationErrors(t *testing.T) {
	cases := map[string]map[string]DefinitionConfig{
		"invalid name uppercase": {"Technology": {}},
		"invalid name digit":     {"1tech": {}},
		"reserved path":          {"writers": {Path: "author"}},
		"duplicate path":         {"topics": {Path: "tag"}},
		"invalid sort":           {"technology": {Sort: "random"}},
	}
	for name, user := range cases {
		if _, _, err := Resolve(user, []string{"author", "page"}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// A custom taxonomy on a non-reserved path passes with the same reserved list.
	if _, _, err := Resolve(map[string]DefinitionConfig{"topics": {}}, []string{"author", "page"}); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestValidateEmptyPath(t *testing.T) {
	defs := map[string]Definition{"x": {Name: "x", Sort: "name"}}
	if err := validate(defs, []string{"x"}, nil); err == nil || !strings.Contains(err.Error(), "empty path") {
		t.Fatalf("err = %v", err)
	}
}

func TestTitleCase(t *testing.T) {
	if titleCase("") != "" || titleCase("tech") != "Tech" {
		t.Fatal("titleCase")
	}
}

func TestBoolOr(t *testing.T) {
	if !boolOr(nil, true) || boolOr(nil, false) || !boolOr(boolPtr(true), false) || boolOr(boolPtr(false), true) {
		t.Fatal("boolOr")
	}
}
