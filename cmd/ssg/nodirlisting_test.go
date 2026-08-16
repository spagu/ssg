package main

// A hole in the output must look like a hole (#146).
//
// Posts with dated permalinks live in /2014/05/…, so the directory exists even
// when no archive was generated — and the dev server used to answer 200 with a
// <pre> list of file names. Every host this project deploys to answers 404
// there, which made the dev server the one place a missing page looked fine.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoDirListing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "2014", "05", "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A post exists under the date directories; no archive was generated.
	if err := os.WriteFile(filepath.Join(root, "2014", "05", "post", "index.html"),
		[]byte("<html>post</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>home</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.FileServer(noDirListing{http.Dir(root)}))
	defer srv.Close()

	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return resp.StatusCode, string(buf[:n])
	}

	// The hole answers 404, the way the production host will.
	if code, body := get("/2014/05/"); code != http.StatusNotFound {
		t.Errorf("a directory without index.html = %d, want 404 (body: %q)", code, body)
	}
	if code, _ := get("/2014/"); code != http.StatusNotFound {
		t.Errorf("/2014/ = %d, want 404", code)
	}
	// Real pages are untouched.
	if code, body := get("/2014/05/post/"); code != http.StatusOK || !strings.Contains(body, "post") {
		t.Errorf("a real page = %d %q", code, body)
	}
	if code, body := get("/"); code != http.StatusOK || !strings.Contains(body, "home") {
		t.Errorf("the root = %d %q", code, body)
	}
	// A missing file is still a plain 404.
	if code, _ := get("/nope.html"); code != http.StatusNotFound {
		t.Errorf("missing file = %d, want 404", code)
	}
}

// TestNoDirListingServesGeneratedArchive: once the archive exists, the same
// address serves it — the wrapper refuses listings, not directories.
func TestNoDirListingServesGeneratedArchive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "2014", "05"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "2014", "05", "index.html"),
		[]byte("<html>May 2014</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(noDirListing{http.Dir(root)}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/2014/05/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a generated archive = %d, want 200", resp.StatusCode)
	}
}
