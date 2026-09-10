package engine

// Adapter and engine-registration paths the GO-054 suite left untested: the
// arities handlebars wraps by hand, the argument that cannot be converted, and
// a template that parses but fails at render time.

import (
	"bytes"
	"strings"
	"testing"
)

// A FuncMap entry returning three values is not the html/template contract, so
// adaptHelper must name that instead of letting the extra results reach an
// engine that only ever reads res[0] and res[1].
func TestAdaptHelperRejectsThreeReturnValues(t *testing.T) {
	if _, err := adaptHelper(func() (int, int, error) { return 1, 2, nil }); err == nil ||
		!strings.Contains(err.Error(), "must return one value") {
		t.Errorf("three-return helper must be rejected, got %v", err)
	}
	// No return value at all is the same contract failure.
	if _, err := adaptHelper(func() {}); err == nil {
		t.Error("a helper returning nothing must be rejected")
	}
}

// A template can hand a helper a value of the wrong shape (a list where the
// helper wants a number). call must report which argument failed rather than
// panicking inside reflect.Call.
func TestCallReportsTheArgumentItCannotConvert(t *testing.T) {
	a, err := adaptHelper(func(n int) string { return strings.Repeat("x", n) })
	if err != nil {
		t.Fatalf("adaptHelper: %v", err)
	}
	_, err = a.call([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "argument 1") {
		t.Errorf("unconvertible argument must be named, got %v", err)
	}
}

// registerHandlebarsHelper wraps each arity in a hand-written closure; the
// 0-argument and 3-argument wrappers had no test, so a mis-wired closure (wrong
// argument order, dropped argument) would have gone unnoticed.
func TestHandlebarsCallsZeroAndThreeArgumentHelpers(t *testing.T) {
	e := NewHandlebarsEngine()
	funcs := go054Funcs()
	funcs["stamp54"] = func() string { return "stamped" }
	funcs["join54"] = func(a, b, c string) string { return a + "-" + b + "-" + c }

	tmpl, err := e.Parse("t", `{{stamp54}} {{join54 "x" name "z"}}`, funcs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"name": "y"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); got != "stamped x-y-z" {
		t.Errorf("output = %q, want %q", got, "stamped x-y-z")
	}
}

// A handlebars template that parses can still fail to render (here: a partial
// the engine was never given). Execute must return that error instead of
// writing raymond's empty result as if the page had rendered.
func TestHandlebarsExecuteReturnsARenderFailure(t *testing.T) {
	e := NewHandlebarsEngine()
	tmpl, err := e.Parse("t", `{{> noSuchPartial54 }}`, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "noSuchPartial54") {
		t.Fatalf("render failure must surface, got err=%v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("nothing must be written on a failed render, got %q", buf.String())
	}
}

// `{{ value|helper }}` gives pongo2 no filter parameter, but a two-argument
// helper cannot be called with one. The filter must supply the missing second
// argument as nil (converted to the parameter's zero value) instead of failing
// with an arity error.
func TestPongo2FilterFillsTheMissingSecondArgument(t *testing.T) {
	e := NewPongo2Engine()
	funcs := go054Funcs()
	funcs["suffix54"] = func(s, suffix string) string { return s + "|" + suffix }

	tmpl, err := e.Parse("t", `{{ name|suffix54 }} {{ name|suffix54:"end" }}`, funcs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{"name": "a"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); got != "a| a|end" {
		t.Errorf("output = %q, want %q", got, "a| a|end")
	}
}
