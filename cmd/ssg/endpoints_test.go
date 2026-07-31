package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func staticNext(marker *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*marker = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("static"))
	})
}

// TestEndpointRedirect: a redirect endpoint issues the configured 3xx + Location.
func TestEndpointRedirect(t *testing.T) {
	var served bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/go", Type: "redirect", To: "/new/", Status: 301},
	}}, staticNext(&served))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/go", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/new/" {
		t.Errorf("Location = %q, want /new/", loc)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("endpoint responses must be no-store, got %q", rec.Header().Get("Cache-Control"))
	}
	if served {
		t.Errorf("matched endpoint must not fall through to static")
	}
}

// TestEndpointProxy: a proxy endpoint forwards to the upstream's target path and
// returns its body; a disallowed method is rejected with 405.
func TestEndpointProxy(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, "hello upstream")
	}))
	defer upstream.Close()

	var served bool
	// httptest listens on loopback, so allow_private is required to reach it.
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/api/quote", Type: "proxy", Target: upstream.URL + "/upstream", Methods: []string{"GET"}, AllowPrivate: true},
	}}, staticNext(&served))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/quote", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello upstream" {
		t.Fatalf("proxy GET = %d %q", rec.Code, rec.Body.String())
	}
	if gotPath != "/upstream" {
		t.Errorf("upstream path = %q, want /upstream (client path dropped)", gotPath)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/quote", nil))
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Errorf("disallowed method = %d, want 405", rec2.Code)
	}
}

// TestEndpointProxySSRFBlocked: without allow_private the SSRF guard refuses a
// loopback upstream at dial time (502), so a proxy can't pivot to internal hosts.
func TestEndpointProxySSRFBlocked(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secret internal")
	}))
	defer upstream.Close()

	var served bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/api", Type: "proxy", Target: upstream.URL}, // AllowPrivate false
	}}, staticNext(&served))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("loopback upstream must be blocked (502), got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "secret internal" {
		t.Errorf("SSRF guard leaked the internal response body")
	}
}

// TestEndpointStaticFallthrough: an unmatched path is served by next.
func TestEndpointStaticFallthrough(t *testing.T) {
	var served bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/go", Type: "redirect", To: "/new/"},
	}}, staticNext(&served))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page/", nil))
	if !served || rec.Body.String() != "static" {
		t.Errorf("unmatched path must fall through to static, got %q", rec.Body.String())
	}
}

// TestEndpointHandlerNoEndpoints: no endpoints returns the next handler untouched.
func TestEndpointHandlerNoEndpoints(t *testing.T) {
	var served bool
	next := staticNext(&served)
	if got := endpointHandler(&config.Config{Quiet: true}, next); got == nil {
		t.Fatal("nil handler")
	}
	// All-invalid endpoints also fall back to next.
	var s2 bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/x", Type: "bogus"},
	}}, staticNext(&s2))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !s2 {
		t.Errorf("an all-invalid endpoint set must serve statically")
	}
}

// TestBuildEndpointErrors: malformed declarations are rejected with a reason.
func TestBuildEndpointErrors(t *testing.T) {
	bad := []config.Endpoint{
		{Path: "no-slash", Type: "redirect", To: "/x"},
		{Path: "/x", Type: "mystery"},
		{Path: "/x", Type: "redirect"},                        // no 'to'
		{Path: "/x", Type: "redirect", To: "/y", Status: 200}, // not 3xx
		{Path: "/x", Type: "proxy"},                           // no target
		{Path: "/x", Type: "proxy", Target: "://bad"},         // unparseable
		{Path: "/x", Type: "proxy", Target: "ftp://h/x"},      // wrong scheme
	}
	for _, ep := range bad {
		if _, err := buildEndpoint(ep); err == nil {
			t.Errorf("expected error for %+v", ep)
		}
	}
	// A well-formed redirect and proxy compile.
	if _, err := buildEndpoint(config.Endpoint{Path: "/x", Type: "redirect", To: "/y"}); err != nil {
		t.Errorf("valid redirect rejected: %v", err)
	}
	if _, err := buildEndpoint(config.Endpoint{Path: "/x", Type: "proxy", Target: "https://api.test/x"}); err != nil {
		t.Errorf("valid proxy rejected: %v", err)
	}
}

// TestEmitEndpoints covers the build-time wiring (#63): no platform is a no-op,
// a configured platform compiles the endpoints into the output tree.
func TestEmitEndpoints(t *testing.T) {
	// No platform selected → self-hosted only, nothing emitted.
	if err := emitEndpoints(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/go", Type: "redirect", To: "/new/"},
	}}); err != nil {
		t.Errorf("no platform must be a no-op: %v", err)
	}
	// No endpoints → no-op even with a platform.
	if err := emitEndpoints(&config.Config{Quiet: true, EndpointsPlatform: "cloudflare"}); err != nil {
		t.Errorf("no endpoints must be a no-op: %v", err)
	}
	// Platform + endpoints → functions written into the output tree.
	out := t.TempDir()
	err := emitEndpoints(&config.Config{
		Quiet: true, OutputDir: out, EndpointsPlatform: "cloudflare",
		Endpoints: []config.Endpoint{{Path: "/api/x", Type: "redirect", To: "/y", Status: 301}},
	})
	if err != nil {
		t.Fatalf("emitEndpoints: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(out, "functions", "api", "x.js")); statErr != nil {
		t.Errorf("expected functions/api/x.js: %v", statErr)
	}
	// Unknown platform surfaces as an error.
	if err := emitEndpoints(&config.Config{
		Quiet: true, OutputDir: out, EndpointsPlatform: "mystery",
		Endpoints: []config.Endpoint{{Path: "/api/x", Type: "redirect", To: "/y"}},
	}); err == nil {
		t.Error("unknown platform must error")
	}
}
