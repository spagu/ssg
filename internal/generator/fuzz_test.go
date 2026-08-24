package generator

// Fuzz target for the invisible-character sanitiser (#176).
//
// This is the trickiest code in the package: it walks a document rune by rune
// while advancing a cursor through a separate, sorted list of protected byte
// spans, and it was rewritten once already from quadratic to linear. Both
// halves index the same string by byte offset, and a rune index used where a
// byte offset belongs is invisible in any test written from an example — the
// examples are all ASCII outside the interesting part.
//
// The input is Markdown a migration produced from someone else's site, so
// deliberately hostile bytes are the realistic case rather than a hypothetical.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzSanitizeHTML asserts the properties the sanitiser promises, not a
// particular output: what it removes is a policy that may change, but these
// three cannot.
func FuzzSanitizeHTML(f *testing.F) {
	f.Add("plain text")
	f.Add("a\u200bb")                                // zero-width space
	f.Add("<pre><code>a\u200bb</code></pre>")        // protected: must survive
	f.Add("<PRE><CODE>x</CODE></PRE>trailing\u200b") // case-insensitive tags
	f.Add("<pre><code>unclosed \u200b")              // a span that never ends
	f.Add("<pre><code>a</code></pre><pre><code>b</code></pre>")
	f.Add("\u202e reversed \u202d") // bidi overrides
	f.Add("    runs")               // non-breaking space run
	f.Add("\u017b\u200bółć")        // multi-byte before the target
	f.Add("")

	f.Fuzz(func(t *testing.T, in string) {
		out, counts := sanitizeHTML(in)

		// 1. Valid UTF-8 in, valid UTF-8 out. Slicing a string by a byte offset
		//    that is not a rune boundary is how this function would corrupt a
		//    document rather than clean it.
		if utf8.ValidString(in) && !utf8.ValidString(out) {
			t.Fatalf("valid input produced invalid UTF-8: %q → %q", in, out)
		}

		// 2. It only ever removes. Anything longer than its input means the
		//    cursor and the span list disagreed about where they were.
		if len(out) > len(in) {
			t.Fatalf("sanitising grew the text: %d → %d bytes (%q)", len(in), len(out), in)
		}

		// 3. The count and the effect agree. A non-zero count with an unchanged
		//    document — or the reverse — means the build reports a number that
		//    describes nothing, which is worse than reporting none.
		if (counts.total() > 0) != (out != in) {
			t.Fatalf("counts.total()=%d but changed=%v (%q → %q)",
				counts.total(), out != in, in, out)
		}
	})
}

// FuzzVerbatimSpans asserts the span list the sanitiser trusts: sorted,
// non-overlapping, and inside the string. The main loop advances a pointer
// through it exactly once and would read out of range if any of that were
// untrue.
func FuzzVerbatimSpans(f *testing.F) {
	f.Add("<pre><code>a</code></pre>")
	f.Add("<pre><code>a</code></pre><pre><code>b</code></pre>")
	f.Add("<pre><code><pre><code>nested</code></pre></code></pre>")
	f.Add("<pre><code>unclosed")
	f.Add(strings.Repeat("<pre><code>x</code></pre>", 8))
	f.Add("</code></pre><pre><code>")
	f.Add("")

	f.Fuzz(func(t *testing.T, in string) {
		spans := verbatimSpans(in)
		prev := -1
		for i, s := range spans {
			if s.from < 0 || s.to < s.from || s.to > len(in) {
				t.Fatalf("span %d = [%d,%d) outside a %d-byte string (%q)", i, s.from, s.to, len(in), in)
			}
			// Sorted and disjoint: the cursor never goes backwards, so an
			// out-of-order or overlapping span would silently protect the wrong
			// region of the document.
			if s.from < prev {
				t.Fatalf("span %d starts at %d, before the previous end %d (%q)", i, s.from, prev, in)
			}
			prev = s.to
		}
	})
}
