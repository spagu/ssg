package main

// Server, endpoint-delivery and archive error-path tests (coverage raise,
// 1.8.28).

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestEndpointGuardSkippedOnBadConfig: a misconfigured auth endpoint is skipped
// with a warning while the rest of the server keeps working — a typo must not
// take the dev server down.
func TestEndpointGuardSkippedOnBadConfig(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Type: "auth", Path: "/admin/", User: ""},       // invalid: no user
		{Type: "form", Path: "no-slash", To: "bad url"}, // invalid: bad target
	}}, next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/x", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("all endpoints skipped must fall through to static, got %d", rec.Code)
	}
}

// TestFormEndpointDeliveryFailures: the form proxy translates upstream
// problems into 502 for the visitor and a filled honeypot into a silent
// accept — bots get no signal, humans get honest errors.
func TestFormEndpointDeliveryFailures(t *testing.T) {
	delivered := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	ep := config.Endpoint{Type: "form", Path: "/api/contact", To: upstream.URL,
		AllowPrivate: true, Honeypot: "trap"}
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{ep}},
		http.NotFoundHandler())

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := post("trap=gotcha&name=bot"); rec.Code >= 400 {
		t.Fatalf("honeypot must accept silently, got %d", rec.Code)
	}
	if delivered != 0 {
		t.Fatal("honeypot submission must never be delivered")
	}
	if rec := post("name=human"); rec.Code != http.StatusBadGateway {
		t.Fatalf("upstream 500 must map to 502, got %d", rec.Code)
	}
	// GET is refused with the allowed method advertised.
	req := httptest.NewRequest(http.MethodGet, "/api/contact", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
		t.Fatalf("GET = %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}

	upstream.Close() // now unreachable → delivery failed, still 502
	if rec := post("name=human"); rec.Code != http.StatusBadGateway {
		t.Fatalf("unreachable upstream must map to 502, got %d", rec.Code)
	}
}

// TestCreateTarXzErrorPaths: an unwritable target and a missing source both
// surface as errors — a truncated archive must never look like success.
func TestCreateTarXzErrorPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createTarXz("src", filepath.Join("no-such-dir", "out.tar.xz")); err == nil {
		t.Fatal("unwritable target must error")
	}
	if err := createTarXz("does-not-exist", "out.tar.xz"); err == nil {
		t.Fatal("missing source must error")
	}
}

// TestServeOnListenerModes: the plain mode reports a dead listener instead of
// hanging, and manual TLS surfaces unreadable cert files.
func TestServeOnListenerModes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	srv := &http.Server{ReadHeaderTimeout: 0}
	if err := serveOnListener(srv, ln, &config.Config{}, "", nil); err == nil {
		t.Fatal("serving on a closed listener must error")
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln2.Close() }()
	cfg := &config.Config{TLSCert: "missing.crt", TLSKey: "missing.key"}
	if err := serveOnListener(&http.Server{ReadHeaderTimeout: 0}, ln2, cfg, "manual", nil); err == nil {
		t.Fatal("missing cert files must error")
	}
}

// TestBuildServerHandlerAccessControl: an IP allowlist turns the access-control
// layer on — allowed clients pass, everyone else is refused before the file
// server is reached.
func TestBuildServerHandlerAccessControl(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("out", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("out", "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{OutputDir: "out", IPAllowlist: []string{"127.0.0.1"}, Quiet: false}
	h := buildServerHandler(cfg, false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowlisted client = %d", rec.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.0.2.44:9999"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusOK {
		t.Fatalf("blocked client must not be served, got %d", rec2.Code)
	}
}

// TestBuildGeneratePhaseWrapped: build() wraps a failed generation with
// context so the operator knows which phase died.
func TestBuildGeneratePhaseWrapped(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Source: "missing", Template: "simple", Domain: "x",
		ContentDir: "content", TemplatesDir: "templates", OutputDir: "out", Quiet: true}
	genCfg := createGeneratorConfig(cfg)
	err := build(genCfg, cfg)
	if err == nil || !strings.Contains(err.Error(), "generating site") {
		t.Fatalf("missing content must fail the generate phase, got %v", err)
	}
}

// TestRunArchivesZipError: a vanished output directory fails ZIP creation
// loudly (GO-024: a truncated archive with exit 0 is the worst outcome).
func TestRunArchivesZipError(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Domain: "x.example.com", OutputDir: "never-built", Zip: true, Quiet: true}
	if err := runArchives(cfg); err == nil {
		t.Fatal("zip of a missing output dir must error")
	}
	cfg = &config.Config{Domain: "x.example.com", OutputDir: "never-built", TarGz: true, Quiet: true}
	if err := runArchives(cfg); err == nil {
		t.Fatal("tar.gz of a missing output dir must error")
	}
}
