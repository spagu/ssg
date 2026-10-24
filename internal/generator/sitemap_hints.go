package generator

// A page's own sitemap hints (#356). Every document used to say
// <changefreq>monthly</changefreq> and its type's priority; a page recounted
// on every build said the same as an about page that never changes. Front
// matter (sitemap_changefreq, sitemap_priority) and content_map set them per
// page. Google ignores both; Bing and other crawlers still read them.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// sitemapChangefreqs are the values the sitemap protocol allows.
var sitemapChangefreqs = map[string]bool{
	"always": true, "hourly": true, "daily": true, "weekly": true,
	"monthly": true, "yearly": true, "never": true,
}

// pageChangefreq is the page's own change frequency, or def. A value the
// protocol does not allow is reported and the default kept: a sitemap with
// an invalid value is rejected whole by a strict reader.
func (g *Generator) pageChangefreq(page models.Page, def string) string {
	v := strings.ToLower(strings.TrimSpace(page.SitemapChangefreq))
	switch {
	case v == "":
		return def
	case sitemapChangefreqs[v]:
		return v
	}
	g.warnSitemapHint(page, "sitemap_changefreq", page.SitemapChangefreq,
		"always, hourly, daily, weekly, monthly, yearly or never")
	return def
}

// pagePriority is the page's own priority, 0.0 to 1.0, or def.
func (g *Generator) pagePriority(page models.Page, def string) string {
	raw := strings.TrimSpace(page.SitemapPriority)
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 1 {
		g.warnSitemapHint(page, "sitemap_priority", page.SitemapPriority, "a number from 0.0 to 1.0")
		return def
	}
	out := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(out, ".") {
		out += ".0" // "1" → "1.0", as every other entry writes it
	}
	return out
}

// warnSitemapHint reports a hint the sitemap could not use.
func (g *Generator) warnSitemapHint(page models.Page, key, value, want string) {
	if g.config.Quiet {
		return
	}
	where := page.SourceFile
	if where == "" {
		where = page.GetURL()
	}
	fmt.Printf("   ⚠️  %s: %s %q is not %s; the default is used\n", where, key, value, want)
}
