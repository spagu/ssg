package config

// Refusal paths of the in-place YAML editor (1.8.31). Both callers — the MCP
// designer tools and `ssg migrate` — edit a file the user owns, so every case
// where the document is not what we expect must return an error and leave the
// file untouched, never write a guess over it.

import (
	"strings"
	"testing"
)

func TestSetYAMLKeyRefusals(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"unparseable", "key: [unclosed\n"},
		{"scalar document", "just a string\n"},
		{"sequence document", "- one\n- two\n"},
		{"empty document", ""},
	}
	for _, c := range cases {
		out, err := SetYAMLKey([]byte(c.src), "title", "New")
		if err == nil {
			t.Errorf("%s: must refuse, got %q", c.name, out)
		}
		if out != nil {
			t.Errorf("%s: a refusal must return no document, got %q", c.name, out)
		}
	}
}

// TestHasYAMLKeySafeAnswers: when the file cannot be read as a mapping the
// answer is "it already has it", so a filler never overwrites anything.
func TestHasYAMLKeySafeAnswers(t *testing.T) {
	for _, src := range []string{"key: [unclosed\n", "just a scalar\n", "- a\n"} {
		if !HasYAMLKey([]byte(src), "title") {
			t.Errorf("unreadable/non-mapping %q must answer true (touch nothing)", src)
		}
	}
	if HasYAMLKey([]byte("title: Set\n"), "description") {
		t.Fatal("absent key must answer false")
	}
}

// TestSetYAMLKeyKeepsTheRestOfTheDocument: the point of editing the node tree
// is that everything around the change survives verbatim.
func TestSetYAMLKeyKeepsTheRestOfTheDocument(t *testing.T) {
	src := "# top comment\nsource: site   # inline\ntemplate: simple\n"
	out, err := SetYAMLKey([]byte(src), "title", "My Site")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"# top comment", "# inline", "source: site", "title: My Site"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q from:\n%s", want, got)
		}
	}
}
