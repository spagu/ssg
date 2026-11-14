package modules

import (
	"fmt"
	"regexp"
	"strings"
)

// basePathRe is the grammar of a module's base path: the manifest schema's
// endpoint-path rule (non-empty segments that do not start with a dot, so no
// "." or ".." segment; no scheme, host, "//", backslash, query or fragment),
// with an optional trailing slash. "/" alone is refused: a module mounted at
// the site root would own every page.
var basePathRe = regexp.MustCompile(`^(?:` + pathSegment + `)+/?$`)

// pathSegment is one segment of a path on this site's host: a slash and a
// name that does not start with a dot.
const pathSegment = `/[A-Za-z0-9_~-][A-Za-z0-9._~-]*`

// filePathRe is a file on this host: the base-path grammar without the
// trailing slash, so it always names a file, never a directory.
var filePathRe = regexp.MustCompile(`^(?:` + pathSegment + `)+$`)

// SiteFilePath reports whether p names a file on this site's own host
// (/img/logo.svg): no scheme, host, "//", "." or ".." segment, backslash,
// query, fragment, percent-escape or whitespace. The admin theme's logo and
// font files use it (GO-152): the panels' content security policy allows
// images and fonts from 'self' only, and a value that passes can be written
// into CSS and JSON without escaping.
func SiteFilePath(p string) bool {
	return len(p) <= 256 && filePathRe.MatchString(p)
}

// idRe is the manifest schema's id rule.
var idRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Ref is one `modules:` entry: a module the build cannot see, because it is
// deployed on its own (a standalone Worker such as newsletter-ui).
type Ref struct {
	// ID is the module's id; it must equal the manifest's when Dir is set.
	ID string
	// URL is the module's base path on the site's own host.
	URL string
	// Dir optionally points at the module's source, so its module.json is
	// read and validated at build time; without it the core worker fetches
	// the manifest from the module at runtime.
	Dir string
}

// CheckRef refuses a `modules:` entry that cannot be used at all — a missing
// or malformed id, or a base that is not a path on this host. key is the
// entry's position ("modules[0]"), file the configuration file; both are
// named in the message. These are configuration errors, never warnings: a
// registry pointing a navigation at another site is not a degraded mode.
func CheckRef(file, key string, r Ref) error {
	if !idRe.MatchString(r.ID) || len(r.ID) > 64 {
		return fmt.Errorf("%s: %s.id %q must be a module id (lowercase letters, digits and dashes, starting with a letter)", file, key, r.ID)
	}
	if !basePathRe.MatchString(r.URL) {
		return fmt.Errorf("%s: %s.url %q must be a path on this site's host, like /newsletter-admin — "+
			"no scheme, host, \"//\", \"..\", backslash, query or fragment", file, key, r.URL)
	}
	return nil
}

// cleanBase drops a trailing slash, so /a/ and /a are one base.
func cleanBase(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimSuffix(p, "/")
}

// within reports whether path p is base or below it, on segment boundaries.
func within(p, base string) bool {
	return base == "/" || p == base || strings.HasPrefix(p, base+"/")
}

// panelURL is where a module's panel lives on the site: its base joined with
// the manifest's panel path, without a #fragment (hash routing stays inside
// one page).
func panelURL(base, panel string) string {
	panel, _, _ = strings.Cut(panel, "#")
	if base == "/" {
		return panel
	}
	return base + panel
}
