package ai

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadCacheMigrating covers the GO-091 move: answers cached under the
// legacy (unversioned) key — in the configured dir or the old .ai-cache root —
// are found and adopted under the current key BY COPY, never regenerated. AI
// output is non-deterministic, so anything but a copy would silently rewrite
// generated content site-wide.
func TestReadCacheMigrating(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work) // legacyCacheDir is relative to the working directory

	r := resolved{url: "http://x", model: "m1", system: "s", maxTokens: 10, temperature: 0.5}
	question := "summarise this"
	keyNew := cacheKey(r, question)
	keyOld := cacheKeyLegacy(r, question)
	if keyNew == keyOld {
		t.Fatal("versioned and legacy keys must differ")
	}

	newDir := filepath.Join(work, "aicache")
	c := &Client{cacheDir: newDir, mem: map[string]string{}}

	// 1. Miss everywhere.
	if _, ok := c.readCacheMigrating(keyNew, keyOld); ok {
		t.Fatal("expected a clean miss")
	}

	// 2. Legacy entry in the OLD DEFAULT ROOT (.ai-cache) → found + adopted.
	if err := os.MkdirAll(legacyCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyCacheDir, keyOld+".txt"), []byte("old answer"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, ok := c.readCacheMigrating(keyNew, keyOld)
	if !ok || v != "old answer" {
		t.Fatalf("legacy-root fallback failed: %q %v", v, ok)
	}
	// Adopted under the current key in the new dir — next read needs no fallback.
	if b, err := os.ReadFile(filepath.Join(newDir, keyNew+".txt")); err != nil || string(b) != "old answer" {
		t.Fatalf("migrate-by-copy did not adopt the entry: %q %v", b, err)
	}

	// 3. Legacy key inside the CONFIGURED dir (custom cache_dir users).
	c2 := &Client{cacheDir: filepath.Join(work, "custom"), mem: map[string]string{}}
	if err := os.MkdirAll(c2.cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c2.cacheDir, keyOld+".txt"), []byte("custom old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, ok := c2.readCacheMigrating(keyNew, keyOld); !ok || v != "custom old" {
		t.Fatalf("custom-dir legacy fallback failed: %q %v", v, ok)
	}

	// 4. Current key wins over legacy when both exist.
	if err := os.WriteFile(filepath.Join(newDir, keyNew+".txt"), []byte("new answer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.readCacheMigrating(keyNew, keyOld); v != "new answer" {
		t.Fatalf("current key must win, got %q", v)
	}
}
