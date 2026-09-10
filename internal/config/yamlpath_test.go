package config

// Editing a config by path, without disturbing the rest of the file (GO-101).

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// annotated is a config shaped like the ones this project ships: comments above
// keys, comments at the end of lines, blank lines between sections, a nested
// mapping, a key that is not an identifier, and a list.
const annotated = `# The site itself.
title: Example        # shown in <title>
domain: example.com

# How it looks.
highlight: true
highlight_style: github   # a Chroma style name

taxonomies:
  audience:
    multiple: false
  technology:
    multiple: true

headers:
  "/css/*":
    Cache-Control: "public, max-age=86400"

outputs:
  - html
  - markdown

# The last word.
strict: true
`

func TestParseYAMLPath(t *testing.T) {
	cases := map[string][]pathSeg{
		"title":                       {{key: "title"}},
		"a.b.c":                       {{key: "a"}, {key: "b"}, {key: "c"}},
		`headers."/css/*".Cache-Ctl`:  {{key: "headers"}, {key: "/css/*"}, {key: "Cache-Ctl"}},
		"robots_rules[1].allow":       {{key: "robots_rules"}, {index: 1, isIndex: true}, {key: "allow"}},
		"a[0][2]":                     {{key: "a"}, {index: 0, isIndex: true}, {index: 2, isIndex: true}},
		`"a.b"`:                       {{key: "a.b"}},
		`sitemaps[0].name`:            {{key: "sitemaps"}, {index: 0, isIndex: true}, {key: "name"}},
		`headers."/x".a."b.c"`:        {{key: "headers"}, {key: "/x"}, {key: "a"}, {key: "b.c"}},
		"trailing_ok":                 {{key: "trailing_ok"}},
		`mixed."quoted"unquoted`:      {{key: "mixed"}, {key: "quotedunquoted"}},
		"nested.list[2]":              {{key: "nested"}, {key: "list"}, {index: 2, isIndex: true}},
		"deep.a.b.c.d.e.f.g.h.i.j.k":  {{key: "deep"}, {key: "a"}, {key: "b"}, {key: "c"}, {key: "d"}, {key: "e"}, {key: "f"}, {key: "g"}, {key: "h"}, {key: "i"}, {key: "j"}, {key: "k"}},
		"one_segment_with_underscore": {{key: "one_segment_with_underscore"}},
	}
	for path, want := range cases {
		got, err := ParseYAMLPath(path)
		if err != nil {
			t.Errorf("%q: %v", path, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%q: %d segments, want %d (%v)", path, len(got), len(want), got)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q segment %d: %+v, want %+v", path, i, got[i], want[i])
			}
		}
	}
}

func TestParseYAMLPathRejects(t *testing.T) {
	for _, path := range []string{"", "   ", "a..b", ".a", "a.", `a."b`, "a[", "a[x]", "a[-1]", "a]"} {
		if segs, err := ParseYAMLPath(path); err == nil {
			t.Errorf("%q should not parse, got %+v", path, segs)
		}
	}
}

// TestSetChangesExactlyOneLine is the point of the whole exercise: the file
// people read keeps its blank lines, its comments and their alignment.
func TestSetChangesExactlyOneLine(t *testing.T) {
	out, err := SetYAMLPath([]byte(annotated), "highlight_style", "monokai")
	if err != nil {
		t.Fatal(err)
	}
	before, after := strings.Split(annotated, "\n"), strings.Split(string(out), "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d → %d\n%s", len(before), len(after), out)
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
			if !strings.Contains(after[i], "monokai") {
				t.Errorf("line %d changed unexpectedly: %q → %q", i+1, before[i], after[i])
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d lines changed, want 1", changed)
	}
	// The comment stays in the column it was written in, so a file whose
	// comments line up still does.
	col := func(text, key string) int {
		for _, l := range strings.Split(text, "\n") {
			if strings.HasPrefix(l, key) {
				return strings.Index(l, "#")
			}
		}
		return -1
	}
	if !strings.Contains(string(out), "# a Chroma style name") {
		t.Errorf("the trailing comment was not kept:\n%s", out)
	}
	if got, want := col(string(out), "highlight_style"), col(annotated, "highlight_style"); got != want {
		t.Errorf("the comment moved from column %d to %d:\n%s", want, got, out)
	}
}

