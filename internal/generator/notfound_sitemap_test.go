package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestNotFoundPageNeverInSitemap covers #333: the slug-404 page is the error
// page, not content, so it stays out of sitemap.xml without any robots: key.
func TestNotFoundPageNeverInSitemap(t *testing.T) {
	notFound := models.Page{Title: "Page not found", Slug: "404", Type: "page", Status: "publish"}
	if !excludeFromSitemap(notFound) {
		t.Error("the 404 page must be excluded from the sitemap")
	}
	if excludeFromSitemap(models.Page{Title: "Error codes", Slug: "404", Type: "page", LangPrefix: "pl"}) {
		t.Error("only the root error page is special; /pl/404/ is an ordinary page")
	}
}

// TestNotFoundPageSEOBlock: the injected SEO block marks the error page
// noindex and gives it no og:url or hreflang, unless the theme set robots.
func TestNotFoundPageSEOBlock(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SEO = true
	page := models.Page{Title: "Page not found", Slug: "404", Type: "page"}
	out := g.seoHTMLString("<html><head></head><body></body></html>", page, false)
	if !strings.Contains(out, `<meta name="robots" content="noindex">`) {
		t.Errorf("404 page must be noindex: %s", out)
	}
	for _, banned := range []string{"og:url", "hreflang", "BreadcrumbList"} {
		if strings.Contains(out, banned) {
			t.Errorf("404 page must not carry %s: %s", banned, out)
		}
	}
	themed := g.seoHTMLString(`<html><head><meta name="robots" content="noindex, nofollow"></head></html>`, page, false)
	if strings.Count(themed, `name="robots"`) != 1 {
		t.Errorf("the theme's robots tag must not be duplicated: %s", themed)
	}
}
