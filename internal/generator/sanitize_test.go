package generator

// Invisible characters in published output (#176).
//
// Half of these tests are about restraint: a page documenting these characters
// needs them literally, and that page is exactly the one a careless pass would
// ruin. Every character is written as an escape — a test file carrying them
// literally is one nobody can review.

import (
	"strings"
	"testing"
	"time"
)

const (
	zwsp     = "\u200B" // zero-width space
	rlo      = "\u202E" // right-to-left override
	pdf      = "\u202C" // pop directional formatting
	tagChar  = "\U000E0041"
	emSpace  = "\u2003"
	nbsp     = "\u00A0"
	bomChar  = "\uFEFF"
	softHyph = "\u00AD"
)

// TestInvisibleCharactersAreRemoved: the classes that break a page.
func TestInvisibleCharactersAreRemoved(t *testing.T) {
	cases := map[string]string{
		"zero-width space":  "<p>zero" + zwsp + "width</p>",
		"bidi override":     "<p>" + rlo + "evil" + pdf + "</p>",
		"tag characters":    "<p>hidden" + tagChar + "</p>",
		"soft hyphen":       "<p>mid" + softHyph + "word</p>",
		"byte order mark":   "<p>stray" + bomChar + "mark</p>",
		"zero-width joiner": "<p>a\u200Cb</p>",
		"word joiner":       "<p>a\u2060b</p>",
	}
	for name, in := range cases {
		out, counts := sanitizeHTML(in)
		if strings.ContainsAny(out, zwsp+rlo+pdf+softHyph+bomChar+"\u200C\u2060") || strings.Contains(out, tagChar) {
			t.Errorf("%s survived: %q", name, out)
		}
		if counts.total() == 0 {
			t.Errorf("%s was removed without being counted", name)
		}
	}
}

// TestVerbatimRegionsAreUntouched: the restraint that matters most. A page
// about these characters, and a code sample containing one, must survive.
func TestVerbatimRegionsAreUntouched(t *testing.T) {
	for _, tag := range []string{"pre", "code", "script", "style", "textarea"} {
		in := "<p>gone" + zwsp + "</p><" + tag + ">kept" + zwsp + rlo + "</" + tag + ">"
		out, _ := sanitizeHTML(in)

		at := strings.Index(out, "<"+tag)
		if at < 0 {
			t.Fatalf("<%s> disappeared: %q", tag, out)
		}
		if inside := out[at:]; !strings.Contains(inside, zwsp) || !strings.Contains(inside, rlo) {
			t.Errorf("<%s> content was modified: %q", tag, out)
		}
		if before := out[:at]; strings.Contains(before, zwsp) {
			t.Errorf("text outside <%s> was not cleaned: %q", tag, out)
		}
	}
	// An attribute-bearing open tag is still the same element.
	if out, _ := sanitizeHTML(`<pre class="x" data-a="1">kept` + zwsp + `</pre>`); !strings.Contains(out, zwsp) {
		t.Errorf("a <pre> with attributes must be protected: %q", out)
	}
	// A tag whose name merely starts the same is not it.
	if out, _ := sanitizeHTML("<codex>cleaned" + zwsp + "</codex>"); strings.Contains(out, zwsp) {
		t.Errorf("<codex> is not <code>: %q", out)
	}
	// An unclosed verbatim tag protects the remainder rather than guessing.
	out, _ := sanitizeHTML("<p>gone" + zwsp + "</p><pre>rest" + zwsp)
	if !strings.HasSuffix(out, "rest"+zwsp) {
		t.Errorf("an unclosed <pre> must protect what follows: %q", out)
	}
}

// TestLeadingBOMIsKept: a leading BOM is a BOM. Only a stray one mid-document
// is residue.
func TestLeadingBOMIsKept(t *testing.T) {
	out, counts := sanitizeHTML(bomChar + "<p>text</p>")
	if !strings.HasPrefix(out, bomChar) {
		t.Errorf("a leading BOM must survive: %q", out)
	}
	if counts.total() != 0 {
		t.Errorf("a leading BOM is not a finding: %v", counts)
	}
	if out, _ := sanitizeHTML("<p>a" + bomChar + "b</p>"); strings.Contains(out, bomChar) {
		t.Errorf("a mid-document BOM must go: %q", out)
	}
}

