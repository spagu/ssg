package main

// An option ssg does not accept is named, not ignored (#152). `--output=public`
// reads like it should work — the config key is `output_dir` — and the build
// then wrote to output/ and said nothing, which looks like the flag was
// honoured and the site landed somewhere else.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestWarnUnknownFlagsSuggests(t *testing.T) {
	cfg := config.DefaultConfig()
	out := captureStderr(t, func() { warnUnknownFlags([]string{"--output=public"}, cfg) })
	if !strings.Contains(out, "--output") || !strings.Contains(out, "--output-dir") {
		t.Fatalf("the warning must name the option and the one meant: %q", out)
	}
	// A one-character typo is worth a suggestion too.
	out = captureStderr(t, func() { warnUnknownFlags([]string{"--minify-htm"}, cfg) })
	if !strings.Contains(out, "--minify-html") {
		t.Errorf("a near miss must be suggested: %q", out)
	}
}

// TestWarnUnknownFlagsStaysQuiet: every option ssg accepts, in both spellings,
// and everything that is not an option at all.
func TestWarnUnknownFlagsStaysQuiet(t *testing.T) {
	cfg := config.DefaultConfig()
	quiet := []string{
		"--http", "--watch", "--zip", "-zip", "--quiet", "-q",
		"--output-dir=public", "--output-dir", "public",
		"--config=.ssg.yaml", "--paginate=10",
		"--check-links", "--check-schema", "--seo-off",
		"--auto-reload", "--no-auto-reload",
		"my-blog", "simple", "example.com", "-", "--",
	}
	if out := captureStderr(t, func() { warnUnknownFlags(quiet, cfg) }); out != "" {
		t.Fatalf("accepted options and positionals must stay silent, got: %s", out)
	}
}

// TestKnownFlagNamesReadsTheParsersTables: the check must not drift from what
// the parsers accept, so it reads the same tables rather than a copy.
func TestKnownFlagNamesReadsTheParsersTables(t *testing.T) {
	cfg := config.DefaultConfig()
	known := knownFlagNames(cfg)

	for name := range valueFlags() {
		if !known[name] {
			t.Errorf("value flag %s is not known to the check", name)
		}
	}
	for name := range boolFlagTargets(cfg) {
		if !known[name] {
			t.Errorf("toggle %s is not known to the check", name)
		}
	}
	if !known["--help"] || !known["--version"] {
		t.Error("the standalone options must be known")
	}
}

func TestNearestFlag(t *testing.T) {
	known := map[string]bool{"--output-dir": true, "--minify-html": true, "--watch": true}
	if got := nearestFlag("--output", known); got != "--output-dir" {
		t.Errorf("prefix match = %q", got)
	}
	if got := nearestFlag("--minify-htm", known); got != "--minify-html" {
		t.Errorf("one edit away = %q", got)
	}
	// Nothing close enough is better than a misleading guess.
	if got := nearestFlag("--completely-different", known); got != "" {
		t.Errorf("distant option suggested %q", got)
	}
}

func TestEditDistanceAtMostOne(t *testing.T) {
	cases := map[[2]string]bool{
		{"abc", "abc"}:   true,
		{"abc", "abd"}:   true,  // one substitution
		{"abc", "abcd"}:  true,  // one insertion
		{"abcd", "abc"}:  true,  // one deletion
		{"abc", "axd"}:   false, // two changes
		{"abc", "abcde"}: false, // two insertions
		{"", "a"}:        true,
	}
	for pair, want := range cases {
		if got := editDistanceAtMostOne(pair[0], pair[1]); got != want {
			t.Errorf("editDistanceAtMostOne(%q, %q) = %v, want %v", pair[0], pair[1], got, want)
		}
	}
}
