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

// TestThemeJSONLDIsNotDoubled covers #349 and #351: the social tags and the
// page's derived type are each added only when the theme wrote none; the
// BreadcrumbList and FAQPage sit beside any type, so they are added unless
// the theme wrote those too.
func TestThemeJSONLDIsNotDoubled(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SEO = true
	page := models.Page{Title: "x/a", Slug: "a", Type: "post", URLFormat: "slug",
		Extra: map[string]interface{}{"faq": []interface{}{map[string]interface{}{"question": "Q?", "answer": "A."}}}}
	const ld = `<script type="application/ld+json">{"@type":"SoftwareApplication"}</script>`
	const og = `<meta property="og:title" content="x/a">`
	const crumbs = `<script type="application/ld+json">{"@type":"BreadcrumbList"}</script>`
	cases := []struct {
		name, head               string
		wantOG, wantType         bool
		wantCrumbs, wantFAQBlock bool
	}{
		{"own JSON-LD, no og:title (#349)", ld, true, false, true, true},
		{"own og:title, no JSON-LD", og, false, true, true, true},
		{"neither", "", true, true, true, true},
		{"both", og + ld, false, false, true, true},
		{"own breadcrumb too (#351)", og + ld + crumbs, false, false, false, true},
	}
	for _, c := range cases {
		out := g.seoHTMLString("<html><head>"+c.head+"</head><body></body></html>", page, true)
		added := strings.TrimPrefix(out, "<html><head>"+c.head)
		got := [4]bool{
			strings.Contains(added, `property="og:title"`),
			strings.Contains(added, "BlogPosting"),
			strings.Contains(added, `"BreadcrumbList"`),
			strings.Contains(added, `"FAQPage"`),
		}
		want := [4]bool{c.wantOG, c.wantType, c.wantCrumbs, c.wantFAQBlock}
		if got != want {
			t.Errorf("%s: injected og/type/crumbs/faq = %v, want %v\n%s", c.name, got, want, out)
		}
	}
}

// TestBreadcrumbSchemaInTemplates covers #351: a theme that writes its own
// JSON-LD gets the derived trail as data, ready for toJSON.
func TestBreadcrumbSchemaInTemplates(t *testing.T) {
	g := newTestGen(t, "")
	data := g.pageToTemplateData(models.Page{Title: "A", Slug: "a", Type: "page", Link: "/x/a/"}, false)
	bc, ok := data["BreadcrumbSchema"].(map[string]interface{})
	if !ok || bc["@type"] != "BreadcrumbList" {
		t.Fatalf("BreadcrumbSchema = %v", data["BreadcrumbSchema"])
	}
	if home := g.pageToTemplateData(models.Page{Title: "Home", Slug: "", Type: "page", Link: "/"}, false); home["BreadcrumbSchema"] != nil && home["BreadcrumbSchema"].(map[string]interface{}) != nil {
		t.Errorf("the front page has no trail: %v", home["BreadcrumbSchema"])
	}
}
