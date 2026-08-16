package generator

// A pinned post leads the listing (#155). WordPress lets an editor pin a post
// to the top of the blog and the export carries the flag; sorting by date alone
// put it wherever its date fell — sixth of ten on the site that reported it,
// while the source showed it first.

import (
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func datedPostSticky(title string, day int, sticky bool) models.Page {
	return models.Page{
		Title: title, Slug: title, Status: "publish", Type: "post", Sticky: sticky,
		Date: time.Date(2024, time.January, day, 0, 0, 0, 0, time.UTC),
	}
}

func TestSortPostsByDatePinsSticky(t *testing.T) {
	posts := []models.Page{
		datedPostSticky("newest", 10, false),
		datedPostSticky("pinned-old", 1, true),
		datedPostSticky("middle", 5, false),
		datedPostSticky("pinned-new", 8, true),
	}
	got := titlesOf(t, sortPostsByDate(posts))
	if want := "pinned-new,pinned-old,newest,middle"; got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

// TestSortPostsByDateUnchangedWithoutSticky: a site that pins nothing gets the
// order it has always had — the flag is additive, not a new sort.
func TestSortPostsByDateUnchangedWithoutSticky(t *testing.T) {
	posts := []models.Page{
		datedPostSticky("b", 5, false),
		datedPostSticky("c", 1, false),
		datedPostSticky("a", 10, false),
	}
	if got := titlesOf(t, sortPostsByDate(posts)); got != "a,b,c" {
		t.Fatalf("order = %q, want a,b,c", got)
	}
}

// TestStickyReachesTemplates: a theme marks the pinned post the way the source
// CMS did, which is what makes a migrated listing look right rather than merely
// be ordered right.
func TestStickyReachesTemplates(t *testing.T) {
	g := newTestGen(t, "")
	data := g.pageToTemplateData(datedPostSticky("pinned", 1, true), true)
	if data["Sticky"] != true {
		t.Fatalf("Sticky = %v, want true", data["Sticky"])
	}
	plain := g.pageToTemplateData(datedPostSticky("ordinary", 1, false), true)
	if plain["Sticky"] != false {
		t.Fatalf("an ordinary post = %v, want false", plain["Sticky"])
	}
}

// TestStickyOrdersTheArchive: the flag reaches the listings that sort by date,
// not only the raw sorter.
func TestStickyOrdersTheArchive(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}{{range .Posts}}[{{.Title}}]{{end}}{{end}}`)
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Blog", Slug: "blog"}}
	for _, p := range []models.Page{
		datedPostSticky("newest", 10, false),
		datedPostSticky("pinned", 1, true),
	} {
		p.Categories = []int{1}
		g.siteData.Posts = append(g.siteData.Posts, p)
	}
	if err := g.generateCategories(); err != nil {
		t.Fatal(err)
	}
	body := mustReadOutput(t, g, "category/blog/index.html")
	if body[:len("[pinned]")] != "[pinned]" {
		t.Fatalf("the pinned post must lead the archive: %s", body)
	}
}
