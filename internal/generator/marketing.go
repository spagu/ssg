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
	ids := g.analyticsIDs()
	if len(ids) == 0 {
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

// analyticsIDs is the set of tracking ids this build will emit.
//
// Two sources with different consent rules (FE-001). Ids the site declares in
// `analytics_ids:` are the owner asking for them, so they render on their own.
// Ids a migration's crawl recorded still need `analytics: true`, because
// nobody chose those — they are what the old site happened to be running.
func (g *Generator) analyticsIDs() map[string]string {
	if g.config.Analytics {
		return g.siteData.Analytics
	}
	if len(g.config.AnalyticsIDs) == 0 {
		return nil
	}
	declared := make(map[string]string, len(g.config.AnalyticsIDs))
	for vendor, id := range g.config.AnalyticsIDs {
		if strings.TrimSpace(id) != "" {
			declared[vendor] = id
		}
	}
	return declared
}

// injectAnalytics places both halves of the tracking snippets: the scripts in
// the head, and the tag manager's iframe right after <body>.
func (g *Generator) injectAnalytics(s string) string {
	if head := g.analyticsSnippet(s); head != "" {
		if i := strings.LastIndex(s, "</head>"); i >= 0 {
			s = s[:i] + head + s[i:]
		} else {
			s = head + s
		}
	}
	return injectAnalyticsNoscript(s, g.analyticsNoscript())
}

// analyticsNoscript is the half of Google Tag Manager that goes in the body.
//
// GTM is two tags: a script in the head and an iframe right after <body>. Only
// the first was ever emitted, so a visitor with JavaScript off — or a
// consent-mode setup that defers the script — was counted by neither. A tag
// manager that is half installed is not installed.
func (g *Generator) analyticsNoscript() string {
	var b strings.Builder
	ids := g.analyticsIDs()
	for _, vendor := range sortedKeys(ids) {
		id := strings.TrimSpace(ids[vendor])
		if id == "" {
			continue
		}
		switch strings.ToLower(vendor) {
		case "gtm", "google_tag_manager", "googletagmanager":
			fmt.Fprintf(&b, `<noscript><iframe src="https://www.googletagmanager.com/ns.html?id=%s"`+
				` height="0" width="0" style="display:none;visibility:hidden"></iframe></noscript>`+"\n",
				stdhtml.EscapeString(id))
		}
	}
	return b.String()
}

// injectAnalyticsNoscript places the body half immediately after <body>, where
// the vendor requires it.
func injectAnalyticsNoscript(s, snippet string) string {
	if snippet == "" || strings.Contains(s, "googletagmanager.com/ns.html") {
		return s
	}
	i := strings.Index(s, "<body")
	if i < 0 {
		return s
	}
	end := strings.Index(s[i:], ">")
	if end < 0 {
		return s
	}
	at := i + end + 1
	return s[:at] + "\n" + snippet + s[at:]
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
