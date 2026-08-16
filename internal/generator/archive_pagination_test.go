package generator

// Paginating a category archive (#149) and the posts_page collision (#150).

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func postN(i int) models.Page {
	return models.Page{
		Title: "Post", Slug: "p", Status: "publish", Type: "post",
		Date:       time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		Categories: []int{1}, Content: "Body.",
	}
}

// TestCategoryArchivePaginates: `paginate` applied to the index and left every
// category archive whole — a migrated site shipped 205 articles in one file
// while /category/<slug>/page/2/ did not exist (#149).
func TestCategoryArchivePaginates(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<p>{{.Pager.Current}}/{{.Pager.Total}} {{len .Posts}}</p>{{end}}`)
	g.config.Paginate = 2
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Blog", Slug: "blog"}}
	for i := 0; i < 5; i++ {
		g.siteData.Posts = append(g.siteData.Posts, postN(i))
	}

	if err := g.generateCategories(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"category/blog/index.html",
		"category/blog/page/2/index.html",
		"category/blog/page/3/index.html",
	} {
		if !fileExists(t, g, want) {
			t.Errorf("missing %s", want)
		}
	}
	// Page one holds the page size, and the pager tells the truth about the rest.
	first := mustReadOutput(t, g, "category/blog/index.html")
	if !strings.Contains(first, "1/3 2") {
		t.Errorf("page 1 = %q, want pager 1/3 with 2 posts", strings.TrimSpace(first))
	}
	last := mustReadOutput(t, g, "category/blog/page/3/index.html")
	if !strings.Contains(last, "3/3 1") {
		t.Errorf("page 3 = %q, want pager 3/3 with the remaining post", strings.TrimSpace(last))
	}
}

// TestCategoryArchiveUnpaginated: without `paginate` the archive is one file,
// exactly as every existing site has always had it.
func TestCategoryArchiveUnpaginated(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<p>{{len .Posts}}</p>{{end}}`)
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Blog", Slug: "blog"}}
	for i := 0; i < 5; i++ {
		g.siteData.Posts = append(g.siteData.Posts, postN(i))
	}
	if err := g.generateCategories(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, g, "category/blog/index.html") {
		t.Fatal("the archive must exist")
	}
	if fileExists(t, g, "category/blog/page/2/index.html") {
		t.Fatal("an unpaginated site must not grow page/2/")
	}
	if body := mustReadOutput(t, g, "category/blog/index.html"); !strings.Contains(body, "5") {
		t.Errorf("all posts on one page: %q", strings.TrimSpace(body))
	}
}

// TestPostsPageOwner: the page whose address posts_page names is found, so the
// listing can take that URL the way the source CMS does (#150).
func TestPostsPageOwner(t *testing.T) {
	g := newTestGen(t, "")
	pages := []models.Page{
		{Title: "About", Slug: "about", Type: "page"},
		{Title: "Blog", Slug: "blog", Type: "page", Link: "/blog/"},
	}

	if got := g.postsPageOwner(pages, ""); got != nil {
		t.Fatalf("no posts_page → no collision, got %+v", got)
	}
	g.config.PostsPage = "blog"
	got := g.postsPageOwner(pages, "")
	if got == nil || got.Title != "Blog" {
		t.Fatalf("the colliding page must be found, got %+v", got)
	}
	// A posts_page nothing occupies is the ordinary case.
	g.config.PostsPage = "news"
	if got := g.postsPageOwner(pages, ""); got != nil {
		t.Fatalf("free address → no collision, got %+v", got)
	}
	// Spelling with slashes is the same address.
	g.config.PostsPage = "/blog/"
	if got := g.postsPageOwner(pages, ""); got == nil {
		t.Fatal("slashes must not change the answer")
	}
}

// TestWithoutPage drops exactly the colliding document and keeps the rest.
func TestWithoutPage(t *testing.T) {
	pages := []models.Page{
		{Title: "About", Slug: "about", Type: "page"},
		{Title: "Blog", Slug: "blog", Type: "page"},
	}
	got := withoutPage(pages, pages[1])
	if len(got) != 1 || got[0].Title != "About" {
		t.Fatalf("withoutPage = %+v", got)
	}
	// Dropping something not present changes nothing.
	if got := withoutPage(pages, models.Page{Slug: "nope", Type: "page"}); len(got) != 2 {
		t.Fatalf("unrelated page removed: %+v", got)
	}
}

// captureBuildOutput collects what a build step printed, so a report can be
// asserted on rather than eyeballed.
func captureBuildOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

func mustReadOutput(t *testing.T, g *Generator, rel string) string {
	t.Helper()
	return mustRead(t, g.config.OutputDir+"/"+rel)
}

// TestArchivePerPage: the archive page size is the site-wide `paginate`, and
// an unset one means a single page — the shape every existing site has.
func TestArchivePerPage(t *testing.T) {
	g := newTestGen(t, "")
	if got := g.archivePerPage(); got != 0 {
		t.Errorf("unset paginate = %d, want 0", got)
	}
	g.config.Paginate = 7
	if got := g.archivePerPage(); got != 7 {
		t.Errorf("archivePerPage = %d, want 7", got)
	}
}

// TestReportPostsPageCollision: the operator is told which document is being
// served and what to do — the silence was what cost an afternoon (#150).
func TestReportPostsPageCollision(t *testing.T) {
	g := newTestGen(t, "")
	g.config.PostsPage = "blog"
	page := models.Page{Title: "Blog", Slug: "blog", Type: "page", SourceFile: "blog.md"}

	out := captureBuildOutput(t, func() { g.reportPostsPageCollision(&page) })
	for _, want := range []string{"/blog/", "posts_page", "blog.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report must name %q:\n%s", want, out)
		}
	}
	// Nothing to report, and a quiet build, both stay silent.
	if out := captureBuildOutput(t, func() { g.reportPostsPageCollision(nil) }); out != "" {
		t.Errorf("no collision → no line, got %q", out)
	}
	g.config.Quiet = true
	if out := captureBuildOutput(t, func() { g.reportPostsPageCollision(&page) }); out != "" {
		t.Errorf("a quiet build must stay quiet, got %q", out)
	}
}

// TestCategoryArchiveYieldsToContent: an author's own page at the archive's
// address keeps it (GO-050), and nothing is written over it.
func TestCategoryArchiveYieldsToContent(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<p>archive</p>{{end}}`)
	g.siteData.Categories = map[int]models.Category{1: {ID: 1, Name: "Blog", Slug: "blog"}}
	g.siteData.Posts = []models.Page{postN(0)}
	g.siteData.Pages = []models.Page{{
		Title: "Blog", Slug: "blog", Type: "page", Status: "publish", Link: "/category/blog/",
	}}
	if err := g.generateCategories(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, g, "category/blog/index.html") {
		t.Fatal("a generated archive must not overwrite the page that owns the URL")
	}
}
