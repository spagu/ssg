package webp

// The conversion loop's failure paths, driven by a stand-in cwebp on PATH (CI
// has none, and the real one never fails on demand). FAKE_CWEBP_MODE picks
// the misbehaviour: "nowrite" reports success without writing, "eatsrc"
// deletes its own input, "eatother" deletes FAKE_CWEBP_VICTIM.

import (
	"os"
	"path/filepath"
	"testing"
)

const fakeCwebpScript = `#!/bin/sh
for a in "$@"; do src="$prev"; prev="$last"; last="$a"; done
src="$src"; dst="$last"
[ "$FAKE_CWEBP_MODE" = nowrite ] && exit 0
printf 'WEBP' > "$dst"
[ "$FAKE_CWEBP_MODE" = eatsrc ] && rm -f "$src"
[ "$FAKE_CWEBP_MODE" = eatother ] && rm -f "$FAKE_CWEBP_VICTIM"
exit 0
`

// fakeCwebp puts the stand-in first on PATH in the given mode.
func fakeCwebp(t *testing.T, mode string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "cwebp"), []byte(fakeCwebpScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CWEBP_MODE", mode)
}

func TestConvertDirectoryMissingDir(t *testing.T) {
	fakeCwebp(t, "")
	if _, _, err := ConvertDirectory(filepath.Join(t.TempDir(), "missing"), ConvertOptions{Quiet: true}); err == nil {
		t.Error("missing directory: no error")
	}
}

// TestConvertDirectoryNoOutputKeepsOriginal: a "successful" run that wrote
// nothing must not cost the original (GO-016).
func TestConvertDirectoryNoOutputKeepsOriginal(t *testing.T) {
	fakeCwebp(t, "nowrite")
	root := t.TempDir()
	img := filepath.Join(root, "a.png")
	writePNG(t, img)
	if n, _, err := ConvertDirectory(root, ConvertOptions{Quiet: true}); err != nil || n != 0 {
		t.Fatalf("converted %d, %v", n, err)
	}
	if _, err := os.Stat(img); err != nil {
		t.Errorf("original lost: %v", err)
	}
}

// TestConvertDirectoryRemoveFailureIsReported: the original already gone when
// replace mode removes it is reported, and the conversion still counts.
func TestConvertDirectoryRemoveFailureIsReported(t *testing.T) {
	fakeCwebp(t, "eatsrc")
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "a.png"))
	if n, _, err := ConvertDirectory(root, ConvertOptions{}); err != nil || n != 1 {
		t.Fatalf("converted %d, %v", n, err)
	}
}

// TestConvertDirectoryVanishedSource: an image removed after the scan is
// skipped, not an error.
func TestConvertDirectoryVanishedSource(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.png"), filepath.Join(root, "b.png")
	writePNG(t, a)
	writePNG(t, b)
	fakeCwebp(t, "eatother")
	t.Setenv("FAKE_CWEBP_VICTIM", b)
	if n, _, err := ConvertDirectory(root, ConvertOptions{Quiet: true, Workers: 1}); err != nil || n != 1 {
		t.Fatalf("converted %d, %v", n, err)
	}
}

// TestEmitSrcsetUnreadablePage: a page that cannot be read fails the pass.
func TestEmitSrcsetUnreadablePage(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "index.html")
	if err := os.WriteFile(page, []byte(`<img src="a.webp">`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(page, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(page, 0o600) })
	if _, err := os.ReadFile(page); err == nil {
		t.Skip("running with privileges that read any file")
	}
	if err := EmitSrcset(root, []int{320}, ""); err == nil {
		t.Error("unreadable page: no error")
	}
}
