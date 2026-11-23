package main

// Saying so when `domain` was not the bare host (#162).
//
// The value reaches the canonical tag, og:url, the sitemap, the JSON-LD @id and
// the feed. A scheme in it published `https://https://example.com/…` into all of
// them and nothing said a word — so the correction is silent-proof: it happens
// once, names both values, and leaves the build running.

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

// domainFlag overrides `domain:` for one build (#439). It is parsed like every
// other value flag (stringEqualFlags), and read again here because it must
// also win over the positional <domain>, which is applied after the flags.
const domainFlag = "--domain"

// domainFlagOf returns the value of the last --domain on the command line,
// in either spelling, and whether one was given. Last wins, as in parseFlags.
func domainFlagOf(args []string) (value string, given bool) {
	for i, arg := range args {
		switch {
		case strings.HasPrefix(arg, domainFlag+"="):
			value, given = strings.TrimPrefix(arg, domainFlag+"="), true
		case arg == domainFlag && i+1 < len(args):
			value, given = args[i+1], true
		}
	}
	return value, given
}

// resolveDomain settles cfg.Domain before anything derives a URL from it:
// --domain wins over the file and the positional argument, otherwise
// `$NAME`/`${NAME}` in the configured value are expanded (#439). The result is
// normalised as every domain is (#162); a value that came from the flag or the
// environment is then checked, because nobody has seen it in a file. A
// literal domain keeps exactly the pre-#439 handling.
func resolveDomain(cfg *config.Config, args []string) error {
	if value, given := domainFlagOf(args); given {
		// The file's value is not expanded at all, so a reference to a
		// variable this build does not set cannot fail it.
		cfg.Domain = value
		return checkResolvedDomain(cfg, domainFlag)
	}
	raw := cfg.Domain
	expanded, err := config.ExpandDomain(raw)
	if err != nil {
		return err
	}
	if expanded == raw {
		normalizeDomain(cfg)
		return nil
	}
	cfg.Domain = expanded
	return checkResolvedDomain(cfg, fmt.Sprintf("domain %q", raw))
}

// checkResolvedDomain normalises a computed domain and refuses one that is
// not a host, naming where it came from.
func checkResolvedDomain(cfg *config.Config, origin string) error {
	normalizeDomain(cfg)
	if err := config.ValidateDomain(cfg.Domain); err != nil {
		return fmt.Errorf("%s: %w", origin, err)
	}
	return nil
}

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
