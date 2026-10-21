package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestSocialURL covers #350: what an og:image may be written as.
func TestSocialURL(t *testing.T) {
	g := newTestGen(t, "")
	cases := map[string]string{
		"/card.png":                     "https://example.com/card.png",
		"media/card.png":                "https://example.com/media/card.png",
		"//cdn.example.org/c.png":       "https://cdn.example.org/c.png",
		"https://cdn.example.org/c.png": "https://cdn.example.org/c.png",
		"  /padded.png ":                "https://example.com/padded.png",
		"":                              "",
	}
	for in, want := range cases {
		if got := g.socialURL(in); got != want {
			t.Errorf("socialURL(%q) = %q, want %q", in, got, want)
		}
	}
	g.config.Domain = ""
	if got := g.socialURL("/card.png"); got != "/card.png" {
		t.Errorf("without a domain the value is kept, not lost: %q", got)
	}
}

// TestMarketingOGImageIsAbsolute: the site-wide fallback card too (#350).
func TestMarketingOGImageIsAbsolute(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Marketing = models.Marketing{OGImage: "/card.png"}
	if head := g.buildMarketingHead(""); !strings.Contains(head, `<meta property="og:image" content="https://example.com/card.png">`) {
		t.Errorf("head:\n%s", head)
	}
}

// TestThemeJSONLDIsNotDoubled covers #349: the social tags and the derived
// JSON-LD are each added only when the theme did not write its own.
func TestThemeJSONLDIsNotDoubled(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SEO = true
	page := models.Page{Title: "x/a", Slug: "a", Type: "post", URLFormat: "slug"}
	const ld = `<script type="application/ld+json">{"@type":"SoftwareApplication"}</script>`
	const og = `<meta property="og:title" content="x/a">`
	cases := []struct {
		name, head string
		wantOG     bool
		wantLD     bool
	}{
		{"own JSON-LD, no og:title (#349)", ld, true, false},
		{"own og:title, no JSON-LD", og, false, true},
		{"neither", "", true, true},
		{"both", og + ld, false, false},
	}
	for _, c := range cases {
		out := g.seoHTMLString("<html><head>"+c.head+"</head><body></body></html>", page, true)
		gotOG := strings.Count(out, `property="og:title"`) > strings.Count(c.head, `property="og:title"`)
		gotLD := strings.Count(out, "application/ld+json") > strings.Count(c.head, "application/ld+json")
		if gotOG != c.wantOG || gotLD != c.wantLD {
			t.Errorf("%s: injected og=%v ld=%v, want og=%v ld=%v\n%s", c.name, gotOG, gotLD, c.wantOG, c.wantLD, out)
		}
		if c.name == "own JSON-LD, no og:title (#349)" && strings.Contains(out, "BlogPosting") {
			t.Errorf("a BlogPosting beside the theme's SoftwareApplication:\n%s", out)
		}
	}
}
