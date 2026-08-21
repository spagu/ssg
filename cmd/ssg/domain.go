package main

// Saying so when `domain` was not the bare host (#162).
//
// The value reaches the canonical tag, og:url, the sitemap, the JSON-LD @id and
// the feed. A scheme in it published `https://https://example.com/…` into all of
// them and nothing said a word — so the correction is silent-proof: it happens
// once, names both values, and leaves the build running.

import (
	"github.com/spagu/ssg/internal/config"
)

// warnedDomains remembers what has already been reported, so a watch session
// that reloads its config every save does not repeat the same line into the log
// it is competing with for the operator's attention.
var warnedDomains = map[string]bool{}

// normalizeDomain reduces cfg.Domain to the bare host absolute URLs are built
// from, reporting the correction the first time it is needed.
func normalizeDomain(cfg *config.Config) {
	original := cfg.Domain
	fixed, changed := config.NormalizeDomain(original)
	if !changed {
		return
	}
	if w := config.DomainSchemeWarning(original); w != "" && !cfg.Quiet && !warnedDomains[original] {
		warnedDomains[original] = true
		errf("⚠️  %s\n", w)
	}
	cfg.Domain = fixed
}
