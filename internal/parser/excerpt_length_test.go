package parser

// The derived excerpt was capped at 200 while the generator's own meta check
// flagged anything over 160, so the default path failed the default check on
// every page that wrote no excerpt of its own (#265).

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestDerivedExcerptFitsTheDescriptionWindow: the number search engines
// actually display, which is the number the check enforces.
func TestDerivedExcerptFitsTheDescriptionWindow(t *testing.T) {
	if ExcerptMaxRunes > 160 {
		t.Fatalf("ExcerptMaxRunes = %d; a derived excerpt cannot pass a 160-character check", ExcerptMaxRunes)
	}
	long := "# Title\n\n" + strings.Repeat("Deployment always happens after generation and enabled post-processing. ", 10)

	got := DeriveExcerpt(long)

	if n := utf8.RuneCountInString(got); n > ExcerptMaxRunes {
		t.Errorf("derived excerpt is %d runes, over the %d cap", n, ExcerptMaxRunes)
	}
	// Still a usable summary, not a stub.
	if utf8.RuneCountInString(got) < 70 {
		t.Errorf("derived excerpt is too short to be a description: %q", got)
	}
	// Truncated on a word boundary, so it does not end mid-word.
	if strings.HasSuffix(got, "-") || strings.Contains(got, "  ") {
		t.Errorf("truncation looks wrong: %q", got)
	}
}

// TestShortProseIsNotTruncated: a first paragraph already inside the window is
// used whole.
func TestShortProseIsNotTruncated(t *testing.T) {
	const prose = "One command takes a live site to a working SSG project."
	if got := DeriveExcerpt("# T\n\n" + prose); got != prose {
		t.Errorf("DeriveExcerpt = %q, want %q", got, prose)
	}
}
