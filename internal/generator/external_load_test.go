package generator

// loadExternalSources and mergeCMSContent as an operator sees them: the
// per-source report, the cache-hit counter, a cache that cannot be cleared and
// a site with no authors of its own.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
)

func TestLoadExternalSourcesReportsAndCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":1}]`)
	}))
	defer srv.Close()
	yes := true
	g := newTestGen(t, "")
	g.profile = NewProfile(ProfileText)
	g.config.ExternalSources = externalsource.Config{Enabled: true, CacheDir: t.TempDir(),
		Sources: map[string]externalsource.SourceConfig{
			"api": {Type: "http", URL: srv.URL + "/items.json", AllowHTTP: &yes, AllowPrivate: &yes, CacheTTL: "1h"},
		}}
	for range 2 { // the second load is served from the disk cache
		out, err := capture(t, func() error { return g.loadExternalSources() })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "🔌 api (http, json, 1 records)") {
			t.Errorf("report:\n%s", out)
		}
	}
	counters := map[string]int64{}
	for _, c := range g.profile.Counters() {
		counters[c.Name] = c.Value
	}
	if counters["external sources"] != 2 || counters["external cache hits"] != 1 {
		t.Errorf("counters = %v", counters)
	}
}

// TestLoadExternalSourcesCannotClearTheCache: --clear-external-cache that
// fails stops the build rather than serving what it was told to drop.
func TestLoadExternalSourcesCannotClearTheCache(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes a read-only directory's entries")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	mustWrite(t, filepath.Join(locked, "cache", "entry"), "x")
	if err := os.Chmod(locked, 0o500); err != nil { // #nosec G302 -- a test dir made unwritable on purpose
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) // #nosec G302 -- restored so TempDir can clean up
	g := newTestGen(t, "")
	g.config.ExternalSources = externalsource.Config{Enabled: true, ClearCache: true, CacheDir: filepath.Join(locked, "cache")}
	if err := g.loadExternalSources(); err == nil || !strings.Contains(err.Error(), "clearing external-source cache") {
		t.Fatalf("err = %v", err)
	}
}

// TestMergeCMSContentIntoASiteWithoutAuthors: the imported authors become
// the site's, and the merge is reported.
func TestMergeCMSContentIntoASiteWithoutAuthors(t *testing.T) {
	g := newTestGen(t, "")
	g.cmsImports = []*externalsource.CMSImportResult{{Source: "wp",
		Posts: []models.Page{{Title: "a"}}, Authors: map[int]models.Author{1: {Name: "Ann"}}}}
	out, _ := capture(t, func() error { g.mergeCMSContent(); return nil })
	if g.siteData.Authors[1].Name != "Ann" || g.siteData.Posts[0].ExternalSource != "wp" {
		t.Errorf("merged: %+v %+v", g.siteData.Authors, g.siteData.Posts)
	}
	if !strings.Contains(out, "Merged CMS content: 0 pages, 1 posts, 1 authors") {
		t.Errorf("report:\n%s", out)
	}
}
