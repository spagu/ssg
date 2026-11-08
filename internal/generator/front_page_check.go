package generator

// A front page that hides itself (#394).
//
// Since #129, a page at `/` replaces the theme's home page. WordPress exports
// carry the old static front page as `link: "https://example.com/"`, often
// still marked `robots: noindex` because the site stopped using it long ago.
// Upgrading such a site turned its live home page into that leftover, served
// noindex with another title and description. The build said nothing.
//
// A home page that tells crawlers to skip it is almost never what anyone wants.
// So the build stops and names the file. Removing the leftover's `link:` keeps
// the theme's home page. Removing the noindex keeps the page as the front page.
// Two cases pass. `sitemap: "no"` on a front page was the documented
// workaround for a duplicate root entry, and `/` stays in the sitemap anyway.
// A redirect at the root is a real pattern (`/` → `/en/`).

import (
	"fmt"
	"strings"
)

// checkHiddenFrontPage refuses a page that would become a language's front
// page while saying robots: noindex.
func (g *Generator) checkHiddenFrontPage() error {
	for _, p := range g.siteData.Pages {
		if p.Layout == "redirect" || !g.isLanguageRootPage(p) {
			continue
		}
		if strings.Contains(strings.ToLower(p.Robots), "noindex") {
			return fmt.Errorf("%s would replace the home page at %s, and it says robots: %q, "+
				"so the site's home page would ship noindex; it looks like a migration leftover: "+
				"remove its link: to keep the theme's home page, or remove the noindex to make it the front page (#394)",
				frontPageLabel(p), p.GetURL(), p.Robots)
		}
	}
	return nil
}
