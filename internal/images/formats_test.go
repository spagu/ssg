package images

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/tiff"
)

// SEC-013 regression: image.Decode sniffs magic bytes, so a TIFF renamed
// "photo.png" used to decode (the TIFF decoder is registered transitively by
// github.com/disintegration/imaging) and reach imaging's transforms — the code
// path that panics in CVE-2023-36308, for which imaging has no fixed release.
// Both entry points must now reject it on format, before any pixel work.

// writeTIFF creates a valid TIFF at path, whatever the path's extension says.
func writeTIFF(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := tiff.Encode(f, image.NewNRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedDecodeFormatRejected(t *testing.T) {
	p, src := testEnv(t)
	writeTIFF(t, filepath.Join(src, "photo.png"), 8, 8)

	if _, err := p.Info("photo.png"); err == nil || !strings.Contains(err.Error(), "not a supported image format") {
		t.Errorf("Info on a disguised TIFF = %v, want a format rejection", err)
	}
	_, err := p.ResizeDict("photo.png", map[string]any{"width": 4})
	if err == nil || !strings.Contains(err.Error(), "not a supported image format") {
		t.Errorf("ResizeDict on a disguised TIFF = %v, want a format rejection", err)
	}
	// The supported formats still pass the same check.
	writePNG(t, filepath.Join(src, "real.png"), 8, 8, false)
	if _, err := p.Info("real.png"); err != nil {
		t.Errorf("Info on a real PNG = %v, want success", err)
	}
}

func TestCheckDecodable(t *testing.T) {
	for _, format := range []string{"jpeg", "png", "gif", "webp"} {
		if err := checkDecodable("h", "s", format); err != nil {
			t.Errorf("checkDecodable(%q) = %v, want nil", format, err)
		}
	}
	for _, format := range []string{"tiff", "bmp", ""} {
		if err := checkDecodable("h", "s", format); err == nil {
			t.Errorf("checkDecodable(%q) = nil, want a rejection", format)
		}
	}
}
