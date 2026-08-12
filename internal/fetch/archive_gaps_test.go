package fetch

// End-to-end download+extract tests over httptest (second coverage raise,
// 1.8.27): the paths a worker-archive fetch actually takes in production.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadOnceAndExtract(t *testing.T) {
	payload := zipBytes(t, map[string]string{
		"functions/api/hello.ts": "export const onRequest = () => new Response('hi')",
		"README.md":              "worker",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	path, retriable, err := downloadOnce(srv.URL+"/w.zip", Auth{}, 0)
	if err != nil || retriable {
		t.Fatalf("downloadOnce = %q %v %v", path, retriable, err)
	}
	defer func() { _ = os.Remove(path) }()

	dest := filepath.Join(t.TempDir(), "vendored", "worker")
	if err := extractAtomic(path, dest); err != nil {
		t.Fatalf("extractAtomic: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil || string(b) != "worker" {
		t.Fatalf("extracted content: %q %v", b, err)
	}
	// Re-extract over an existing destDir replaces it atomically.
	if err := extractAtomic(path, dest); err != nil {
		t.Fatalf("re-extract: %v", err)
	}
}

func TestDownloadOnceHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
		case "/flaky":
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	// 404 → error, NOT retriable.
	if _, retriable, err := downloadOnce(srv.URL+"/notfound", Auth{}, 0); err == nil || retriable {
		t.Fatalf("404 should be a final error, retriable=%v err=%v", retriable, err)
	}
	// 503 → error, retriable.
	if _, retriable, err := downloadOnce(srv.URL+"/flaky", Auth{}, 0); err == nil || !retriable {
		t.Fatalf("503 should be retriable, retriable=%v err=%v", retriable, err)
	}
	// Invalid URL → immediate final error.
	if _, retriable, err := downloadOnce("http://\x7f", Auth{}, 0); err == nil || retriable {
		t.Fatalf("invalid url: retriable=%v err=%v", retriable, err)
	}
	// Unreachable host → transport error, retriable.
	if _, retriable, err := downloadOnce("http://127.0.0.1:1/x.zip", Auth{}, 0); err == nil || !retriable {
		t.Fatalf("transport error should be retriable, retriable=%v err=%v", retriable, err)
	}
}

func TestDownloadArchiveRetries(t *testing.T) {
	hits := 0
	payload := zipBytes(t, map[string]string{"a": "b"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusBadGateway) // first attempt fails retriably
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	path, err := downloadArchive(srv.URL, Auth{}, Options{Retries: 2, RetryDelay: 1})
	if err != nil || hits != 2 {
		t.Fatalf("retry flow: path=%q hits=%d err=%v", path, hits, err)
	}
	_ = os.Remove(path)

	// A 404 is not retried: one hit only.
	hits = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv2.Close()
	if _, err := downloadArchive(srv2.URL, Auth{}, Options{Retries: 3, RetryDelay: 1}); err == nil || hits != 1 {
		t.Fatalf("404 must not retry: hits=%d err=%v", hits, err)
	}
}

func TestExtractAtomicBadArchive(t *testing.T) {
	// A corrupt zip never leaves a half-populated destDir.
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(bad, []byte("this is not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out", "worker")
	if err := extractAtomic(bad, dest); err == nil {
		t.Fatal("corrupt zip must error")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("destDir must not exist after a failed extraction")
	}
}