// TestNonBreakingSpaceIsTypographyUntilItIsNot: one between a number and its
// unit is deliberate; a run of them is a word processor holding a line
// together, and it stops the line wrapping where wrapping was wanted.
func TestNonBreakingSpaceIsTypographyUntilItIsNot(t *testing.T) {
	single := "<p>10" + nbsp + "km</p>"
	if out, counts := sanitizeHTML(single); out != single || counts.total() != 0 {
		t.Errorf("a single nbsp is typography: %q %v", out, counts)
	}
	out, counts := sanitizeHTML("<p>a" + nbsp + nbsp + nbsp + "b</p>")
	if strings.Contains(out, nbsp+nbsp) {
		t.Errorf("a run must collapse: %q", out)
	}
	if counts["non-breaking space run"] == 0 {
		t.Errorf("the run must be counted: %v", counts)
	}
}

// TestExoticSpacesBecomeOrdinaryOnes: the word break is meant, the width is
// residue that breaks wrapping and makes copied text fail to match its source.
func TestExoticSpacesBecomeOrdinaryOnes(t *testing.T) {
	out, counts := sanitizeHTML("<p>a" + emSpace + "b</p>")
	if out != "<p>a b</p>" {
		t.Errorf("out = %q, want an ordinary space", out)
	}
	if counts["exotic space"] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

// TestCleanDocumentIsByteIdentical: every already-clean site must get exactly
// what it got before, which is the whole backward-compatibility claim.
func TestCleanDocumentIsByteIdentical(t *testing.T) {
	clean := "<!DOCTYPE html>\n<html lang=\"pl\"><body><p>Zażółć gęślą jaźń — 10" + nbsp +
		"km, „cytat”</p><pre>code</pre></body></html>"
	out, counts := sanitizeHTML(clean)
	if out != clean {
		t.Errorf("a clean document changed:\n%q\n%q", clean, out)
	}
	if counts.total() != 0 {
		t.Errorf("a clean document produced findings: %v", counts)
	}
}

// TestMultibyteOffsetsSurvive: the walk yields byte offsets and the spans are
// byte offsets, so the two agree — but only if nothing in between converts. A
// page with non-ASCII text before a <pre> is what catches a mismatch.
func TestMultibyteOffsetsSurvive(t *testing.T) {
	in := "<p>Zażółć gęślą jaźń" + zwsp + "</p><pre>kept" + zwsp + "</pre>"
	out, _ := sanitizeHTML(in)
	if !strings.Contains(out, "<pre>kept"+zwsp+"</pre>") {
		t.Errorf("the protected span shifted: %q", out)
	}
	if strings.Contains(out[:strings.Index(out, "<pre>")], zwsp) {
		t.Errorf("the prose was not cleaned: %q", out)
	}
	if !strings.Contains(out, "Zażółć gęślą jaźń") {
		t.Errorf("the text itself must be intact: %q", out)
	}
}

// TestSanitizeModes: on by default, because the failure is invisible in every
// sense — and a site can ask to be told without being changed.
func TestSanitizeModes(t *testing.T) {
	g := newTestGen(t, "")
	dirty := "<p>a" + zwsp + "b</p>"

	if got := g.sanitizeMode(); got != "on" {
		t.Errorf("unset = %q, want on", got)
	}
	if out := g.sanitizeHTMLString(dirty); strings.Contains(out, zwsp) {
		t.Errorf("the default must clean: %q", out)
	}

	for _, off := range []string{"off", "OFF", "false", "no"} {
		g2 := newTestGen(t, "")
		g2.config.SanitizeOutput = off
		if out := g2.sanitizeHTMLString(dirty); out != dirty {
			t.Errorf("%q must leave the document alone: %q", off, out)
		}
	}

	warn := newTestGen(t, "")
	warn.config.SanitizeOutput = "warn"
	if out := warn.sanitizeHTMLString(dirty); out != dirty {
		t.Errorf("warn must not change the document: %q", out)
	}
	if warn.sanitized.total() == 0 {
		t.Error("warn must still record what it found")
	}
}

// TestReportIsOneLineForTheBuild, and silent when there is nothing to say.
func TestReportIsOneLineForTheBuild(t *testing.T) {
	g := newTestGen(t, "")
	if out := captureBuildOutput(t, g.reportSanitized); out != "" {
		t.Errorf("a clean build says nothing, got %q", out)
	}

	g.sanitizeHTMLString("<p>a" + zwsp + rlo + pdf + "b</p>")
	g.sanitizeHTMLString("<p>c" + zwsp + "d</p>")
	out := captureBuildOutput(t, g.reportSanitized)
	for _, want := range []string{"2 page(s)", "zero-width space", "bidi override"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report must mention %q:\n%s", want, out)
		}
	}
	// A quiet build stays quiet.
	g.config.Quiet = true
	if out := captureBuildOutput(t, g.reportSanitized); out != "" {
		t.Errorf("quiet build printed %q", out)
	}
}

