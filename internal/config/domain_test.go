package config

// A domain that is not a bare host (#162).

import (
	"strings"
	"testing"
)

// TestNormalizeDomain: what gets corrected, and — more important — what does
// not. Every value an existing site already builds with must survive untouched.
func TestNormalizeDomain(t *testing.T) {
	corrected := map[string]string{
		"https://example.com":       "example.com",
		"http://example.com":        "example.com",
		"HTTPS://Example.com":       "Example.com", // scheme case is not the host's case
		"https://example.com/":      "example.com",
		"example.com/":              "example.com",
		"example.com///":            "example.com",
		"  https://example.com  ":   "example.com",
		"https://www.example.co.uk": "www.example.co.uk",
	}
	for in, want := range corrected {
		got, changed := NormalizeDomain(in)
		if got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
		if !changed {
			t.Errorf("NormalizeDomain(%q) must report the change", in)
		}
	}

	untouched := []string{
		"example.com",
		"www.example.com",
		"example.com:8080",   // a port is part of the host
		"example.com/blog",   // a subdirectory deploy addresses itself this way
		"localhost",          //
		"192.0.2.10",         //
		"",                   // an empty domain is reportMissingSettings' business
		"my-site.example.io", //
	}
	for _, in := range untouched {
		got, changed := NormalizeDomain(in)
		if got != in {
			t.Errorf("NormalizeDomain(%q) = %q — it was already a host", in, got)
		}
		if changed {
			t.Errorf("NormalizeDomain(%q) reported a change it did not make", in)
		}
	}
}

// TestDomainSchemeWarning: the message names both values, so the fix is obvious
// in the config file and not merely in the log.
func TestDomainSchemeWarning(t *testing.T) {
	w := DomainSchemeWarning("https://example.com")
	for _, want := range []string{`"https://example.com"`, `"example.com"`, "https://https://example.com"} {
		if !strings.Contains(w, want) {
			t.Errorf("the warning must contain %q:\n%s", want, w)
		}
	}
	// Nothing to correct, nothing to say.
	for _, quiet := range []string{"example.com", "example.com/blog", ""} {
		if got := DomainSchemeWarning(quiet); got != "" {
			t.Errorf("DomainSchemeWarning(%q) = %q, want silence", quiet, got)
		}
	}
	// A value that normalises to nothing is a missing domain, which is reported
	// elsewhere and in different words — this one must not speak over it.
	if got := DomainSchemeWarning("https://"); got != "" {
		t.Errorf("an empty host is not this warning's business: %q", got)
	}
}
