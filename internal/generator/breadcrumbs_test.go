package generator

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// crumbs returns the (name, item) pairs of a page's BreadcrumbList, in order,
// checking the positions run 1..n with no gaps.
func crumbs(t *testing.T, g *Generator, page models.Page) [][2]string {
	t.Helper()
	bc := g.breadcrumbLD(page)
	if bc == nil {
		return nil
	}
	var out [][2]string
	for i, it := range bc["itemListElement"].([]interface{}) {
		m := it.(map[string]interface{})
		if m["position"] != i+1 {
			t.Fatalf("position %v at index %d: steps must be renumbered after a gap", m["position"], i)
		}
		out = append(out, [2]string{m["name"].(string), m["item"].(string)})
	}
	return out
}

func article() models.Page {
	return models.Page{Title: "My article", Slug: "my-article", Type: "page", Link: "/posts/my-article/"}
}

// TestBreadcrumbSkipsSectionWithoutPage covers #335: /posts/ is not written,
// so the trail must not link to it.
func TestBreadcrumbSkipsSectionWithoutPage(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{article()}
	got := crumbs(t, g, article())
	want := [][2]string{{"Home", "https://example.com/"}, {"My article", "https://example.com/posts/my-article/"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("crumbs = %v, want %v", got, want)
	}
}

// TestBreadcrumbUsesRealPageTitle: a page that exists at the section's URL
// gives the step its own title, diacritics and all.
func TestBreadcrumbUsesRealPageTitle(t *testing.T) {
	g := newTestGen(t, "")
	hub := models.Page{Title: "Usługi", Slug: "uslugi", Type: "page"}
	svc := models.Page{Title: "Audyt", Slug: "audyt", Type: "page", Link: "/uslugi/audyt/"}
	g.siteData.Pages = []models.Page{hub, svc}
	got := crumbs(t, g, svc)
	if len(got) != 3 || got[1] != [2]string{"Usługi", "https://example.com/uslugi/"} {
		t.Fatalf("crumbs = %v, want the hub page as step 2", got)
	}
}

// TestBreadcrumbConfiguredSection: a configured section names the step and
// points it at its hub; home gets the configured label.
func TestBreadcrumbConfiguredSection(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Breadcrumbs = BreadcrumbOptions{Home: "Strona główna", Sections: map[string]BreadcrumbSection{
		"posts": {Name: "Baza wiedzy", URL: "/baza-wiedzy/"},
	}}
	got := crumbs(t, g, article())
	want := [][2]string{
		{"Strona główna", "https://example.com/"},
		{"Baza wiedzy", "https://example.com/baza-wiedzy/"},
		{"My article", "https://example.com/posts/my-article/"},
	}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("crumbs = %v, want %v", got, want)
	}
}

// TestBreadcrumbSectionNameNeedsAPage: a name without a URL relabels a step
// that exists, and invents none that does not.
func TestBreadcrumbSectionNameNeedsAPage(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Breadcrumbs.Sections = map[string]BreadcrumbSection{"posts": {Name: "Wpisy"}, "uslugi": {Name: "Usługi"}}
	hub := models.Page{Title: "Services", Slug: "uslugi", Type: "page"}
	svc := models.Page{Title: "Audyt", Slug: "audyt", Type: "page", Link: "/uslugi/audyt/"}
	g.siteData.Pages = []models.Page{hub, svc, article()}
	if got := crumbs(t, g, svc); len(got) != 3 || got[1][0] != "Usługi" {
		t.Errorf("configured name must relabel an existing step: %v", got)
	}
	if got := crumbs(t, g, article()); len(got) != 2 {
		t.Errorf("a name alone must not create a step without a page: %v", got)
	}
}

// TestBreadcrumbConfiguredURLFallsBackToItsTitle: a section with a URL but no
// name takes the title of the page at that URL.
func TestBreadcrumbConfiguredURLFallsBackToItsTitle(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Breadcrumbs.Sections = map[string]BreadcrumbSection{"posts": {URL: "/blog/"}}
	g.siteData.Pages = []models.Page{{Title: "Blog", Slug: "blog", Type: "page"}, article()}
	if got := crumbs(t, g, article()); len(got) != 3 || got[1][0] != "Blog" {
		t.Errorf("crumbs = %v, want step 2 titled Blog", got)
	}
	g.config.Breadcrumbs.Sections = map[string]BreadcrumbSection{"posts": {URL: "/nowhere/"}}
	g.resetCrumbIndex()
	if got := crumbs(t, g, article()); len(got) != 3 || got[1][0] != "Posts" {
		t.Errorf("crumbs = %v, want the titleized segment as last resort", got)
	}
}

// TestBreadcrumbOff: breadcrumbs: false emits none.
func TestBreadcrumbOff(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Breadcrumbs.Off = true
	if bc := g.breadcrumbLD(article()); bc != nil {
		t.Errorf("breadcrumbs: false must emit no BreadcrumbList, got %v", bc)
	}
}

// TestBreadcrumbNotFoundPage: the error page has no place in the hierarchy.
func TestBreadcrumbNotFoundPage(t *testing.T) {
	g := newTestGen(t, "")
	if bc := g.breadcrumbLD(models.Page{Title: "Not found", Slug: "404", Type: "page"}); bc != nil {
		t.Errorf("the 404 page must emit no BreadcrumbList, got %v", bc)
	}
}

// TestBreadcrumbLanguageHome: under i18n the trail starts at the language's
// home rather than at a step named after the language code.
func TestBreadcrumbLanguageHome(t *testing.T) {
	g := newTestGen(t, "")
	p := models.Page{Title: "O nas", Slug: "o-nas", Type: "page", LangPrefix: "pl"}
	got := crumbs(t, g, p)
	if len(got) != 2 || got[0][1] != "https://example.com/pl/" {
		t.Fatalf("crumbs = %v, want Home at /pl/", got)
	}
}

// TestBreadcrumbIndexCoversListingsAndArchives: the post listing, taxonomy,
// type and date archives are documents too.
func TestBreadcrumbIndexCoversListingsAndArchives(t *testing.T) {
	g := newTestGen(t, "")
	g.config.DateArchives = true
	g.postsListings = map[string]string{"blog": "", "": ""}
	post := models.Page{Title: "P", Slug: "p", Type: "post", Date: time.Date(2024, 5, 3, 0, 0, 0, 0, time.UTC)}
	g.siteData.Posts = []models.Page{post}
	g.siteData.Pages = []models.Page{{Title: "Ref", Slug: "ref", Type: "realizacje"}}
	g.config.TypeArchives = map[string]bool{"realizacje": true}
	idx := g.crumbIndex()
	for _, want := range []string{"/blog/", "/2024/", "/2024/05/", "/realizacje/"} {
		if _, ok := idx[want]; !ok {
			t.Errorf("crumb index lacks %s: %v", want, idx)
		}
	}
	if got := crumbs(t, g, post); len(got) != 5 || !strings.HasSuffix(got[1][1], "/2024/") {
		t.Errorf("a dated post must walk its existing date archives: %v", got)
	}
}

// TestBreadcrumbHomeOnlyTrailIsDropped: a page whose only segment is a
// language prefix leaves nothing but home, which is no trail.
func TestBreadcrumbHomeOnlyTrailIsDropped(t *testing.T) {
	g := newTestGen(t, "")
	if bc := g.breadcrumbLD(models.Page{Title: "EN", Slug: "en", Type: "page", Link: "/en/", LangPrefix: "en"}); bc != nil {
		t.Errorf("a language home must emit no trail, got %v", bc)
	}
}
