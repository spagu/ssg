package main

// Watch mode must not cache anything (#185).

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// withoutHub clears the live-reload hub for the duration of a test, so a test
// that means "not a preview" is not decided by whichever test ran before it.
func withoutHub(t *testing.T) {
	t.Helper()
	old := currentReloadHub()
	setReloadHub(nil)
	t.Cleanup(func() { setReloadHub(old) })
}

// TestPreviewModeRecognisesBothPreviewServers. Keying on --watch alone would
// leave `ssg mcp --http` stale, which is the case the report singles out.
func TestPreviewModeRecognisesBothPreviewServers(t *testing.T) {
	withoutHub(t)

	plain := config.DefaultConfig()
	if previewMode(plain) {
		t.Error("plain serving is not a preview")
	}

	watching := config.DefaultConfig()
	watching.Watch = true
	if !previewMode(watching) {
		t.Error("--watch is a preview")
	}

	// `ssg mcp --http` installs a hub without setting Watch.
	setReloadHub(newLiveReloadHub())
	if !previewMode(plain) {
		t.Error("a reload hub means a preview even without --watch")
	}
}

// setsCacheControl is an inner handler that writes headers the way the file
// server and the `_headers` rules do: after the outer middleware has run.
func setsCacheControl(value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(cacheControlHeader, value)
		w.Header().Set("Expires", "Thu, 31 Dec 2037 23:59:59 GMT")
		w.Header().Set("Pragma", "cache")
		_, _ = w.Write([]byte("body"))
	})
}

// TestTheHeadersFileCannotOutrankThePreview: the exact reported failure. A
// `_headers` block caching /css/* for a year is written after the cache
// middleware ran, so setting a header up front would lose to it.
func TestTheHeadersFileCannotOutrankThePreview(t *testing.T) {
	h := previewNoCacheMiddleware(setsCacheControl("public, max-age=31536000, immutable"))
	rec := getPath(h, "/css/style.css")

	if got := rec.Header().Get(cacheControlHeader); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	// Both of these outrank Cache-Control in an HTTP/1.0 cache, so a stale one
	// left behind would undo the rewrite above.
	if got := rec.Header().Get("Expires"); got != "" {
		t.Errorf("Expires = %q, want it dropped", got)
	}
	if got := rec.Header().Get("Pragma"); got != "" {
		t.Errorf("Pragma = %q, want it dropped", got)
	}
	if rec.Body.String() != "body" {
		t.Errorf("body = %q", rec.Body)
	}
}

// TestNoStoreIsNotWeakened: endpoints choose no-store because their bodies can
// hold per-request data. Rewriting that to no-cache would let it reach disk.
func TestNoStoreIsNotWeakened(t *testing.T) {
	h := previewNoCacheMiddleware(setsCacheControl("no-store"))
	if got := getPath(h, "/api/whatever").Header().Get(cacheControlHeader); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store kept", got)
	}
}

// TestTheBrowsersValidatorsAreDropped: a browser reload re-offers the ETag it
// holds, and the file server will answer 304 to it. The stale asset then comes
// from the browser's own cache and no header on the response can help.
func TestTheBrowsersValidatorsAreDropped(t *testing.T) {
	var seen http.Header
	h := previewNoCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	req.Header.Set("If-None-Match", `"abc"`)
	req.Header.Set("If-Modified-Since", "Thu, 01 Jan 2026 00:00:00 GMT")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got := seen.Get("If-None-Match"); got != "" {
		t.Errorf("If-None-Match reached the file server: %q", got)
	}
	if got := seen.Get("If-Modified-Since"); got != "" {
		t.Errorf("If-Modified-Since reached the file server: %q", got)
	}
}

// TestTheStatusIsPreserved, including the ones a preview shows most often.
func TestTheStatusIsPreserved(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusFound, http.StatusInternalServerError} {
		h := previewNoCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		rec := getPath(h, "/missing")
		if rec.Code != code {
			t.Errorf("status = %d, want %d", rec.Code, code)
		}
		if got := rec.Header().Get(cacheControlHeader); got != "no-cache" {
			t.Errorf("a %d must not be cached either: %q", code, got)
		}
	}
}

// TestStampingHappensOnce: a handler that writes without calling WriteHeader
// must still be stamped, and a second write must not re-stamp over a header the
// first one settled.
func TestStampingHappensOnce(t *testing.T) {
	w := &noCacheWriter{ResponseWriter: httptest.NewRecorder()}
	w.Header().Set(cacheControlHeader, "public, max-age=3600")
	_, _ = w.Write([]byte("a"))
	if got := w.Header().Get(cacheControlHeader); got != "no-cache" {
		t.Fatalf("a bare Write must stamp: %q", got)
	}
	// Something downstream sets it again; the wrapper is spent and leaves it.
	w.Header().Set(cacheControlHeader, "public, max-age=60")
	_, _ = w.Write([]byte("b"))
	if got := w.Header().Get(cacheControlHeader); got != "public, max-age=60" {
		t.Errorf("stamped twice: %q", got)
	}
}

// flusherRecorder reports whether Flush reached the wrapped writer.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flusherRecorder) Flush() { f.flushed = true }

// TestFlushReachesTheWriterBelow: streaming handlers assert http.Flusher and
// refuse the request when the assertion fails, so the wrapper must forward it —
// and must not panic when there is nothing to forward to.
func TestFlushReachesTheWriterBelow(t *testing.T) {
	inner := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	w := &noCacheWriter{ResponseWriter: inner}
	if _, ok := any(w).(http.Flusher); !ok {
		t.Fatal("the wrapper must satisfy http.Flusher")
	}
	w.Flush()
	if !inner.flushed {
		t.Error("Flush did not reach the writer below")
	}

	// nonFlusher (coverage3_test.go) hides Flush; forwarding must not panic.
	(&noCacheWriter{ResponseWriter: nonFlusher{h: http.Header{}}}).Flush()
}

// serveDir writes one asset and returns a config serving it.
func serveDir(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "css", "style.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.OutputDir = dir
	return cfg
}

// TestTheStackCachesOrNotByMode: the assertion that the middleware is actually
// wired, and that a server for published output is left alone.
func TestTheStackCachesOrNotByMode(t *testing.T) {
	withoutHub(t)
	resetLiveEndpoints(t)

	published := serveDir(t)
	if got := getPath(buildServerHandler(published, false), "/css/style.css").
		Header().Get(cacheControlHeader); got == "no-cache" {
		t.Errorf("published output must keep its cache policy: %q", got)
	}

	preview := serveDir(t)
	preview.Watch = true
	if got := getPath(buildServerHandler(preview, false), "/css/style.css").
		Header().Get(cacheControlHeader); got != "no-cache" {
		t.Errorf("a watched asset must not be cached: %q", got)
	}
}
