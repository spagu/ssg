package main

// The image and packaging passes on an unusual machine (1.8.60).
//
// Everything here pins a path the build takes when something outside the
// generator is missing or refuses: no encoder on PATH, a page that cannot be
// read or rewritten, a vanished file, a sink that stops accepting bytes. Those
// are the paths a green suite never walks and a user hits on their first
// machine that is not the developer's, and each of them is the difference
// between a clear message and a silently truncated deployment.

import (
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// icovStubTools puts an executable of each name on a PATH containing nothing
// else, so a test decides for itself whether an optional encoder is "installed"
// instead of inheriting the answer from the machine running it. The stub exits
// 0 and writes an empty file at its last argument — the contract every encoder
// here has with its caller (source in, file out).
func icovStubTools(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		script := "#!/bin/sh\nfor a; do last=\"$a\"; done\n: > \"$last\"\n"
		// #nosec G306 -- a stub the test must be able to execute
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// icovNoTools empties PATH so LookPath fails for every optional encoder.
func icovNoTools(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// icovCaptureStdout collects what a build step printed for the operator.
func icovCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// icovWrite writes one output-tree file with an explicit mode.
func icovWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // umask must not soften a deliberate mode
		t.Fatal(err)
	}
}

// icovPNG writes a real PNG the encoders and the width probe can decode.
func icovPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7 % 256), G: uint8(y * 11 % 256), B: 90, A: 255})
		}
	}
	f, err := os.Create(path) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// icovJPEG writes a real JPEG beside the PNG, so a conversion test covers both
// source formats the pass claims to handle.
func icovJPEG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(x % 256), B: uint8(y % 256), A: 255})
		}
	}
	f, err := os.Create(path) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// icovNoise returns deterministic incompressible bytes: a sink only fails on a
// buffered writer once real bytes reach it, and zeros would never leave the
// deflate window.
func icovNoise(n int) []byte {
	b := make([]byte, n)
	rng := rand.New(rand.NewSource(1)) // #nosec G404 -- test fixture, determinism is the point
	_, _ = rng.Read(b)
	return b
}

// icovFailingWriter refuses every write: the full disk an archive meets
// halfway through.
type icovFailingWriter struct{}

func (icovFailingWriter) Write(p []byte) (int, error) { return 0, errors.New("sink refused") }

// icovSkipIfRoot skips a test that relies on file permissions being enforced.
func icovSkipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
}

// TestWantsFormatIgnoresSpacingAndCase: image_formats comes from hand-written
// YAML, where "AVIF" and " webp " are what people actually type. Matching them
// literally would silently drop the format the operator asked for.
func TestWantsFormatIgnoresSpacingAndCase(t *testing.T) {
	cfg := &config.Config{ImageFormats: []string{" WebP ", "AVIF"}}
	for _, name := range []string{"webp", "avif"} {
		if !wantsFormat(cfg, name) {
			t.Errorf("wantsFormat(%q) = false, want true for %v", name, cfg.ImageFormats)
		}
	}
	if wantsFormat(cfg, "jxl") {
		t.Error("wantsFormat(jxl) = true, but image_formats never names it")
	}
}

// TestRunAVIFWarnsInsteadOfFailingWhenTheEncoderIsMissing: a missing optional
// tool must not fail a build that has already produced a whole site — it warns,
// names the packages that install it, and leaves the WebP and the originals.
func TestRunAVIFWarnsInsteadOfFailingWhenTheEncoderIsMissing(t *testing.T) {
	icovNoTools(t)
	cfg := &config.Config{ImageFormats: []string{"avif"}, OutputDir: t.TempDir()}
	var err error
	out := icovCaptureStdout(t, func() { err = runAVIF(cfg) })
	if err != nil {
		t.Fatalf("a missing avifenc must not fail the build: %v", err)
	}
	for _, want := range []string{"avifenc is not installed", "libavif-bin", "still publishes WebP"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning missing %q, got:\n%s", want, out)
		}
	}
	// --quiet means quiet: no advice on a stream the operator is parsing.
	quiet := &config.Config{ImageFormats: []string{"avif"}, OutputDir: cfg.OutputDir, Quiet: true}
	if out := icovCaptureStdout(t, func() { _ = runAVIF(quiet) }); out != "" {
		t.Errorf("quiet build printed %q", out)
	}
}

