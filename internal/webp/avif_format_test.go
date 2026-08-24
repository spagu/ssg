package webp

// The AVIF pass must check the decoded format, not the file extension (#198).

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/tiff"
)

// tinyImage is a 2×2 image any encoder can write.
func tinyImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	return img
}

// writeTIFF writes a real TIFF under whatever name it is given.
func writeTIFF(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := tiff.Encode(f, tinyImage(), nil); err != nil {
		t.Fatal(err)
	}
}

// writePNG writes a real PNG.
func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, tinyImage()); err != nil {
		t.Fatal(err)
	}
}

// TestATIFFWearingAPNGNameNeverReachesImaging is the whole point. avifSources
// selects by extension; image.Decode dispatches on magic bytes; importing
// imaging registers the TIFF decoder. Without this check the file reaches
// imaging's scanner, which is where CVE-2023-36308 panics — and imaging has no
// fixed release to upgrade to, so the allowlist is the only mitigation.
func TestATIFFWearingAPNGNameNeverReachesImaging(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png") // the extension lies
	writeTIFF(t, src)

	err := resizeToPNG(src, filepath.Join(dir, "out.png"), 64)
	if err == nil {
		t.Fatal("a TIFF renamed .png must be refused before imaging decodes it")
	}
	if !strings.Contains(err.Error(), "tiff") {
		t.Errorf("the refusal must name the real format, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out.png")); statErr == nil {
		t.Error("nothing must be written for a refused source")
	}
}

// TestARealPNGStillResizes: the guard must not cost the ordinary case.
func TestARealPNGStillResizes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	dst := filepath.Join(dir, "out.png")
	writePNG(t, src)

	if err := resizeToPNG(src, dst, 1); err != nil {
		t.Fatalf("a real PNG must still resize: %v", err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Errorf("no output written: %v", err)
	}
}

// TestAnUnreadableOrUndecodableSourceIsReportedNotPanicked.
func TestAnUnreadableOrUndecodableSourceIsReportedNotPanicked(t *testing.T) {
	dir := t.TempDir()
	if err := resizeToPNG(filepath.Join(dir, "absent.png"), filepath.Join(dir, "o.png"), 64); err == nil {
		t.Error("a missing source must be an error")
	}

	junk := filepath.Join(dir, "junk.png")
	if err := os.WriteFile(junk, []byte("not an image at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resizeToPNG(junk, filepath.Join(dir, "o2.png"), 64); err == nil {
		t.Error("an undecodable source must be an error")
	}
}
