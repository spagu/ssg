package generator

// Coverage for the comment-aware scanner (#106): the JS template-literal state
// machine, the "unsure → keep" fallbacks and the regex-vs-division heuristic.
// The contract under test everywhere: stripping may leave a comment behind,
// but must never delete bytes that were not a comment.

import (
	"strings"
	"testing"
)

// TestStripCommentsTemplateInterpolation: a `${…}` interpolation switches the
// scanner back to code and a later `}` resumes the literal — so a /* inside the
// literal text is never treated as a comment, while a real comment outside is
// still removed.
func TestStripCommentsTemplateInterpolation(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "single interpolation resumes literal",
			in:   "const s = `a${x}b /* kept */`; /* gone */",
			want: "const s = `a${x}b /* kept */`; ",
		},
		{
			name: "back-to-back interpolations re-enter code twice",
			in:   "const s = `${a}${b}`; // tail comment\n",
			want: "const s = `${a}${b}`; \n",
		},
		{
			name: "nested braces inside interpolation",
			in:   "const s = `${ {k: 1} }`; /* out */",
			want: "const s = `${ {k: 1} }`; ",
		},
	}
	for _, c := range cases {
		if got := stripComments(c.in, styleJS, false); got != c.want {
			t.Errorf("%s: stripComments(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestStripCommentsUnsureKeepsBytes: the fallback branches where the scanner
// cannot prove it is looking at a removable comment.
func TestStripCommentsUnsureKeepsBytes(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			// An unterminated /* is NOT removed: truncating the file would be a
			// silent corruption, so the rest is kept verbatim.
			name: "unterminated block comment kept",
			in:   "var x = 1; /* never closed",
			want: "var x = 1; /* never closed",
		},
		{
			// A // comment with no trailing newline runs to EOF and is dropped
			// without eating anything else.
			name: "line comment at EOF",
			in:   "var x = 1; // tail",
			want: "var x = 1; ",
		},
		{
			// keepLines replaces a removed block comment with its newlines so
			// line-based source maps stay exact.
			name: "keepLines preserves line count",
			in:   "a;/* one\ntwo\nthree */b;",
			want: "a;\n\nb;",
		},
	}
	for _, c := range cases {
		keep := strings.Contains(c.name, "keepLines")
		if got := stripComments(c.in, styleJS, keep); got != c.want {
			t.Errorf("%s: stripComments(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestScanQuotedTermination: an unterminated string consumes to end of line —
// where real JS/CSS would have failed to parse — or to EOF, never beyond.
func TestScanQuotedTermination(t *testing.T) {
	// Newline ends an unterminated literal (returns the newline's index).
	if got := scanQuoted("\"abc\nrest", 0); got != 4 {
		t.Errorf("newline-terminated scan = %d, want 4", got)
	}
	// EOF ends it too.
	if got := scanQuoted(`"abc`, 0); got != 4 {
		t.Errorf("EOF-terminated scan = %d, want 4", got)
	}
}

// TestScanRegexUnterminated: a slash whose "regex" never closes (and holds no
// newline) was not a regex — return one past the slash so nothing is deleted.
func TestScanRegexUnterminated(t *testing.T) {
	if got := scanRegex("/ab", 0); got != 1 {
		t.Errorf("unterminated regex scan = %d, want 1", got)
	}
}

// TestRegexCanStart: division follows a value, a regex follows an operator;
// start-of-file counts as operator position.
func TestRegexCanStart(t *testing.T) {
	if !regexCanStart(0) {
		t.Error("start of file must allow a regex literal")
	}
	for _, prev := range []byte{')', ']', '"', '\'', '`'} {
		if regexCanStart(prev) {
			t.Errorf("after %q a slash is division, not a regex", prev)
		}
	}
	if regexCanStart('x') || regexCanStart('9') || regexCanStart('_') {
		t.Error("after an identifier/number a slash is division")
	}
	if !regexCanStart('=') || !regexCanStart('(') {
		t.Error("after an operator a slash starts a regex")
	}
}
