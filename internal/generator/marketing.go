package generator

// Site-level marketing metadata → <head>. A migrated site arrives with the
// identity its old theme carried (icons, social defaults, verification tokens)
// and the tracking ids it ran (GTM, GA4, Pixel, …), discovered by the
// exporter's crawl and stored in metadata.json. Without this the operator has
// to retype all of it into a new theme by hand — the very step that gets
// forgotten until search console and analytics have been dark for a month.
//
// Split in two on purpose:
//   - identity/icons ride with `seo:` — they are metadata, they load nothing,
//     they cannot track anyone;
//   - tracking snippets need `analytics: true`, an explicit, separate consent
//     to run third-party JavaScript on every page. Never on by default: that
//     decision belongs to the site owner, not to a migration tool.

import (
	"fmt"
	stdhtml "html"

	"strings"

	"github.com/spagu/ssg/internal/models"
)

// buildMarketingHead renders the icons, social identity and verification tags
// the site declared, skipping anything the theme already emits.
func (g *Generator) buildMarketingHead(existing string) string {
	m := g.siteData.Marketing
	if m.Empty() {
		return ""
	}
	var b strings.Builder
	addLink := func(rel, href string, marker string) {
		if href == "" || strings.Contains(existing, marker) {
			return
		}
		fmt.Fprintf(&b, `<link rel="%s" href="%s">`+"\n", rel, stdhtml.EscapeString(href))
	}
	addMeta := func(attr, name, content, marker string) {
		if content == "" || strings.Contains(existing, marker) {
			return
		}
		fmt.Fprintf(&b, `<meta %s="%s" content="%s">`+"\n",
			attr, name, stdhtml.EscapeString(content))
	}

	addLink("icon", m.Favicon, `rel="icon"`)
	addLink("apple-touch-icon", m.AppleTouchIcon, `rel="apple-touch-icon"`)
	addMeta("name", "theme-color", m.ThemeColor, `name="theme-color"`)
	addMeta("property", "og:site_name", m.OGSiteName, "og:site_name")
	// og:image is per-page when the page has a featured image; this is the
	// site-wide fallback for pages that have none.
	addMeta("property", "og:image", m.OGImage, "og:image")
	addMeta("name", "twitter:site", m.TwitterSite, "twitter:site")

	// Verification tokens keep search-console and business-manager ownership
	// after the move — losing them costs a re-verification round trip.
	for _, name := range sortedKeys(m.Verification) {
		addMeta("name", name, m.Verification[name], `name="`+name+`"`)
	}
	return b.String()
}

// analyticsSnippet renders the tracking tags for the ids the crawl found.
// Only vendors with a well-known, stable embed are emitted; anything else is
// left in .Site.Analytics for a theme to place deliberately, because guessing
// a vendor's snippet wrong is worse than not emitting it.
func (g *Generator) analyticsSnippet(existing string) string {
	ids := g.siteData.Analytics
	if !g.config.Analytics || len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	for _, vendor := range sortedKeys(ids) {
		id := strings.TrimSpace(ids[vendor])
		if id == "" || strings.Contains(existing, id) { // already wired by the theme
			continue
		}
		switch strings.ToLower(vendor) {
		case "gtm", "google_tag_manager", "googletagmanager":
			fmt.Fprintf(&b, `<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':`+
				`new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],`+
				`j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;`+
				`j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);`+
				`})(window,document,'script','dataLayer','%s');</script>`+"\n", jsString(id))
		case "ga4", "ga", "google_analytics", "gtag":
			fmt.Fprintf(&b, `<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>`+"\n"+
				`<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}`+
				`gtag('js',new Date());gtag('config','%s');</script>`+"\n",
				stdhtml.EscapeString(id), jsString(id))
		}
	}
	return b.String()
}

// jsString escapes an id for a single-quoted JavaScript literal. Tracking ids
// are plain tokens, but they come from a crawled page — treat them as data.
func jsString(s string) string {
	// < > & become unicode escapes so a crafted id can never close the script
	// element or start a new tag.
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `"`, `\"`, "\n", "", "\r", "",
		"<", `\u003c`, ">", `\u003e`, "&", `\u0026`)
	return r.Replace(s)
}

// marketingSummary describes what a build inherited from the source site, so
// the operator sees it once instead of discovering it in the page source.
func marketingSummary(m models.Marketing, analytics map[string]string, analyticsOn bool) string {
	var parts []string
	if m.Favicon != "" || m.AppleTouchIcon != "" {
		parts = append(parts, "icons")
	}
	if m.OGImage != "" || m.OGSiteName != "" || m.TwitterSite != "" {
		parts = append(parts, "social defaults")
	}
	if len(m.SocialProfiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d social profile(s)", len(m.SocialProfiles)))
	}
	if len(m.Verification) > 0 {
		parts = append(parts, fmt.Sprintf("%d verification token(s)", len(m.Verification)))
	}
	if len(analytics) > 0 {
		state := "set `analytics: true` to render"
		if analyticsOn {
			state = "rendered"
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", strings.Join(sortedKeys(analytics), "+"), state))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}
