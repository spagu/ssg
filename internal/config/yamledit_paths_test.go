package config

// The edges of editing a config by path (GO-101): the paths people mistype,
// the shapes a value can take, and the one promise the whole feature rests on
// — setting one thing changes one line, and everything else is byte for byte
// where it was.

import (
	"strings"
	"testing"
)

// noComments is a config written the way a generated one is: no comments, no
// alignment to preserve, nothing but keys.
const noComments = `title: Example
domain: example.com
tags: [go, ssg]
nested:
  inner: 1
`

func TestParseYAMLPathRejectsATrailingDot(t *testing.T) {
	if _, err := ParseYAMLPath("taxonomies."); err == nil {
		t.Error("a trailing dot names nothing")
	} else if !strings.Contains(err.Error(), "empty path segment") {
		t.Errorf("error = %v", err)
	}
}

// TestPathStringNamesAListEntryTheWayItWasWritten, because it is what an error
// message shows back to the person who mistyped it.
func TestPathStringNamesAListEntryTheWayItWasWritten(t *testing.T) {
	segs, err := ParseYAMLPath("headers[2].value")
	if err != nil {
		t.Fatal(err)
	}
	if got := pathString(segs); got != "headers[2].value" {
		t.Errorf("pathString = %q", got)
	}
	if got := pathString(nil); got != "the document root" {
		t.Errorf("pathString(nil) = %q", got)
	}
}

// TestSetRefusesToWalkThroughTheWrongKindOfNode: descending into a scalar as
// if it were a mapping would either panic or invent structure, and the person
// who typed the path needs to know which part of it was wrong.
func TestSetRefusesToWalkThroughTheWrongKindOfNode(t *testing.T) {
	cases := []struct{ path, want string }{
		{"title.inner", "is not a mapping"},
		{"tags[0].x", "is not a mapping"},
		{"title[0]", "is not a list"},
		{"tags[9].x", "has 2 entries"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			_, err := SetYAMLPath([]byte(noComments), tc.path, "x")
			if err == nil {
				t.Fatalf("%s should be refused", tc.path)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestANewListEntryIsRefusedRatherThanInvented: appending to a list through a
// path that does not exist would have to guess where, and a config is not a
// place to guess.
func TestANewListEntryIsRefusedRatherThanInvented(t *testing.T) {
	_, err := SetYAMLPath([]byte(noComments), "brand.new[0]", "x")
	if err == nil || !strings.Contains(err.Error(), "must already exist") {
		t.Errorf("error = %v", err)
	}
}

// TestUnsetRefusesAListEntryThatIsNotThere: "removed it" and "it was never
// there" are different answers, and only one of them means the caller was right.
func TestUnsetRefusesAListEntryThatIsNotThere(t *testing.T) {
	for _, path := range []string{"tags[9]", "title[0]", "missing"} {
		if _, err := UnsetYAMLPath([]byte(noComments), path); err == nil {
			t.Errorf("%s is not set and should be refused", path)
		}
	}
}

// TestSettingAListKeepsTheStyleItWasWrittenIn: a flow list rewritten as a block
// would change lines nobody asked to change, which is the whole point of
// splicing rather than re-encoding.
func TestSettingAListKeepsTheStyleItWasWrittenIn(t *testing.T) {
	out, err := SetYAMLPath([]byte(noComments), "tags", []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "tags: [a, b, c]") {
		t.Errorf("a flow list should stay on its key's line:\n%s", out)
	}
	if strings.Count(string(out), "\n") != strings.Count(noComments, "\n") {
		t.Errorf("the file gained or lost lines:\n%s", out)
	}
}

// TestSettingAKeyWithNoCommentLeavesNoStrayPadding: the splice preserves a
// trailing comment by re-aligning it, and a line without one must not be
// padded to a column that is not there.
func TestSettingAKeyWithNoCommentLeavesNoStrayPadding(t *testing.T) {
	out, err := SetYAMLPath([]byte(noComments), "title", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "title:") && line != "title: Renamed" {
			t.Errorf("line = %q", line)
		}
	}
}

// TestALongValuePushesItsCommentAlongInsteadOfOverlapping.
func TestALongValuePushesItsCommentAlongInsteadOfOverlapping(t *testing.T) {
	src := "title: Short   # the name\n"
	out, err := SetYAMLPath([]byte(src), "title",
		"A title considerably longer than the column its comment used to sit in")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(strings.Split(string(out), "\n")[0], " ")
	if !strings.HasSuffix(line, "# the name") {
		t.Errorf("the comment was lost: %q", line)
	}
	if !strings.Contains(line, "considerably longer") {
		t.Errorf("the value was lost: %q", line)
	}
	if strings.Contains(line, "column its comment used to sit in# ") {
		t.Errorf("the comment ran into the value: %q", line)
	}
}

// TestANewKeyInAnEmptyFileIsWrittenAnyway: a config that is only comments and
// blank lines has no key to anchor against, and refusing would make
// `ssg config set` useless on a fresh file.
func TestANewKeyInAnEmptyFileIsWrittenAnyway(t *testing.T) {
	out, err := SetYAMLPath([]byte("\n\n"), "domain", "example.com")
	if err != nil {
		t.Fatalf("SetYAMLPath on a blank file: %v", err)
	}
	if !strings.Contains(string(out), "domain: example.com") {
		t.Errorf("out = %q", out)
	}
}

// TestParseYAMLMapReadsValuesRatherThanTheDocument, which is what a form
// filling its fields in needs — the write still goes back through SetYAMLPath
// so the file keeps its shape.
func TestParseYAMLMapReadsValuesRatherThanTheDocument(t *testing.T) {
	m, err := ParseYAMLMap([]byte(noComments))
	if err != nil {
		t.Fatal(err)
	}
	if m["title"] != "Example" {
		t.Errorf("title = %v", m["title"])
	}
	inner, ok := m["nested"].(map[string]interface{})
	if !ok || inner["inner"] != 1 {
		t.Errorf("nested = %v", m["nested"])
	}
	if _, err := ParseYAMLMap([]byte("title: [unclosed\n")); err == nil {
		t.Error("YAML that does not parse is an error, not an empty map")
	}
}
