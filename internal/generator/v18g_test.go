package generator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// mockMddbServer serves the minimal mddb HTTP surface loadContentFromMddb touches:
// a healthy /v1/health and an empty /v1/search result set.
func mockMddbServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/search":
			w.Header().Set("X-Total-Count", "0")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLoadContentFromMddbSuccess exercises the full mddb success path (health,
// GetByType for pages+posts, ToPages, loadMetadataFromMddb) against a mock server.
func TestLoadContentFromMddbSuccess(t *testing.T) {
	srv := mockMddbServer(t)
	g := newTestGen(t, "")
	g.config.Mddb.Enabled = true
	g.config.Mddb.URL = srv.URL
	g.config.Mddb.Protocol = "http"
	g.config.Mddb.Collection = "content"
	g.config.Mddb.Lang = "en_US"
	g.config.Mddb.Timeout = 5
	g.config.Mddb.BatchSize = 100

	if err := g.loadContentFromMddb(); err != nil {
		t.Fatalf("loadContentFromMddb against mock server: %v", err)
	}
	if len(g.siteData.Posts) != 0 || len(g.siteData.Pages) != 0 {
		t.Errorf("expected empty content from empty mock, got %d posts / %d pages",
			len(g.siteData.Posts), len(g.siteData.Pages))
	}
}

// TestLoadContentViaLoadContentMddb drives the loadContent dispatcher through the
// mddb branch so finalizeLoadedContent also runs for a mddb source.
func TestLoadContentViaLoadContentMddb(t *testing.T) {
	srv := mockMddbServer(t)
	g := newTestGen(t, "")
	g.config.Mddb.Enabled = true
	g.config.Mddb.URL = srv.URL
	g.config.Mddb.Protocol = "http"
	g.config.Mddb.Collection = "content"
	g.config.Mddb.Timeout = 5
	if err := g.loadContent(); err != nil {
		t.Fatalf("loadContent (mddb): %v", err)
	}
}

// mockMddbServerWithDoc returns one rich document for every search so the ToPages
// conversion loops and the category/media/user extractors all run.
func mockMddbServerWithDoc(t *testing.T) *httptest.Server {
	t.Helper()
	doc := `[{"id":"d1","key":"one","lang":"en_US","contentMd":"# Body\n\ntext",` +
		`"meta":{"title":["One"],"id":[1],"slug":["one"],"type":["post"],` +
		`"status":["publish"],"date":["2024-01-02"],"author":[1],"name":["News"],` +
		`"description":["desc"],"url":["/media/x.png"]},"addedAt":1700000000,"updatedAt":1700000100}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/search":
			w.Header().Set("X-Total-Count", "1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(doc))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLoadContentFromMddbWithDocs covers the document-conversion loops in
// loadContentFromMddb and loadMetadataFromMddb (extract* helpers).
func TestLoadContentFromMddbWithDocs(t *testing.T) {
	srv := mockMddbServerWithDoc(t)
	g := newTestGen(t, "")
	// loadMetadataFromMddb populates these maps; New() would init them in production.
	g.siteData.Media = map[int]models.MediaItem{}
	g.siteData.Authors = map[int]models.Author{}
	g.config.Mddb.Enabled = true
	g.config.Mddb.URL = srv.URL
	g.config.Mddb.Protocol = "http"
	g.config.Mddb.Collection = "content"
	g.config.Mddb.Timeout = 5
	g.config.Mddb.BatchSize = 100
	if err := g.loadContentFromMddb(); err != nil {
		t.Fatalf("loadContentFromMddb with docs: %v", err)
	}
	if len(g.siteData.Posts) == 0 {
		t.Error("expected at least one post from mock doc")
	}
}
