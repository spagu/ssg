package mcp

// A line is not a unit of size (#204).

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// minifiedCSS is the reported shape: one line, tens of kilobytes, three colour
// tokens buried in it.
func minifiedCSS() string {
	filler := strings.Repeat("a{color:#111}b{color:#222}", 400) // ~10 KB
	return filler + ":root{--color-bg-canvas:#FFFFFF;}" + filler + "\n"
}

// TestTheReportedCase: three colour changes across a minified stylesheet cost
// ~40k tokens, because "the changed line in context" was the whole file.
func TestTheReportedCase(t *testing.T) {
	s, root := newTestServer(t, nil)
	css := minifiedCSS()
	abs := writeProjectFile(t, root, "static/css/site.css", css)

	res := call(t, s, "designer_edit", map[string]any{
		"path": "static/css/site.css",
		"old":  "--color-bg-canvas:#FFFFFF;",
		"new":  "--color-bg-canvas:#0B1220;",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", text(res))
	}
	out := text(res)

	// The file is ~20 KB. The reply must be a small multiple of the change, not
	// a fraction of the file — that distinction is the whole ticket.
	if len(out) > 1024 {
		t.Errorf("reply is %d bytes for a %d-byte file; the cost still tracks the file",
			len(out), len(css))
	}
	if !strings.Contains(out, "#0B1220") {
		t.Errorf("the reply must show the change:\n%s", out)
	}
	if !strings.Contains(out, "chars omitted") {
		t.Errorf("the reply must say what it left out:\n%s", out)
	}
	// And the edit itself still landed in full.
	if !strings.Contains(readProjectFile(t, abs), "--color-bg-canvas:#0B1220;") {
		t.Error("the file was not edited")
	}
}

// TestFindOnAMinifiedFileAnswersWithAWindow, and says which columns, so an
// anchored edit still has an exact target.
func TestFindOnAMinifiedFileAnswersWithAWindow(t *testing.T) {
	s, root := newTestServer(t, nil)
	css := minifiedCSS()
	writeProjectFile(t, root, "static/css/site.css", css)

	out := text(call(t, s, "designer_find", map[string]any{"query": "--color-bg-canvas"}))
	if len(out) > 1024 {
		t.Errorf("find returned %d bytes for a %d-byte file:\n%s", len(out), len(css), out[:200])
	}
	if !strings.Contains(out, "--color-bg-canvas") {
		t.Errorf("the match must be shown:\n%s", out)
	}
	// A locus of "1-1" in a one-line file says nothing. Columns are what make
	// it a location.
	if !strings.Contains(out, "site.css:1:") {
		t.Errorf("the locus must carry a column range:\n%s", out)
	}
}

// TestOrdinarySourceIsUnchanged: lines remain the right unit where lines are a
// sensible size, and this must not become a second output format for everyone.
func TestOrdinarySourceIsUnchanged(t *testing.T) {
	s, root := newTestServer(t, nil)
	writeProjectFile(t, root, "static/css/style.css", themeCSS)

	out := text(call(t, s, "designer_edit", map[string]any{
		"path": "static/css/style.css",
		"old":  "  background: #fff;",
		"new":  "  background: #0b1220;",
	}))
	if strings.Contains(out, "chars omitted") {
		t.Errorf("a short line must be printed whole:\n%s", out)
	}
	if !strings.Contains(out, "→ ") || !strings.Contains(out, "color: var(--ink);") {
		t.Errorf("the neighbouring lines must still be shown:\n%s", out)
	}
}

// TestTheWindowNeverSplitsARune. Slicing UTF-8 at an offset that is not a rune
// boundary produces a replacement character where there was a letter — a tool
// that corrupts text rather than trimming it.
func TestTheWindowNeverSplitsARune(t *testing.T) {
	// Multi-byte either side of the match, at every offset the window can land
	// on, so a boundary error cannot hide between cases.
	for pad := 0; pad < 8; pad++ {
		line := strings.Repeat("ł", longLine+pad) + "MATCH" + strings.Repeat("ż", longLine+pad)
		at := strings.Index(line, "MATCH")
		frag, _, _, _ := charWindow(line, at, at+len("MATCH"))
		if !utf8.ValidString(frag) {
			t.Fatalf("pad %d produced invalid UTF-8", pad)
		}
		if strings.Contains(frag, "�") {
			t.Fatalf("pad %d split a rune: %q", pad, frag)
		}
		if !strings.Contains(frag, "MATCH") {
			t.Fatalf("pad %d lost the match", pad)
		}
	}
}

// TestColumnsAreCountedInCharactersNotBytes — a column a human can count.
func TestColumnsAreCountedInCharactersNotBytes(t *testing.T) {
	line := strings.Repeat("ż", 500) + "TARGET"
	at := strings.Index(line, "TARGET")
	_, fromCol, toCol, _ := charWindow(line, at, at+len("TARGET"))

	if fromCol != 501 {
		t.Errorf("fromCol = %d, want 501 (500 characters, not 1000 bytes)", fromCol)
	}
	if toCol != 506 {
		t.Errorf("toCol = %d, want 506", toCol)
	}
}

// TestAWindowAtEitherEndDoesNotClaimToOmitNothing.
func TestAWindowAtEitherEndDoesNotClaimToOmitNothing(t *testing.T) {
	line := "MATCH" + strings.Repeat("x", longLine*2)
	frag, _, _, trimmed := charWindow(line, 0, 5)
	if strings.HasPrefix(frag, "…") {
		t.Errorf("nothing precedes a match at the start: %q", frag[:40])
	}
	if !trimmed || !strings.Contains(frag, "chars omitted") {
		t.Errorf("the tail must be reported as omitted: %q", frag[len(frag)-60:])
	}

	short := "all of it"
	if _, _, _, trimmed := charWindow(short, 0, 3); trimmed {
		t.Error("a short line has nothing to trim")
	}
}

// TestEveryFragmentIsCapped, whatever produced it — a run of matching short
// lines can outgrow a minified one.
func TestEveryFragmentIsCapped(t *testing.T) {
	huge := strings.Repeat("x", maxFragmentBytes*2)
	got := capFragment(huge)
	if len(got) > maxFragmentBytes+128 {
		t.Errorf("capped fragment is %d bytes", len(got))
	}
	if !strings.Contains(got, "more chars omitted") {
		t.Error("the cap must be announced, not silent")
	}
	if short := capFragment("small"); short != "small" {
		t.Errorf("a small fragment must pass through: %q", short)
	}
}

// TestLocusFormatting: a single line, a range, and a column range.
func TestLocusFormatting(t *testing.T) {
	cases := []struct {
		from, to, fc, tc int
		want             string
	}{
		{4, 8, 0, 0, "4-8"},
		{4, 4, 0, 0, "4"},
		{1, 1, 40, 96, "1:40-96"},
	}
	for _, c := range cases {
		if got := locusOf(c.from, c.to, c.fc, c.tc); got != c.want {
			t.Errorf("locusOf(%d,%d,%d,%d) = %q, want %q", c.from, c.to, c.fc, c.tc, got, c.want)
		}
	}
}

// TestClampSpanKeepsTheWindowInsideTheLine, so a match reported past the end —
// or backwards — cannot index out of range.
func TestClampSpanKeepsTheWindowInsideTheLine(t *testing.T) {
	line := "short"
	for _, c := range [][2]int{{-5, 2}, {0, 999}, {999, 1}, {3, 1}} {
		frag, fromCol, toCol, _ := charWindow(line, c[0], c[1])
		if !strings.Contains(line, strings.TrimSpace(frag)) && frag != line {
			t.Errorf("charWindow(%q, %d, %d) = %q", line, c[0], c[1], frag)
		}
		if fromCol < 1 || toCol < 0 {
			t.Errorf("columns = %d-%d for span %v", fromCol, toCol, c)
		}
	}
}

// TestTrimLineIsOnlyForLongLines.
func TestTrimLineIsOnlyForLongLines(t *testing.T) {
	if got := trimLine("normal source line"); got != "normal source line" {
		t.Errorf("a short line must be untouched: %q", got)
	}
	long := strings.Repeat("y", longLine*3)
	got := trimLine(long)
	if len(got) >= len(long) || !strings.Contains(got, "chars omitted") {
		t.Errorf("a long line must be trimmed and say so: %d bytes", len(got))
	}
	_ = fmt.Sprint(got)
}

// TestSnapToRuneMovesOffBoundaries in both directions and at both ends.
//
// Driven directly because the window arithmetic can, with the right rune width,
// happen to land on boundaries and never exercise the loop — which is how a
// boundary bug survives a test that looks like it covers this.
func TestSnapToRuneMovesOffBoundaries(t *testing.T) {
	// 3-byte and 4-byte runes, so an offset lands mid-rune at most positions.
	s := "€€€" + "🙂🙂" + "€€€" // 3+3+3 + 4+4 + 3+3+3
	for at := 0; at <= len(s); at++ {
		for _, dir := range []int{-1, +1} {
			got := snapToRune(s, at, dir)
			if got < 0 || got > len(s) {
				t.Fatalf("snapToRune(%d, %d) = %d, out of range", at, dir, got)
			}
			if got != len(s) && !utf8.RuneStart(s[got]) {
				t.Fatalf("snapToRune(%d, %d) = %d, not a rune boundary", at, dir, got)
			}
			// Slicing there must produce valid UTF-8 on both sides, which is
			// the property the whole function exists for.
			if !utf8.ValidString(s[:got]) || !utf8.ValidString(s[got:]) {
				t.Fatalf("slicing at %d produced invalid UTF-8", got)
			}
		}
	}
	// Out-of-range offsets are clamped rather than panicking.
	if got := snapToRune(s, -100, -1); got != 0 {
		t.Errorf("a negative offset must clamp to 0, got %d", got)
	}
	if got := snapToRune(s, 1000, +1); got != len(s) {
		t.Errorf("an offset past the end must clamp to len, got %d", got)
	}
}
