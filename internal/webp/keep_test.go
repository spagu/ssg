package webp

import (
	"fmt"
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

// TestConvertDirectoryParallel stresses the worker pool: many independent images
// convert concurrently to a byte-identical result (all .webp present, all
// originals removed, exact converted count). The -race build guards the shared
// atomic accumulators.
func TestConvertDirectoryParallel(t *testing.T) {
	withFakeCwebp(t)
	dir := t.TempDir()
	const n = 24
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("img%02d.png", i))
		if err := os.WriteFile(p, []byte("PNGDATA-PNGDATA-PNGDATA"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	converted, saved, err := ConvertDirectory(dir, ConvertOptions{Quality: 60, Quiet: true, Workers: 4})
	if err != nil || converted != n {
		t.Fatalf("converted = %d (want %d), err %v", converted, n, err)
	}
	if saved == 0 {
		t.Errorf("expected some bytes saved across %d images", n)
	}
	for i := 0; i < n; i++ {
		base := filepath.Join(dir, fmt.Sprintf("img%02d", i))
		if _, e := os.Stat(base + ".webp"); e != nil {
			t.Errorf("missing %s.webp: %v", base, e)
		}
		if _, e := os.Stat(base + ".png"); !os.IsNotExist(e) {
			t.Errorf("original %s.png must be removed in replace mode", base)
		}
	}
}
