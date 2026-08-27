package generator

// Whitespace the CSS minifier must not remove (#222).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minified runs the full CSS minifier over a stylesheet and returns the result.
func minified(t *testing.T, css string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.css")
	if err := os.WriteFile(path, []byte(css), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := minifyCSSFile(path); err != nil {
		t.Fatalf("minify: %v", err)
	}
	return mustRead(t, path)
}

// TestTheDescendantCombinatorSurvives — the reported case. A space before a
// pseudo-class is a combinator, and removing it turns a descendant selector
// into a compound one that matches nothing. The rules still parse, so nothing
// errors and the styling is simply gone.
func TestTheDescendantCombinatorSurvives(t *testing.T) {
	got := minified(t, `.prose :where(p){margin-top:1.25em}
.card :is(h2,h3){color:red}
.menu :not(.active){opacity:.5}
.panel :hover{outline:1px solid}
`)
	for _, want := range []string{
		".prose :where(p)", ".card :is(h2,h3)", ".menu :not(.active)", ".panel :hover",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the combinator was removed from %q:\n%s", want, got)
		}
	}
}

// TestACompoundSelectorStaysCompound: the fix must not go the other way and
// start inserting space that was never there.
func TestACompoundSelectorStaysCompound(t *testing.T) {
	got := minified(t, ".ok:hover{color:blue}\na:focus-visible{outline:none}\n")
	for _, want := range []string{".ok:hover", "a:focus-visible"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was altered:\n%s", want, got)
		}
	}
	if strings.Contains(got, ".ok :hover") {
		t.Errorf("a compound selector gained a combinator:\n%s", got)
	}
}

// TestDeclarationsAreStillMinified, which is where the compression actually is:
// people write `color: red`, not `color :red`.
func TestDeclarationsAreStillMinified(t *testing.T) {
	got := minified(t, "p {\n  color: green;\n  margin:  0   auto;\n}\n")
	for _, want := range []string{"p{", "color:green", "margin:0 auto"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("newlines survived minification:\n%q", got)
	}
}

// TestOtherSelectorWhitespaceIsUntouched. A colon was the only delimiter in the
// list that can carry a combinator before it, and this is the assertion that
// says so — an attribute selector or a child combinator gaining or losing a
// space would break the same way and just as quietly.
func TestOtherSelectorWhitespaceIsUntouched(t *testing.T) {
	got := minified(t, `.a [data-x]{color:red}
.b > .c{color:red}
.d + .e{color:red}
.f ~ .g{color:red}
.h .i{color:red}
`)
	for _, want := range []string{".a [data-x]", ".b > .c", ".d + .e", ".f ~ .g", ".h .i"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was altered:\n%s", want, got)
		}
	}
}

// TestMediaQueriesStillMinify: a colon inside a media feature has no combinator
// before it, so the declaration rule applies there too.
func TestMediaQueriesStillMinify(t *testing.T) {
	got := minified(t, "@media (min-width: 600px) {\n  .a :hover{color:red}\n}\n")
	if !strings.Contains(got, "min-width:600px") {
		t.Errorf("the media feature was not minified:\n%s", got)
	}
	if !strings.Contains(got, ".a :hover") {
		t.Errorf("the combinator inside the query was removed:\n%s", got)
	}
}

// TestCommentsAndStringsAreStillHandled, so this change did not disturb what
// #106 fixed.
func TestCommentsAndStringsAreStillHandled(t *testing.T) {
	got := minified(t, "a{content:\"/*\";color:red}/* dropped */\n.x :hover{color:blue}\n")
	if strings.Contains(got, "dropped") {
		t.Errorf("the comment survived:\n%s", got)
	}
	if !strings.Contains(got, `content:"/*"`) {
		t.Errorf("a comment marker inside a string was treated as a comment:\n%s", got)
	}
	if !strings.Contains(got, ".x :hover") {
		t.Errorf("the combinator was removed:\n%s", got)
	}
}