// TestCountsSummaryIsStable so two builds of one site read the same way.
func TestCountsSummaryIsStable(t *testing.T) {
	c := sanitizeCounts{"b": 2, "a": 2, "c": 5}
	if got := c.summary(); got != "c 5 · a 2 · b 2" {
		t.Errorf("summary = %q", got)
	}
	if got := (sanitizeCounts{}).summary(); got != "" {
		t.Errorf("empty summary = %q", got)
	}
}

// TestSanitizeEmptyDocuments must not panic on the degenerate shapes.
func TestSanitizeEmptyDocuments(t *testing.T) {
	for _, in := range []string{"", "<p></p>", "<pre>", "</pre>", zwsp, bomChar} {
		out, _ := sanitizeHTML(in)
		_ = out
	}
}

// TestLargeDocumentWithManyCodeSpans is a regression on cost, not on
// correctness. The first version asked "is this offset inside any protected
// span?" per character, which is quadratic in the number of spans: a
// documentation page with a few thousand inline <code> elements took twenty
// minutes and timed out a CI job. Linear now, so this finishes instantly.
func TestLargeDocumentWithManyCodeSpans(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 4000; i++ {
		b.WriteString("<p>Some prose with a <code>span" + zwsp + "</code> inside it, and more text.</p>\n")
	}
	doc := b.String()

	done := make(chan struct{})
	var out string
	var counts sanitizeCounts
	go func() {
		out, counts = sanitizeHTML(doc)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("sanitizeHTML did not finish — the span lookup is superlinear again")
	}

	// Every one of them is inside <code> and must survive.
	if strings.Count(out, zwsp) != 4000 {
		t.Errorf("protected characters removed: %d of 4000 left", strings.Count(out, zwsp))
	}
	if counts.total() != 0 {
		t.Errorf("nothing outside a code span to remove, got %v", counts)
	}
}

// TestOverlappingVerbatimRegions: <pre><code> nests, which produces two spans
// that overlap. A walk that assumed them disjoint would leave the cursor behind
// and start cleaning protected text.
func TestOverlappingVerbatimRegions(t *testing.T) {
	in := "<p>gone" + zwsp + "</p><pre><code>kept" + zwsp + rlo + "</code></pre><p>gone" + zwsp + "</p>"
	out, counts := sanitizeHTML(in)

	if !strings.Contains(out, "kept"+zwsp+rlo) {
		t.Errorf("nested verbatim content was modified: %q", out)
	}
	if counts["zero-width space"] != 2 {
		t.Errorf("both unprotected characters must be removed: %v", counts)
	}
	// And the text after the block is cleaned, so the cursor did not stick.
	if strings.Count(out, zwsp) != 1 {
		t.Errorf("exactly the protected one survives: %q", out)
	}
}