// TestRunAVIFReportsHowManyDerivativesItWrote: the AVIF pass is invisible
// otherwise — without the summary an operator cannot tell an encoder that ran
// from one that silently made nothing.
func TestRunAVIFReportsHowManyDerivativesItWrote(t *testing.T) {
	out := t.TempDir()
	icovPNG(t, filepath.Join(out, "hero.png"), 24, 16)
	icovStubTools(t, "avifenc")
	cfg := &config.Config{ImageFormats: []string{"avif"}, OutputDir: out}
	var err error
	printed := icovCaptureStdout(t, func() { err = runAVIF(cfg) })
	if err != nil {
		t.Fatalf("runAVIF: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(out, "hero.avif")); statErr != nil {
		t.Fatalf("the encoder was never asked for hero.avif: %v", statErr)
	}
	if !strings.Contains(printed, "Wrote 1 AVIF file(s)") {
		t.Errorf("summary missing the count, got %q", printed)
	}
}

// TestRunWebPStopsWhenCwebpIsMissing: --webp without the encoder must fail
// loudly with installation advice, not publish a site whose pages already
// reference .webp files that were never made.
func TestRunWebPStopsWhenCwebpIsMissing(t *testing.T) {
	icovNoTools(t)
	cfg := &config.Config{WebP: true, Quiet: true, OutputDir: t.TempDir()}
	err := runWebP(cfg, generator.NewProfile(generator.ProfileText))
	if err == nil || !strings.Contains(err.Error(), "converting to WebP") {
		t.Fatalf("missing cwebp must fail the images phase, got %v", err)
	}
	if !strings.Contains(err.Error(), "cwebp tool not found") {
		t.Errorf("error must say which tool is missing, got %v", err)
	}
}

// TestRunWebPReportsAPageItCannotRead: reference rewriting reads every page it
// converted for. A page it cannot open means the published HTML still points at
// deleted originals, so the phase must fail rather than return success.
func TestRunWebPReportsAPageItCannotRead(t *testing.T) {
	icovSkipIfRoot(t)
	icovStubTools(t, "cwebp")
	out := t.TempDir()
	icovWrite(t, filepath.Join(out, "index.html"), "<html></html>", 0o000)
	cfg := &config.Config{WebP: true, Quiet: true, OutputDir: out}
	err := runWebP(cfg, generator.NewProfile(generator.ProfileText))
	if err == nil || !strings.Contains(err.Error(), "updating image references") {
		t.Fatalf("an unreadable page must fail reference rewriting, got %v", err)
	}
}

// TestRunWebPReportsAPageItCannotRewriteWithSrcset: the srcset pass is the
// second writer of the same file. A read-only page fails only there, and
// swallowing that would ship pages without the responsive variants on disk.
func TestRunWebPReportsAPageItCannotRewriteWithSrcset(t *testing.T) {
	icovSkipIfRoot(t)
	icovStubTools(t, "cwebp")
	out := t.TempDir()
	icovWrite(t, filepath.Join(out, "a-400.webp"), "", 0o644)
	icovWrite(t, filepath.Join(out, "index.html"), `<img src="a.webp">`, 0o444)
	cfg := &config.Config{WebP: true, Quiet: true, OutputDir: out, ImageSizes: []int{400}}
	err := runWebP(cfg, generator.NewProfile(generator.ProfileText))
	if err == nil || !strings.Contains(err.Error(), "emitting responsive srcset") {
		t.Fatalf("a read-only page must fail the srcset pass, got %v", err)
	}
}

// TestRunWebPReportsAPageItCannotOfferAVIFIn: the <picture> wrap runs last, on
// pages every earlier pass accepted. Its failure is the one that would
// otherwise be reported as a clean build with no AVIF anywhere.
func TestRunWebPReportsAPageItCannotOfferAVIFIn(t *testing.T) {
	icovSkipIfRoot(t)
	icovStubTools(t, "cwebp", "avifenc")
	out := t.TempDir()
	icovWrite(t, filepath.Join(out, "a.webp"), "", 0o644)
	icovWrite(t, filepath.Join(out, "a.avif"), "", 0o644)
	icovWrite(t, filepath.Join(out, "index.html"), `<img src="a.webp">`, 0o444)
	cfg := &config.Config{ImageFormats: []string{"avif"}, Quiet: true, OutputDir: out}
	err := runWebP(cfg, generator.NewProfile(generator.ProfileText))
	if err == nil || !strings.Contains(err.Error(), "offering AVIF via <picture>") {
		t.Fatalf("a read-only page must fail the <picture> pass, got %v", err)
	}
}

// TestRunWebPConvertsRealImagesAndReportsTheSaving: the happy path with the
// real encoder — PNG and JPEG both become .webp, the page that referenced them
// is rewritten, and the operator is told how much the build saved. Skipped
// where cwebp is not installed, since there is nothing to drive.
func TestRunWebPConvertsRealImagesAndReportsTheSaving(t *testing.T) {
	out := t.TempDir()
	icovPNG(t, filepath.Join(out, "hero.png"), 64, 48)
	icovJPEG(t, filepath.Join(out, "photo.jpg"), 64, 48)
	icovWrite(t, filepath.Join(out, "index.html"),
		`<img src="hero.png"><img src="photo.jpg">`, 0o644)
	cfg := &config.Config{WebP: true, OutputDir: out, WebPQuality: 60}
	var err error
	printed := icovCaptureStdout(t, func() {
		err = runWebP(cfg, generator.NewProfile(generator.ProfileText))
	})
	if err != nil {
		if strings.Contains(err.Error(), "cwebp tool not found") {
			t.Skip("cwebp is not installed; the conversion itself cannot be exercised")
		}
		t.Fatalf("runWebP: %v", err)
	}
	for _, name := range []string{"hero.webp", "photo.webp"} {
		if _, statErr := os.Stat(filepath.Join(out, name)); statErr != nil {
			t.Errorf("%s was not produced: %v", name, statErr)
		}
	}
	page, readErr := os.ReadFile(filepath.Join(out, "index.html")) // #nosec G304 -- test output
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(page), "hero.png") || strings.Contains(string(page), "photo.jpg") {
		t.Errorf("page still references the originals: %s", page)
	}
	if !strings.Contains(printed, "Converted 2 images") {
		t.Errorf("build did not report the conversion, got %q", printed)
	}
}

// TestImageCacheGCFailureWarnsWithoutFailingTheBuild: GC runs after a build
// that already succeeded. A broken cache directory is worth a warning on
// stderr, never the loss of a finished site.
func TestImageCacheGCFailureWarnsWithoutFailingTheBuild(t *testing.T) {
	t.Chdir(t.TempDir())
	gen, err := generator.New(generator.Config{
		Source: "s", Template: "simple", Domain: "example.com",
		ContentDir: "content", TemplatesDir: "templates", OutputDir: "out", Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A file where the cache directory belongs: reading it is ENOTDIR, which
	// is exactly what a cache root clobbered by another tool looks like.
	icovWrite(t, filepath.Join(".ssg-cache", "images"), "not a directory", 0o644)
	out := captureStderr(t, func() { runImagesGC(gen, &config.Config{ImagesGC: true}) })
	if !strings.Contains(out, "Image cache GC") {
		t.Errorf("a failed GC must warn on stderr, got %q", out)
	}
}
