package generator

// Tracking ids a site declares for itself, and the half of GTM that was
// missing (FE-001).

import (
	"strings"
	"testing"
)

// gtmSite builds a one-page site with the given analytics configuration.
func gtmSite(t *testing.T, apply func(cfg *Config)) string {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, func(name string) string {
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	if apply != nil {
		apply(&cfg)
	}
	buildSiteFixture(t, cfg)
	return mustRead(t, cfg.OutputDir+"/about/index.html")
}

// TestDeclaredGTMRendersBothHalves: a tag manager is a script in the head and
// an iframe after <body>, and only the first was ever emitted.
func TestDeclaredGTMRendersBothHalves(t *testing.T) {
	got := gtmSite(t, func(cfg *Config) {
		cfg.AnalyticsIDs = map[string]string{"gtm": "GTM-ABC1234"}
	})
	if !strings.Contains(got, "googletagmanager.com/gtm.js") {
		t.Errorf("the head half is missing:\n%s", got)
	}
	if !strings.Contains(got, `<noscript><iframe src="https://www.googletagmanager.com/ns.html?id=GTM-ABC1234"`) {
		t.Errorf("the body half is missing:\n%s", got)
	}
	// The iframe belongs immediately after <body>, where the vendor puts it.
	body := strings.Index(got, "<body")
	ns := strings.Index(got, "ns.html")
	if body < 0 || ns < body || ns > body+200 {
		t.Errorf("the noscript is not right after <body> (body=%d noscript=%d)", body, ns)
	}
}

// TestDeclaredIDsAreTheirOwnConsent: an id in the config is the owner asking
// for it, so it does not also need `analytics: true` — while an id a migration
// found still does, because nobody chose that one.
func TestDeclaredIDsAreTheirOwnConsent(t *testing.T) {
	declared := gtmSite(t, func(cfg *Config) {
		cfg.AnalyticsIDs = map[string]string{"gtm": "GTM-DECLARED"}
		cfg.Analytics = false
	})
	if !strings.Contains(declared, "GTM-DECLARED") {
		t.Errorf("a declared id should render on its own:\n%s", declared)
	}

	// Nothing declared, nothing found: nothing emitted.
	silent := gtmSite(t, nil)
	if strings.Contains(silent, "googletagmanager") {
		t.Errorf("a site that asked for nothing got a tag manager:\n%s", silent)
	}
	// An empty id is not an id.
	empty := gtmSite(t, func(cfg *Config) {
		cfg.AnalyticsIDs = map[string]string{"gtm": "   "}
	})
	if strings.Contains(empty, "googletagmanager") {
		t.Errorf("an empty id must render nothing:\n%s", empty)
	}
}

// TestConfigIDsWinOverAMigration: the config is the owner speaking, and
// metadata.json is a crawl reporting.
func TestConfigIDsWinOverAMigration(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[],"analytics":{"gtm":"GTM-FROMCRAWL"}}`,
		map[string]string{"pages/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n"},
		func(string) string { return `<html><head><title>x</title></head><body><p>x</p></body></html>` })
	cfg.Analytics = true
	cfg.AnalyticsIDs = map[string]string{"gtm": "GTM-FROMCONFIG"}
	buildSiteFixture(t, cfg)
	got := mustRead(t, cfg.OutputDir+"/a/index.html")
	if !strings.Contains(got, "GTM-FROMCONFIG") || strings.Contains(got, "GTM-FROMCRAWL") {
		t.Errorf("the config should win:\n%s", got)
	}
}

// TestAnalyticsNoscriptPlacement: the injector is careful about documents that
// are not shaped the way it hopes.
func TestAnalyticsNoscriptPlacement(t *testing.T) {
	snippet := `<noscript><iframe src="https://www.googletagmanager.com/ns.html?id=X"></iframe></noscript>`
	got := injectAnalyticsNoscript(`<html><body class="a b"><p>hi</p></body></html>`, snippet)
	if !strings.Contains(got, `<body class="a b">`+"\n"+snippet) {
		t.Errorf("placement = %q", got)
	}
	// Nothing to inject, no change; no body, no change; already there, no
	// second copy.
	page := "<html><body>x</body></html>"
	if injectAnalyticsNoscript(page, "") != page {
		t.Error("an empty snippet must change nothing")
	}
	if got := injectAnalyticsNoscript("<p>fragment</p>", snippet); got != "<p>fragment</p>" {
		t.Errorf("a document with no body: %q", got)
	}
	twice := injectAnalyticsNoscript(got, snippet)
	if strings.Count(twice, "ns.html") > 1 {
		t.Error("the snippet was injected twice")
	}
	if got := injectAnalyticsNoscript("<body", snippet); got != "<body" {
		t.Errorf("an unclosed body tag: %q", got)
	}
}

// TestAnalyticsIDsAreEscaped: the id reaches both a script literal and an
// attribute, and it comes from a config a migration may have written.
func TestAnalyticsIDsAreEscaped(t *testing.T) {
	got := gtmSite(t, func(cfg *Config) {
		cfg.AnalyticsIDs = map[string]string{"gtm": `X"><script>alert(1)</script>`}
	})
	if strings.Contains(got, "<script>alert(1)") {
		t.Errorf("an id broke out of its context:\n%s", got)
	}
}

// TestGA4StillRenders through the same path, so the new consent rule did not
// narrow what was already supported.
func TestGA4StillRenders(t *testing.T) {
	got := gtmSite(t, func(cfg *Config) {
		cfg.AnalyticsIDs = map[string]string{"ga4": "G-ABC1234"}
	})
	if !strings.Contains(got, "gtag/js?id=G-ABC1234") {
		t.Errorf("GA4 missing:\n%s", got)
	}
	if strings.Contains(got, "ns.html") {
		t.Error("GA4 has no noscript half and must not get one")
	}
}

// TestTrackingReachesEveryPage: the home page and the archives are what an
// analytics report is mostly about, and they used to get nothing.
func TestTrackingReachesEveryPage(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ntags: [go]\n---\n\nOne.\n",
		"pages/about.md":    "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, func(string) string { return `<html><head><title>x</title></head><body><p>x</p></body></html>` })
	cfg.AnalyticsIDs = map[string]string{"gtm": "GTM-EVERYWHERE"}
	cfg.SEO = false // deliberately: tracking is not part of the SEO decision
	buildSiteFixture(t, cfg)

	for _, page := range []string{"/index.html", "/about/index.html", "/2024/01/02/one/index.html", "/tag/go/index.html"} {
		got := mustRead(t, cfg.OutputDir+page)
		if !strings.Contains(got, "GTM-EVERYWHERE") {
			t.Errorf("%s carries no tracking", page)
		}
		if !strings.Contains(got, "ns.html?id=GTM-EVERYWHERE") {
			t.Errorf("%s carries no noscript half", page)
		}
	}
}

// TestThemeSuppliedTrackingIsNotDuplicated: a theme that already wired the id
// keeps its own snippet.
func TestThemeSuppliedTrackingIsNotDuplicated(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, func(string) string {
		return `<html><head><title>x</title><script>gtm('GTM-THEME')</script></head><body><p>x</p></body></html>`
	})
	cfg.AnalyticsIDs = map[string]string{"gtm": "GTM-THEME"}
	buildSiteFixture(t, cfg)
	got := mustRead(t, cfg.OutputDir+"/about/index.html")
	if strings.Count(got, "googletagmanager.com/gtm.js") > 0 {
		t.Errorf("the theme already wired it; the generator should not add a second:\n%s", got)
	}
}
