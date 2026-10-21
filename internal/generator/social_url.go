package generator

// Open Graph requires absolute URLs (#350). Facebook, LinkedIn, Slack,
// Mastodon and X fetch og:image without resolving it against the page, so
// marketing.og_image: "/card.png" — documented as "absolute or
// site-relative" — posted every link as bare text. og:image, twitter:image
// and the JSON-LD image are written absolute against the domain. The WebP
// pass still finds them: it treats the site's own absolute URLs as local.

import "strings"

// socialURL makes an image URL absolute for a social card: a site-relative
// path gets the domain, a scheme-relative one gets https:, an absolute one
// stays. Without a domain there is nothing to resolve against, and the value
// is kept as written rather than lost.
func (g *Generator) socialURL(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return ""
	case strings.HasPrefix(raw, "//"):
		return "https:" + raw
	}
	if abs := g.absoluteURL(raw); abs != "" {
		return abs
	}
	return raw
}
