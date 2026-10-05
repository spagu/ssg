package generator

// The post listing in every language (#321). /blog/, /en/blog/ and
// /pl/blog/ are translations of one another, and the sitemap has said so
// since #281; the listing's own template context did not, so a theme writing
// its own head rebuilt hreflang by hand from .Site.Languages and the prefix
// rules. Built here once, the same way, for every listing page.

import (
	"path"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// listingTranslations lists each language's post listing — its first page —
// with the current language marked; nil without i18n.
func (g *Generator) listingTranslations() []Translation {
	if !g.config.I18n.Enabled {
		return nil
	}
	var out []Translation
	for _, lang := range g.siteData.Languages {
		prefix, ok := g.listingTarget(lang.Code)
		if !ok {
			continue // this language's root is a page and it has no posts_page
		}
		link := "/"
		if p := strings.Trim(prefix, "/"); p != "" {
			link = "/" + path.Clean(p) + "/"
		}
		out = append(out, Translation{
			Lang:      lang.Code,
			Locale:    lang.Locale,
			Title:     lang.Name,
			URL:       link,
			Canonical: g.servedCanonical(models.Page{Link: link, Type: "page"}),
			IsCurrent: lang.Code == g.currentLang,
			IsDefault: lang.Code == g.config.DefaultLanguage,
		})
	}
	return out
}
