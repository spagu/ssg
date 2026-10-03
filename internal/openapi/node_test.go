package openapi

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// node parses a YAML snippet into its root node.
func node(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func TestToAny(t *testing.T) {
	cases := []struct {
		name, src string
		want      any
	}{
		{"scalars", "[1, 1.5, true, null, text, 2024-01-02]", []any{1, 1.5, true, nil, "text", "2024-01-02"}},
		{"nested", "{a: {1: b}, c: [x]}", map[string]any{"a": map[string]any{"1": "b"}, "c": []any{"x"}}},
		{"anchors", "{a: &x {k: v}, b: *x}", map[string]any{"a": map[string]any{"k": "v"}, "b": map[string]any{"k": "v"}}},
		{"badTag", "!!int abc", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toAny(node(t, tc.src)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("toAny = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestToAnyCycle(t *testing.T) {
	n := node(t, "&a [*a]")
	got := toAny(n)
	depth := 0
	for v, ok := got.([]any); ok && len(v) == 1; v, ok = v[0].([]any) {
		depth++
	}
	if depth != maxDepth+1 {
		t.Errorf("depth = %d", depth)
	}
}

func TestPointerAndLookup(t *testing.T) {
	if got := pointer("#/paths", "/a/{b}", "x~y"); got != "#/paths/~1a~1{b}/x~0y" {
		t.Errorf("pointer = %q", got)
	}
	if got := unescape("~01~1"); got != "~1/" {
		t.Errorf("unescape = %q", got)
	}
	p := &parser{root: node(t, "{list: [a, {k: v}], 'a/b': {c: 1}}")}
	cases := map[string]string{
		"#/list/1/k":   "v",
		"#/a~1b/c":     "1",
		"#/list/%31/k": "v",
		"#/list/9":     "",
		"#/list/x":     "",
		"#/list/-1":    "",
		"#/nope/x":     "",
		"other":        "",
	}
	for ref, want := range cases {
		if got := str(p.lookup(ref)); got != want {
			t.Errorf("lookup(%q) = %q, want %q", ref, got, want)
		}
	}
	if p.lookup("#") != p.root {
		t.Error("lookup(#) is not the root")
	}
}

func TestDerefToRoot(t *testing.T) {
	p := &parser{root: node(t, "{a: {$ref: '#'}}")}
	if got := p.deref(get(p.root, "a"), "#/a"); got != p.root {
		t.Errorf("deref = %v", got)
	}
}
