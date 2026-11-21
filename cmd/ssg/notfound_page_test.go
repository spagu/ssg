package main

// A missing path in the preview answers with the site's own 404.html (#432),
// the way static hosts that honour a root 404.html do. Without that file the
// answer stays Go's plain-text 404.

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

const notFoundBody = "<html><body>custom not found</body></html>"

// notFoundSite writes an output tree with a home page, an empty date
// directory and, when with404 is set, a 404.html.
func notFoundSite(t *testing.T, with404 bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2014", "05"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"index.html": "<html>home</html>"}
	if with404 {
		files["404.html"] = notFoundBody
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func serverFor(dir, basePath string) http.Handler {
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.OutputDir = dir
	cfg.BasePath = basePath
	return buildServerHandler(cfg, false)
}

func requestPath(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// TestAMissingPathServesTheSite404Page: the status stays 404, the body is the
// page the site wrote, and it is served as HTML.
func TestAMissingPathServesTheSite404Page(t *testing.T) {
	h := serverFor(notFoundSite(t, true), "")
	for _, p := range []string{"/nope/", "/nope.html", "/2014/05/", "/css/missing.css"} {
		rec := requestPath(h, http.MethodGet, p)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
		if rec.Body.String() != notFoundBody {
			t.Errorf("%s: body %q, want the site's 404.html", p, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("%s: Content-Type %q", p, ct)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: lost the security headers", p)
		}
	}
	// Existing pages are untouched, and the page itself is still reachable.
	if rec := requestPath(h, http.MethodGet, "/"); rec.Code != http.StatusOK || rec.Body.String() != "<html>home</html>" {
		t.Errorf("home = %d %q", rec.Code, rec.Body.String())
	}
	if rec := requestPath(h, http.MethodGet, "/404.html"); rec.Code != http.StatusOK || rec.Body.String() != notFoundBody {
		t.Errorf("/404.html = %d %q", rec.Code, rec.Body.String())
	}
}

// TestAHeadRequestForAMissingPathHasNoBody: same status and headers, no body.
func TestAHeadRequestForAMissingPathHasNoBody(t *testing.T) {
	rec := requestPath(serverFor(notFoundSite(t, true), ""), http.MethodHead, "/nope/")
	if rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
		t.Errorf("HEAD = %d with %d body bytes", rec.Code, rec.Body.Len())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("HEAD Content-Type %q", ct)
	}
	if rec.Header().Get("Content-Length") != strconv.Itoa(len(notFoundBody)) {
		t.Errorf("HEAD Content-Length %q", rec.Header().Get("Content-Length"))
	}
}

// TestWithout404PageTheAnswerIsPlainText: no file, the old answer.
func TestWithout404PageTheAnswerIsPlainText(t *testing.T) {
	rec := requestPath(serverFor(notFoundSite(t, false), ""), http.MethodGet, "/nope/")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "404 page not found") {
		t.Errorf("no 404.html = %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type %q", ct)
	}
}

// TestA404DirectoryIsNotA404Page: a directory named 404.html cannot be served
// as a page, so the plain answer stands.
func TestA404DirectoryIsNotA404Page(t *testing.T) {
	dir := notFoundSite(t, false)
	if err := os.MkdirAll(filepath.Join(dir, "404.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := requestPath(serverFor(dir, ""), http.MethodGet, "/nope/")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "404 page not found") {
		t.Errorf("404.html directory = %d %q", rec.Code, rec.Body.String())
	}
}

// TestUnderABasePathThe404PageIsTheSites: inside the prefix a missing path
// gets the site's page; outside it the host would answer, not this site.
func TestUnderABasePathThe404PageIsTheSites(t *testing.T) {
	h := serverFor(notFoundSite(t, true), "/docs")
	if rec := requestPath(h, http.MethodGet, "/docs/nope/"); rec.Code != http.StatusNotFound || rec.Body.String() != notFoundBody {
		t.Errorf("/docs/nope/ = %d %q", rec.Code, rec.Body.String())
	}
	if rec := requestPath(h, http.MethodGet, "/docs/404.html"); rec.Code != http.StatusOK || rec.Body.String() != notFoundBody {
		t.Errorf("/docs/404.html = %d %q", rec.Code, rec.Body.String())
	}
	if rec := requestPath(h, http.MethodGet, "/elsewhere/"); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "custom") {
		t.Errorf("outside the base path = %d %q", rec.Code, rec.Body.String())
	}
}

// TestNotFoundWriterPassesOtherStatusesThrough: a 200 and a redirect reach
// the client as written; only a 404 is replaced.
func TestNotFoundWriterPassesOtherStatusesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/there/", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	h := notFoundPageHandler(inner, http.Dir(notFoundSite(t, true)))
	if rec := requestPath(h, http.MethodGet, "/x"); rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("200 = %d %q", rec.Code, rec.Body.String())
	}
	if rec := requestPath(h, http.MethodGet, "/moved"); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/there/" {
		t.Errorf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// TestThe404PageIsGzippedLikeAnyPage: http.FileServer drops Content-Encoding
// from its error answer, but under gzip the replacement body is compressed,
// so the header must come back with it.
func TestThe404PageIsGzippedLikeAnyPage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.OutputDir = notFoundSite(t, true)
	cfg.Gzip = true
	req := httptest.NewRequest(http.MethodGet, "/nope/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	buildServerHandler(cfg, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip 404 = %d, Content-Encoding %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(zr)
	if err != nil || string(body) != notFoundBody {
		t.Errorf("decompressed body %q (%v)", body, err)
	}
}
