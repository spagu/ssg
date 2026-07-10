package engine

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// TestAltEnginesRender covers GO-007: pongo2/mustache/handlebars parse and render.
func TestAltEnginesRender(t *testing.T) {
	cases := []struct {
		engine  string
		content string
		want    string
	}{
		{"pongo2", "Hi {{ Title }}", "Hi World"},
		{"mustache", "Hi {{Title}}", "Hi World"},
		{"handlebars", "Hi {{Title}}", "Hi World"},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			eng, err := New(tc.engine)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.engine, err)
			}
			tmpl, err := eng.Parse("t", tc.content, nil)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, map[string]interface{}{"Title": "World"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("%s render = %q, want to contain %q", tc.engine, buf.String(), tc.want)
			}
		})
	}
}

// TestHelperRegistrationIdempotent covers GO-007: parsing multiple templates with
// the same funcs must not panic on re-registration (handlebars/pongo2).
func TestHelperRegistrationIdempotent(t *testing.T) {
	funcs := template.FuncMap{
		"upper": strings.ToUpper,
		"safe":  func(s string) template.HTML { return template.HTML(s) }, // not a valid raymond helper
	}
	for _, name := range []string{"handlebars", "pongo2"} {
		eng, _ := New(name)
		for i := 0; i < 3; i++ { // repeated Parse must not panic
			if _, err := eng.Parse("t", "{{ x }}", funcs); err != nil {
				t.Fatalf("%s Parse #%d: %v", name, i, err)
			}
		}
	}
}

// TestUnknownEngine rejects an unknown engine name.
func TestUnknownEngine(t *testing.T) {
	if _, err := New("twig"); err == nil {
		t.Error("expected error for unknown engine")
	}
}
