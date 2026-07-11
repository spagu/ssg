package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileHash(t *testing.T) {
	// Deterministic: same content+ext → same 32-hex-char hash.
	h1 := fileHash([]byte("hello"), "html")
	h2 := fileHash([]byte("hello"), "html")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 32 {
		t.Errorf("hash length = %d, want 32", len(h1))
	}
	// Extension participates in the hash.
	if fileHash([]byte("hello"), "css") == h1 {
		t.Error("extension should change the hash")
	}
}

func TestContentTypeFor(t *testing.T) {
	if ct := contentTypeFor("a.css"); ct == "" || ct == "application/octet-stream" {
		t.Errorf("css content type = %q", ct)
	}
	if ct := contentTypeFor("noext"); ct != "application/octet-stream" {
		t.Errorf("unknown ext = %q, want octet-stream", ct)
	}
}

func TestCollectSiteFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	writes := map[string]string{
		"index.html":   "<html></html>",
		"css/app.css":  "body{}",
		"_headers":     "/*\n  X-Frame-Options: DENY",
		"_redirects":   "/old /new 301",
		"_routes.json": `{"version":1}`,
	}
	for rel, content := range writes {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sf, err := collectSiteFiles(dir)
	if err != nil {
		t.Fatalf("collectSiteFiles: %v", err)
	}
	// Control files go to special, not assets.
	if len(sf.assets) != 2 {
		t.Errorf("expected 2 hashed assets, got %d", len(sf.assets))
	}
	for _, name := range []string{"_headers", "_redirects", "_routes.json"} {
		if _, ok := sf.special[name]; !ok {
			t.Errorf("control file %q not captured in special", name)
		}
	}
	// Manifest maps server paths (leading slash) to hashes.
	m := sf.manifest()
	if _, ok := m["/index.html"]; !ok {
		t.Errorf("manifest missing /index.html: %#v", m)
	}
}

func TestUniqueByHash(t *testing.T) {
	dir := t.TempDir()
	// Two identical files share a hash → uploaded once.
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("same"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("diff"), 0o644)
	sf, err := collectSiteFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(sf.assets))
	}
	if u := sf.uniqueByHash(); len(u) != 2 {
		t.Errorf("uniqueByHash = %d, want 2 (a.txt/b.txt dedupe)", len(u))
	}
}
