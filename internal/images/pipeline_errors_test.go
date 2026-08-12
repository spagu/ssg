package images

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeTool installs an executable shell stub named like an optional
// encoder, so tool-dependent paths run without the real binary installed.
func writeFakeTool(t *testing.T, dir, name string) {
	t.Helper()
	// #nosec G306 -- test executable stub
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestEncodeAVIFFakeSuccess: with a (stubbed) avifenc on PATH the AVIF encoder
// completes without error — the success contract, minus the real binary.
func TestEncodeAVIFFakeSuccess(t *testing.T) {
	tools := t.TempDir()
	writeFakeTool(t, tools, "avifenc")
	t.Setenv("PATH", tools)
	out, err := os.Create(filepath.Join(t.TempDir(), "out.avif"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	if err := encodeAVIF(out, image.NewRGBA(image.Rect(0, 0, 2, 2)), 50, 6); err != nil {
		t.Errorf("stubbed avifenc must succeed: %v", err)
	}
}

// TestEncodeTempFileFailure: both shell-out encoders fail cleanly when the
// scratch PNG cannot be created next to the target file.
func TestEncodeTempFileFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	tools := t.TempDir()
	writeFakeTool(t, tools, "avifenc")
	writeFakeTool(t, tools, "cwebp")
	t.Setenv("PATH", tools)
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "out.bin")) // open before locking the dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := encodeAVIF(out, img, 50, 6); err == nil {
		t.Error("avif temp file in a read-only dir must error")
	}
	if err := encodeWebP(out, img, 50); err == nil {
		t.Error("webp temp file in a read-only dir must error")
	}
}

// TestOpGuards: an op that slipped past parsing is rejected by both the
// validator and the pipeline executor.
func TestOpGuards(t *testing.T) {
	if err := validateOp("imageProcess", 0, &request{Op: "weird"}); err == nil {
		t.Error("validateOp must reject an unknown op")
	}
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 20, 10, false)
	if _, err := p.run("imageProcess", "img.png", []request{{Op: "weird"}}); err == nil {
		t.Error("run must reject an unknown op at apply time")
	}
}

// TestFitNoUpscaleBothBounds: fit with both bounds beyond the source is a
// no-op without upscale.
func TestFitNoUpscaleBothBounds(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)
	res, err := p.ResizeDict("img.png", map[string]any{"width": 500, "height": 400, "mode": "fit"})
	if err != nil || res.Width != 100 || res.Height != 50 {
		t.Errorf("oversized fit must keep the source size: %+v, %v", res, err)
	}
}

// TestGeometryHelpers: clampFill floors the width side too, clampInt passes an
// in-range value through, and an oversize anchored crop clamps to the source.
func TestGeometryHelpers(t *testing.T) {
	if w, h := clampFill(2, 2, 1, 4000); w != 1 || h < 1 {
		t.Errorf("clampFill width floor = %dx%d", w, h)
	}
	if got := clampInt(3, 1, 5); got != 3 {
		t.Errorf("clampInt in-range = %d, want 3", got)
	}
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)
	res, err := p.CropDict("img.png", map[string]any{"width": 200, "height": 300, "anchor": "center"})
	if err != nil || res.Width != 100 || res.Height != 50 {
		t.Errorf("oversize crop must clamp to source: %+v, %v", res, err)
	}
}

// TestOpacityFilterAndUnknownDefault: the opacity filter runs end to end, and
// applyFilter's defensive default returns the image untouched.
func TestOpacityFilterAndUnknownDefault(t *testing.T) {
	p, src := testEnv(t)
	writePNG(t, filepath.Join(src, "img.png"), 10, 10, true)
	if _, err := p.FilterDict("img.png", []any{map[string]any{"name": "opacity", "amount": 0.5}}, nil); err != nil {
		t.Errorf("opacity filter: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if got := applyFilter(img, &request{Name: "nope"}); got != img {
		t.Error("unknown filter name must pass the image through")
	}
}

// TestOrientationFromTIFFHeaderGuards: an unknown byte order and an IFD offset
// beyond the buffer both fall back to orientation 1.
func TestOrientationFromTIFFHeaderGuards(t *testing.T) {
	if got := orientationFromTIFF([]byte("XX\x2a\x00\x00\x00\x00\x08")); got != 1 {
		t.Errorf("unknown byte order = %d, want 1", got)
	}
	if got := orientationFromTIFF([]byte{'I', 'I', 0x2a, 0x00, 0xff, 0x00, 0x00, 0x00}); got != 1 {
		t.Errorf("IFD beyond buffer = %d, want 1", got)
	}
}

// TestDecodeSourceGuards: a vanished path, a source above max_dimension, and a
// header-only PNG (config parses, pixels do not) each error descriptively.
func TestDecodeSourceGuards(t *testing.T) {
	p, src := testEnv(t)
	if _, _, err := p.decodeSource("t", filepath.Join(t.TempDir(), "gone.png")); err == nil {
		t.Error("decodeSource on a missing path must error")
	}

	writePNG(t, filepath.Join(src, "img.png"), 100, 50, false)
	tiny := New(Config{SourceDirs: []string{src}, OutputDir: t.TempDir(), CacheDir: t.TempDir(),
		MaxDimension: 5, Quiet: true})
	if _, err := tiny.ResizeDict("img.png", map[string]any{"width": 4}); err == nil ||
		!strings.Contains(err.Error(), "max_dimension") {
		t.Errorf("oversized source must trip max_dimension, got: %v", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	// Signature (8) + IHDR chunk (25) parse as config; the pixel data is gone.
	if err := os.WriteFile(filepath.Join(src, "trunc.png"), buf.Bytes()[:33], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ResizeDict("trunc.png", map[string]any{"width": 2}); err == nil ||
		!strings.Contains(err.Error(), "decoding") {
		t.Errorf("truncated PNG must fail at pixel decode, got: %v", err)
	}
}

// TestPublishOutputDirBlocked: an output root occupied by a regular file fails
// the publish copy with a descriptive error.
func TestPublishOutputDirBlocked(t *testing.T) {
	src := t.TempDir()
	writePNG(t, filepath.Join(src, "img.png"), 10, 10, false)
	blocker := filepath.Join(t.TempDir(), "outfile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(Config{SourceDirs: []string{src}, OutputDir: blocker, CacheDir: t.TempDir(), Quiet: true})
	if _, err := p.ResizeDict("img.png", map[string]any{"width": 5}); err == nil ||
		!strings.Contains(err.Error(), "publishing output") {
		t.Errorf("blocked output dir must fail the publish copy, got: %v", err)
	}
}

// TestWithinRootRelMismatch: a relative root against an absolute path cannot be
// related, which surfaces as an error rather than a false negative.
func TestWithinRootRelMismatch(t *testing.T) {
	if _, err := withinRoot(".", "/"); err == nil {
		t.Error("relative root vs absolute path must error")
	}
}
