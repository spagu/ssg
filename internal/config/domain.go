package config

// Normalising `domain` (#162).
//
// `domain` is the host alone — canonical URLs are built as `https://<domain>` +
// the page's path. A value that already carries a scheme therefore produced
// `https://https://example.com/…` in the canonical tag, og:url, the sitemap, the
// JSON-LD @id and the feed: every address whose whole purpose is being a correct
// absolute URL, and every one of them unread by a human after a build.
//
// It is an easy mistake to make. `ssg migrate wordpress https://example.com`
// takes a full URL one line away in the same docs, and that URL is the site
// being migrated — the same string a person reaches for when the next argument
// is called a "domain". The two look alike and mean different things.
//
// Stripping rather than erroring keeps every existing site building; saying so
// once is the point, because a site publishing `https://https://…` has been
// submitting that sitemap to Search Console.

import "strings"

// NormalizeDomain returns the host form of a configured domain, and whether it
// had to change anything.
//
// A scheme is removed and trailing slashes are trimmed — `https://example.com/`
// and `example.com` name the same site, and the second form is the one the
// canonical builder expects. A path is left alone: `example.com/blog` is how a
// site deployed under a subdirectory addresses itself, and rewriting it would
// break a deliberate configuration to fix an accidental one.
func NormalizeDomain(domain string) (normalized string, changed bool) {
	out := strings.TrimSpace(domain)
	for _, scheme := range []string{"https://", "http://"} {
		if len(out) >= len(scheme) && strings.EqualFold(out[:len(scheme)], scheme) {
			out = out[len(scheme):]
			break
		}
	}
	out = strings.TrimRight(out, "/")
	return out, out != domain
}

// DomainSchemeWarning explains a domain that had to be corrected, or "" when
// there was nothing to correct. The message names both values so the fix is
// obvious in the config file rather than merely in the log.
func DomainSchemeWarning(original string) string {
	fixed, changed := NormalizeDomain(original)
	if !changed || fixed == "" {
		return ""
	}
	return "domain " + quote(original) + " is not a bare host — using " + quote(fixed) + ".\n" +
		"   `domain` is the host alone; canonical URLs, og:url, the sitemap and the\n" +
		"   JSON-LD @id are built as https://<domain>/… — a scheme here published\n" +
		"   https://" + original + "/ into every one of them."
}

// quote wraps a value in double quotes for a message.
func quote(s string) string { return `"` + s + `"` }
