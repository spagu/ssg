package generator

// Which occurrences of an asset name fingerprinting may rewrite (#316).
//
// The rewriter used to replace a fingerprinted file name wherever it stood in
// an HTML file, bounded by a slash or a quote: in prose ("the bundle
// (dist/app.js) is…"), in a <pre> snippet meant to be copied, and in URLs on
// other hosts that merely end in the same name. A CDN one-liner then pointed
// at app.c9c8999c.js on jsDelivr, a file that exists only on this site, and
// readers copied the 404.
//
// Now an HTML file is rewritten only where it references something: URL
// attributes of a tag, <style> blocks and <script> blocks. And in any file —
// HTML, CSS or JS — an absolute URL is rewritten only when its host is the
// site's own.

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// fingerprintHTMLPartRe finds what an HTML file references assets from:
	// script and style blocks whole, and every other start tag.
	fingerprintHTMLPartRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>|<[a-zA-Z][^<>]*>`)
	// fingerprintAttrRe finds the attributes of a tag whose values can name
	// an asset: URLs, srcset lists, data-* attributes and inline styles.
	fingerprintAttrRe = regexp.MustCompile(`(?i)(\s(?:src|href|srcset|poster|style|data-[\w-]+)\s*=\s*)("[^"]*"|'[^']*'|[^\s>]+)`)
	// fingerprintOpenTagRe is the opening tag of a script or style block.
	fingerprintOpenTagRe = regexp.MustCompile(`(?is)^<[a-z]+\b[^>]*>`)
)

// rewriteHTML rewrites asset names in the referencing parts of an HTML
// document and leaves its text alone.
func (rw *assetRefRewriter) rewriteHTML(s string) string {
	return fingerprintHTMLPartRe.ReplaceAllStringFunc(s, func(part string) string {
		lower := strings.ToLower(part)
		if strings.HasPrefix(lower, "<script") || strings.HasPrefix(lower, "<style") {
			open := fingerprintOpenTagRe.FindString(part)
			return rw.rewriteTag(open) + rw.rewrite(part[len(open):])
		}
		return rw.rewriteTag(part)
	})
}

// rewriteTag rewrites only the URL-carrying attribute values of one tag.
func (rw *assetRefRewriter) rewriteTag(tag string) string {
	return fingerprintAttrRe.ReplaceAllStringFunc(tag, func(attr string) string {
		m := fingerprintAttrRe.FindStringSubmatch(attr)
		if v := m[2]; v != "" && v[0] != '"' && v[0] != '\'' {
			// An unquoted value (href=js/app.js, hand-written or minified) has no closing
			// delimiter for the name; give it one, then take it back.
			out := rw.rewrite("=" + v + " ")
			return m[1] + out[1:len(out)-1]
		}
		return m[1] + rw.rewrite(m[2])
	})
}

// foreignURLAt reports whether the asset name matched at s[at:] belongs to
// an absolute URL on another host. It walks back from the match to the start
// of the URL — a quote, a parenthesis, whitespace, "=" or "," ends it — and
// compares the URL's host with the site's own.
func (rw *assetRefRewriter) foreignURLAt(s string, at int) bool {
	start := at
	for start > 0 && !strings.ContainsRune("\"'( \t\r\n=,", rune(s[start-1])) {
		start--
	}
	token := s[start:at]
	if !strings.HasPrefix(token, "//") && !strings.Contains(token, "://") {
		return false // relative or root-relative: the site's own
	}
	if strings.HasPrefix(token, "//") {
		token = "https:" + token
	}
	u, err := url.Parse(token)
	if err != nil || u.Host == "" {
		return true // unparseable absolute URL: not provably ours, leave it
	}
	host := strings.ToLower(u.Hostname())
	for _, own := range rw.ownHosts {
		if host == own {
			return false
		}
	}
	return true
}

// siteHosts is the host part of the configured domain, lower-cased, with and
// without "www.", or nothing when no domain is set.
func siteHosts(domain string) []string {
	host := strings.ToLower(strings.TrimSpace(domain))
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, ":")
	if host == "" {
		return nil
	}
	bare := strings.TrimPrefix(host, "www.")
	return []string{bare, "www." + bare}
}
