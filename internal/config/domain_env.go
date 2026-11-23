package config

// `domain:` from the environment, and the check a computed domain gets (#439).
//
// One config file often serves several hosts — a placeholder, staging and
// production — and before this the file had to be rewritten for each build.
// `domain: ${SITE_DOMAIN}` lets the environment choose. Only `domain` is
// expanded: other values (prices, regular expressions, inline scripts) may
// hold a literal `$` and must keep it.
//
// A reference that resolves to nothing is an error, never an empty host: an
// empty domain would publish `https:///…` into the canonical tag, og:url, the
// sitemap and the feed, and nothing in the build would look wrong.

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spagu/ssg/internal/externalsource"
)

// ExpandDomain resolves "$NAME" and "${NAME}" references in a domain, with the
// same rules as the external-source settings ("$$" is a literal dollar). A
// value with no reference is returned unchanged.
func ExpandDomain(domain string) (string, error) {
	out, missing := externalsource.ExpandEnv(domain)
	if missing == "" {
		return out, nil
	}
	state := "is not set in the environment"
	if _, set := os.LookupEnv(missing); set {
		state = "is empty"
	}
	return "", fmt.Errorf("domain %s references $%s, which %s — export it before the build or pass --domain", quote(domain), missing, state)
}

// domainForbidden are characters no host-and-path can carry: whitespace,
// quotes and brackets that would break out of an HTML attribute, and the
// URL delimiters (`?`, `#`, `@`) that would turn part of the domain into a
// query, a fragment or credentials. `$` is here because a domain still holding
// one after expansion ("$$" escaped) is not a host either.
const domainForbidden = " \t\r\n\"'<>`$\\{}|^?#@"

// ValidateDomain reports a (normalised) domain that cannot be the host of
// https://<domain>/: empty, carrying a forbidden character, or with no host
// before an optional port and path.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is empty")
	}
	if strings.ContainsAny(domain, domainForbidden) {
		return fmt.Errorf("domain %s is not a host: it contains a space, quote or one of $?#@", quote(domain))
	}
	u, err := url.Parse("https://" + domain)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("domain %s is not a host (expected e.g. example.com, example.com:8080 or example.com/blog)", quote(domain))
	}
	return nil
}