// TestSetNested reaches into a mapping and into a key that is not an
// identifier — the two cases a top-level-only editor could not do at all.
func TestSetNested(t *testing.T) {
	out, err := SetYAMLPath([]byte(annotated), "taxonomies.audience.multiple", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "    multiple: true\n  technology:") {
		t.Errorf("nested set went wrong:\n%s", out)
	}
	out, err = SetYAMLPath([]byte(annotated), `headers."/css/*".Cache-Control`, "public, max-age=60")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Cache-Control: public, max-age=60") {
		t.Errorf("quoted segment went wrong:\n%s", out)
	}
	// Every other line survived both edits.
	if strings.Count(string(out), "\n") != strings.Count(annotated, "\n") {
		t.Error("the file changed length")
	}
}

// TestSetListEntryAndWholeList: a position in a list, and the list itself.
func TestSetListEntryAndWholeList(t *testing.T) {
	out, err := SetYAMLPath([]byte(annotated), "outputs[1]", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  - html\n  - json\n") {
		t.Errorf("list entry:\n%s", out)
	}
	out, err = SetYAMLPath([]byte(annotated), "outputs", []interface{}{"html", "markdown", "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "outputs:\n  - html\n  - markdown\n  - json\n") {
		t.Errorf("whole list:\n%s", out)
	}
	if !strings.Contains(string(out), "# The last word.\nstrict: true") {
		t.Errorf("replacing a list disturbed what followed it:\n%s", out)
	}
}

// TestSetCreatesMissingPath writes a path whose parents do not exist yet,
// rather than making the author create them by hand first.
func TestSetCreatesMissingPath(t *testing.T) {
	out, err := SetYAMLPath([]byte(annotated), "marketing.og_site_name", "Example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "marketing:\n  og_site_name: Example") {
		t.Errorf("new nested path:\n%s", out)
	}
	// Appended at the end, so nothing above it moved.
	if !strings.HasPrefix(string(out), annotated[:strings.Index(annotated, "# The last word.")]) {
		t.Errorf("the file above the insertion changed:\n%s", out)
	}
	// A new key inside an existing mapping lands with its siblings.
	out, err = SetYAMLPath([]byte(annotated), "taxonomies.format.multiple", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  format:\n    multiple: false\n\nheaders:") {
		t.Errorf("new key inside a mapping:\n%s", out)
	}
}

// TestSetStructuredValue: a mapping or list is written in block form, which is
// how a person writes one, and the existing callers depend on it.
func TestSetStructuredValue(t *testing.T) {
	out, err := SetYAMLPath([]byte(annotated), "colors", map[string]string{"primary": "#7b2ff7"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "colors:\n  primary: '#7b2ff7'") {
		t.Errorf("block form expected:\n%s", out)
	}
	out, err = SetYAMLPath([]byte(annotated), "empty", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "empty: {}") {
		t.Errorf("an empty collection belongs inline:\n%s", out)
	}
}

// TestUnset removes a key with its block and the comment that documents it,
// and refuses a path that was never there.
func TestUnset(t *testing.T) {
	out, err := UnsetYAMLPath([]byte(annotated), "highlight_style")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "highlight_style") {
		t.Errorf("key still present:\n%s", out)
	}
	if !strings.Contains(string(out), "highlight: true") {
		t.Errorf("a key with the same prefix was taken too:\n%s", out)
	}
	out, err = UnsetYAMLPath([]byte(annotated), "taxonomies.audience")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "audience") || !strings.Contains(string(out), "technology") {
		t.Errorf("nested unset:\n%s", out)
	}
	out, err = UnsetYAMLPath([]byte(annotated), "outputs[0]")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "- html") || !strings.Contains(string(out), "- markdown") {
		t.Errorf("list entry unset:\n%s", out)
	}
	for _, path := range []string{"nope", "taxonomies.nope", "outputs[9]", "title.nope", "outputs.nope"} {
		if _, err := UnsetYAMLPath([]byte(annotated), path); err == nil {
			t.Errorf("%q: unsetting what is not set must be an error", path)
		}
	}
}

// TestUnsetTakesTheKeysOwnComment: the comment block above a key documents it,
// and outliving the key it describes would make the file wrong.
func TestUnsetTakesTheKeysOwnComment(t *testing.T) {
	out, err := UnsetYAMLPath([]byte(annotated), "strict")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "# The last word.") {
		t.Errorf("the key's comment outlived it:\n%s", out)
	}
}

