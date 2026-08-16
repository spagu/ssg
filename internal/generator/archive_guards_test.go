package generator

// The guards around writing an archive: real content outranks a generated
// listing, and a template that cannot render one must not take the build down.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// TestDateArchiveYieldsToRealContent: an author's own /2014/ page is the page;
// a generated listing must never overwrite it (#146).
func TestDateArchiveYieldsToRealContent(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<h1>{{.Name}}</h1>{{end}}`)
	g.config.DateArchives = true
	g.siteData.Posts = []models.Page{{
		Title: "Post", Slug: "post", Status: "publish", Type: "post",
		Date: time.Date(2014, time.May, 12, 0, 0, 0, 0, time.UTC), Content: "Body.",
	}}
	// A real page claims the year.
	g.siteData.Pages = []models.Page{{
		Title: "Year in review", Slug: "2014", Status: "publish", Type: "page", Link: "/2014/",
	}}

	written, err := g.writeDateArchive(dateArchive{Path: "2014", Label: "2014", Depth: depthYear,
		Posts: g.siteData.Posts})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("a generated archive must yield to the page that owns the URL")
	}
}

// TestDateArchiveRenderFailureIsReported: a theme without category.html loses
// the archive, not the build — the site still ships.
func TestDateArchiveRenderFailureIsReported(t *testing.T) {
	g := newTestGen(t, `{{define "other.html"}}x{{end}}`) // no category.html
	g.config.DateArchives = true
	g.config.Quiet = true
	g.siteData.Posts = []models.Page{{
		Title: "Post", Slug: "post", Status: "publish", Type: "post",
		Date: time.Date(2014, time.May, 12, 0, 0, 0, 0, time.UTC),
	}}

	if err := g.generateDateArchives(); err != nil {
		t.Fatalf("a missing template must not fail the build: %v", err)
	}
	if fileExists(t, g, "2014/index.html") {
		t.Fatal("nothing renderable, nothing written")
	}
}

// TestGenerateDateArchivesNoPosts: a site with no posts has no archives and
// says nothing about it.
func TestGenerateDateArchivesNoPosts(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}x{{end}}`)
	g.config.DateArchives = true
	if err := g.generateDateArchives(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(g.config.OutputDir)
	if err == nil && len(entries) != 0 {
		t.Fatalf("no posts → no output, got %v", entries)
	}
}

// TestLoadMetadataFailures: a source without metadata.json, and one whose file
// is not JSON, both surface as errors the caller reports — silently building a
// site with no categories or authors would be worse.
func TestLoadMetadataFailures(t *testing.T) {
	g := newTestGen(t, "")
	dir := t.TempDir()

	if err := g.loadMetadata(filepath.Join(dir, "metadata.json")); err == nil {
		t.Fatal("a missing metadata.json must be reported")
	}
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.loadMetadata(broken); err == nil {
		t.Fatal("unparseable metadata must be reported")
	}
}

// TestLoadMetadataFillsSiteData: categories, authors, tags and media all reach
// the site view under the ids the content references them by.
func TestLoadMetadataFillsSiteData(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Media = map[int]models.MediaItem{}
	g.siteData.Authors = map[int]models.Author{}
	g.siteData.Tags = map[int]models.Category{}

	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	body := `{"categories":[{"id":1,"name":"News","slug":"news"}],
	          "users":[{"id":5,"name":"Ada","slug":"ada"}],
	          "tags":[{"id":9,"name":"Go","slug":"go"}],
	          "media":[{"id":3,"source_url":"/media/a.png"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.loadMetadata(path); err != nil {
		t.Fatal(err)
	}
	if g.siteData.Categories[1].Name != "News" {
		t.Errorf("category not loaded: %+v", g.siteData.Categories)
	}
	if g.siteData.Authors[5].Name != "Ada" {
		t.Errorf("author not loaded: %+v", g.siteData.Authors)
	}
	if g.siteData.Tags[9].Slug != "go" {
		t.Errorf("tag not loaded: %+v", g.siteData.Tags)
	}
	if _, ok := g.siteData.Media[3]; !ok {
		t.Errorf("media not loaded: %+v", g.siteData.Media)
	}
}
