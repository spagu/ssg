package webp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// withFakeCwebp puts a stub cwebp on PATH that writes a tiny file to the -o
// target, so conversion tests run without the real encoder.
func withFakeCwebp(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake cwebp stub is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nout=\"\"\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then out=\"$2\"; shift; fi\n  shift\ndone\nprintf 'RIFFWEBP' > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cwebp"), []byte(script), 0o755); err != nil { // #nosec G306 -- executable test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestConvertKeepOriginal: keep mode emits the .webp NEXT TO the original;
// replace mode (default) removes the original — including the skip branch for
// already-converted images (GO-052).
func TestConvertKeepOriginal(t *testing.T) {
	withFakeCwebp(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(img, []byte("PNGDATA-PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	converted, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true, KeepOriginal: true})
	if err != nil || converted != 1 {
		t.Fatalf("convert: %d, %v", converted, err)
	}
	if _, err := os.Stat(img); err != nil {
		t.Fatal("keep mode must preserve the original")
	}
	if _, err := os.Stat(filepath.Join(dir, "logo.webp")); err != nil {
		t.Fatal("keep mode must emit the .webp sibling")
	}

	// Second run (webp exists): the skip branch must also keep the original.
	if _, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true, KeepOriginal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(img); err != nil {
		t.Fatal("keep mode skip branch must preserve the original")
	}

	// Default replace mode removes the leftover original on the skip branch.
	if _, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(img); !os.IsNotExist(err) {
		t.Fatal("replace mode must remove the original (historical behaviour)")
	}
}

// TestConvertReplaceRemovesAfterConversion pins the historical default:
// a fresh conversion in replace mode deletes the source image.
func TestConvertReplaceRemovesAfterConversion(t *testing.T) {
	withFakeCwebp(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(img, []byte("JPEGDATA-JPEGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	converted, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true})
	if err != nil || converted != 1 {
		t.Fatalf("convert: %d, %v", converted, err)
	}
	if _, err := os.Stat(img); !os.IsNotExist(err) {
		t.Fatal("replace mode must remove the original after conversion")
	}
	if _, err := os.Stat(filepath.Join(dir, "photo.webp")); err != nil {
		t.Fatal("webp output missing")
	}
}