// TestGetYAMLPath returns scalars as themselves and collections as YAML.
func TestGetYAMLPath(t *testing.T) {
	got, err := GetYAMLPath([]byte(annotated), "highlight_style")
	if err != nil || strings.TrimSpace(string(got)) != "github" {
		t.Errorf("scalar = %q, %v", got, err)
	}
	got, err = GetYAMLPath([]byte(annotated), "taxonomies.technology")
	if err != nil || !strings.Contains(string(got), "multiple: true") {
		t.Errorf("mapping = %q, %v", got, err)
	}
	got, err = GetYAMLPath([]byte(annotated), "outputs[1]")
	if err != nil || strings.TrimSpace(string(got)) != "markdown" {
		t.Errorf("list entry = %q, %v", got, err)
	}
	for _, path := range []string{"nope", "taxonomies.nope", "outputs[7]", "title[0]", "highlight_style.deeper", "a..b"} {
		if _, err := GetYAMLPath([]byte(annotated), path); err == nil {
			t.Errorf("%q should not resolve", path)
		}
	}
}

// TestPathErrorsNameThePath: an error has to say which part of the path was
// wrong, or the person editing has no idea what to fix.
func TestPathErrorsNameThePath(t *testing.T) {
	_, err := SetYAMLPath([]byte(annotated), "title.deeper", "x")
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("got %v", err)
	}
	_, err = SetYAMLPath([]byte(annotated), "outputs[9]", "x")
	if err == nil || !strings.Contains(err.Error(), "2 entries") {
		t.Errorf("got %v", err)
	}
	_, err = SetYAMLPath([]byte(annotated), "title[0]", "x")
	if err == nil || !strings.Contains(err.Error(), "not a list") {
		t.Errorf("got %v", err)
	}
	_, err = SetYAMLPath([]byte(annotated), "outputs[0].deeper", "x")
	if err == nil {
		t.Errorf("a scalar list entry is not a mapping: %v", err)
	}
	_, err = SetYAMLPath([]byte(annotated), "taxonomies[0]", "x")
	if err == nil || !strings.Contains(err.Error(), "not a list") {
		t.Errorf("got %v", err)
	}
	if _, err := SetYAMLPath([]byte(annotated), "a..b", "x"); err == nil {
		t.Error("a malformed path must be refused before anything is written")
	}
	if _, err := SetYAMLPath([]byte("- a\n- b\n"), "title", "x"); err == nil {
		t.Error("a document that is not a mapping cannot be edited by key")
	}
	if _, err := UnsetYAMLPath([]byte("{{ bad"), "title"); err == nil {
		t.Error("unparseable YAML must error")
	}
	if _, err := GetYAMLPath([]byte("{{ bad"), "title"); err == nil {
		t.Error("unparseable YAML must error on read too")
	}
	if _, err := UnsetYAMLPath([]byte(annotated), "a..b"); err == nil {
		t.Error("a malformed path must be refused")
	}
}

// TestSetListEntryToACollectionIsRefused: writing a mapping under a "-" marker
// needs indentation this editor does not compute, so it says so instead of
// writing something subtly wrong.
func TestSetListEntryToACollectionIsRefused(t *testing.T) {
	_, err := SetYAMLPath([]byte(annotated), "outputs[0]", map[string]string{"a": "b"})
	if err == nil || !strings.Contains(err.Error(), "scalar") {
		t.Errorf("got %v", err)
	}
	// A flow list has no marker to keep either.
	_, err = SetYAMLPath([]byte("outputs: [html, markdown]\n"), "outputs[0]", "json")
	if err == nil || !strings.Contains(err.Error(), "flow sequence") {
		t.Errorf("got %v", err)
	}
}

// TestParseYAMLValue types a command-line string the way the file should read.
func TestParseYAMLValue(t *testing.T) {
	cases := []struct {
		raw   string
		force bool
		want  interface{}
	}{
		{"true", false, true},
		{"FALSE", false, false},
		{"8080", false, 8080},
		{"1.5", false, 1.5},
		{"github-dark", false, "github-dark"},
		{"true", true, "true"},
		{"null", false, nil},
		{"~", false, nil},
	}
	for _, c := range cases {
		if got := ParseYAMLValue(c.raw, c.force); got != c.want {
			t.Errorf("ParseYAMLValue(%q, %v) = %#v, want %#v", c.raw, c.force, got, c.want)
		}
	}
	list, ok := ParseYAMLValue("[html, markdown, 3]", false).([]interface{})
	if !ok || len(list) != 3 || list[0] != "html" || list[2] != 3 {
		t.Errorf("list = %#v", ParseYAMLValue("[html, markdown, 3]", false))
	}
	if empty, ok := ParseYAMLValue("[]", false).([]interface{}); !ok || len(empty) != 0 {
		t.Errorf("empty list = %#v", empty)
	}
}

