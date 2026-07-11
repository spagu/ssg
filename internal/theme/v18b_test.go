package theme

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"
)

// oneEntryZip returns a *zip.File for a single "a.txt" entry, for unit-testing
// extractZipEntry in isolation.
func oneEntryZip(t *testing.T) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr.File[0]
}

// TestExtractZipEntryTotalLimit covers the remaining-budget guard (SEC-006).
func TestExtractZipEntryDirectTotalLimit(t *testing.T) {
	f := oneEntryZip(t)
	if _, err := extractZipEntry(f, filepath.Join(t.TempDir(), "a.txt"), 0); err == nil {
		t.Error("expected error when remaining total budget is exhausted")
	}
}

// TestExtractZipEntryOpenError covers the destination-open failure branch.
func TestExtractZipEntryDirectOpenError(t *testing.T) {
	f := oneEntryZip(t)
	// Target a path under a non-existent directory → OpenFile fails.
	bad := filepath.Join(t.TempDir(), "nope", "deep", "a.txt")
	if _, err := extractZipEntry(f, bad, 1024); err == nil {
		t.Error("expected error opening destination under a missing directory")
	}
}

// TestExtractZipEntryOK covers the successful extraction path.
func TestExtractZipEntryDirectOK(t *testing.T) {
	f := oneEntryZip(t)
	n, err := extractZipEntry(f, filepath.Join(t.TempDir(), "a.txt"), 1024)
	if err != nil {
		t.Fatalf("extractZipEntry: %v", err)
	}
	if n != 5 {
		t.Errorf("wrote %d bytes, want 5", n)
	}
}
