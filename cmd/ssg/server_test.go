package main

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quic-go/quic-go/http3"
	"github.com/spagu/ssg/internal/config"
)

func TestServerTLSMode(t *testing.T) {
	cases := []struct {
		cfg  *config.Config
		want string
	}{
		{&config.Config{TLSCert: "c", TLSKey: "k"}, "manual"},
		{&config.Config{TLSAuto: true, TLSDomain: "example.com"}, "auto"},
		{&config.Config{TLSCert: "c"}, ""},  // key missing
		{&config.Config{TLSAuto: true}, ""}, // domain missing
		{&config.Config{}, ""},
	}
	for _, c := range cases {
		if got := serverTLSMode(c.cfg); got != c.want {
			t.Errorf("serverTLSMode(%+v) = %q, want %q", c.cfg, got, c.want)
		}
	}
}

func TestCacheControlMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := cacheControlMiddleware(next)
	cases := map[string]string{
		"/app.a1b2c3d4.css": "public, max-age=31536000, immutable",
		"/index.html":       "no-cache",
		"/":                 "no-cache",
		"/blog/":            "no-cache",
		"/img/logo.png":     "public, max-age=3600",
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("Cache-Control for %s = %q, want %q", path, got, want)
		}
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	rec := httptest.NewRecorder()
	securityHeadersMiddleware(next, false).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should be absent without TLS")
	}

	rec = httptest.NewRecorder()
	securityHeadersMiddleware(next, true).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Error("HSTS should be present under TLS")
	}
}

func TestGzipMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("hello ", 100)))
	})
	h := gzipMiddleware(next)

	// With gzip acceptance.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected gzip encoding")
	}
	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	buf := make([]byte, 6)
	_, _ = gz.Read(buf)
	if string(buf) != "hello " {
		t.Errorf("decompressed = %q", buf)
	}

	// Without gzip acceptance → plain.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not gzip without Accept-Encoding")
	}
}

func TestBuildServerHandler(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{OutputDir: dir, Gzip: true}
	h := buildServerHandler(cfg, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/nope", nil)
	h.ServeHTTP(rec, req)
	// Security headers applied regardless of 404.
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("handler chain missing security headers")
	}
}

func TestParseByteSize(t *testing.T) {
	cases := map[string]int64{
		"512MiB":  512 << 20,
		"1GiB":    1 << 30,
		"256KiB":  256 << 10,
		"1048576": 1048576,
		"10MB":    10_000_000,
	}
	for in, want := range cases {
		got, err := parseByteSize(in)
		if err != nil || got != want {
			t.Errorf("parseByteSize(%q) = %d,%v want %d", in, got, err, want)
		}
	}
	if _, err := parseByteSize("nonsense"); err == nil {
		t.Error("expected error for invalid size")
	}
}

func TestApplyMemLimit(t *testing.T) {
	applyMemLimit("", true)        // no-op
	applyMemLimit("128MiB", true)  // valid
	applyMemLimit("garbage", true) // invalid, warns quietly
}

func TestAutocertCacheDir(t *testing.T) {
	dir := autocertCacheDir()
	if dir == "" {
		t.Fatal("autocert cache dir should not be empty")
	}
	// Must end in the dedicated subdir and never live in the shared system temp
	// dir (S5445) — the cache holds TLS private keys.
	if filepath.Base(dir) != "autocert" {
		t.Errorf("autocert cache dir = %q, want it to end in /autocert", dir)
	}
	if strings.HasPrefix(dir, os.TempDir()) {
		t.Errorf("autocert cache dir %q must not be under the shared temp dir", dir)
	}
}

func TestAutocertCacheDirFallback(t *testing.T) {
	// With no cache/home env, the function must fall back to a private relative
	// path — never the system temp dir.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	dir := autocertCacheDir()
	if strings.HasPrefix(dir, os.TempDir()) {
		t.Errorf("fallback cache dir %q must not be under the shared temp dir", dir)
	}
	if filepath.Base(dir) != "autocert" {
		t.Errorf("fallback cache dir = %q, want it to end in /autocert", dir)
	}
}

func TestAltSvcMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h3 := &http3.Server{Addr: ":8443", Port: 8443}
	rec := httptest.NewRecorder()
	// SetQUICHeaders only emits Alt-Svc once a QUIC listener is live (verified e2e
	// with curl); here we assert the middleware wraps and delegates without panic.
	altSvcMiddleware(next, h3).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Error("altSvcMiddleware must call the next handler")
	}
}
