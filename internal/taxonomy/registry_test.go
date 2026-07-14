package taxonomy

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func testSlug(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), "-"))
}

func newTestRegistry(t *testing.T, user map[string]DefinitionConfig) *Registry {
	t.Helper()
	defs, names, err := Resolve(user, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewRegistry(defs, names, testSlug)
}

func TestNormalizeKey(t *testing.T) {
	key, display := NormalizeKey("  Machine   Learning  ", false)
	if key != "machine learning" || display != "Machine Learning" {
		t.Fatalf("key=%q display=%q", key, display)
	}
	key, display = NormalizeKey("Go", true)
	if key != "Go" || display != "Go" {
		t.Fatalf("case-sensitive key=%q", key)
	}
}

func TestAssignAndTerms(t *testing.T) {
	r := newTestRegistry(t, map[string]DefinitionConfig{"technology": {}})
	p1 := models.Page{Slug: "one"}
	p2 := models.Page{Slug: "two"}
	if err := r.Assign("technology", "", []string{"Go", "go", "Machine Learning", ""}, p1); err != nil {
		t.Fatal(err)
	}
	if err := r.Assign("technology", "", []string{"Go"}, p2); err != nil {
		t.Fatal(err)
	}
	if err := r.Assign("nope", "", []string{"x"}, p1); err == nil {
		t.Fatal("unknown taxonomy must error")
	}

	terms := r.Terms("technology", "")
	if len(terms) != 2 || terms[0].Name != "Go" || terms[0].Count != 2 || terms[0].Slug != "go" ||
		terms[1].Name != "Machine Learning" || terms[1].Slug != "machine-learning" {
		t.Fatalf("terms = %+v", terms)
	}
	pages := r.Pages("technology", "", "go")
	if len(pages) != 2 || pages[0].Slug != "one" || pages[1].Slug != "two" {
		t.Fatalf("pages = %+v", pages)
	}
	if r.Term("technology", "", " GO ") == nil || r.Term("nope", "", "x") != nil {
		t.Fatal("Term lookup")
	}
}

func TestAssignLanguageBucketsAndNoSlugify(t *testing.T) {
	r := newTestRegistry(t, map[string]DefinitionConfig{"technology": {Slugify: boolPtr(false)}})
	if err := r.Assign("technology", "pl", []string{"Sieci Neuronowe"}, models.Page{Slug: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Assign("technology", "en", []string{"Neural Networks"}, models.Page{Slug: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(r.Terms("technology", "pl")) != 1 || len(r.Terms("technology", "en")) != 1 ||
		len(r.Terms("technology", "")) != 0 {
		t.Fatal("language buckets leak")
	}
	if got := r.Terms("technology", "pl")[0].Slug; got != "Sieci Neuronowe" {
		t.Fatalf("slugify off: slug = %q", got)
	}
}

func TestApplyTermMeta(t *testing.T) {
	r := newTestRegistry(t, map[string]DefinitionConfig{"technology": {}})
	if err := r.Assign("technology", "", []string{"golang"}, models.Page{Slug: "p"}); err != nil {
		t.Fatal(err)
	}
	r.ApplyTermMeta("technology", "golang", map[string]interface{}{
		"name": "Go", "slug": "go-lang", "description": "The Go language",
		"weight": 5, "data": map[string]interface{}{"color": "#00ADD8"},
	}, "")
	tm := r.Term("technology", "", "golang")
	if tm.Name != "Go" || tm.Slug != "go-lang" || tm.Description != "The Go language" ||
		tm.Weight != 5 || tm.Data["color"] != "#00ADD8" || tm.Count != 1 {
		t.Fatalf("meta = %+v", tm)
	}
	// float64 weight (JSON data files) and creation of a zero-count term.
	r.ApplyTermMeta("technology", "Rust", map[string]interface{}{"weight": 7.0}, "")
	if got := r.Term("technology", "", "rust"); got == nil || got.Weight != 7 || got.Count != 0 {
		t.Fatalf("float weight = %+v", got)
	}
	// Unknown taxonomy is a no-op.
	r.ApplyTermMeta("nope", "x", map[string]interface{}{"name": "X"}, "")
	if r.Term("nope", "", "x") != nil {
		t.Fatal("unknown taxonomy created a term")
	}
}

func TestTermsSortingAndGenerateEmpty(t *testing.T) {
	user := map[string]DefinitionConfig{
		"bycount":  {Sort: "count"},
		"byweight": {Sort: "weight", GenerateEmpty: boolPtr(true)},
	}
	r := newTestRegistry(t, user)
	for i, vals := range [][]string{{"A", "B"}, {"B"}, {"B", "C"}} {
		if err := r.Assign("bycount", "", vals, models.Page{Slug: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.Terms("bycount", "")
	if got[0].Name != "B" || got[1].Name != "A" || got[2].Name != "C" {
		t.Fatalf("count sort = %v %v %v", got[0].Name, got[1].Name, got[2].Name)
	}

	r.ApplyTermMeta("byweight", "Light", map[string]interface{}{"weight": 1}, "")
	r.ApplyTermMeta("byweight", "Heavy", map[string]interface{}{"weight": 9}, "")
	r.ApplyTermMeta("byweight", "Alpha", map[string]interface{}{}, "")
	w := r.Terms("byweight", "")
	if len(w) != 3 || w[0].Name != "Heavy" || w[1].Name != "Light" || w[2].Name != "Alpha" {
		t.Fatalf("weight sort = %+v", w)
	}
	// Zero-count terms hidden without GenerateEmpty.
	r.ApplyTermMeta("bycount", "Ghost", map[string]interface{}{}, "")
	for _, tm := range r.Terms("bycount", "") {
		if tm.Name == "Ghost" {
			t.Fatal("zero-count term listed without generate_empty")
		}
	}
}

func TestValidateSlugs(t *testing.T) {
	r := newTestRegistry(t, map[string]DefinitionConfig{"technology": {}})
	if err := r.Assign("technology", "", []string{"C++", "Go"}, models.Page{Slug: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateSlugs(); err != nil {
		t.Fatalf("no collision expected: %v", err)
	}
	// Force two distinct terms onto one slug via metadata.
	r.ApplyTermMeta("technology", "Go", map[string]interface{}{"slug": "c++"}, "")
	if err := r.ValidateSlugs(); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("err = %v", err)
	}
	// The same slug in different languages is fine.
	r2 := newTestRegistry(t, nil)
	if err := r2.Assign("tag", "pl", []string{"Go"}, models.Page{Slug: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r2.Assign("tag", "en", []string{"Go"}, models.Page{Slug: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := r2.ValidateSlugs(); err != nil {
		t.Fatalf("cross-language slugs must not collide: %v", err)
	}
}
