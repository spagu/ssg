package generator

// A nested category renders where the source site served it, and its old flat
// path becomes a redirect (#138).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestAddCategoryRedirect(t *testing.T) {
	g := &Generator{}
	g.addCategoryRedirect("dzien-bociana", "blog/dzien-bociana")
	if len(g.aliasRedirects) != 1 {
		t.Fatalf("redirect not recorded: %+v", g.aliasRedirects)
	}
	r := g.aliasRedirects[0]
	if r.From != "/category/dzien-bociana/" || r.To != "/category/blog/dzien-bociana/" || r.Status != 301 {
		t.Fatalf("redirect wrong: %+v", r)
	}

	// Nothing to redirect when the path did not move, and nothing recorded for
	// an empty slug — a rule pointing at itself would be a loop.
	g.addCategoryRedirect("blog", "blog")
	g.addCategoryRedirect("", "blog")
	g.addCategoryRedirect("blog", "")
	if len(g.aliasRedirects) != 1 {
		t.Fatalf("only a real move is a redirect: %+v", g.aliasRedirects)
	}
}

// categoryGen builds a generator whose only template is the category archive,
// with one post filed under catID — the smallest site that renders an archive.
func categoryGen(t *testing.T, cats map[int]models.Category, catID int) *Generator {
	t.Helper()
	g := newTestGen(t, `{{define "category.html"}}<html>{{.Name}}</html>{{end}}`)
	g.siteData.Categories = cats
	g.siteData.Posts = []models.Page{{
		Title: "Post", Slug: "post", Status: "publish", Type: "post",
		Categories: []int{catID}, Content: "Body.",
	}}
	if err := g.generateCategories(); err != nil {
		t.Fatalf("generateCategories: %v", err)
	}
	return g
}

// TestNestedCategoryArchiveAndRedirect drives the generator: a child category
// must render under its parent and leave a 301 behind at the flat address the
// site used to serve, so links that survived the migration still work.
func TestNestedCategoryArchiveAndRedirect(t *testing.T) {
	g := categoryGen(t, map[int]models.Category{
		1: {ID: 1, Name: "Blog", Slug: "blog"},
		3: {ID: 3, Name: "Dzien Bociana", Slug: "dzien-bociana", Parent: 1},
	}, 3)

	if !fileExists(t, g, "category/blog/dzien-bociana/index.html") {
		t.Fatal("the nested archive is missing — this is the 404 the issue reports")
	}
	var found bool
	for _, r := range g.aliasRedirects {
		if r.From == "/category/dzien-bociana/" && r.To == "/category/blog/dzien-bociana/" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the old flat path must redirect: %+v", g.aliasRedirects)
	}
}

// TestTopLevelCategoryUnchanged: a site with no nesting keeps the URLs it has
// always had, and earns no redirects.
func TestTopLevelCategoryUnchanged(t *testing.T) {
	g := categoryGen(t, map[int]models.Category{
		1: {ID: 1, Name: "News", Slug: "news"},
	}, 1)

	if !fileExists(t, g, "category/news/index.html") {
		t.Fatal("a flat category must keep its address")
	}
	if len(g.aliasRedirects) != 0 {
		t.Fatalf("nothing moved, so nothing redirects: %+v", g.aliasRedirects)
	}
}

// fileExists reports whether the build wrote a file, which is the whole
// question when an archive lands at the wrong address.
func fileExists(t *testing.T, g *Generator, rel string) bool {
	t.Helper()
	info, err := os.Stat(filepath.Join(g.config.OutputDir, rel))
	return err == nil && info.Size() > 0
}
