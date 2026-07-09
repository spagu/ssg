// Package theme - tests for the SEC-006/008/010 download & extraction hardening.
package theme

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// createZipWithMode writes a single-entry zip whose file carries an explicit
// (possibly hostile) mode, under a "root/" prefix that extractZip strips.
func createZipWithMode(t *testing.T, zipPath, name, content string, mode os.FileMode) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetMode(mode)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatalf("create header: %v", err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// withLimits temporarily overrides the package extraction limits for a test and
// restores them afterwards.
func withLimits(t *testing.T, total, perFile int64, entries int) {
	t.Helper()
	ot, of, oe := maxTotalSize, maxFileSize, maxEntries
	maxTotalSize, maxFileSize, maxEntries = total, perFile, entries
	t.Cleanup(func() { maxTotalSize, maxFileSize, maxEntries = ot, of, oe })
}

// TestExtractZipClampsFileMode verifies SEC-010: a world-writable/executable
// mode in the archive is not honored — files land as 0644.
func TestExtractZipClampsFileMode(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "t.zip")
	createZipWithMode(t, zipPath, "root/evil.sh", "#!/bin/sh\n", 0o777)

	dest := filepath.Join(tmp, "out")
	if err := extractZip(zipPath, dest); err != nil {
		t.Fatalf("extractZip: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "evil.sh"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("file mode = %o, want 0644 (archive 0777 must be clamped)", got)
	}
}

// TestExtractZipTooManyEntries verifies SEC-006: an archive with an implausible
// number of entries is rejected before extraction.
func TestExtractZipTooManyEntries(t *testing.T) {
	withLimits(t, 500<<20, 100<<20, 2) // allow 2 entries
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "t.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/a.txt": "a", "root/b.txt": "b", "root/c.txt": "c",
	}); err != nil {
		t.Fatalf("create zip: %v", err)
	}

	err := extractZip(zipPath, filepath.Join(tmp, "out"))
	if err == nil {
		t.Fatal("expected error for too many entries")
	}
}

// TestExtractZipPerFileCap verifies SEC-006: a single entry larger than the
// per-file limit aborts extraction (zip-bomb guard).
func TestExtractZipPerFileCap(t *testing.T) {
	withLimits(t, 500<<20, 4, 10000) // 4-byte per-file cap
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "t.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/big.txt": "0123456789", // 10 bytes > 4
	}); err != nil {
		t.Fatalf("create zip: %v", err)
	}

	if err := extractZip(zipPath, filepath.Join(tmp, "out")); err == nil {
		t.Fatal("expected error when an entry exceeds the per-file limit")
	}
}

// TestExtractZipTotalCap verifies SEC-006: the cumulative extracted size is
// bounded across multiple entries.
func TestExtractZipTotalCap(t *testing.T) {
	withLimits(t, 6, 100<<20, 10000) // 6-byte total budget
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "t.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/a.txt": "aaaa", "root/b.txt": "bbbb", // 8 bytes total > 6
	}); err != nil {
		t.Fatalf("create zip: %v", err)
	}

	if err := extractZip(zipPath, filepath.Join(tmp, "out")); err == nil {
		t.Fatal("expected error when cumulative size exceeds the total limit")
	}
}

// TestExtractZipEntryOpenError verifies the extraction error path: when a file
// entry collides with an already-created directory of the same name, opening
// the output file fails and the error is propagated.
func TestExtractZipEntryOpenError(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "t.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	// Directory entry first, then a file with the same name → the file cannot
	// be opened for writing because the path is already a directory.
	dirHdr := &zip.FileHeader{Name: "root/clash/"}
	dirHdr.SetMode(os.ModeDir | 0o755)
	if _, err := zw.CreateHeader(dirHdr); err != nil {
		t.Fatalf("dir header: %v", err)
	}
	fileHdr := &zip.FileHeader{Name: "root/clash", Method: zip.Deflate}
	fileHdr.SetMode(0o644)
	w, err := zw.CreateHeader(fileHdr)
	if err != nil {
		t.Fatalf("file header: %v", err)
	}
	_, _ = w.Write([]byte("data"))
	_ = zw.Close()
	_ = f.Close()

	if err := extractZip(zipPath, filepath.Join(tmp, "out")); err == nil {
		t.Fatal("expected error when a file entry collides with a directory")
	}
}

// TestDownloadStopsRedirectLoop verifies SEC-008: the bounded client refuses to
// follow an endless redirect chain instead of hanging.
func TestDownloadStopsRedirectLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop.zip", http.StatusFound)
	}))
	defer srv.Close()

	err := Download(srv.URL+"/loop.zip", filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Fatal("expected error on redirect loop")
	}
}

// TestDownloadRejectsOversizedArchive verifies SEC-006 on the download path: a
// response body larger than the cap is refused before extraction.
func TestDownloadRejectsOversizedArchive(t *testing.T) {
	withLimits(t, 8, 100<<20, 10000) // 8-byte download cap
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 64)) // 64 bytes > 8
	}))
	defer srv.Close()

	err := Download(srv.URL+"/big.zip", filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Fatal("expected error for oversized archive")
	}
}
