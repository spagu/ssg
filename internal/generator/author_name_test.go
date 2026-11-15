package generator

import (
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// getAuthorName takes the page too, so a theme can name a plain frontmatter
// author ("author: Jane Doe") that no metadata registers — and get "" when the
// page has none, to leave the author out rather than print an empty name.
func TestTmplGetAuthorNameFromPage(t *testing.T) {
	gen := &Generator{siteData: &models.SiteData{Authors: map[int]models.Author{1: {ID: 1, Name: "Admin"}}}}
	plain := models.Page{AuthorRaw: "  Maria Halloran "}
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"a registered id", 1, "Admin"},
		{"an unknown id", 7, ""},
		{"a page with a registered author", models.Page{Author: 1, AuthorRaw: "admin"}, "Admin"},
		{"a page with a plain author", plain, "Maria Halloran"},
		{"a pointer to that page", &plain, "Maria Halloran"},
		{"a page without an author", models.Page{}, ""},
		{"a nil page pointer", (*models.Page)(nil), ""},
		{"anything else", "Maria Halloran", ""},
	}
	for _, c := range cases {
		if got := gen.tmplGetAuthorName(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
