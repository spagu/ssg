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

// TestMultibyteOffsetsSurvive: verbatim regions are located by rune index, so a
// page with non-ASCII text before a <pre> must still protect the right span —
// a byte offset would land mid-character.
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
