package generator

// Which HTML comments survive minification (#263).
//
// The minifier kept exactly one shape — the downlevel-revealed conditional
// `<!--[if …]>` — and deleted the rest. That was fine while a comment in a
// template could only be a note to the author, and wrong the moment #256
// documented `safeHTML` as the way to emit a **host directive**: the remedy
// worked with minification off and silently did nothing with it on, which is
// how every production build runs.
//
// The consequence was not cosmetic. A page carrying a mailto: without
// `<!--email_off-->` has it rewritten by Cloudflare Email Obfuscation into a
// /cdn-cgi/l/email-protection link whose bare endpoint answers 404, which a
// crawl of five sites reported from every page carrying an address.
//
// The rule is the one the CSS and JS comment scanners already follow: where it
// is unsure, keep the comment. A comment a theme wrote deliberately, through
// safeHTML, having been told to by the docs, is the clearest possible signal
// that something downstream is meant to read it.

import "strings"

// keptCommentPrefixes are the comment openings minification preserves.
//
// Prefixes rather than exact strings, because a directive carries arguments:
// `<!--#include virtual="/header.html" -->` is one comment, not a family of
// them. The list covers the directives that exist today; a host that invents
// another is covered by `minify_html_keep_comments` without waiting for a
// release.
var keptCommentPrefixes = []string{
	"<!--[if",          // downlevel-revealed conditional comment
	"<!--email_off",    // Cloudflare Email Address Obfuscation, opt-out region
	"<!--/email_off",   //   …and its close
	"<!--#",            // SSI: #include, #echo, #set
	"<!--esi",          // ESI, where a CDN reads it as a comment
	"<!--/esi",         //   …and its close
	"<!--googleoff",    // Google Search Appliance / programmable search
	"<!--googleon",     //
	"<!--noindex",      // Yandex noindex region
	"<!--/noindex",     //
	"<!--htmlmin:keep", // an explicit "leave this alone" a theme can always use
}

// keepsHTMLComment reports whether a comment is a directive rather than a note.
//
// Matching ignores the space a formatter may leave after the opener, so
// `<!-- email_off -->` is the same directive as `<!--email_off-->`; a theme
// author should not have to know which one the minifier recognises.
func keepsHTMLComment(comment string, extra []string) bool {
	normalized := "<!--" + strings.TrimLeft(strings.TrimPrefix(comment, "<!--"), " \t")
	for _, prefix := range keptCommentPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	for _, prefix := range extra {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		// A site names the directive ("email_off") or the whole opening
		// ("<!--email_off"); both mean the same thing and both should work.
		if !strings.HasPrefix(prefix, "<!--") {
			prefix = "<!--" + prefix
		}
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}
