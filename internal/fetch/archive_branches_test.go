package fetch

// Branch-level tests for the worker-archive download+extract path (coverage
// raise, 1.8.28): the fallback and failure branches the happy-path tests in
// archive_test.go and archive_gaps_test.go do not reach. No real network I/O —
// every download goes to an httptest server.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZipFile stores an in-memory zip on disk so the extract functions can be
// exercised without a download.
func writeZipFile(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "src.zip")
	if err := os.WriteFile(p, zipBytes(t, files), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// lowerInt64Limit shrinks one of the extraction caps for the duration of a
// test, so tripping it needs no huge fixture.
func lowerInt64Limit(t *testing.T, limit *int64, n int64) {
	t.Helper()
	old := *limit
	*limit = n
	t.Cleanup(func() { *limit = old })
}

// A repo whose default branch is still master: the main-branch archive 404s
// and Archive must fetch the master-branch variant instead of failing.
func TestArchiveFallsBackToMasterBranch(t *testing.T) {
	payload := zipBytes(t, map[string]string{"repo-master/f.txt": "ok"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/archive/refs/heads/main.zip") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "worker")
	// The path contains github.com, so toArchiveURL appends the main-branch
	// archive suffix and masterFallback knows how to rewrite it.
	if err := Archive(srv.URL+"/github.com/u/r", Auth{}, dest, Options{}); err != nil {
		t.Fatalf("Archive with master fallback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "f.txt")); err != nil {
		t.Errorf("master-branch archive not extracted: %v", err)
	}
}

// A destination whose parent path runs through a regular file cannot be
// created; extractAtomic must fail cleanly instead of extracting anywhere.
func TestExtractAtomicParentIsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeZipFile(t, map[string]string{"a.txt": "x"})
	if err := extractAtomic(src, filepath.Join(file, "sub", "worker")); err == nil {
		t.Fatal("a file in the destination's parent path must fail the extraction")
	}
}

// When the staging directory cannot be created next to destDir (unwritable
// parent), the error surfaces before anything is extracted.
func TestExtractAtomicStagingFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	parent := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	src := writeZipFile(t, map[string]string{"a.txt": "x"})
	if err := extractAtomic(src, filepath.Join(parent, "worker")); err == nil {
		t.Fatal("an unwritable parent must fail staging creation")
	}
}

// An existing destDir that cannot be removed (a read-only subdirectory blocks
// deletion) fails the install — and the previous content survives, because the
// swap is all-or-nothing (GO-081).
func TestExtractAtomicRemoveDestFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	dest := filepath.Join(t.TempDir(), "worker")
	locked := filepath.Join(dest, "locked")
	if err := os.MkdirAll(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	src := writeZipFile(t, map[string]string{"a.txt": "x"})
	if err := extractAtomic(src, dest); err == nil {
		t.Fatal("an unremovable destDir must fail the install")
	}
	if _, err := os.Stat(filepath.Join(locked, "keep")); err != nil {
		t.Errorf("previous worker content lost on a failed swap: %v", err)
	}
}

// A rename that cannot land (empty destination path) is reported as an install
// failure, and the staging directory is cleaned up rather than left behind.
func TestExtractAtomicRenameFails(t *testing.T) {
	t.Chdir(t.TempDir())
	src := writeZipFile(t, map[string]string{"a.txt": "x"})
	err := extractAtomic(src, "")
	if err == nil || !strings.Contains(err.Error(), "installing worker") {
		t.Fatalf("rename failure not surfaced: %v", err)
	}
	entries, readErr := os.ReadDir(".")
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".ssg-worker-") {
			t.Errorf("staging directory %s left behind", e.Name())
		}
	}
}

// Negative retries clamp to a single attempt, and a single attempt reports its
// error bare — no "after N attempts" wrapper.
func TestDownloadArchiveNegativeRetriesClamped(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := downloadArchive(srv.URL+"/x.zip", Auth{}, Options{Retries: -3})
	if err == nil || hits != 1 {
		t.Fatalf("clamped attempts: hits=%d err=%v", hits, err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("single attempt must not carry a retry summary: %v", err)
	}
}

// A misconfigured credential (bearer without a token) fails before any request
// is sent, and is never retried.
func TestDownloadOnceAuthError(t *testing.T) {
	_, retriable, err := downloadOnce("http://127.0.0.1:1/x.zip", Auth{Type: "bearer"}, 0)
	if err == nil || retriable {
		t.Fatalf("bad auth: retriable=%v err=%v", retriable, err)
	}
}

// The download's temp file cannot be created (broken TMPDIR): a final,
// non-retriable error.
func TestDownloadOnceTempFileError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("zip"))
	}))
	defer srv.Close()
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	_, retriable, err := downloadOnce(srv.URL+"/x.zip", Auth{}, 0)
	if err == nil || retriable {
		t.Fatalf("broken TMPDIR: retriable=%v err=%v", retriable, err)
	}
}

