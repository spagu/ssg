package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// TestEnsureWithinOutput verifies the defense-in-depth guard against writing
// outside the configured output directory (SEC-001).
func TestEnsureWithinOutput(t *testing.T) {
	g := &Generator{config: Config{OutputDir: "/tmp/site/output"}}
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"inside", "/tmp/site/output/blog/index.html", false},
		{"root itself", "/tmp/site/output", false},
		{"escape via dotdot", "/tmp/site/output/../../etc/passwd", true},
		{"sibling prefix trick", "/tmp/site/output-evil/x.html", true},
		{"absolute elsewhere", "/etc/passwd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := g.ensureWithinOutput(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ensureWithinOutput(%q) err=%v, wantErr=%v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestFixMediaPaths_EmptyMediaFile is a regression test for GO-001: an empty
// MediaDetails.File must not panic (filepath.Base("") == "." would previously
// trigger a slice-bounds-out-of-range panic).
func TestFixMediaPaths_EmptyMediaFile(t *testing.T) {
	media := map[int]models.MediaItem{
		1048: {ID: 1048}, // MediaDetails.File intentionally empty
	}
	content := `<img class="wp-image-1048" src="http://old.example/img.jpg">`

	// Must not panic and must return content unchanged for the empty-file case.
	got := fixMediaPaths(content, media)
	if got != content {
		t.Errorf("expected content unchanged for empty media file, got: %q", got)
	}
}

// TestFixMediaPaths_RewritesWithFilename verifies the happy path still rewrites
// WordPress URLs when a real media filename is present.
func TestFixMediaPaths_RewritesWithFilename(t *testing.T) {
	media := map[int]models.MediaItem{1: {ID: 1}}
	m := media[1]
	m.MediaDetails.File = "2026/01/cow.jpg"
	media[1] = m

	content := `<img class="wp-image-1" src="http://old.example/wp/2026/01/cow-scaled.jpg">`
	got := fixMediaPaths(content, media)
	if !strings.Contains(got, "/media/1_cow.jpg") {
		t.Errorf("expected rewritten local path, got: %q", got)
	}
}

// TestExtractMediaFromDoc_MediaDetails is a regression test for GO-006: the
// mddb extractor must populate MediaDetails (file/width/height).
func TestExtractMediaFromDoc_MediaDetails(t *testing.T) {
	doc := mddb.Document{
		Key: "cow",
		Metadata: map[string]any{
			"id":         float64(42),
			"media_type": "image",
			"mime_type":  "image/jpeg",
			"source_url": "https://ex/cow.jpg",
			"media_details": map[string]interface{}{
				"width":  float64(800),
				"height": float64(600),
				"file":   "2026/01/cow.jpg",
			},
		},
	}
	got := extractMediaFromDoc(doc)
	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}
	if got.MediaDetails.File != "2026/01/cow.jpg" {
		t.Errorf("File = %q, want 2026/01/cow.jpg", got.MediaDetails.File)
	}
	if int(got.MediaDetails.Width) != 800 || int(got.MediaDetails.Height) != 600 {
		t.Errorf("dimensions = %dx%d, want 800x600", int(got.MediaDetails.Width), int(got.MediaDetails.Height))
	}
	// filepath.Base of a populated File must be non-empty (feeds fixMediaPaths safely).
	if filepath.Base(got.MediaDetails.File) == "." {
		t.Error("populated File should yield a real base name")
	}
}
