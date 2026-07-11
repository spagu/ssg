package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
)

func makeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "sub", "a.css"), []byte("a{}"), 0644)
	return dir
}

// tarNames returns the entry names in a tar stream.
func tarNames(t *testing.T, r *tar.Reader) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for {
		hdr, err := r.Next()
		if err != nil {
			break
		}
		names[hdr.Name] = true
	}
	return names
}

func TestCreateTarGz(t *testing.T) {
	src := makeTree(t)
	out := filepath.Join(t.TempDir(), "site.tar.gz")
	if err := createTarGz(src, out); err != nil {
		t.Fatalf("createTarGz: %v", err)
	}
	f, err := os.Open(out) // #nosec G304 -- test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip open: %v", err)
	}
	names := tarNames(t, tar.NewReader(gz))
	if !names["index.html"] || !names["sub/a.css"] {
		t.Errorf("tar.gz missing entries: %v", names)
	}
}

func TestCreateTarXz(t *testing.T) {
	src := makeTree(t)
	out := filepath.Join(t.TempDir(), "site.tar.xz")
	if err := createTarXz(src, out); err != nil {
		t.Fatalf("createTarXz: %v", err)
	}
	f, err := os.Open(out) // #nosec G304 -- test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	xr, err := xz.NewReader(f)
	if err != nil {
		t.Fatalf("xz open: %v", err)
	}
	names := tarNames(t, tar.NewReader(xr))
	if !names["index.html"] || !names["sub/a.css"] {
		t.Errorf("tar.xz missing entries: %v", names)
	}
}

func TestCreateTarGzBadSource(t *testing.T) {
	// A non-existent source still produces a (near-empty) archive without error;
	// an unwritable destination path errors.
	if err := createTarGz(t.TempDir(), filepath.Join(t.TempDir(), "nope", "x.tar.gz")); err == nil {
		t.Error("expected error writing to a missing directory")
	}
}

// TestWriteTarballSymlink verifies GO-035: a symlink in the output tree is
// archived as a proper link entry (target in the header, no body) instead of
// aborting the whole archive with "write too long".
func TestWriteTarballSymlink(t *testing.T) {
	src := makeTree(t)
	if err := os.Symlink("index.html", filepath.Join(src, "latest.html")); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	out := filepath.Join(t.TempDir(), "site.tar.gz")
	if err := createTarGz(src, out); err != nil {
		t.Fatalf("createTarGz with symlink: %v", err)
	}

	f, err := os.Open(out) // #nosec G304 -- test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip open: %v", err)
	}

	tr := tar.NewReader(gz)
	var found bool
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "latest.html" {
			found = true
			if hdr.Typeflag != tar.TypeSymlink {
				t.Errorf("latest.html typeflag = %v, want TypeSymlink", hdr.Typeflag)
			}
			if hdr.Linkname != "index.html" {
				t.Errorf("latest.html linkname = %q, want index.html", hdr.Linkname)
			}
			if hdr.Size != 0 {
				t.Errorf("symlink entry size = %d, want 0", hdr.Size)
			}
		}
	}
	if !found {
		t.Error("symlink entry missing from the archive")
	}
}
