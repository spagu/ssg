package generator

// Language alternates in the sitemap, for every URL that has them (#281).
//
// Pages and posts carried their xhtml:link block from the start, because each
// knows its translation group. The front page and the post listing did not: on
// a three-language site the six URLs a crawler weights most were the six with
// no alternates, while the front page's own HTML listed all three languages.
//
// The front page is claimed by a site-level entry rather than by the page
// record that resolved there, so that record's translations never reached it.
// The listing is generated, not authored, so it has no translation group to
// read — but /blog/, /pl/blog/ and /ro/blog/ are translations of one another
// all the same, and the build already knows which language wrote each.

import (
	"fmt"
	stdhtml "html"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// alternateLink is one language variant as a sitemap names it.
type alternateLink struct {
	lang string // "" without i18n
	href string // absolute
}

// groupAlternates renders the block for a group the build assembled itself —
// the front pages, the listings. Fewer than two variants is no group: a URL is
// not an alternate of itself.
func (g *Generator) groupAlternates(links []alternateLink) string {
	if !g.config.I18n.Enabled || len(links) < 2 {
		return ""
	}
	return alternatesBlock(links, g.config.DefaultLanguage)
}

// sitemapAlternates is a page's or post's alternates, from its translation group.
//
// A group of one still renders its single link, as it always has: that output
// is what the golden corpora pin, and a crawler reads it as harmless.
func (g *Generator) sitemapAlternates(page models.Page) string {
	if !g.config.I18n.Enabled {
		return ""
	}
	links := make([]alternateLink, 0, len(page.Translations))
	for _, tr := range page.Translations {
		links = append(links, alternateLink{lang: tr.Lang, href: tr.Canonical})
	}
	return alternatesBlock(links, g.config.DefaultLanguage)
}

// alternatesBlock renders xhtml:link elements, with x-default beside the
// default language's entry.
func alternatesBlock(links []alternateLink, defaultLang string) string {
	var sb strings.Builder
	for _, l := range links {
		href := stdhtml.EscapeString(l.href)
		fmt.Fprintf(&sb, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", stdhtml.EscapeString(l.lang), href)
		if l.lang == defaultLang {
			fmt.Fprintf(&sb, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n", href)
		}
	}
	return sb.String()
}

// homepageLocs is every address the front-page entry claims: one per language
// under i18n, the site root otherwise.
func (g *Generator) homepageLocs() []alternateLink {
	if !g.config.I18n.Enabled {
		return []alternateLink{{href: fmt.Sprintf("https://%s/", g.config.Domain)}}
	}
	locs := make([]alternateLink, 0, len(g.siteData.Languages))
	for _, lang := range g.siteData.Languages {
		locs = append(locs, alternateLink{
			lang: lang.Code,
			href: fmt.Sprintf("https://%s%s", g.config.Domain, g.languageURL(lang.Code)),
		})
	}
	return locs
}

// homeAlternates is one front page's alternates.
//
// A page that resolves to this address speaks for it, so the sitemap says what
// that page's HTML already says. Without one, the front pages the build
// generated per language are the group.
func (g *Generator) homeAlternates(home alternateLink, homes []alternateLink) string {
	if !g.config.I18n.Enabled {
		return ""
	}
	for _, page := range g.siteData.Pages {
		if len(page.Translations) > 1 && page.Lang == home.lang && g.servedCanonical(page) == home.href {
			return g.sitemapAlternates(page)
		}
	}
	return g.groupAlternates(homes)
}

// postsListingEntries names each language's post listing, when it was written
// somewhere other than the site root.
//
// Priority sits between the home page and an ordinary page: the listing is the
// entry point for the whole blog section, and on the site found by this bug it
// was the most internally-linked page after the home page. Only the first page
// is listed — a paginated tail is left out on purpose — and a listing whose own
// output marks itself noindex keeps itself out, the same rule every other
// document follows.
//
// The listings that are listed are each other's alternates (#281): built from
// the entries actually written, so every link in the block is reciprocated.
func (g *Generator) postsListingEntries(claimed map[string]bool) []sitemapEntry {
	prefixes := make([]string, 0, len(g.postsListings))
	for prefix := range g.postsListings {
		if prefix == "" {
			continue // the site root, already claimed by the front-page entry
		}
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	out := make([]sitemapEntry, 0, len(prefixes))
	var links []alternateLink
	var translated []int // indexes into out of the listings that have a language
	for _, prefix := range prefixes {
		loc := g.servedURL(httpsScheme + g.config.Domain + "/" + prefix + "/")
		if claimed[loc] || g.renderedExcludesItself(models.Page{}, prefix) {
			continue
		}
		claimed[loc] = true
		if lang := g.postsListings[prefix]; lang != "" {
			links = append(links, alternateLink{lang: lang, href: loc})
			translated = append(translated, len(out))
		}
		out = append(out, sitemapEntry{
			loc: loc, changefreq: "daily", priority: "0.9", kind: kindListing,
		})
	}
	block := g.groupAlternates(links)
	for _, i := range translated {
		out[i].alternates = block
	}
	return out
}
