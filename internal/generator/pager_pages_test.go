package generator

// Numbered pagination (#156). Prev/next is enough for "← →" and not enough for
// the control most themes draw: a reader on page six reaches page two by
// clicking 2, not by stepping back four times. A template cannot build the
// addresses itself — they depend on the language prefix and on posts_page.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestPagerPagesCarryEveryAddress(t *testing.T) {
	p := Pager{Current: 2, Total: 3, PerPage: 2}
	p = p.withPages(func(n int) string { return pageURLWithPrefix("", n) })

	if len(p.Pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(p.Pages))
	}
	want := []string{"/", "/page/2/", "/page/3/"}
	for i, page := range p.Pages {
		if page.Number != i+1 || page.URL != want[i] {
			t.Errorf("page %d = %+v, want number %d at %s", i, page, i+1, want[i])
		}
	}
	if !p.Pages[1].Current || p.Pages[0].Current {
		t.Errorf("the current page must be marked once: %+v", p.Pages)
	}
}

// TestPagerPagesFollowThePrefix: a listing moved by posts_page or a language
// prefix numbers its own addresses, which is exactly what a theme cannot guess.
func TestPagerPagesFollowThePrefix(t *testing.T) {
	p := Pager{Current: 1, Total: 2}.withPages(func(n int) string { return pageURLWithPrefix("blog", n) })
	if p.Pages[0].URL != "/blog/" || p.Pages[1].URL != "/blog/page/2/" {
		t.Fatalf("prefixed addresses wrong: %+v", p.Pages)
	}
}

// TestPagerWindow: a long run comes back as first … window … last, one
// ellipsis per gap — the shape WordPress draws, so a migrated theme's
// .page-numbers markup keeps working.
func TestPagerWindow(t *testing.T) {
	p := Pager{Current: 6, Total: 10}.withPages(func(n int) string { return pageURLWithPrefix("", n) })

	got := p.Window(1)
	var shape []string
	for _, page := range got {
		if page.Ellipsis {
			shape = append(shape, "…")
			continue
		}
		shape = append(shape, strconv.Itoa(page.Number))
	}
	if strings.Join(shape, " ") != "1 … 5 6 7 … 10" {
		t.Fatalf("window = %v", shape)
	}

	// A run short enough to show whole comes back whole, with no gaps.
	short := Pager{Current: 2, Total: 3}.withPages(func(n int) string { return pageURLWithPrefix("", n) })
	for _, page := range short.Window(1) {
		if page.Ellipsis {
			t.Fatalf("a short run needs no ellipsis: %+v", short.Window(1))
		}
	}
	// A negative radius is treated as zero rather than panicking.
	if got := p.Window(-3); len(got) == 0 {
		t.Fatal("a negative radius must still yield the ends")
	}
}

// TestSinglePagePagerHasItsPage: an unpaginated archive still renders "1"
// rather than an empty control.
func TestSinglePagePagerHasItsPage(t *testing.T) {
	p := singlePagePager(4)
	if len(p.Pages) != 1 || !p.Pages[0].Current || p.Pages[0].Number != 1 {
		t.Fatalf("single-page pager = %+v", p.Pages)
	}
}

// TestPaginatedArchiveCarriesNumbers: the whole point, through a real build.
func TestPaginatedArchiveCarriesNumbers(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}{{range .Pager.Pages}}<a href="{{.URL}}">{{.Number}}</a>{{end}}{{end}}`)
	g.config.Paginate = 1
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Blog", Slug: "blog"}}
	g.siteData.Posts = []models.Page{postN(0), postN(1)}

	if err := g.generateCategories(); err != nil {
		t.Fatal(err)
	}
	body := mustReadOutput(t, g, "category/blog/index.html")
	if !strings.Contains(body, `href="/category/blog/page/2/">2`) {
		t.Fatalf("page two is not linked from page one: %s", body)
	}
}
