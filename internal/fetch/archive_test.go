package fetch

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipBytes builds an in-memory zip from name->content.
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchiveExtractsAndStripsWrapper(t *testing.T) {
	// A GitHub-style archive wraps everything in one top dir; it should be stripped.
	zipped := zipBytes(t, map[string]string{
		"repo-main/functions/api/hello.ts": "export const onRequest = () => new Response('hi')\n",
		"repo-main/README.md":              "# worker\n",
	})
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write(zipped)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "worker")
	if err := Archive(srv.URL+"/x.zip", Auth{Type: "bearer", Token: "tok"}, dest); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth not sent: %q", gotAuth)
	}
	if _, err := os.Stat(filepath.Join(dest, "functions", "api", "hello.ts")); err != nil {
		t.Errorf("expected wrapper stripped and file extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "repo-main")); err == nil {
		t.Error("wrapper directory was not stripped")
	}
}

func TestArchiveRejectsPathEscape(t *testing.T) {
	zipped := zipBytes(t, map[string]string{"../evil.ts": "x"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipped)
	}))
	defer srv.Close()
	err := Archive(srv.URL+"/x.zip", Auth{}, filepath.Join(t.TempDir(), "w"))
	if err == nil || !strings.Contains(err.Error(), "escape") {
		t.Fatalf("path escape not rejected: %v", err)
	}
}

func TestArchiveHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := Archive(srv.URL+"/x.zip", Auth{}, filepath.Join(t.TempDir(), "w"))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("HTTP error not surfaced: %v", err)
	}
}

func TestArchiveRejectsTarball(t *testing.T) {
	if err := Archive("https://example.com/x.tar.gz", Auth{}, t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("tarball not rejected: %v", err)
	}
}

func TestToArchiveURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/r":      "https://github.com/u/r/archive/refs/heads/main.zip",
		"https://github.com/u/r.git":  "https://github.com/u/r/archive/refs/heads/main.zip",
		"https://gitlab.com/u/r":      "https://gitlab.com/u/r/-/archive/main/archive.zip",
		"https://example.com/w.zip":   "https://example.com/w.zip",
		"https://example.com/raw/dir": "https://example.com/raw/dir",
	}
	for in, want := range cases {
		if got := toArchiveURL(in); got != want {
			t.Errorf("toArchiveURL(%q) = %q, want %q", in, got, want)
		}
	}
}
