package generator

// What a <loc> may contain (Google's "Build and submit a sitemap"): a URL
// written as the site serves it, entity-escaped because the file is XML, and
// with anything outside ASCII percent-encoded, since a URL is ASCII.
//
// A canonical with a query string ("?a=1&b=2") wrote a bare "&" into the
// file, which is not XML, and a crawler rejects the whole sitemap over it.

import (
	stdhtml "html"
	"strings"
)

// sitemapLoc makes a URL safe for a <loc> or an href in a sitemap.
func sitemapLoc(raw string) string {
	return stdhtml.EscapeString(asciiURL(raw))
}

// asciiURL percent-encodes every byte outside printable ASCII, and spaces,
// leaving the rest — "%2F" already encoded, reserved characters, the scheme —
// as written.
func asciiURL(raw string) string {
	var sb strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c <= ' ' || c >= 0x7f {
			sb.WriteByte('%')
			sb.WriteByte("0123456789ABCDEF"[c>>4])
			sb.WriteByte("0123456789ABCDEF"[c&15])
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
