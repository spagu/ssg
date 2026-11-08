package webp

// SkipDirs: social preview cards stay PNG/JPEG through the WebP and AVIF
// passes (1.9.0).

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSkipDir(t *testing.T) {
	root := filepath.Join("out")
	for path, want := range map[string]error{
		filepath.Join(root, "og"):         filepath.SkipDir,
		filepath.Join(root, "x", "cards"): filepath.SkipDir,
		filepath.Join(root, "ogx"):        nil,
		root:                              nil,
	} {
		if got := skipDir(root, path, []string{"/og/", "x/cards", ""}); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	// A path that cannot be made relative to root is never skipped.
	if got := skipDir("/abs", "rel", []string{"og"}); got != nil {
		t.Errorf("unrelated path: %v", got)
	}
}

func TestAVIFSourcesSkipDirs(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"a.png", "og/card.png"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := avifSources(root, []string{"og"})
	if len(got) != 1 || filepath.Base(got[0]) != "a.png" {
		t.Fatalf("sources %v", got)
	}
}

func TestConvertDirectorySkipDirs(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not installed")
	}
	root := t.TempDir()
	card := filepath.Join(root, "og", "card.png")
	if err := os.MkdirAll(filepath.Dir(card), 0o750); err != nil {
		t.Fatal(err)
	}
	writePNG(t, card)
	if _, _, err := ConvertDirectory(root, ConvertOptions{Quiet: true, SkipDirs: []string{"og"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(card); err != nil {
		t.Errorf("card was replaced: %v", err)
	}
	if _, err := os.Stat(webpTargetPath(card)); err == nil {
		t.Error("card was converted")
	}
}

// TestAVIFVariantSkipsAndFailures: an existing variant is kept, a failing
// one is reported and passed over, and the resize step's own failures are
// errors.
func TestAVIFVariantSkipsAndFailures(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "hero.png")
	pngFixture(t, src, 100, 80)
	dst := avifTargetPath(src)
	if err := os.WriteFile(dst, []byte("made"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(avifVariantPath(dst, 40), []byte("made"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The full size and the 40 px variant exist; the 60 px one fails.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "avifenc"), []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if made := convertOneAVIF(src, AVIFOptions{Sizes: []int{40, 60}}); made != 0 {
		t.Errorf("made %d, want 0", made)
	}
	// No temporary file can be made beside a destination in a missing directory.
	if err := encodeAVIFFile(src, filepath.Join(root, "missing", "x.avif"), 45, 6, 40); err == nil {
		t.Error("missing directory: no error")
	}
	// A source the resizer cannot decode.
	junk := filepath.Join(root, "junk.png")
	if err := os.WriteFile(junk, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := encodeAVIFFile(junk, filepath.Join(root, "j.avif"), 45, 6, 40); err == nil {
		t.Error("undecodable source: no error")
	}
}
