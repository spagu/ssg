package generator

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/notify"
)

// TestSendNotifications covers #1.8.16 f2: published posts are announced with a
// canonical URL and excerpt/description fallback; no notifier is a no-op.
func TestSendNotifications(t *testing.T) {
	var got notify.Post
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
	}))
	defer srv.Close()

	g := newTestGen(t, "")
	g.config.Domain = "example.com"
	g.config.Notify = notify.New([]notify.Dest{{Name: "hook", URL: srv.URL, AllowPrivate: true}},
		filepath.Join(t.TempDir(), "s.json"))
	g.siteData.Posts = []models.Page{{
		Slug: "hello", Title: "Hello", Type: "post", URLFormat: "slug",
		Description: "desc fallback", Date: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Tags: []string{"go"},
	}}
	if err := g.sendNotifications(); err != nil {
		t.Fatalf("sendNotifications: %v", err)
	}
	if got.URL != "https://example.com/hello/" || got.Title != "Hello" {
		t.Errorf("payload URL/title = %q / %q", got.URL, got.Title)
	}
	if got.Excerpt != "desc fallback" || got.Date != "2026-08-01" {
		t.Errorf("payload excerpt/date = %q / %q", got.Excerpt, got.Date)
	}

	// No notifier configured → no-op, no error.
	g2 := newTestGen(t, "")
	g2.siteData.Posts = []models.Page{{Slug: "x", Type: "post"}}
	if err := g2.sendNotifications(); err != nil {
		t.Errorf("no-notifier must be a no-op: %v", err)
	}
}
