package main

// The correction reaching the build (#162).

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestNormalizeDomainCorrectsAndReportsOnce: the value the build uses is the
// value the message names, and a watch session that reloads its config on every
// save does not repeat the line.
func TestNormalizeDomainCorrectsAndReportsOnce(t *testing.T) {
	warnedDomains = map[string]bool{}
	cfg := config.DefaultConfig()
	cfg.Domain = "https://example.com"

	out := captureStderr(t, func() { normalizeDomain(cfg) })
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q, want the bare host", cfg.Domain)
	}
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "bare host") {
		t.Errorf("the correction must be reported: %q", out)
	}

	// Reloading the same config says nothing further.
	cfg.Domain = "https://example.com"
	if out := captureStderr(t, func() { normalizeDomain(cfg) }); out != "" {
		t.Errorf("the same domain must not be reported twice: %q", out)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("it must still be corrected: %q", cfg.Domain)
	}
}

// TestNormalizeDomainLeavesAHostAlone: the case every existing site is in.
func TestNormalizeDomainLeavesAHostAlone(t *testing.T) {
	warnedDomains = map[string]bool{}
	cfg := config.DefaultConfig()
	cfg.Domain = "example.com"
	if out := captureStderr(t, func() { normalizeDomain(cfg) }); out != "" {
		t.Errorf("a bare host is not worth a word: %q", out)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("domain = %q", cfg.Domain)
	}
}

// TestNormalizeDomainQuietStillCorrects: --quiet suppresses the message, never
// the fix — a silent build must not publish https://https://…
func TestNormalizeDomainQuietStillCorrects(t *testing.T) {
	warnedDomains = map[string]bool{}
	cfg := config.DefaultConfig()
	cfg.Domain = "https://example.com"
	cfg.Quiet = true
	if out := captureStderr(t, func() { normalizeDomain(cfg) }); out != "" {
		t.Errorf("a quiet build stays quiet: %q", out)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("quiet must not skip the correction: %q", cfg.Domain)
	}
}

// TestCreateGeneratorConfigNormalizesDomain: the chokepoint every build path
// meets — a plain build, a watch reload and `migrate` — and the last point
// before the domain becomes every canonical URL the site publishes.
func TestCreateGeneratorConfigNormalizesDomain(t *testing.T) {
	warnedDomains = map[string]bool{}
	cfg := config.DefaultConfig()
	cfg.Domain = "http://example.com/"
	cfg.Quiet = true
	genCfg := createGeneratorConfig(cfg)
	if genCfg.Domain != "example.com" {
		t.Fatalf("the generator got domain %q", genCfg.Domain)
	}
}
