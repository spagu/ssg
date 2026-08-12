package generator

// Site-level marketing metadata reaching the page (1.8.30): a migration
// discovers the source site's icons, social identity, verification tokens and
// tracking ids; losing them is how a migrated site goes dark in search console
// and analytics.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func marketingGen(t *testing.T, m models.Marketing, analytics map[string]string, on bool) *Generator {
	t.Helper()
	g := &Generator{siteData: &models.SiteData{Marketing: m, Analytics: analytics}}
	g.config.SEO = true
	g.config.Analytics = on
	return g
}

var sampleMarketing = models.Marketing{
	Verification:   map[string]string{"google-site-verification": "abc123"},
	SocialProfiles: map[string]string{"facebook": "https://facebook.com/example"},
	OGSiteName:     "Example Site",
	OGImage:        "https://example.com/social.jpg",
	TwitterSite:    "@example",
	Favicon:        "https://example.com/favicon-192x192.png",
	AppleTouchIcon: "https://example.com/apple-touch-icon.png",
	ThemeColor:     "#0f172a",
}

func TestBuildMarketingHead(t *testing.T) {
	g := marketingGen(t, sampleMarketing, nil, false)
	head := g.buildMarketingHead("<html><head></head>")
	for _, want := range []string{
		`<link rel="icon" href="https://example.com/favicon-192x192.png">`,
		`<link rel="apple-touch-icon"`,
		`<meta name="theme-color" content="#0f172a">`,
		`<meta property="og:site_name" content="Example Site">`,
		`<meta property="og:image" content="https://example.com/social.jpg">`,
		`<meta name="twitter:site" content="@example">`,
		`<meta name="google-site-verification" content="abc123">`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("missing %s in:\n%s", want, head)
		}
	}
}

// TestBuildMarketingHeadDefersToTheme: a theme that already declares an icon or
// og:image owns it — the site block must not emit a second, conflicting tag.
func TestBuildMarketingHeadDefersToTheme(t *testing.T) {
	g := marketingGen(t, sampleMarketing, nil, false)
	existing := `<link rel="icon" href="/theme.ico"><meta property="og:image" content="/theme.png">`
	head := g.buildMarketingHead(existing)
	if strings.Contains(head, `rel="icon"`) || strings.Contains(head, "og:image") {
		t.Fatalf("theme's own tags must win:\n%s", head)
	}
	if !strings.Contains(head, "og:site_name") {
		t.Fatal("untaken tags must still be emitted")
	}
}

func TestBuildMarketingHeadEmpty(t *testing.T) {
	if got := marketingGen(t, models.Marketing{}, nil, false).buildMarketingHead(""); got != "" {
		t.Fatalf("nothing discovered → no markup, got %q", got)
	}
}

// TestAnalyticsSnippetNeedsConsent: tracking scripts are NOT a side effect of
// migrating content — they render only when the owner sets analytics: true.
func TestAnalyticsSnippetNeedsConsent(t *testing.T) {
	ids := map[string]string{"gtm": "GTM-ABC123", "ga4": "G-XYZ789"}

	if got := marketingGen(t, models.Marketing{}, ids, false).analyticsSnippet(""); got != "" {
		t.Fatalf("default must render nothing, got %q", got)
	}

	got := marketingGen(t, models.Marketing{}, ids, true).analyticsSnippet("")
	if !strings.Contains(got, "GTM-ABC123") || !strings.Contains(got, "googletagmanager.com/gtm.js") {
		t.Errorf("GTM container missing:\n%s", got)
	}
	if !strings.Contains(got, "gtag/js?id=G-XYZ789") || !strings.Contains(got, "gtag('config','G-XYZ789')") {
		t.Errorf("GA4 tag missing:\n%s", got)
	}
}

// TestAnalyticsSnippetSkipsWired: a theme that already carries the id keeps
// ownership — a second container would double-count every visit.
func TestAnalyticsSnippetSkipsWired(t *testing.T) {
	ids := map[string]string{"gtm": "GTM-ABC123"}
	g := marketingGen(t, models.Marketing{}, ids, true)
	if got := g.analyticsSnippet(`<script>...'GTM-ABC123'...</script>`); got != "" {
		t.Fatalf("already-wired id must not be injected again: %q", got)
	}
	// An unknown vendor is left for the theme rather than guessed at.
	g2 := marketingGen(t, models.Marketing{}, map[string]string{"hotjar": "12345"}, true)
	if got := g2.analyticsSnippet(""); got != "" {
		t.Fatalf("unknown vendor must not be invented: %q", got)
	}
	// A blank id is skipped.
	g3 := marketingGen(t, models.Marketing{}, map[string]string{"gtm": "  "}, true)
	if got := g3.analyticsSnippet(""); got != "" {
		t.Fatalf("blank id must be skipped: %q", got)
	}
}

// TestJSStringEscapes: ids come from a crawled page, so they are data — a
// crafted one must not be able to close the script element.
func TestJSStringEscapes(t *testing.T) {
	got := jsString(`a'b"c\d<script>&`)
	if strings.Contains(got, "<script>") {
		t.Fatalf("tag survived escaping: %q", got)
	}
	// Every quote must be preceded by a backslash — an unescaped one would
	// terminate the literal the id sits in.
	if strings.Count(got, `'`) != strings.Count(got, `\'`) ||
		strings.Count(got, `"`) != strings.Count(got, `\"`) {
		t.Fatalf("bare quote left in %q", got)
	}
	if !strings.Contains(got, `\u003c`) || !strings.Contains(got, `\'`) {
		t.Fatalf("escapes missing: %q", got)
	}
}

func TestMarketingSummary(t *testing.T) {
	s := marketingSummary(sampleMarketing, map[string]string{"gtm": "GTM-1"}, false)
	for _, want := range []string{"icons", "social defaults", "1 social profile(s)",
		"1 verification token(s)", "analytics: true"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q: %s", want, s)
		}
	}
	if s := marketingSummary(sampleMarketing, map[string]string{"gtm": "GTM-1"}, true); !strings.Contains(s, "rendered") {
		t.Errorf("enabled state must say so: %s", s)
	}
	if s := marketingSummary(models.Marketing{}, nil, false); s != "" {
		t.Errorf("nothing found → no line, got %q", s)
	}
}