// A body that dies mid-stream (the server promises more bytes than it sends)
// is a retriable download error.
func TestDownloadOnceTruncatedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()
	_, retriable, err := downloadOnce(srv.URL+"/x.zip", Auth{}, 0)
	if err == nil || !retriable {
		t.Fatalf("truncated body: retriable=%v err=%v", retriable, err)
	}
}

// An archive over maxArchiveBytes is refused at download time, before any
// extraction — and not retried, since retrying cannot shrink it.
func TestDownloadOnceOverCap(t *testing.T) {
	lowerInt64Limit(t, &maxArchiveBytes, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 64))
	}))
	defer srv.Close()
	_, retriable, err := downloadOnce(srv.URL+"/x.zip", Auth{}, 0)
	if err == nil || retriable || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("over-cap archive: retriable=%v err=%v", retriable, err)
	}
}

// An archive with more entries than maxArchiveFiles is rejected outright
// (decompression-bomb guard, SEC-006).
func TestExtractZipTooManyEntries(t *testing.T) {
	old := maxArchiveFiles
	maxArchiveFiles = 2
	t.Cleanup(func() { maxArchiveFiles = old })
	src := writeZipFile(t, map[string]string{"a": "1", "b": "2", "c": "3"})
	err := extractZip(src, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("entry-count cap not enforced: %v", err)
	}
}

// A single entry over maxArchiveFile is rejected by its declared size, before
// any bytes are inflated.
func TestExtractZipEntryOverFileCap(t *testing.T) {
	lowerInt64Limit(t, &maxArchiveFile, 4)
	src := writeZipFile(t, map[string]string{"big.txt": "ten bytes!"})
	err := extractZip(src, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("per-entry cap not enforced: %v", err)
	}
}

// destDir itself cannot be created when its path runs through a regular file.
func TestExtractZipDestUnderFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeZipFile(t, map[string]string{"a.txt": "x"})
	if err := extractZip(src, filepath.Join(file, "out")); err == nil {
		t.Fatal("destDir under a file must fail")
	}
}

// Explicit directory entries: the wrapper's own entry vanishes with the strip,
// and an inner directory entry is materialized as a directory.
func TestExtractZipDirectoryEntries(t *testing.T) {
	src := writeZipFile(t, map[string]string{
		"repo-main/":          "",
		"repo-main/sub/":      "",
		"repo-main/sub/a.txt": "x",
	})
	dest := filepath.Join(t.TempDir(), "out")
	if err := extractZip(src, dest); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dest, "sub"))
	if err != nil || !fi.IsDir() {
		t.Errorf("inner directory entry not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "repo-main")); err == nil {
		t.Error("wrapper directory entry survived the strip")
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "a.txt")); err != nil {
		t.Errorf("file under the stripped wrapper missing: %v", err)
	}
}

// A directory entry colliding with an existing regular file fails extraction
// rather than silently skipping the directory.
func TestExtractZipDirCollidesWithFile(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "sub"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The extra top-level file defeats wrapper stripping, so "sub/" stays.
	src := writeZipFile(t, map[string]string{"sub/": "", "keep.txt": "x"})
	if err := extractZip(src, dest); err == nil {
		t.Fatal("directory entry over an existing file must fail")
	}
}

// A file entry whose parent directory collides with an existing regular file
// fails the extraction the same way.
func TestExtractZipFileParentCollidesWithFile(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "sub"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeZipFile(t, map[string]string{"sub/file.txt": "x", "z.txt": "x"})
	if err := extractZip(src, dest); err == nil {
		t.Fatal("file entry under a file-blocked parent must fail")
	}
}

// A zip whose central directory is intact but whose local entry header is
// corrupt: the entry fails to open and extraction reports it.
func TestExtractZipCorruptLocalHeader(t *testing.T) {
	raw := zipBytes(t, map[string]string{"a.txt": "hello"})
	raw[0] ^= 0xff // break the first local-file-header signature only
	src := filepath.Join(t.TempDir(), "src.zip")
	if err := os.WriteFile(src, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(src, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("a corrupt local header must fail the entry open")
	}
}

// The write target cannot be opened because an existing directory sits where
// the file should land: the entry write fails.
func TestExtractZipTargetIsDirectory(t *testing.T) {
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	src := writeZipFile(t, map[string]string{"x": "content"})
	if err := extractZip(src, dest); err == nil {
		t.Fatal("writing a file over an existing directory must fail")
	}
}
