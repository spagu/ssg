package generator

// <lastmod> for the pages that list other pages (#339).
//
// A post or a page carried its own lastmod; the hubs that list them — the
// front page, the post listing, category, tag, series and author archives,
// taxonomy indexes and term pages — carried none, so the URLs a crawler
// should revisit most said nothing about when they last changed. A hub now
// takes the newest lastmod of the posts it lists (modified, falling back to
// date, or the git date under lastmod_from_git). A hub that lists nothing
// dated keeps no lastmod, as before.

import (
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// hubKinds are the entries a lastmod is derived for.
var hubKinds = map[sitemapKind]bool{
	kindHome: true, kindListing: true, kindCategory: true, kindTag: true,
	kindSeries: true, kindAuthor: true, kindTaxonomy: true,
}

// dateHubs fills in the lastmod of every hub entry that has none.
func (g *Generator) dateHubs(entries []sitemapEntry) {
	hubs := g.hubLastmods()
	prefix := httpsScheme + g.config.Domain
	for i := range entries {
		e := &entries[i]
		if !hubKinds[e.kind] || !e.lastmod.IsZero() {
			continue
		}
		e.lastmod = hubs[hubKey(strings.TrimPrefix(e.loc, prefix))]
	}
}

// hubKey normalises a site path ("/tag/go/", "/tag/go", "tag/go") to the key
// hubLastmods uses ("tag/go"); the site root is "".
func hubKey(p string) string {
	return strings.Trim(strings.TrimSuffix(p, ".html"), "/")
}

// hubLastmods maps each hub's path to the newest lastmod of what it lists.
func (g *Generator) hubLastmods() map[string]time.Time {
	hubs := map[string]time.Time{}
	if g.siteData == nil {
		return hubs
	}
	put := func(path string, t time.Time) {
		k := hubKey(path)
		if t.After(hubs[k]) {
			hubs[k] = t
		}
	}
	// Front pages and post listings list the posts of their language.
	for _, home := range g.homepageLocs() {
		put(strings.TrimPrefix(home.href, httpsScheme+g.config.Domain), g.homeLastmod(home.lang))
	}
	for prefix, lang := range g.postsListings {
		put(prefix, g.newestLastmod(g.postsInLang(lang)))
	}
	// Every taxonomy in the registry — the built-in category, tag and series
	// folded in, and custom ones — by term, and the index by all its terms.
	if g.taxonomies != nil {
		for _, name := range g.taxonomies.Names {
			def := g.taxonomies.Definitions[name]
			for _, lang := range g.taxonomyLangs() {
				base := g.termBase(def, lang)
				for _, t := range g.taxonomies.Terms(name, lang) {
					newest := g.newestLastmod(g.taxonomies.Pages(name, lang, t.Key))
					put(base+t.Slug, newest)
					put(base, newest)
				}
			}
		}
	}
	// Category archives at the address they were actually written to, which
	// a category with its own link moves away from /category/ (#228).
	for catID, path := range g.categoryArchives {
		var posts []models.Page
		for _, p := range g.siteData.Posts {
			for _, id := range p.Categories {
				if id == catID {
					posts = append(posts, p)
					break
				}
			}
		}
		put(path, g.newestLastmod(posts))
	}
	return hubs
}

// taxonomyLastmod is the newest lastmod of any post with a term of the
// taxonomy, in one language.
func (g *Generator) taxonomyLastmod(name, lang string) time.Time {
	var newest time.Time
	for _, t := range g.taxonomies.Terms(name, lang) {
		if m := g.newestLastmod(g.taxonomies.Pages(name, lang, t.Key)); m.After(newest) {
			newest = m
		}
	}
	return newest
}

// homeLastmod is a front page's lastmod: the page's own when a page is the
// front page, otherwise the newest post of the language.
func (g *Generator) homeLastmod(lang string) time.Time {
	prefix := ""
	if g.config.I18n.Enabled {
		prefix = g.languagePrefix(lang)
	}
	if root := rootPage(g.siteData.Pages, lang, prefix); root != nil {
		return g.lastModFor(*root)
	}
	return g.newestLastmod(g.postsInLang(lang))
}

// postsInLang is the posts of one language ("" is every post).
func (g *Generator) postsInLang(lang string) []models.Page {
	if lang == "" {
		return g.siteData.Posts
	}
	var out []models.Page
	for _, p := range g.siteData.Posts {
		if p.Lang == lang {
			out = append(out, p)
		}
	}
	return out
}

// newestLastmod is the newest lastmod among pages; zero when none is dated.
func (g *Generator) newestLastmod(pages []models.Page) time.Time {
	var newest time.Time
	for _, p := range pages {
		if t := g.lastModFor(p); t.After(newest) {
			newest = t
		}
	}
	return newest
}
