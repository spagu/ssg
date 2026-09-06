package generator

// The author archive is driven outside the taxonomy registry (#44), so termURL
// cannot name it and a byline had to hardcode /author/<slug>/ and guess the slug
// rule. A theme that did not guess left the archive an orphan: published, in the
// sitemap, linked from nowhere (#245).

import (
	"html/template"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func authorTestGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	g.siteData.Authors = map[int]models.Author{
		1: {ID: 1, Name: "Ian Zane", Slug: "ian-zane"},
		2: {ID: 2, Name: "Ada Lovelace"}, // no slug of its own
	}
	return g
}

// TestAuthorURLFromTheShapesATemplateHolds: a byline has the id, the post, the
// record or the name, and all four name the same archive.
func TestAuthorURLFromTheShapesATemplateHolds(t *testing.T) {
	g := authorTestGen(t)
	const want = "/author/ian-zane/"

	cases := map[string]any{
		"id":      1,
		"int64":   int64(1),
		"float64": float64(1), // an id arriving through data/JSON
		"page":    models.Page{Author: 1},
		"pointer": &models.Page{Author: 1},
		"record":  g.siteData.Authors[1],
		"name":    "Ian Zane",
		"slug":    "ian-zane",
	}
	for label, value := range cases {
		if got := g.tmplAuthorURL(value); got != want {
			t.Errorf("authorURL(%s) = %q, want %q", label, got, want)
		}
	}
}

// TestAuthorURLDerivesTheSlugTheArchiveUses: an author record with no slug is
// published under its slugified name, and the link must follow the same rule.
func TestAuthorURLDerivesTheSlugTheArchiveUses(t *testing.T) {
	g := authorTestGen(t)

	if got, want := g.tmplAuthorURL(2), "/author/ada-lovelace/"; got != want {
		t.Errorf("authorURL = %q, want %q", got, want)
	}
	// The record itself answers the same way the id does.
	if got, want := g.tmplAuthorURL(g.siteData.Authors[2]), "/author/ada-lovelace/"; got != want {
		t.Errorf("authorURL(record) = %q, want %q", got, want)
	}
	// An id the site has no record for still has an archive — generateAuthors
	// writes it under the same placeholder slug.
	if got, want := g.tmplAuthorURL(9), "/author/author-9/"; got != want {
		t.Errorf("authorURL = %q, want %q", got, want)
	}
	// A name nobody registered is slugified rather than dropped: it is what the
	// archive would be written under if that author gains a post.
	if got, want := g.tmplAuthorURL("Grace Hopper"), "/author/grace-hopper/"; got != want {
		t.Errorf("authorURL = %q, want %q", got, want)
	}
}

// TestAuthorURLOnNoAuthor: nothing to link is an empty string, not a link to a
// placeholder archive that was never written.
func TestAuthorURLOnNoAuthor(t *testing.T) {
	g := authorTestGen(t)

	for label, value := range map[string]any{
		"unset id":            0,
		"empty name":          "   ",
		"post with no author": models.Page{},
		"unsupported":         struct{}{},
		"nil":                 nil,
	} {
		if got := g.tmplAuthorURL(value); got != "" {
			t.Errorf("authorURL(%s) = %q, want empty", label, got)
		}
	}
}

// TestAuthorURLIsRegistered: the helper exists under the name the docs promise,
// in the template set themes actually render with.
func TestAuthorURLIsRegistered(t *testing.T) {
	g := authorTestGen(t)

	for _, funcs := range []map[string]any{g.buildTemplateFuncs(nil), g.shortcodeFuncMap()} {
		if _, ok := funcs["authorURL"]; !ok {
			t.Error("authorURL is not registered")
		}
	}
	// And it renders: a byline is the whole point.
	tmpl, err := template.New("byline").Funcs(g.buildTemplateFuncs(nil)).
		Parse(`<a href="{{authorURL .Author}}">{{getAuthorName .Author}}</a>`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, models.Page{Author: 1}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if want := `<a href="/author/ian-zane/">Ian Zane</a>`; out.String() != want {
		t.Errorf("byline rendered %q, want %q", out.String(), want)
	}
}
