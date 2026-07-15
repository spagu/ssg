package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFileSigCacheReuse proves the PERF-008 contract: while size+mtime are
// unchanged the cached hash is returned WITHOUT re-reading the file (we swap
// the bytes on disk, restore mtime/size, and still get the old hash), and a
// real content change (new mtime) is re-hashed.
func TestFileSigCacheReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	c := newFileSigCache()
	first, err := c.hashFor(path, info)
	if err != nil {
		t.Fatalf("hashFor: %v", err)
	}

	// Same bytes-length, different content, mtime restored → cache hit, no re-read.
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	info2, _ := os.Stat(path)
	cached, err := c.hashFor(path, info2)
	if err != nil {
		t.Fatalf("hashFor cached: %v", err)
	}
	if cached != first {
		t.Error("expected the cached hash (no re-read) while size+mtime are unchanged")
	}

	// Bump mtime → re-hash picks up the new bytes.
	future := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	info3, _ := os.Stat(path)
	rehashed, err := c.hashFor(path, info3)
	if err != nil {
		t.Fatalf("hashFor rehash: %v", err)
	}
	if rehashed == first {
		t.Error("expected a fresh hash after the mtime changed")
	}
}

// TestSignatureTouchStable verifies PLAT-006 semantics survive the cache: a
// touch (mtime bump, same bytes) re-hashes just that file and the combined
// signature stays identical.
func TestSignatureTouchStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post.md")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newFileSigCache()
	sig1 := c.signature([]string{dir})

	future := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if sig2 := c.signature([]string{dir}); sig2 != sig1 {
		t.Error("touch-only change must not alter the signature")
	}

	// A real edit changes it.
	if err := os.WriteFile(path, []byte("content v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sig3 := c.signature([]string{dir}); sig3 == sig1 {
		t.Error("content change must alter the signature")
	}
}

// TestFileSigCacheUnreadable covers the skip path for unreadable files.
func TestFileSigCacheUnreadable(t *testing.T) {
	c := newFileSigCache()
	info, err := os.Stat(t.TempDir()) // a dir stat reused as a bogus file info
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hashFor(filepath.Join(t.TempDir(), "absent"), info); err == nil {
		t.Error("expected an error for a missing file")
	}
	// signature() itself must not fail on unreadable entries.
	_ = c.signature([]string{filepath.Join(t.TempDir(), "no-such-dir")})
}

// TestSignatureSkipsUnreadable covers the per-file error skip inside signature():
// a file that vanishes mid-walk (simulated by an unreadable entry) is skipped.
func TestSignatureSkipsUnreadable(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(ok, []byte("fine"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "locked.md")
	if err := os.WriteFile(bad, []byte("secret"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	c := newFileSigCache()
	sig := c.signature([]string{dir})
	if sig == "" {
		t.Error("signature should still be produced, skipping unreadable files")
	}
}

// TestContentSignatureWrapper covers the one-shot helper.
func TestContentSignatureWrapper(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if newFileSigCache().signature([]string{dir}) == "" {
		t.Error("wrapper must return a signature")
	}
}