// TestMergeSpans coalesces what nesting produces.
func TestMergeSpans(t *testing.T) {
	got := mergeSpans([]span{{10, 20}, {0, 5}, {12, 30}, {40, 50}})
	want := []span{{0, 5}, {10, 30}, {40, 50}}
	if len(got) != len(want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged = %v, want %v", got, want)
		}
	}
	if got := mergeSpans(nil); got != nil {
		t.Errorf("nothing to merge = %v", got)
	}
	one := []span{{1, 2}}
	if got := mergeSpans(one); len(got) != 1 {
		t.Errorf("a single span = %v", got)
	}
}

// TestFormatCharactersBeyondTheNamedList: the catch-all. Unicode keeps adding
// invisible format characters, and a list written today would silently stop
// covering the next one — so anything in the Cf category is removed and
// reported under its own name.
func TestFormatCharactersBeyondTheNamedList(t *testing.T) {
	// U+2061 FUNCTION APPLICATION: invisible, in Cf, and not on the named list.
	// Written as an escape for the same reason this feature exists — a source
	// file carrying one literally is a source file nobody can review.
	const funcApp = "\u2061"
	out, counts := sanitizeHTML("<p>a" + funcApp + "b</p>")
	if strings.Contains(out, funcApp) {
		t.Errorf("an unnamed format character must go: %q", out)
	}
	if counts["format character"] != 1 {
		t.Errorf("it must be counted under the catch-all: %v", counts)
	}
	// And an ordinary letter in no such category is untouched.
	if got, c := sanitizeHTML("<p>ą</p>"); got != "<p>ą</p>" || c.total() != 0 {
		t.Errorf("a letter must survive: %q %v", got, c)
	}
}

// TestReportInWarnModeSaysItChangedNothing, or an operator reads a removal
// report and looks for changes that were never made.
func TestReportInWarnModeSaysItChangedNothing(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SanitizeOutput = "warn"
	g.sanitizeHTMLString("<p>a" + zwsp + "b</p>")

	out := captureBuildOutput(t, g.reportSanitized)
	if !strings.Contains(out, "Found") {
		t.Errorf("warn mode must say it found rather than removed: %s", out)
	}
	if !strings.Contains(out, "reporting only") {
		t.Errorf("it must say nothing was changed: %s", out)
	}
}

// TestStrippedImageReportNamesWhatTravelsInEXIF: the number alone does not tell
// an operator why they should care.
func TestStrippedImageReport(t *testing.T) {
	g := newTestGen(t, "")
	if out := captureBuildOutput(t, g.reportStrippedImages); out != "" {
		t.Errorf("nothing stripped, nothing said: %q", out)
	}

	g.recordStrippedImage()
	g.recordStrippedImage()
	out := captureBuildOutput(t, g.reportStrippedImages)
	for _, want := range []string{"2", "GPS", "image_metadata: keep"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report must mention %q: %s", want, out)
		}
	}

	// A quiet build stays quiet.
	g.config.Quiet = true
	if out := captureBuildOutput(t, g.reportStrippedImages); out != "" {
		t.Errorf("quiet build printed %q", out)
	}
}

// TestStripImageMetadataDefaultsToStripping, because a photo published with GPS
// is the failure this exists to prevent.
func TestStripImageMetadataMode(t *testing.T) {
	g := newTestGen(t, "")
	if !g.stripImageMetadata() {
		t.Error("unset must strip")
	}
	for _, keep := range []string{"keep", "KEEP", "preserve"} {
		g.config.ImageMetadata = keep
		if g.stripImageMetadata() {
			t.Errorf("%q must publish the metadata", keep)
		}
	}
	g.config.ImageMetadata = "strip"
	if !g.stripImageMetadata() {
		t.Error("an explicit strip must strip")
	}
}

// TestIsJPEGPath: only JPEG is rewritten, because it is the marker-segment
// format where the removal is exact rather than a lossy re-encode.
func TestIsJPEGPath(t *testing.T) {
	for _, p := range []string{"a.jpg", "b.JPEG", "dir/c.Jpg"} {
		if !isJPEGPath(p) {
			t.Errorf("%q is a JPEG", p)
		}
	}
	for _, p := range []string{"a.png", "b.webp", "c.avif", "d.gif", "e"} {
		if isJPEGPath(p) {
			t.Errorf("%q is not a JPEG", p)
		}
	}
}
