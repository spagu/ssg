package webp

// A source whose header decodes but whose pixels do not is an error, not an
// empty variant.

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestResizeToPNGTruncatedSource(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	// Keep the signature and IHDR (33 bytes) so DecodeConfig succeeds, and cut
	// the image data off.
	if err := os.WriteFile(src, buf.Bytes()[:40], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resizeToPNG(src, filepath.Join(dir, "out.png"), 32); err == nil {
		t.Error("want an error for a truncated source")
	}
}
