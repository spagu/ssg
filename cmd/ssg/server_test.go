package main

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"golang.org/x/crypto/acme/autocert"
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

// TestAltSvcValue verifies GO-033: the Alt-Svc value is derived locally from the
// configured address (SetQUICHeaders emits nothing until a QUIC listener is up).
func TestAltSvcValue(t *testing.T) {
	cases := map[string]string{
		":8443":          `h3=":8443"; ma=2592000`,
		"127.0.0.1:8443": `h3=":8443"; ma=2592000`,
		"[::1]:9443":     `h3=":9443"; ma=2592000`,
		"no-port":        "",
		"":               "",
	}
	for addr, want := range cases {
		if got := altSvcValue(addr); got != want {
			t.Errorf("altSvcValue(%q) = %q, want %q", addr, got, want)
		}
	}
}

// TestAltSvcMiddleware verifies GO-033: every TCP response carries the HTTP/3
// advertisement from the very first request, without any live QUIC listener.
func TestAltSvcMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	altSvcMiddleware(next, altSvcValue("127.0.0.1:8443")).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Error("altSvcMiddleware must call the next handler")
	}
	if got, want := rec.Header().Get("Alt-Svc"), `h3=":8443"; ma=2592000`; got != want {
		t.Errorf("Alt-Svc = %q, want %q", got, want)
	}

	// An unparsable address yields no header rather than a broken advertisement.
	rec = httptest.NewRecorder()
	altSvcMiddleware(next, altSvcValue("garbage")).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Alt-Svc") != "" {
		t.Error("no Alt-Svc expected for an invalid address")
	}
}

// TestTLSHostPolicy verifies GO-020: --tls-domain accepts a comma-separated
// list (with spaces) and every listed domain passes the autocert whitelist.
func TestTLSHostPolicy(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		domains string
		host    string
		wantOK  bool
	}{
		{"first of list", "a.com,b.com", "a.com", true},
		{"second of list", "a.com,b.com", "b.com", true},
		{"not listed", "a.com,b.com", "c.com", false},
		{"spaces trimmed", "a.com, b.com", "b.com", true},
		{"single domain", "example.com", "example.com", true},
		{"single domain other", "example.com", "other.com", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := tlsHostPolicy(tt.domains)(ctx, tt.host)
			if (err == nil) != tt.wantOK {
				t.Errorf("tlsHostPolicy(%q)(%q) err=%v, wantOK=%v", tt.domains, tt.host, err, tt.wantOK)
			}
		})
	}
}

// TestGzipMiddlewareRangeBypass verifies GO-012: Range requests are served
// uncompressed with a Content-Length that matches the body, so resumed
// downloads and media seeking keep working under --gzip.
func TestGzipMiddlewareRangeBypass(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("0123456789", 300)
	if err := os.WriteFile(filepath.Join(dir, "big.js"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	h := gzipMiddleware(http.FileServer(http.Dir(dir)))

	req := httptest.NewRequest("GET", "/big.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-999")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	if res.Header.Get("Content-Encoding") == "gzip" {
		t.Error("Range responses must not be gzip-compressed (GO-012)")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1000 {
		t.Errorf("body length = %d, want 1000", len(data))
	}
	if cl := res.Header.Get("Content-Length"); cl != "1000" {
		t.Errorf("Content-Length = %q, want 1000", cl)
	}
	if string(data) != body[:1000] {
		t.Error("range body must be the uncompressed slice")
	}
}

// TestGzipMiddlewareStripsStaleHeaders verifies GO-012: a compressed full GET
// must not advertise byte ranges or the uncompressed length, and still carries
// Vary: Accept-Encoding.
func TestGzipMiddlewareStripsStaleHeaders(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("hello world ", 200)
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	h := gzipMiddleware(http.FileServer(http.Dir(dir)))

	req := httptest.NewRequest("GET", "/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected gzip encoding for a full GET")
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
	}
	if rec.Header().Get("Accept-Ranges") != "" {
		t.Error("Accept-Ranges must be stripped from compressed responses (GO-012)")
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Error("Content-Length (uncompressed size) must be stripped from compressed responses (GO-012)")
	}
	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if string(data) != body {
		t.Error("decompressed body mismatch")
	}
}

// TestNewServerListenerMaxConns verifies GO-019: the shared listener helper
// enforces --max-conns — connection N+1 is not accepted until a slot frees up.
func TestNewServerListenerMaxConns(t *testing.T) {
	ln, err := newServerListener("127.0.0.1:0", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	c1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	s1, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}

	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		if s2, err := ln.Accept(); err == nil {
			accepted <- s2
		}
	}()

	select {
	case <-accepted:
		t.Fatal("connection above --max-conns must not be accepted while the slot is taken")
	case <-time.After(150 * time.Millisecond):
		// expected: Accept blocks until the first connection is released
	}

	_ = s1.Close() // release the slot
	select {
	case s2 := <-accepted:
		_ = s2.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("second connection should be accepted after the slot frees up")
	}
}

// TestServeOnListenerAutoTLS verifies GO-019: in --tls-auto mode the server
// serves TLS on the externally provided (capped) listener instead of binding
// its own, so the --max-conns wrap is honoured.
func TestServeOnListenerAutoTLS(t *testing.T) {
	ln, err := newServerListener("127.0.0.1:0", 4)
	if err != nil {
		t.Fatal(err)
	}
	acm := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: tlsHostPolicy("example.com"),
		Cache:      autocert.DirCache(t.TempDir()),
	}
	server := &http.Server{Addr: ln.Addr().String(), ReadHeaderTimeout: 2 * time.Second}
	done := make(chan error, 1)
	go func() { done <- serveOnListener(server, ln, &config.Config{}, "auto", acm) }()
	defer func() {
		_ = server.Close()
		<-done
	}()

	// Handshake with an SNI outside the whitelist: the host policy rejects it
	// fast (no ACME network round-trip). Receiving a TLS alert proves the
	// connection was accepted through our listener and TLS is active on it.
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	tlsConn := tls.Client(conn, &tls.Config{ServerName: "other.test", MinVersion: tls.VersionTLS12}) // #nosec G402 -- test client
	if err := tlsConn.Handshake(); err == nil {
		t.Error("expected a handshake error for a non-whitelisted SNI")
	} else if strings.Contains(err.Error(), "deadline") || strings.Contains(err.Error(), "timeout") {
		t.Errorf("server did not respond on the provided listener: %v", err)
	}
	if server.TLSConfig == nil {
		t.Error("auto mode must install the autocert TLS config")
	}
}
