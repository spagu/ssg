package webp

// The conversion paths, driven by stand-in encoders rather than the real ones.
//
// The tests beside these skip when cwebp or avifenc is absent, which is right
// for what they check — that the real tool accepts the arguments we build — and
// wrong for coverage: the same suite then measured four points lower in CI than
// on a developer's machine, and the difference was invisible until a per-package
// floor made it fail. What each conversion does with its inputs and outputs does
// not depend on which binary produced the bytes, so it is exercised here with a
// stub and stays the same everywhere.

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubCwebp puts a cwebp on PATH that honours -o and -resize, writing a file
// whose contents record the width it was asked for.
func stubCwebp(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"out=\"\"\nwidth=\"full\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -o) out=\"$2\"; shift ;;\n" +
		"    -resize) width=\"$2\"; shift 2 ;;\n" +
		"  esac\n" +
		"  shift\n" +
		"done\n" +
		"printf 'RIFFWEBP-%s' \"$width\" > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cwebp"), []byte(script), 0o700); err != nil { // #nosec G306 -- executable test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestConversionEmitsOneVariantPerWidthAndNeverUpscales.
//
// The variants are derived from the ORIGINAL, before it is removed, so quality
// is best (ASSET-004) — and a width at or above the original is skipped,
// because a 1600w derived from a 400px picture is a bigger file of the same
// picture.
func TestConversionEmitsOneVariantPerWidthAndNeverUpscales(t *testing.T) {
	stubCwebp(t)
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "hero.png"), 800, 600)

	converted, _, err := ConvertDirectory(dir, ConvertOptions{
		Quality: 75, Quiet: true, Sizes: []int{400, 800, 1600, 0, -20},
	})
	if err != nil || converted != 1 {
		t.Fatalf("converted = %d, err = %v", converted, err)
	}
	if body := mustReadFile(t, filepath.Join(dir, "hero.webp")); !strings.HasSuffix(body, "full") {
		t.Errorf("the full-size conversion was resized: %q", body)
	}
	if body := mustReadFile(t, filepath.Join(dir, "hero-400.webp")); !strings.HasSuffix(body, "400") {
		t.Errorf("the 400w variant was not resized to 400: %q", body)
	}
	for _, absent := range []string{"hero-800.webp", "hero-1600.webp", "hero-0.webp", "hero--20.webp"} {
		if _, err := os.Stat(filepath.Join(dir, absent)); err == nil {
			t.Errorf("%s should not exist: it would be an upscale or a nonsense width", absent)
		}
	}
	// Replace mode removes the original once the .webp is confirmed on disk.
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); err == nil {
		t.Error("replace mode should have removed the original")
	}
}

// TestAFailedVariantDoesNotFailThePass: one width that will not encode must not
// cost the site every other size, or a single awkward image takes the build
// down with it.
func TestAFailedVariantDoesNotFailThePass(t *testing.T) {
	stubCwebp(t)
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "hero.png"), 800, 600)
	// A directory where the 400w variant has to go: the encoder cannot write it.
	if err := os.MkdirAll(filepath.Join(dir, "hero-400.webp"), 0o750); err != nil {
		t.Fatal(err)
	}

	out := captureConversionOutput(t, func() {
		if converted, _, err := ConvertDirectory(dir, ConvertOptions{
			Quality: 75, Sizes: []int{400, 600},
		}); err != nil || converted != 1 {
			t.Errorf("converted = %d, err = %v", converted, err)
		}
	})
	if !strings.Contains(out, "400w variant") {
		t.Errorf("the failed variant should be named:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero-600.webp")); err != nil {
		t.Errorf("the other width should still have been written: %v", err)
	}
}

// TestConversionReportsProgressAndSkipsWhatIsAlreadyDone.
//
// The header, the per-image line and the skip count are the only feedback a
// long image pass gives, and a build that prints nothing for a minute reads as
// a hang.
func TestConversionReportsProgressAndSkipsWhatIsAlreadyDone(t *testing.T) {
	stubCwebp(t)
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "new.png"), 40, 30)
	pngFixture(t, filepath.Join(dir, "done.png"), 40, 30)
	if err := os.WriteFile(filepath.Join(dir, "done.webp"), []byte("RIFFWEBP"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := captureConversionOutput(t, func() {
		// Keep mode, so the second pass below still has originals to look at.
		if _, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 75, KeepOriginal: true}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"Converting 1 images", "Skipping 1 images", "Converting 1/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should mention %q:\n%s", want, out)
		}
	}

	// A second pass has nothing left to do and says so rather than printing a
	// header for zero images.
	out = captureConversionOutput(t, func() {
		if converted, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 75, KeepOriginal: true}); err != nil || converted != 0 {
			t.Errorf("converted = %d, err = %v", converted, err)
		}
	})
	if !strings.Contains(out, "already converted") {
		t.Errorf("output = %q", out)
	}
}

// TestAVIFConversionResizesFromTheOriginalAndKeepsIt.
//
// AVIF derives each width by decoding the source and re-encoding, so the
// resizer has to produce something avifenc accepts, and the original is never
// removed: AVIF is an addition to the page, not a replacement for the file.
func TestAVIFConversionResizesFromTheOriginalAndKeepsIt(t *testing.T) {
	fakeEncoder(t)
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "hero.png"), 800, 600)

	converted, err := ConvertDirectoryAVIF(dir, AVIFOptions{Quality: 50, Quiet: true, Sizes: []int{400, 1600}})
	if err != nil {
		t.Fatal(err)
	}
	if converted == 0 {
		t.Fatal("nothing was converted")
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.avif")); err != nil {
		t.Errorf("the full-size AVIF is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero-400.avif")); err != nil {
		t.Errorf("the 400w AVIF is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero-1600.avif")); err == nil {
		t.Error("1600w is larger than the original and must not be upscaled")
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); err != nil {
		t.Errorf("the original must survive an AVIF pass: %v", err)
	}
}

// TestAVIFSkipsWhatItAlreadyMade, so a rebuild does not re-encode the whole
// media directory.
func TestAVIFSkipsWhatItAlreadyMade(t *testing.T) {
	fakeEncoder(t)
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "hero.png"), 200, 100)
	if _, err := ConvertDirectoryAVIF(dir, AVIFOptions{Quality: 50, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	again, err := ConvertDirectoryAVIF(dir, AVIFOptions{Quality: 50, Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("a second pass re-encoded %d image(s)", again)
	}
}

// captureConversionOutput collects what a pass printed, so the progress a long
// image conversion gives can be asserted rather than eyeballed.
func captureConversionOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		body, _ := io.ReadAll(r)
		done <- string(body)
	}()
	fn()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}
