package main

// The remaining branches of `ssg migrate`'s own flag parsing, and of the tar
// writer's entry handling.

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseMigrateFlagsSpaceForms: every flag that takes a value accepts both
// spellings, and a value-taking flag at the end of the line is an error rather
// than a silent empty value.
func TestParseMigrateFlagsSpaceForms(t *testing.T) {
	f, code := parseMigrateFlags([]string{"--host", "0.0.0.0", "--port", "9999"})
	if code != -1 || f.host != "0.0.0.0" || f.port != 9999 {
		t.Fatalf("space form: %+v code=%d", f, code)
	}
	f, code = parseMigrateFlags([]string{"--host=127.0.0.2", "--port=8123"})
	if code != -1 || f.host != "127.0.0.2" || f.port != 8123 {
		t.Fatalf("= form: %+v code=%d", f, code)
	}
	// A port that is not a number must not silently become 0, which would mean
	// "any free port" and point the operator at the wrong address.
	if _, code := parseMigrateFlags([]string{"--port=notanumber"}); code != 2 {
		t.Fatalf("bad port = %d, want 2", code)
	}
}

// TestParseMigrateFlagsRejectsEngineFlags: the engine's own flags are one
// command away from ssg's, and passing them here silently costs a whole
// content type — so they are refused with the --content equivalent.
func TestParseMigrateFlagsRejectsEngineFlags(t *testing.T) {
	for _, arg := range []string{"--no-comments", "--no-posts", "--no-pages"} {
		if _, code := parseMigrateFlags([]string{arg}); code != 2 {
			t.Errorf("%s must be refused, got code %d", arg, code)
		}
	}
}

// TestTarAddEntryKinds: a directory, a regular file and a symlink each get the
// entry shape tar expects — a symlink copied as content aborts the archive
// with "write too long" (GO-035).
func TestTarAddEntryKinds(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/file.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	out := filepath.Join(t.TempDir(), "out.tar.gz")
	if err := createTarGz(dir, out); err != nil {
		t.Fatalf("createTarGz: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	kinds := map[string]byte{}
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading archive: %v", err)
		}
		kinds[hdr.Name] = hdr.Typeflag
		if hdr.Typeflag == tar.TypeSymlink && hdr.Linkname == "" {
			t.Errorf("symlink %s lost its target", hdr.Name)
		}
	}
	if kinds["sub/"] != tar.TypeDir {
		t.Errorf("directory entry = %v", kinds["sub/"])
	}
	if kinds["sub/file.txt"] != tar.TypeReg {
		t.Errorf("file entry = %v", kinds["sub/file.txt"])
	}
	if kinds["link.txt"] != tar.TypeSymlink {
		t.Errorf("symlink entry = %v", kinds["link.txt"])
	}
}

// TestEngineFlagSelectsTheBinary: --engine names the wpexporter to run, in both
// spellings, so an operator whose snap bundles an older engine can reach the one
// they installed (#160).
func TestEngineFlagSelectsTheBinary(t *testing.T) {
	f, code := parseMigrateFlags([]string{"--engine", "/opt/wpexporter"})
	if code >= 0 || f.engine != "/opt/wpexporter" {
		t.Fatalf("--engine <path> = %q, code %d", f.engine, code)
	}
	if f, code := parseMigrateFlags([]string{"--engine=/opt/x"}); code >= 0 || f.engine != "/opt/x" {
		t.Fatalf("--engine=<path> = %q, code %d", f.engine, code)
	}
}

// TestValueFlagWithNothingAfterItIsRejected: `ssg migrate wordpress URL --engine`
// is a typo, not a request to use the next flag as the value. Swallowing the
// following argument is how a flag ends up silently eating another one.
func TestValueFlagWithNothingAfterItIsRejected(t *testing.T) {
	out := captureStderr(t, func() {
		if _, code := parseMigrateFlags([]string{"--engine"}); code != 2 {
			t.Errorf("a value-less --engine must stop the run, got %d", code)
		}
	})
	if !strings.Contains(out, "--engine") {
		t.Errorf("the message must name the flag: %q", out)
	}
}
