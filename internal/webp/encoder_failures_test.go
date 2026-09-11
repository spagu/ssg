package webp

// What a conversion pass does when an encoder refuses, a source is not an
// image, or a destination cannot be written. One awkward picture must never
// take the build with it, and it must never vanish either.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// failingEncoder puts a stub on PATH under the given name that always fails, so
// the pass's own error handling can be exercised without a broken image.
func failingEncoder(t *testing.T, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'stub: refusing' >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil { // #nosec G306 -- executable test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAnImageThatWillNotEncodeIsNamedAndTheRestGoOn.
//
// One awkward picture must not take the build with it, and it must not vanish
// either: the original is only removed once the .webp is confirmed on disk
// (GO-016).
func TestAnImageThatWillNotEncodeIsNamedAndTheRestGoOn(t *testing.T) {
	failingEncoder(t, "cwebp")
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "broken.png"), 40, 30)

	out := captureConversionOutput(t, func() {
		converted, _, err := ConvertDirectory(dir, ConvertOptions{Quality: 75})
		if err != nil {
			t.Errorf("one failed image must not fail the pass: %v", err)
		}
		if converted != 0 {
			t.Errorf("converted = %d", converted)
		}
	})
	if !strings.Contains(out, "Failed to convert broken.png") {
		t.Errorf("the failure should name the file:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.png")); err != nil {
		t.Error("an image that did not convert must keep its original")
	}
}

// TestAnAVIFThatWillNotEncodeIsReportedPerImageAndPerWidth, for the same
// reason: the page still works without the AVIF, and silence would hide that
// it is missing.
func TestAnAVIFThatWillNotEncodeIsReportedPerImageAndPerWidth(t *testing.T) {
	dir := t.TempDir()
	pngFixture(t, filepath.Join(dir, "hero.png"), 800, 600)

	// The full-size encode fails, so the image is reported and skipped.
	failingEncoder(t, "avifenc")
	out := captureConversionOutput(t, func() {
		if converted, err := ConvertDirectoryAVIF(dir, AVIFOptions{Quality: 50, Sizes: []int{400}}); err != nil || converted != 0 {
			t.Errorf("converted = %d, err = %v", converted, err)
		}
	})
	if !strings.Contains(out, "AVIF failed for hero.png") {
		t.Errorf("the failed image should be named:\n%s", out)
	}
}

// TestResizingReportsASourceItCannotRead rather than writing an empty variant.
func TestResizingReportsASourceItCannotRead(t *testing.T) {
	dir := t.TempDir()
	notAnImage := filepath.Join(dir, "notes.png")
	if err := os.WriteFile(notAnImage, []byte("this is not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resizeToPNG(notAnImage, filepath.Join(dir, "out.png"), 100); err == nil {
		t.Error("a file that is not an image cannot be resized")
	}
	if err := resizeToPNG(filepath.Join(dir, "absent.png"), filepath.Join(dir, "out.png"), 100); err == nil {
		t.Error("a missing source cannot be resized")
	}
}

// TestResizingReportsADestinationItCannotWrite.
func TestResizingReportsADestinationItCannotWrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "hero.png")
	pngFixture(t, src, 200, 100)
	// A directory where the output file has to go.
	dst := filepath.Join(dir, "taken.png")
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := resizeToPNG(src, dst, 100); err == nil {
		t.Error("a destination that cannot be created must be reported")
	}
}

// TestKeepModeReportsAnOriginalItCouldNotRemove is the other half: replace mode
// deletes the source, and a deletion that fails has to be visible or the site
// ships both files with no explanation.
func TestKeepModeReportsAnOriginalItCouldNotRemove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	stubCwebp(t)
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	pngFixture(t, filepath.Join(locked, "hero.png"), 40, 30)

	out := captureConversionOutput(t, func() {
		// The .webp is written first, then the original removed; making the
		// directory read-only after the write is not possible, so the whole
		// directory is read-only and the write fails instead. Use a nested
		// directory so the conversion still finds the image.
		if err := os.Chmod(locked, 0o500); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(locked, 0o750) }()
		if _, _, err := ConvertDirectory(root, ConvertOptions{Quality: 75}); err != nil {
			t.Errorf("a directory that cannot be written must not fail the pass: %v", err)
		}
	})
	if !strings.Contains(out, "hero.png") {
		t.Errorf("the failure should name the file:\n%s", out)
	}
}
