package generator

import (
	"strings"
	"testing"
	"text/template"

	"github.com/spagu/ssg/internal/models"
)

func TestInNotIn(t *testing.T) {
	cases := []struct {
		name       string
		value      any
		collection any
		want       bool
		wantErr    bool
	}{
		{"string hit", "guide", []string{"guide", "tutorial"}, true, false},
		{"string miss", "post", []string{"guide", "tutorial"}, false, false},
		{"int cross-type", 5, []float64{1, 5, 9}, true, false},
		{"bool", true, []bool{false, true}, true, false},
		{"any slice", "go", tmplSliceOf("go", "web"), true, false},
		{"not a collection", "x", "guide", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tmplIn(c.value, c.collection)
			if (err != nil) != c.wantErr {
				t.Fatalf("in err = %v, wantErr %v", err, c.wantErr)
			}
			if !c.wantErr && got != c.want {
				t.Errorf("in = %v, want %v", got, c.want)
			}
			if c.wantErr {
				return
			}
			neg, _ := tmplNotIn(c.value, c.collection)
			if neg != !c.want {
				t.Errorf("notIn = %v, want %v", neg, !c.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		name      string
		container any
		value     any
		want      bool
		wantErr   bool
	}{
		{"string substring", "Golang SSG", "Go", true, false},
		{"string miss", "Golang", "Rust", false, false},
		{"slice element", []string{"ssg", "go"}, "ssg", true, false},
		{"array element", [2]int{1, 2}, 2, true, false},
		{"map key", map[string]int{"go": 1}, "go", true, false},
		{"map key miss", map[string]int{"go": 1}, "rust", false, false},
		{"string wants string", "Golang", 42, false, true},
		{"unsupported container", 42, "x", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tmplContains(c.container, c.value)
			if (err != nil) != c.wantErr {
				t.Fatalf("contains err = %v, wantErr %v", err, c.wantErr)
			}
			if !c.wantErr && got != c.want {
				t.Errorf("contains = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMatchesAndAffixes(t *testing.T) {
	if ok, err := tmplMatches(`^guide-`, "guide-intro"); err != nil || !ok {
		t.Errorf("matches = %v, %v", ok, err)
	}
	if ok, _ := tmplMatches(`^guide-`, "post-intro"); ok {
		t.Error("matches should be false")
	}
	if _, err := tmplMatches(`[`, "x"); err == nil || !strings.Contains(err.Error(), "invalid regular expression") {
		t.Errorf("invalid regex err = %v", err)
	}
	// Cached second call takes the fast path.
	if ok, err := tmplMatches(`^guide-`, "guide-two"); err != nil || !ok {
		t.Errorf("cached matches = %v, %v", ok, err)
	}
	if !strings.HasPrefix("guide-x", "guide-") || !strings.HasSuffix("intro.md", ".md") {
		t.Error("startsWith/endsWith stand-ins misbehave")
	}
}

func TestIsNilIsEmptyTernary(t *testing.T) {
	var nilPtr *models.Page
	var nilMap map[string]int
	var iface any = nilPtr

	if !tmplIsNil(nil) || !tmplIsNil(nilPtr) || !tmplIsNil(nilMap) || !tmplIsNil(iface) {
		t.Error("isNil should be true for nil variants")
	}
	if tmplIsNil(0) || tmplIsNil("") || tmplIsNil(models.Page{}) {
		t.Error("isNil must not panic on / flag non-nilable values")
	}

	empties := []any{nil, "", []string{}, map[string]int{}, 0, false, nilPtr}
	for _, e := range empties {
		if !tmplIsEmpty(e) {
			t.Errorf("isEmpty(%#v) should be true", e)
		}
	}
	nonEmpties := []any{"x", []string{"a"}, map[string]int{"a": 1}, 1, true, models.Page{}}
	for _, e := range nonEmpties {
		if tmplIsEmpty(e) {
			t.Errorf("isEmpty(%#v) should be false", e)
		}
	}

	if tmplTernary(true, "yes", "no") != "yes" || tmplTernary(false, "yes", "no") != "no" {
		t.Error("ternary picked the wrong branch")
	}
}

// renderHelperTemplate renders src with the full generator FuncMap and data.
func renderHelperTemplate(t *testing.T, src string, data any) string {
	t.Helper()
	g := newTestGen(t, "")
	tmpl, err := template.New("t").Funcs(g.buildTemplateFuncs(map[string]string{})).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return sb.String()
}

func TestHelpersIntegration(t *testing.T) {
	pages := []models.Page{
		{Title: "Old Guide", Type: "guide", Modified: ht(1)},
		{Title: "Post", Type: "post", Modified: ht(5)},
		{Title: "New Guide", Type: "guide", Modified: ht(4), Tags: []string{"featured"}},
		{Title: "Mid Guide", Type: "guide", Modified: ht(2)},
	}
	data := map[string]any{"Pages": pages, "Page": pages[2]}

	// The spec's primary pipeline: where → sort → first.
	out := renderHelperTemplate(t,
		`{{ range (.Pages | where "Type" "guide" | sort "Modified" "desc" | first 2) }}[{{ .Title }}]{{ end }}`,
		data)
	if out != "[New Guide][Mid Guide]" {
		t.Errorf("pipeline render = %q", out)
	}

	// in + slice literal.
	out = renderHelperTemplate(t,
		`{{ if in .Page.Type (slice "guide" "tutorial") }}supported{{ end }}`, data)
	if out != "supported" {
		t.Errorf("in render = %q", out)
	}

	// and/not/contains composition over native Go template conditionals.
	out = renderHelperTemplate(t,
		`{{ if and (not .Page.HasMath) (contains .Page.Tags "featured") }}featured{{ end }}`, data)
	if out != "featured" {
		t.Errorf("contains render = %q", out)
	}

	// groupBy iterates deterministically (Go templates sort map keys).
	out = renderHelperTemplate(t,
		`{{ range $k, $v := (.Pages | groupBy "Type") }}{{ $k }}={{ len $v }};{{ end }}`, data)
	if out != "guide=3;post=1;" {
		t.Errorf("groupBy render = %q", out)
	}

	// Errors surface as template execution errors, not panics.
	g := newTestGen(t, "")
	tmpl := template.Must(template.New("t").Funcs(g.buildTemplateFuncs(map[string]string{})).
		Parse(`{{ .Pages | sort "Nope" "asc" }}`))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err == nil || !strings.Contains(err.Error(), `field "Nope" does not exist`) {
		t.Errorf("expected descriptive template error, got %v", err)
	}
}
