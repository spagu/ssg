package generator

// One sitemap URL, as data rather than as bytes appended to a buffer.
//
// The sitemap used to be assembled by nine functions each writing directly into
// one strings.Builder, which fixed two things that are now choices: there was
// exactly one file, and nothing could look at an entry after it was written.
// Splitting a large sitemap and routing entries into declared sub-sitemaps both
// need the set in hand before any of it is rendered.
//
// The field order below is the order the elements are emitted in, and the
// renderer reproduces byte for byte what the nine writers produced — the golden
// corpora are the check on that.

import (
	"fmt"
	"strings"
	"time"
)

// sitemapKind labels what a URL is, so a declared sitemap can select by it.
type sitemapKind string

const (
	kindHome     sitemapKind = "home"
	kindListing  sitemapKind = "listing"
	kindStatic   sitemapKind = "static"
	kindPage     sitemapKind = "pages"
	kindPost     sitemapKind = "posts"
	kindCategory sitemapKind = "categories"
	kindTag      sitemapKind = "tags"
	kindAuthor   sitemapKind = "authors"
	kindTaxonomy sitemapKind = "taxonomies"
)

// sitemapKinds is every selectable name, for validating a declared `include:`
// against something better than silence.
var sitemapKinds = []sitemapKind{
	kindHome, kindListing, kindStatic, kindPage, kindPost,
	kindCategory, kindTag, kindAuthor, kindTaxonomy,
}

// sitemapEntry is one <url> element.
type sitemapEntry struct {
	loc string
	// alternates is the pre-rendered xhtml:link block for a translated page.
	// Rendered at collection time because it is the page that knows its
	// translations, and an entry is deliberately not able to reach back.
	alternates string
	lastmod    time.Time
	changefreq string
	priority   string
	kind       sitemapKind
	// source is the content root a page or post came from, which is what
	// `source: blog` in a declared sitemap selects on. Empty for everything
	// that is not a content document.
	source string
}

// archiveEntry is the shape every archive listing shares.
func archiveEntry(loc string, kind sitemapKind) sitemapEntry {
	return sitemapEntry{loc: loc, changefreq: "weekly", priority: "0.5", kind: kind}
}

// writeTo renders one <url> block.
func (e sitemapEntry) writeTo(sb *strings.Builder) {
	sb.WriteString(sitemapURLOpen)
	fmt.Fprintf(sb, "    <loc>%s</loc>\n", e.loc)
	sb.WriteString(e.alternates)
	if !e.lastmod.IsZero() {
		fmt.Fprintf(sb, "    <lastmod>%s</lastmod>\n", e.lastmod.Format(sitemapDateLayout))
	}
	fmt.Fprintf(sb, "    <changefreq>%s</changefreq>\n", e.changefreq)
	fmt.Fprintf(sb, "    <priority>%s</priority>\n", e.priority)
	sb.WriteString(sitemapURLClose)
}

// sitemapDateLayout is the W3C date form sitemaps.org asks for.
const sitemapDateLayout = "2006-01-02"

// renderURLSet renders one sitemap file.
func (g *Generator) renderURLSet(entries []sitemapEntry) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`)
	if g.config.I18n.Enabled {
		sb.WriteString(` xmlns:xhtml="http://www.w3.org/1999/xhtml"`)
	}
	sb.WriteString(`>`)
	sb.WriteString("\n")
	for _, e := range entries {
		e.writeTo(&sb)
	}
	sb.WriteString("</urlset>\n")
	return []byte(sb.String())
}

// renderSitemapIndex renders the <sitemapindex> that names the files below it.
//
// The index keeps the name sitemap.xml, so robots.txt, Search Console and every
// crawler that already knows the site need no change: an index served where a
// urlset used to be is exactly what the protocol expects.
func (g *Generator) renderSitemapIndex(files []sitemapFile, built time.Time) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sb.WriteString("\n")
	for _, f := range files {
		sb.WriteString("  <sitemap>\n")
		fmt.Fprintf(&sb, "    <loc>%s%s/%s</loc>\n", httpsScheme, g.config.Domain, f.path)
		fmt.Fprintf(&sb, "    <lastmod>%s</lastmod>\n", f.lastmod(built).Format(sitemapDateLayout))
		sb.WriteString("  </sitemap>\n")
	}
	sb.WriteString("</sitemapindex>\n")
	return []byte(sb.String())
}
