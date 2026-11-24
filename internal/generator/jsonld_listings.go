package generator

// The structured data of generated listings (1.8.72): every archive and the
// post listing is an ItemList of what it shows; an author archive is that
// author's ProfilePage; the page at the site root — the post listing, or the
// front page that moved it to posts_page (#442) — also names the Organization
// behind the site, from the marketing settings.

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// listingStructuredData is the blocks for one page of a listing.
func (g *Generator) listingStructuredData(kind, name, canonical string, posts []models.Page) []map[string]interface{} {
	var out []map[string]interface{}
	if kind == "author" && name != "" {
		out = append(out, profilePageLD(canonical, personLD(name, canonical, "", "", nil)))
	}
	if list := itemListLD(g.canonicalURLs(posts)); list != nil {
		out = append(out, list)
	}
	return out
}

// canonicalURLs is each post's canonical URL, in listing order.
func (g *Generator) canonicalURLs(posts []models.Page) []string {
	urls := make([]string, 0, len(posts))
	for _, p := range posts {
		urls = append(urls, g.servedCanonical(p))
	}
	return urls
}

// organizationLD describes who publishes the site, from marketing: the site
// name, its logo and its social profiles. nil when there is nothing beyond
// a domain to say — a bare Organization is no information.
func (g *Generator) organizationLD() map[string]interface{} {
	m := g.siteData.Marketing
	if m.Logo == "" && len(m.SocialProfiles) == 0 && m.OGSiteName == "" {
		return nil
	}
	name := firstNonEmpty(m.OGSiteName, g.siteData.Title, g.config.Domain)
	org := map[string]interface{}{"@context": "https://schema.org", "@type": "Organization", "name": name}
	setNonEmpty(org, "url", g.servedCanonical(models.Page{Link: "/", Type: "page"}))
	if logo := g.absoluteURL(m.Logo); logo != "" {
		org["logo"] = logo
	}
	keys := make([]string, 0, len(m.SocialProfiles))
	for k := range m.SocialProfiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sameAs []interface{}
	for _, k := range keys {
		if u := strings.TrimSpace(m.SocialProfiles[k]); u != "" {
			sameAs = append(sameAs, u)
		}
	}
	if len(sameAs) > 0 {
		org["sameAs"] = sameAs
	}
	return org
}

// absoluteURL makes a site path absolute on the site's domain; an absolute
// URL is kept, an empty one stays empty. Like canonicals and the sitemap it
// uses domain with its own path and not base_path, which describes where the
// files are served when the canonical host is elsewhere (docs/DEPLOYMENT.md).
func (g *Generator) absoluteURL(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return ""
	case strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://"):
		return p
	case g.config.Domain == "":
		return ""
	}
	return httpsScheme + strings.TrimSuffix(g.config.Domain, "/") + "/" + strings.TrimPrefix(p, "/")
}

// indexStructuredData is the post listing's blocks: its ItemList, and on the
// site root's first page the Organization.
func (g *Generator) indexStructuredData(outPath string, posts []models.Page) []map[string]interface{} {
	ctx := g.indexPageContext(outPath)
	out := g.listingStructuredData("", "", g.servedCanonical(*ctx), posts)
	if ctx.Link == "/" {
		if org := g.organizationLD(); org != nil {
			out = append([]map[string]interface{}{org}, out...)
		}
	}
	return out
}

// frontPageStructuredData is the Organization for a page document served at
// the site root (#442). The Organization describes the site, so it sits on
// whatever is at "/": with a front page there the post listing is elsewhere
// (posts_page) or not written, and indexStructuredData never reaches "/".
func (g *Generator) frontPageStructuredData(page models.Page) []map[string]interface{} {
	if strings.Trim(page.GetURL(), "/") != "" {
		return nil
	}
	if org := g.organizationLD(); org != nil {
		return []map[string]interface{}{org}
	}
	return nil
}

// listingTitle names the post listing at a site link. A listing that
// posts_page moved off the root is named after the page at that address (the
// one the listing replaces, #150), else after the address itself; the root
// listing keeps the domain, the only name it ever had.
func (g *Generator) listingTitle(link string) string {
	listing := strings.Trim(strings.TrimSpace(g.config.PostsPage), "/")
	at := "/" + strings.Trim(link, "/") + "/"
	i := strings.Index(at, "/"+listing+"/")
	if listing == "" || i < 0 {
		return g.config.Domain
	}
	// The listing's own address: a language prefix before it is kept, a
	// pager's /page/N/ after it is not.
	address := at[:i+len(listing)+2]
	return firstNonEmpty(g.crumbTitle(address), titleize(listing[strings.LastIndex(listing, "/")+1:]))
}
