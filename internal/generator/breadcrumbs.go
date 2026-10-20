package generator

// The derived JSON-LD BreadcrumbList (#61), and the rule #335 added to it: a
// step names a page the build actually writes, or it is left out.
//
// The trail used to be one step per URL segment, each linking to that
// segment's path and titled with the slug. A post at /posts/my-article/ on a
// site whose listing lives at /blog/ got a "Posts" step pointing at a 404 —
// on the site that reported it, 197 of 215 pages — and a Polish site got
// English labels and slugs with their diacritics stripped, all of which
// Google can show in search results.
//
//	breadcrumbs:
//	  home: "Strona główna"
//	  sections:
//	    posts:  {name: "Baza wiedzy", url: /baza-wiedzy/}
//	    uslugi: {name: "Usługi"}
//	# or breadcrumbs: false to emit none and let the theme supply its own

import (
	"path"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// BreadcrumbOptions is `breadcrumbs:` as the generator uses it.
type BreadcrumbOptions struct {
	Off      bool   // emit no BreadcrumbList at all
	Home     string // first step's label; default "Home"
	Sections map[string]BreadcrumbSection
}

// BreadcrumbSection names one URL segment's step and, optionally, where it
// points instead of the segment's own path.
type BreadcrumbSection struct {
	Name string
	URL  string
}

// defaultCrumbHome is the first step's label when none is configured.
const defaultCrumbHome = "Home"

// breadcrumbLD derives a BreadcrumbList from the page's URL path so agents can
// place the page in the site hierarchy. Home (and any page whose URL is "/")
// has no trail and returns nil, as does the error page; a bare domain also
// skips it (item URLs need a host). An intermediate step is kept only when a page exists at its URL or a
// section is configured for it, and positions are numbered after the gaps.
func (g *Generator) breadcrumbLD(page models.Page) map[string]interface{} {
	if g.config.Domain == "" || g.config.Breadcrumbs.Off || page.IsNotFound() {
		return nil
	}
	rel := strings.Trim(page.GetURL(), "/")
	if rel == "" {
		return nil
	}
	segs := strings.Split(rel, "/")
	home := "/"
	if page.LangPrefix != "" && segs[0] == page.LangPrefix {
		// Under i18n the trail starts at the language's own home, not at a
		// step named after the language code.
		home = "/" + page.LangPrefix + "/"
		segs = segs[1:]
	}
	items := []interface{}{crumb(1, g.crumbHome(), g.crumbURL(home))}
	acc := strings.TrimSuffix(home, "/")
	for i, s := range segs {
		acc += "/" + s
		if i == len(segs)-1 { // the page itself: its real title and canonical URL
			name := page.Title
			if name == "" {
				name = titleize(s)
			}
			items = append(items, crumb(len(items)+1, name, g.servedCanonical(page)))
			break
		}
		if name, target, ok := g.crumbStep(s, acc+"/"); ok {
			items = append(items, crumb(len(items)+1, name, g.crumbURL(target)))
		}
	}
	if len(items) < 2 {
		return nil
	}
	return map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	}
}

// crumbHome is the first step's label.
func (g *Generator) crumbHome() string {
	if h := strings.TrimSpace(g.config.Breadcrumbs.Home); h != "" {
		return h
	}
	return defaultCrumbHome
}

// crumbStep resolves one intermediate segment. A configured section wins: its
// name, and its URL when it gives one. Otherwise the step exists only if the
// build writes a document at urlPath, titled the way that document is.
func (g *Generator) crumbStep(seg, urlPath string) (name, target string, ok bool) {
	sec, configured := g.config.Breadcrumbs.Sections[seg]
	if configured && strings.TrimSpace(sec.URL) != "" {
		return firstNonEmpty(sec.Name, g.crumbTitle(sec.URL), titleize(seg)), sec.URL, true
	}
	title, exists := g.crumbIndex()[urlPath]
	if !exists {
		return "", "", false
	}
	if configured && sec.Name != "" {
		return sec.Name, urlPath, true
	}
	return firstNonEmpty(title, titleize(seg)), urlPath, true
}

// crumbTitle is the title of the document at a site path ("/blog/" or
// "blog"), if the build writes one.
func (g *Generator) crumbTitle(sitePath string) string {
	return g.crumbIndex()["/"+strings.Trim(sitePath, "/")+"/"]
}

// crumbURL makes an absolute item URL that follows pretty_urls like every
// other address the site publishes.
func (g *Generator) crumbURL(p string) string {
	return g.servedURL(g.absoluteURL(p))
}

// crumbIndex maps every URL the build writes a document at ("/blog/") to that
// document's title ("" for a listing without one). It is built once per build,
// on first use from the render pool, and dropped by resetCrumbIndex.
func (g *Generator) crumbIndex() map[string]string {
	g.crumbMu.Lock()
	defer g.crumbMu.Unlock()
	if g.crumbURLs == nil {
		g.crumbURLs = g.buildCrumbIndex()
	}
	return g.crumbURLs
}

// resetCrumbIndex forgets the previous build's addresses (watch mode).
func (g *Generator) resetCrumbIndex() {
	g.crumbMu.Lock()
	defer g.crumbMu.Unlock()
	g.crumbURLs = nil
}

// buildCrumbIndex enumerates the documents: pages and posts, the post
// listing(s), taxonomy archives, custom-type archives and date archives.
func (g *Generator) buildCrumbIndex() map[string]string {
	idx := map[string]string{}
	add := func(p, title string) {
		p = "/" + strings.Trim(p, "/") + "/"
		if _, seen := idx[p]; !seen || idx[p] == "" {
			idx[p] = title
		}
	}
	if g.siteData != nil {
		for _, list := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
			for _, p := range list {
				add(p.GetURL(), p.Title)
			}
		}
	}
	for prefix := range g.postsListings {
		if prefix != "" {
			add(prefix, "")
		}
	}
	for _, r := range g.taxonomyRoutes() {
		add(r.Path, r.Title)
	}
	if g.siteData != nil {
		for _, a := range g.collectTypeArchives() {
			add(a.Path, a.Name)
		}
		if g.config.DateArchives {
			for _, a := range collectDateArchives(g.siteData.Posts, g.dateArchiveDepth()) {
				add(path.Join("/", a.Path), a.Label)
			}
		}
	}
	return idx
}
