package images

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCacheKeyGolden pins the cache-key formula byte-for-byte (GO-091). The
// literal below was captured from the pre-refactor implementation; if this test
// ever fails, every user's image cache silently invalidates and full
// reconversion storms follow — do NOT update the literal to make it pass
// without understanding that cost.
func TestCacheKeyGolden(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "golden.png")
	if err := os.WriteFile(src, []byte("golden-image-bytes-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(Config{SourceDirs: []string{dir}, OutputDir: t.TempDir(), Quiet: true})
	ops := []request{{Op: "resize", Width: 320, Format: "webp", Quality: 70}}
	key, err := p.cacheKey(src, ops)
	if err != nil {
		t.Fatal(err)
	}
	if key != "57d8e9e3ba" {
		t.Fatalf("cache key changed: got %q, want 57d8e9e3ba — this INVALIDATES every existing image cache", key)
	}
	if name := outputName(src, key, finalFormat(ops, "png")); name != "golden.57d8e9e3ba.webp" {
		t.Fatalf("output name changed: %q", name)
	}
}
