package js

import (
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestScanDepthSurvivesRegexAndTemplates(t *testing.T) {
	src := "const a = x / 2 / y;\nconst re = /[{(]/g;\nconst s = `${ {a: 1}.a }}`;\nconst t = (a) /= 2;\n" +
		"x = a ? /}/ : 0;\nfunction after() {}"
	s := scanSource([]byte(src))
	sites := s.topLevelSites()
	if i, ok := sites["after"]; !ok || s.toks[i].depth != 0 || s.tokenLine(i) != 6 {
		t.Errorf("after: %v %v", sites, ok)
	}
}

func TestScanDocAttachment(t *testing.T) {
	src := "/** Kept. */\nexport function a() {}\n/** Lost. */\n// line comment\nexport function b() {}\n" +
		"/** Lost too. */ /* block */ export function c() {}\n/**/ export function d() {}\n/***/ export function e() {}"
	s := scanSource([]byte(src))
	sites := s.topLevelSites()
	for name, want := range map[string]bool{"a": true, "b": false, "c": false, "d": false, "e": true} {
		if got := s.toks[sites[name]].doc >= 0; got != want {
			t.Errorf("%s: doc %v, want %v", name, got, want)
		}
	}
}

func TestScanStopsOnLexError(t *testing.T) {
	s := scanSource([]byte("const a = 1;\n\u0001 const b = 2;"))
	if _, ok := s.topLevelSites()["b"]; ok {
		t.Error("tokens after a lexical error were read")
	}
	if s.text(-1) != "" || s.text(len(s.toks)) != "" || s.tokenLine(-1) != 0 {
		t.Error("out-of-range lookups must be empty")
	}
}

func TestSites(t *testing.T) {
	src := "export async function* g() {}\nclass extends {}\nconst {x} = o;\nmodule.exports = 1;\nmodule.exports = 2;\n" +
		"module.other = 3;\nexports.k = 4;\na.exports.z = 5;\nmodule.exports.m = 6;\nlet p = 1,\n  q = 2; r, s;"
	s := scanSource([]byte(src))
	sites := s.topLevelSites()
	for _, k := range []string{"g", "module.exports", "exports.k", "exports.m", "p", "q"} {
		if _, ok := sites[k]; !ok {
			t.Errorf("missing site %q in %v", k, sites)
		}
	}
	for _, k := range []string{"extends", "x", "exports.z", "s"} {
		if _, ok := sites[k]; ok {
			t.Errorf("unexpected site %q", k)
		}
	}
	if s.text(sites["g"]) != "export" {
		t.Errorf("g starts at %q", s.text(sites["g"]))
	}
}

func TestIsIdent(t *testing.T) {
	for in, want := range map[string]bool{"a": true, "_a1": true, "$": true, "é": true, "": false, "1a": false, "a-b": false} {
		if isIdent(in) != want {
			t.Errorf("isIdent(%q) = %v", in, !want)
		}
	}
}

func TestMemberCursorEdges(t *testing.T) {
	s := scanSource([]byte("class A { a() {}"))
	c := s.membersAfter(0, "class")
	if c.end != len(s.toks) || c.find("a") < 0 {
		t.Errorf("unclosed body: %+v", c)
	}
	if s.membersAfter(-1, "").find("a") != -1 || s.membersAfter(0, "nope").find("a") != -1 {
		t.Error("no body: nothing found")
	}
}

func TestSortDiags(t *testing.T) {
	d := apisource.Diagnostic{File: "a", Line: 1, Message: "m"}
	got := sortDiags([]apisource.Diagnostic{{File: "b"}, d, {File: "a", Line: 2}, d, {File: "a", Line: 1, Message: "l"}})
	if len(got) != 4 || got[0].Message != "l" || got[3].File != "b" {
		t.Errorf("sortDiags = %v", got)
	}
}
