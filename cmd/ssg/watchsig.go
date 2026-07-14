// Watch-mode content signatures (PLAT-006) with a per-file hash cache so a
// change event does not re-read the whole content tree (PERF-008): unchanged
// files (same size+mtime) reuse their cached hash, and changed files are
// hashed by streaming — never loaded whole into memory.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// fileSigEntry is one cached per-file content hash with its freshness key.
type fileSigEntry struct {
	size    int64
	modTime time.Time
	hash    [sha256.Size]byte
}

// fileSigCache caches content hashes keyed by path; entries are reused while
// the file's size and mtime are unchanged. Single-goroutine use (watch loop).
type fileSigCache struct {
	entries map[string]fileSigEntry
}

func newFileSigCache() *fileSigCache {
	return &fileSigCache{entries: map[string]fileSigEntry{}}
}

// hashFor returns the file's content hash, re-hashing (streamed via io.Copy)
// only when size or mtime changed since the last call. A touch that bumps
// mtime without changing bytes re-hashes just that one file and yields the
// same hash, so the overall signature stays stable (PLAT-006 semantics).
func (c *fileSigCache) hashFor(path string, info os.FileInfo) ([sha256.Size]byte, error) {
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.modTime.Equal(info.ModTime()) {
		return e.hash, nil
	}
	f, err := os.Open(path) // #nosec G304 -- CLI hashes its own content dirs
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	c.entries[path] = fileSigEntry{size: info.Size(), modTime: info.ModTime(), hash: sum}
	return sum, nil
}

// signature combines the per-file hashes of every file under dirs into one
// deterministic content signature (filepath.Walk visits in lexical order).
func (c *fileSigCache) signature(dirs []string) string {
	h := sha256.New()
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error { // #nosec G703 -- best-effort watch signature
			if err != nil || info.IsDir() {
				return nil
			}
			sum, herr := c.hashFor(path, info)
			if herr != nil {
				return nil // unreadable file: skip, same as the pre-cache behaviour
			}
			_, _ = fmt.Fprintf(h, "%s:%d:", path, info.Size())
			_, _ = h.Write(sum[:])
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// contentSignature returns a content hash of every file under dirs, so a
// rebuild can be skipped when nothing actually changed (PLAT-006). One-shot
// variant with a throwaway cache; the watch loop holds a persistent
// fileSigCache instead so unchanged files are never re-read (PERF-008).
func contentSignature(dirs []string) string {
	return newFileSigCache().signature(dirs)
}