// TestSetKeepsTheFileLoadable checks the edits against the real loader, on this
// project's own config — the one whose comments this feature exists to save.
func TestSetKeepsTheFileLoadable(t *testing.T) {
	src, err := os.ReadFile("../../docs-site.yaml")
	if err != nil {
		t.Skipf("docs-site.yaml not available: %v", err)
	}
	out, err := SetYAMLPath(src, "highlight_style", "monokai")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/.ssg.yaml"
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the edited config no longer loads: %v", err)
	}
	if cfg.HighlightStyle != "monokai" {
		t.Errorf("HighlightStyle = %q", cfg.HighlightStyle)
	}
	// One line differs, out of a file that is mostly comments.
	before, after := strings.Split(string(src), "\n"), strings.Split(string(out), "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d → %d", len(before), len(after))
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("%d lines changed in docs-site.yaml, want 1", changed)
	}
}

// TestSetYAMLKeyTakesItsKeyLiterally: the old entry point must not start
// treating a dotted key as a path now that it shares the engine.
func TestSetYAMLKeyTakesItsKeyLiterally(t *testing.T) {
	out, err := SetYAMLKey([]byte("title: x\n"), "a.b", "value")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "a.b: value") {
		t.Errorf("the key was split into a path:\n%s", out)
	}
	var back map[string]string
	if err := yaml.Unmarshal(out, &back); err != nil || back["a.b"] != "value" {
		t.Errorf("the dotted key does not read back as one key: %v %v", back, err)
	}
}

// TestSetIntoEmptyAndFlowMappings covers the shapes a config rarely has but
// still must edit: an empty document mapping, and an empty nested one.
func TestSetIntoEmptyAndFlowMappings(t *testing.T) {
	out, err := SetYAMLPath([]byte("{}\n"), "title", "Example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "title: Example") {
		t.Errorf("empty document:\n%s", out)
	}
	out, err = SetYAMLPath([]byte("marketing: {}\n"), "marketing.og_site_name", "Example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  og_site_name: Example") {
		t.Errorf("empty nested mapping:\n%s", out)
	}
}

// TestSetQuotesAKeyThatNeedsIt: a new key that YAML would misread is written
// quoted, and reads back as the key that was asked for.
func TestSetQuotesAKeyThatNeedsIt(t *testing.T) {
	out, err := SetYAMLPath([]byte("title: x\n"), `headers."/css/*".Cache-Control`, "public")
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Headers map[string]map[string]string `yaml:"headers"`
	}
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if back.Headers["/css/*"]["Cache-Control"] != "public" {
		t.Errorf("the quoted key did not survive:\n%s", out)
	}
}

// TestRenderValueRefusesTheUnencodable: a value YAML cannot represent fails the
// edit instead of writing something else.
func TestRenderValueRefusesTheUnencodable(t *testing.T) {
	if _, _, err := renderValue(make(chan int), 0); err == nil {
		t.Error("a channel is not a config value")
	}
	if _, err := SetYAMLPath([]byte("title: x\n"), "title", make(chan int)); err == nil {
		t.Error("set must refuse it too")
	}
	if _, err := SetYAMLPath([]byte("title: x\n"), "other", make(chan int)); err == nil {
		t.Error("insert must refuse it too")
	}
	if _, err := SetYAMLPath([]byte("list:\n  - a\n"), "list[0]", make(chan int)); err == nil {
		t.Error("a list entry must refuse it too")
	}
}

// TestQuoteKey: only what YAML would misread gets quoted.
func TestQuoteKey(t *testing.T) {
	for _, plain := range []string{"title", "a.b", "Cache-Control", "a_b"} {
		if got := quoteKey(plain); got != plain {
			t.Errorf("quoteKey(%q) = %q, want it left alone", plain, got)
		}
	}
	for _, needs := range []string{"", "/css/*", "a: b", "a,b", `say "hi"`} {
		if got := quoteKey(needs); !strings.HasPrefix(got, `"`) {
			t.Errorf("quoteKey(%q) = %q, want it quoted", needs, got)
		}
	}
}

// TestKeyColonIndexFallsBack: a line the parser located but that does not look
// the way the editor expects still yields a position rather than panicking.
func TestKeyColonIndexFallsBack(t *testing.T) {
	node := &yaml.Node{Value: "key", Column: 1}
	if got := keyColonIndex("key: value", node); got != 3 {
		t.Errorf("got %d", got)
	}
	if got := keyColonIndex("nocolonhere", node); got != len("nocolonhere")-1 {
		t.Errorf("no colon: got %d", got)
	}
	if got := keyColonIndex("k: v", &yaml.Node{Value: "verylongkey", Column: 1}); got != 1 {
		t.Errorf("column past the line: got %d", got)
	}
}
