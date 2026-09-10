package parser

// Content dimensions come out of the RAW frontmatter, not the typed struct
// (GO-096): `relations:`, `version:`, `version_of:` and `outputs:` stay in
// .Extra as well, so a template already reading .Extra.version keeps working.
//
// That means these readers see whatever YAML produced — an int, a float, a
// bare scalar where a list was expected — and every one of those shapes is
// something a person actually writes in frontmatter.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// mustTime is the value yaml.v3 produces for an unquoted date.
func mustTime() time.Time {
	return time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
}

// parseFile parses one document from a temp file.
func parseFile(t *testing.T, body string) *models.Page {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatalf("ParseMarkdownFile: %v", err)
	}
	return page
}

// TestDimensionsAreReadFromFrontmatter: the whole feature, through the real
// entry point rather than the helpers underneath it.
func TestDimensionsAreReadFromFrontmatter(t *testing.T) {
	page := parseFile(t, `---
title: API Authentication
slug: api-auth-v4
version: 4
version_of: api-auth
relations:
  supersedes: [api-auth-v3]
  see_also: oauth-setup
outputs: [html, json]
---

The new way.
`)
	if page.Version != "4" || page.VersionOf != "api-auth" {
		t.Errorf("version = %q, version_of = %q", page.Version, page.VersionOf)
	}
	want := map[string][]string{"supersedes": {"api-auth-v3"}, "see_also": {"oauth-setup"}}
	if !reflect.DeepEqual(page.Relations, want) {
		t.Errorf("relations = %v, want %v", page.Relations, want)
	}
	if !reflect.DeepEqual(page.Outputs, []string{"html", "json"}) {
		t.Errorf("outputs = %v", page.Outputs)
	}
	// The same keys stay readable the way they always were, which is the
	// promise that lets an existing theme keep working.
	if page.Extra["version"] == nil || page.Extra["relations"] == nil {
		t.Errorf("extra = %v", page.Extra)
	}
}

// TestAPageWithoutDimensionsGetsNone: every one of these keys is optional, and
// absent must not mean empty-but-present.
func TestAPageWithoutDimensionsGetsNone(t *testing.T) {
	page := parseFile(t, "---\ntitle: Plain\n---\n\nBody.\n")
	if page.Version != "" || page.VersionOf != "" || page.Relations != nil || page.Outputs != nil {
		t.Errorf("page = %+v", page)
	}
}

// TestScalarFieldRendersWhatWasWritten: `version: 4` and `version: "4"` are the
// same version, and a float that happens to be whole is not "4.0".
func TestScalarFieldRendersWhatWasWritten(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nothing", nil, ""},
		{"a string", "  4  ", "4"},
		{"an int", 4, "4"},
		{"a big int", int64(9007199254740993), "9007199254740993"},
		{"a whole float", float64(4), "4"},
		{"a fractional float", 4.5, "4.5"},
		{"a bool", true, "true"},
		{"a time, which YAML produces for a date", mustTime(), "2026-03-11 00:00:00 +0000 UTC"},
		// A mapping or a list is not a scalar. Rendering one would invent a
		// value and pass it on as if an author had typed it.
		{"a mapping", map[string]interface{}{"html": true}, ""},
		{"a typed list", []int{1, 2}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scalarField(tc.in); got != tc.want {
				t.Errorf("scalarField(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStringListAcceptsWhatPeopleWrite: a list, one bare value, or nothing.
func TestStringListAcceptsWhatPeopleWrite(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"a YAML list", []interface{}{"a", "b"}, []string{"a", "b"}},
		{"a list with a blank in it", []interface{}{"a", "", nil, "b"}, []string{"a", "b"}},
		{"a list of numbers", []interface{}{1, 2}, []string{"1", "2"}},
		{"a list that is already strings", []string{"a"}, []string{"a"}},
		{"one bare value", "a", []string{"a"}},
		{"one bare number", 7, []string{"7"}},
		{"nothing", nil, nil},
		{"an empty string", "", nil},
		{"an empty list", []interface{}{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringList(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("stringList(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestStringListMapNeedsAMapping: `relations: [a, b]` is not a relation, it is
// a mistake, and taking it would invent a relation named after nothing.
func TestStringListMapNeedsAMapping(t *testing.T) {
	if got := stringListMap([]interface{}{"a"}); got != nil {
		t.Errorf("a list is not a mapping: %v", got)
	}
	if got := stringListMap(nil); got != nil {
		t.Errorf("nothing is not a mapping: %v", got)
	}
	// A mapping whose every value is empty carries no relation at all.
	if got := stringListMap(map[string]interface{}{"supersedes": nil, "see_also": ""}); got != nil {
		t.Errorf("an empty mapping should be no relations: %v", got)
	}
	got := stringListMap(map[string]interface{}{"supersedes": "a", "skip": nil})
	want := map[string][]string{"supersedes": {"a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stringListMap = %v, want %v", got, want)
	}
}

// TestDimensionsIgnoreTheWrongShapes: frontmatter is hand-written, so the
// wrong shape has to be ignored rather than crash the build.
func TestDimensionsIgnoreTheWrongShapes(t *testing.T) {
	page := &models.Page{}
	applyDimensions(page, map[string]interface{}{
		"relations":  "not a mapping",
		"version":    nil,
		"version_of": "",
		"outputs":    map[string]interface{}{"html": true},
	})
	if page.Relations != nil || page.Version != "" || page.VersionOf != "" || page.Outputs != nil {
		t.Errorf("page = %+v", page)
	}
}

// TestUnreadableFrontmatterIsAnErrorNotAnEmptyPage.
//
// A document whose frontmatter is not YAML used to be worth checking because
// the alternative — a page with no title and no fields, built and published —
// is the silent kind of wrong.
func TestUnreadableFrontmatterIsAnErrorNotAnEmptyPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.md")
	if err := os.WriteFile(path, []byte("---\ntitle: [unclosed\n---\n\nBody.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMarkdownFile(path); err == nil {
		t.Error("frontmatter that is not YAML must fail the parse, not produce a blank page")
	}
}

// TestAPathThatCannotBeReadIsReported: a directory opens like a file on Linux
// and fails on the first read, which is the one way the scanner itself errors.
func TestAPathThatCannotBeReadIsReported(t *testing.T) {
	if _, err := ParseMarkdownFile(t.TempDir()); err == nil {
		t.Error("a directory is not a document")
	}
}

// TestExcerptStopsAtACodeFence: the derived excerpt is prose, and a fence that
// follows the opening paragraph must end it rather than be folded in.
func TestExcerptStopsAtACodeFence(t *testing.T) {
	page := parseFile(t, "---\ntitle: T\n---\n\nThe opening sentence.\n\n```go\nfunc main() {}\n```\n\nMore prose.\n")
	got := firstProseParagraph(page.Content)
	if len(got) != 1 || !strings.Contains(got[0], "The opening sentence.") {
		t.Errorf("firstProseParagraph = %q", got)
	}
}
