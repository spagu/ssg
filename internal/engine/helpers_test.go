package engine

import (
	"bytes"
	"fmt"
	"html/template"
	"reflect"
	"strings"
	"testing"
)

// GO-054 test FuncMap: one helper per adapter shape.
func go054Funcs() template.FuncMap {
	return template.FuncMap{
		"shout54":  strings.ToUpper,                                              // 1 arg
		"repeat54": func(s string, n int) string { return strings.Repeat(s, n) }, // 2 args, int param
		"safe54":   func(s string) template.HTML { return template.HTML("<b>" + s + "</b>") },
		"fail54":   func(s string) (string, error) { return "", fmt.Errorf("boom %s", s) },
		"wide54":   func(a, b, c, d string) string { return a + b + c + d }, // arity 4: unsupported everywhere
		"dictish":  func(kv ...interface{}) []interface{} { return kv },     // variadic
	}
}

func TestAdaptHelperValidation(t *testing.T) {
	if _, err := adaptHelper("not a func"); err == nil {
		t.Error("non-func must be rejected")
	}
	if _, err := adaptHelper(func() (int, int) { return 1, 2 }); err == nil {
		t.Error("second non-error return must be rejected")
	}
	a, err := adaptHelper(strings.ToUpper)
	if err != nil {
		t.Fatalf("adaptHelper(ToUpper): %v", err)
	}
	if !a.accepts(1) || a.accepts(2) {
		t.Error("ToUpper arity misdetected")
	}
	out, err := a.call("abc")
	if err != nil || out != "ABC" {
		t.Errorf("call = %v, %v", out, err)
	}
	// Numeric argument into a string parameter goes through fmt.Sprint.
	out, err = a.call(42)
	if err != nil || out != "42" {
		t.Errorf("numeric coercion = %v, %v; want \"42\"", out, err)
	}
}

func TestPongo2RealFilters(t *testing.T) {
	e := NewPongo2Engine()
	tmpl, err := e.Parse("t", `{{ name|shout54 }} {{ "ab"|repeat54:2 }} {{ name|safe54 }}`, go054Funcs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"name": "krowa"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"KROWA", "abab", "<b>krowa</b>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestPongo2UnsupportedFilterErrorsLoudly(t *testing.T) {
	e := NewPongo2Engine()
	tmpl, err := e.Parse("t", `{{ name|wide54 }}`, go054Funcs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	execErr := tmpl.Execute(&buf, map[string]interface{}{"name": "x"})
	if execErr == nil || !strings.Contains(execErr.Error(), "not available as a pongo2 filter") {
		t.Errorf("unsupported helper must fail loudly, got err=%v out=%q", execErr, buf.String())
	}
}

func TestPongo2HelperErrorPropagates(t *testing.T) {
	e := NewPongo2Engine()
	tmpl, err := e.Parse("t", `{{ name|fail54 }}`, go054Funcs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if execErr := tmpl.Execute(&buf, map[string]interface{}{"name": "x"}); execErr == nil ||
		!strings.Contains(execErr.Error(), "boom") {
		t.Errorf("helper error must propagate, got %v", execErr)
	}
}

func TestHandlebarsRealHelpers(t *testing.T) {
	e := NewHandlebarsEngine()
	tmpl, err := e.Parse("t", `{{shout54 name}} {{repeat54 "ab" 2}} {{safe54 name}}`, go054Funcs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"name": "krowa"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"KROWA", "abab", "<b>krowa</b>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestHandlebarsHelperErrorVisibleNotSilent(t *testing.T) {
	e := NewHandlebarsEngine()
	tmpl, err := e.Parse("t", `{{fail54 name}}`, go054Funcs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"name": "x"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "[helper fail54 error:") {
		t.Errorf("runtime helper failure must be visible in output, got %q", buf.String())
	}
}

func TestHandlebarsUnsupportedReported(t *testing.T) {
	if registerHandlebarsHelper("wide54", go054Funcs()["wide54"]) {
		t.Error("arity-4 helper must be unsupported in handlebars")
	}
	if registerHandlebarsHelper("dictish", go054Funcs()["dictish"]) {
		t.Error("variadic helper must be unsupported in handlebars")
	}
}

func TestMustacheWarnsOnceAboutHelpers(t *testing.T) {
	e := NewMustacheEngine()
	if _, err := e.Parse("t", `{{name}}`, go054Funcs()); err != nil {
		t.Fatalf("parse: %v", err)
	}
	key := EngineMustache + "|" + fmt.Sprintf(
		"mustache engine: template helpers are not supported (logic-less); %d helper(s) unavailable", len(go054Funcs()))
	if _, ok := warnedHelpers.Load(key); !ok {
		t.Error("mustache must record its helpers-unavailable warning")
	}
}

func TestConvertArgCoercions(t *testing.T) {
	strT := reflect.TypeOf("")
	intT := reflect.TypeOf(0)

	// nil → zero value of the target type.
	if v, err := convertArg(nil, strT); err != nil || v.Interface() != "" {
		t.Errorf("nil→string = %v, %v", v, err)
	}
	// assignable passes straight through.
	if v, err := convertArg("hi", strT); err != nil || v.Interface() != "hi" {
		t.Errorf("string→string = %v, %v", v, err)
	}
	// numeric → string via fmt.Sprint.
	if v, err := convertArg(7, strT); err != nil || v.Interface() != "7" {
		t.Errorf("int→string = %v, %v", v, err)
	}
	// convertible numeric (float64→int).
	if v, err := convertArg(float64(3), intT); err != nil || v.Interface() != 3 {
		t.Errorf("float64→int = %v, %v", v, err)
	}
	// genuinely unconvertible.
	if _, err := convertArg([]string{"a"}, intT); err == nil {
		t.Error("slice→int should be rejected")
	}
}

func TestAdaptHelperVariadicAndArity(t *testing.T) {
	a, err := adaptHelper(func(parts ...string) string { return parts[0] })
	if err != nil {
		t.Fatalf("variadic adapt: %v", err)
	}
	if !a.accepts(1) || !a.accepts(3) {
		t.Error("variadic helper should accept 1 and 3 args")
	}
	if _, err := a.call(); err == nil {
		t.Error("variadic helper called with too few fixed args should error")
	}
	out, err := a.call("x", "y")
	if err != nil || out != "x" {
		t.Errorf("variadic call = %v, %v", out, err)
	}
}

func TestCallArityMismatch(t *testing.T) {
	a, _ := adaptHelper(strings.ToUpper)
	if _, err := a.call("a", "b"); err == nil {
		t.Error("1-arg helper called with 2 args should error")
	}
}

func TestHelperResultString(t *testing.T) {
	if s, safe := helperResultString(template.HTML("<i>x</i>")); s != "<i>x</i>" || !safe {
		t.Errorf("template.HTML handling wrong: %q %v", s, safe)
	}
	if s, safe := helperResultString(7); s != "7" || safe {
		t.Errorf("int handling wrong: %q %v", s, safe)
	}
	if s, _ := helperResultString(nil); s != "" {
		t.Errorf("nil handling wrong: %q", s)
	}
}
