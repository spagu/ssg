package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestOGCardsReachPageTemplates reproduces #395: a page template saw no card,
// because the flat page map copied FeaturedImage but not OGCard. A theme that
// writes its own og:image could not use the cards on pages at all.
func TestOGCardsReachPageTemplates(t *testing.T) {
	tmp := writeOGCardSite(t)
	mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "contact.md"),
		"---\ntitle: Contact\nslug: contact\nstatus: publish\ntype: page\n---\n\nWrite to us.\n")
	mustWrite(t, filepath.Join(tmp, "templates", "simple", "page.html"),
		`{{define "page.html"}}<html><head><title>{{.Page.Title}}</title></head>`+
			`<body data-flat="{{.OGCard}}" data-page="{{.Page.OGCard}}" data-or="{{or .FeaturedImage .OGCard}}">x</body></html>{{end}}`)
	out, err := buildOGCardSite(t, tmp, OGCardOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	page := readAudioOut(t, out, "contact/index.html")
	for _, attr := range []string{"data-flat", "data-page", "data-or"} {
		if !strings.Contains(page, attr+`="/og/`) {
			t.Errorf("%s is empty on a page without an image:\n%s", attr, page)
		}
	}
	// A page with its own image gets no card, and .FeaturedImage wins.
	about := readAudioOut(t, out, "about/index.html")
	if !strings.Contains(about, `data-flat=""`) || !strings.Contains(about, `data-or="/media/about.jpg"`) {
		t.Errorf("about:\n%s", about)
	}
}

// TestExternalSourceIsFlatToo: the other documented field the map lacked.
func TestExternalSourceIsFlatToo(t *testing.T) {
	g := newTestGen(t, "")
	data := g.pageToTemplateData(models.Page{Slug: "p", ExternalSource: "ports", OGCard: "/og/p.png"}, false)
	if data["ExternalSource"] != "ports" || data["OGCard"] != "/og/p.png" {
		t.Errorf("ExternalSource=%v OGCard=%v", data["ExternalSource"], data["OGCard"])
	}
}
