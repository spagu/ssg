package generator

// A feed's full content is the post's HTML read somewhere else (#347).
//
// On the page, "/images/a.svg" and "img/a.png" resolve against the page. In a
// feed they have nothing to resolve against: RSS 2.0 gives item content no
// base URI, so readers that do not guess one from the channel show broken
// images and dead links; Atom readers resolve against the feed's own URL, so
// a post's document-relative "img/a.png" pointed into the feed's directory.
// Every relative URL in the content is made absolute against the post's own
// address, as Hugo's absURL and Jekyll's absolute_url do in their feeds.

import (
	"net/url"
	"strings"
)

// absoluteFeedHTML resolves every relative URL of a post's HTML against the
// post's absolute address. Absolute and scheme-relative URLs are left as they
// are, and so is everything when the post has no absolute address.
func absoluteFeedHTML(html, postURL string) string {
	base, err := url.Parse(postURL)
	if err != nil || !base.IsAbs() {
		return html
	}
	return rewriteHTMLURLs(html, func(raw string) string {
		return absoluteAgainst(base, raw)
	})
}

// absoluteAgainst resolves one URL; a value that is not a URL is returned
// unchanged rather than mangled.
func absoluteAgainst(base *url.URL, raw string) string {
	if raw == "" || strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() {
		return raw
	}
	return base.ResolveReference(u).String()
}
