package generator

// The comments a migration carried across must reach the page they belong to
// (#142), and a theme must be able to tell it is rendering the front page
// (#141).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func writeComments(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "comments.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const commentsJSON = `{"total":3,"pages":1,"comments":[
  {"id":1,"parent":0,"post_url":"/blog/hello/","author":"Ada","date":"2024-03-01T10:00:00Z","content":"<p>first</p>","status":"approved"},
  {"id":2,"parent":1,"post_url":"/blog/hello/","author":"Bo","date":"2024-03-02T10:00:00Z","content":"<p>reply</p>","status":"approved"},
  {"id":3,"parent":0,"post_url":"/about/","author":"Cy","date":"2024-03-03T10:00:00Z","content":"<p>elsewhere</p>","status":"approved"}
]}`

func TestLoadCommentsAndMatchPage(t *testing.T) {
	g := newTestGen(t, "")
	dir := t.TempDir()
	g.loadComments(writeComments(t, dir, commentsJSON))

	if len(g.siteData.Comments) != 2 {
		t.Fatalf("pages with comments = %d, want 2", len(g.siteData.Comments))
	}
	// A post's own thread reaches it whether or not the two sides spell the
	// URL with a trailing slash.
	post := models.Page{Type: "post", Slug: "hello", Link: "/blog/hello/"}
	thread := g.commentsFor(post)
	if len(thread) != 1 || thread[0].Author != "Ada" || len(thread[0].Replies) != 1 {
		t.Fatalf("thread not matched: %+v", thread)
	}
	// A page with none gets nil, so `{{with .Comments}}` renders nothing
	// rather than an empty section.
	if got := g.commentsFor(models.Page{Type: "page", Slug: "contact", Link: "/contact/"}); got != nil {
		t.Fatalf("a page without comments must get nil, got %+v", got)
	}
}

// TestLoadCommentsTolerates: a site that never had comments and a file that
// cannot be read must both leave the build alone — an archive of old comments
// is not worth failing over.
func TestLoadCommentsTolerates(t *testing.T) {
	dir := t.TempDir()

	g := newTestGen(t, "")
	g.loadComments(filepath.Join(dir, "comments.json")) // missing
	if g.siteData.Comments != nil {
		t.Fatal("a missing file must load nothing")
	}

	g2 := newTestGen(t, "")
	g2.loadComments(writeComments(t, dir, "{not json"))
	if g2.siteData.Comments != nil {
		t.Fatal("an unreadable file must load nothing")
	}

	g3 := newTestGen(t, "")
	g3.loadComments(writeComments(t, dir, `{"total":0,"comments":[]}`))
	if g3.siteData.Comments != nil {
		t.Fatal("an empty file must load nothing")
	}
	// Nothing loaded means nothing to look up.
	if got := g3.commentsFor(models.Page{Slug: "x"}); got != nil {
		t.Fatalf("commentsFor without data = %+v", got)
	}
}

// TestIsFrontPageInTemplateData: the generator knows which page took the root;
// a theme could only guess by comparing .Link against "/" (#141).
func TestIsFrontPageInTemplateData(t *testing.T) {
	g := newTestGen(t, "")
	front := g.pageToTemplateData(models.Page{Type: "page", Slug: "home", Link: "/"}, false)
	if front["IsFrontPage"] != true {
		t.Fatalf("the page at / is the front page: %v", front["IsFrontPage"])
	}
	other := g.pageToTemplateData(models.Page{Type: "page", Slug: "about"}, false)
	if other["IsFrontPage"] != false {
		t.Fatalf("an ordinary page is not the front page: %v", other["IsFrontPage"])
	}
}
