package generator

import (
	"fmt"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// TestTmplRelated covers #1.8.16: in-memory ranking by shared keyword/tag count,
// then recency, then slug; excludes self and zero-overlap posts; respects n.
func TestTmplRelated(t *testing.T) {
	g := newTestGen(t, "")
	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	cur := models.Page{Slug: "cur", Tags: []string{"go", "web"}, Keywords: "ssg, static"}
	g.siteData.Posts = []models.Page{
		cur,
		{Slug: "two", Tags: []string{"go", "web"}, Keywords: "ssg", Date: day(1)}, // overlap 3
		{Slug: "one", Tags: []string{"go"}, Date: day(2)},                         // overlap 1
		{Slug: "oneb", Tags: []string{"web"}, Date: day(2)},                       // overlap 1, same date
		{Slug: "none", Tags: []string{"rust"}, Date: day(3)},                      // overlap 0 -> excluded
	}
	got := g.tmplRelated(cur, 5)
	order := []string{}
	for _, p := range got {
		order = append(order, p.Slug)
	}
	// two (3) first; then the two overlap-1 posts by date desc (equal) then slug asc.
	want := []string{"two", "one", "oneb"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("related order = %v, want %v", order, want)
	}
	// n cap.
	if g2 := g.tmplRelated(cur, 1); len(g2) != 1 || g2[0].Slug != "two" {
		t.Errorf("n=1 = %v", g2)
	}
	// No keywords/tags → nil.
	if r := g.tmplRelated(models.Page{Slug: "x"}, 5); r != nil {
		t.Errorf("no-keyword page must yield nil, got %v", r)
	}
}

type searchMddb struct {
	mddb.MddbClient // embedded: only Search/Close are called
	docs            []mddb.Document
	lastFilter      map[string][]any
	err             error
}

func (f *searchMddb) Search(req mddb.SearchRequest) ([]mddb.Document, int, error) {
	f.lastFilter = req.FilterMeta
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.docs, len(f.docs), nil
}
func (f *searchMddb) Close() error { return nil }

// TestTmplRelatedFromMddb covers the live variant: it filters by the page's tags,
// converts documents to posts, excludes self and caps at n.
func TestTmplRelatedFromMddb(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Mddb.Enabled = true
	fake := &searchMddb{docs: []mddb.Document{
		{Key: "cur", Metadata: map[string]any{"slug": "cur", "title": "Cur", "type": "post"}},
		{Key: "r1", Metadata: map[string]any{"slug": "r1", "title": "R1", "type": "post"}},
		{Key: "r2", Metadata: map[string]any{"slug": "r2", "title": "R2", "type": "post"}},
		{Key: "r3", Metadata: map[string]any{"slug": "r3", "title": "R3", "type": "post"}},
	}}
	g.relatedMddb = fake
	g.relatedMddbOnce.Do(func() {}) // mark the lazy init done so the fake is used

	page := models.Page{Slug: "cur", Tags: []string{"go", "web"}}
	got := g.tmplRelatedFromMddb(page, 2)
	if len(got) != 2 || got[0].Slug != "r1" || got[1].Slug != "r2" {
		t.Fatalf("mddb related = %v (self excluded, capped at 2)", slugsOf(got))
	}
	if v, ok := fake.lastFilter["tags"]; !ok || len(v) != 2 {
		t.Errorf("filter should be by tags, got %v", fake.lastFilter)
	}
	// A search error yields nil (logged, non-fatal).
	g.relatedMddb = &searchMddb{err: fmt.Errorf("boom")}
	if g.tmplRelatedFromMddb(page, 2) != nil {
		t.Error("search error must yield nil")
	}
	// mddb disabled → nil.
	g.config.Mddb.Enabled = false
	if r := g.tmplRelatedFromMddb(page, 2); r != nil {
		t.Errorf("disabled mddb must yield nil")
	}
}

func slugsOf(pages []models.Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Slug
	}
	return out
}

// TestRelatedMddbClientAndFilter covers the filter fallback, the lazy client
// build/close, and the guard paths of the mddb related helper.
func TestRelatedMddbClientAndFilter(t *testing.T) {
	// Keywords-only fallback and the empty case.
	if f := relatedFilter(models.Page{Keywords: "go, web"}); len(f["keywords"]) != 2 {
		t.Errorf("keyword filter = %v", f)
	}
	if relatedFilter(models.Page{}) != nil {
		t.Error("no tags/keywords → nil filter")
	}

	// Lazy client builds (HTTP, no connection) and close clears it (idempotent).
	g := newTestGen(t, "")
	g.config.Mddb.URL = "http://localhost:9"
	if g.relatedMddbClient() == nil {
		t.Fatal("client should build from config")
	}
	g.closeRelatedMddb()
	if g.relatedMddb != nil {
		t.Error("close must clear the client")
	}
	g.closeRelatedMddb() // nil branch, no panic

	// Guards: n<=0 and a page with no keywords both yield nil.
	g2 := newTestGen(t, "")
	g2.config.Mddb.Enabled = true
	if g2.tmplRelatedFromMddb(models.Page{Tags: []string{"go"}}, 0) != nil {
		t.Error("n<=0 → nil")
	}
	if g2.tmplRelatedFromMddb(models.Page{}, 5) != nil {
		t.Error("no keywords → nil")
	}
}
