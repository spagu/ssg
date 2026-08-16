package generator

// Date archives (#146) and the one shape every archive view shares (#145).

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func datedPost(title string, y int, m time.Month, d int) models.Page {
	return models.Page{
		Title: title, Slug: strings.ToLower(title), Status: "publish", Type: "post",
		Date: time.Date(y, m, d, 12, 0, 0, 0, time.UTC), Content: "Body.",
	}
}

// TestCollectDateArchives: one archive per year, month and day that has posts,
// each labelled the way a theme titles the page, newest post first.
func TestCollectDateArchives(t *testing.T) {
	posts := []models.Page{
		datedPost("Old", 2014, time.May, 12),
		datedPost("Newer", 2014, time.May, 20),
		datedPost("June", 2014, time.June, 1),
		datedPost("Next year", 2015, time.January, 2),
	}
	got := collectDateArchives(posts, depthDay)

	paths := map[string]dateArchive{}
	for _, a := range got {
		paths[a.Path] = a
	}
	for _, want := range []string{"2014", "2014/05", "2014/05/12", "2014/05/20", "2014/06", "2015", "2015/01", "2015/01/02"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("missing archive /%s/", want)
		}
	}
	if n := len(paths["2014"].Posts); n != 3 {
		t.Errorf("2014 lists %d posts, want 3", n)
	}
	if n := len(paths["2014/05"].Posts); n != 2 {
		t.Errorf("2014/05 lists %d posts, want 2", n)
	}
	if label := paths["2014/05"].Label; label != "May 2014" {
		t.Errorf("month label = %q", label)
	}
	if label := paths["2014/05/12"].Label; label != "12 May 2014" {
		t.Errorf("day label = %q", label)
	}
	// Newest first, like every other archive.
	if paths["2014/05"].Posts[0].Title != "Newer" {
		t.Errorf("posts not newest-first: %+v", paths["2014/05"].Posts)
	}
	// Deterministic order, so two builds read the same.
	for i := 1; i < len(got); i++ {
		if got[i-1].Path > got[i].Path {
			t.Fatalf("archives out of order: %s before %s", got[i-1].Path, got[i].Path)
		}
	}
}

// TestCollectDateArchivesSkipsUndated: a post with no date belongs to no
// archive — inventing one would publish a page nobody linked. A page is not a
// post and never appears.
func TestCollectDateArchivesSkipsUndated(t *testing.T) {
	posts := []models.Page{
		{Title: "No date", Type: "post", Status: "publish"},
		{Title: "A page", Type: "page", Status: "publish", Date: time.Now()},
	}
	if got := collectDateArchives(posts, depthDay); len(got) != 0 {
		t.Fatalf("nothing to archive, got %+v", got)
	}
}

// TestDateArchiveDepthFollowsPermalinks: a slug-permalink site still publishes
// years and months (WordPress does) but not days, which its URLs never had.
func TestDateArchiveDepthFollowsPermalinks(t *testing.T) {
	g := newTestGen(t, "")
	g.config.PostURLFormat = "slug"
	if got := g.dateArchiveDepth(); got != depthMonth {
		t.Errorf("slug permalinks → month depth, got %v", got)
	}
	g.config.PostURLFormat = ""
	if got := g.dateArchiveDepth(); got != depthDay {
		t.Errorf("dated permalinks → day depth, got %v", got)
	}
}

// TestGenerateDateArchivesOptIn: the default writes nothing — a site that never
// had these URLs must not grow them because it upgraded.
func TestGenerateDateArchivesOptIn(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<h1>{{.Name}}</h1>{{end}}`)
	g.siteData.Posts = []models.Page{datedPost("Hello", 2014, time.May, 12)}

	if err := g.generateDateArchives(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, g, "2014/index.html") {
		t.Fatal("date archives must be opt-in")
	}

	g.config.DateArchives = true
	if err := g.generateDateArchives(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2014/index.html", "2014/05/index.html", "2014/05/12/index.html"} {
		if !fileExists(t, g, want) {
			t.Errorf("missing %s", want)
		}
	}
}

// TestArchiveDataIsUniform: every archive view carries the same keys, and a
// field a template reads but this kind does not fill resolves to nil rather
// than destroying the page (#145).
func TestArchiveDataIsUniform(t *testing.T) {
	g := newTestGen(t, "")
	data := g.archiveData("tag", "Go", models.Category{Name: "Go", Slug: "go"},
		[]models.Page{{Title: "P"}}, singlePagePager(1), "")

	for _, key := range []string{"Site", "Category", "Kind", "Name", "Series",
		"Posts", "Pager", "Lang", "Domain", "Vars", "Data", "ExternalData"} {
		if _, ok := data[key]; !ok {
			t.Errorf("archive data lacks %q", key)
		}
	}
	pager, ok := data["Pager"].(Pager)
	if !ok || pager.Total != 1 || pager.Current != 1 {
		t.Fatalf("an unpaginated archive needs a truthful pager, got %+v", data["Pager"])
	}
	// The whole point: an unknown key is nil, not a build-breaking error.
	if v := data["NoSuchField"]; v != nil {
		t.Fatalf("unknown key = %v, want nil", v)
	}
}
