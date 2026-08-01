package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// TestEndpointForm covers the form primitive: a valid submission is delivered as
// JSON to the webhook and the browser is redirected; the honeypot drops bots
// without delivering; non-POST is rejected.
func TestEndpointForm(t *testing.T) {
	var got map[string]string
	var calls int
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhook.Close()

	var served bool
	// Loopback webhook ⇒ allow_private required.
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{{
		Path: "/api/contact", Type: "form", To: webhook.URL,
		Fields: []string{"name", "email"}, Honeypot: "company",
		Redirect: "/thanks/", AllowPrivate: true,
	}}}, staticNext(&served))

	post := func(v url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Valid submission.
	rec := post(url.Values{"name": {"Ada"}, "email": {"a@b.c"}, "company": {""}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/thanks/" {
		t.Fatalf("valid submit = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if calls != 1 || got["name"] != "Ada" || got["email"] != "a@b.c" {
		t.Errorf("webhook payload = %v (calls %d)", got, calls)
	}
	if _, leaked := got["company"]; leaked {
		t.Errorf("honeypot field must not be forwarded")
	}

	// Honeypot filled → accepted, not delivered.
	calls = 0
	rec = post(url.Values{"name": {"Bot"}, "company": {"spam"}})
	if rec.Code != http.StatusSeeOther {
		t.Errorf("honeypot submit should still 303, got %d", rec.Code)
	}
	if calls != 0 {
		t.Errorf("honeypot must drop delivery, webhook called %d times", calls)
	}

	// GET is rejected.
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/contact", nil))
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on form = %d, want 405", getRec.Code)
	}
}

// TestFormEndpointErrors: a form without a valid delivery target is rejected.
func TestFormEndpointErrors(t *testing.T) {
	for _, ep := range []config.Endpoint{
		{Path: "/f", Type: "form"},                  // no 'to'
		{Path: "/f", Type: "form", To: "ftp://x/y"}, // wrong scheme
		{Path: "/f", Type: "form", To: "://bad"},    // unparseable
	} {
		if _, err := buildEndpoint(ep); err == nil {
			t.Errorf("expected error for %+v", ep)
		}
	}
}

// TestEndpointFormAllFieldsAndErrors covers the all-fields collection + JSON ok
// response (no Redirect) and the delivery-failure path.
func TestEndpointFormAllFieldsAndErrors(t *testing.T) {
	// All-fields, JSON ok (no redirect).
	var got map[string]string
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	var served bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/f", Type: "form", To: ok.URL, Honeypot: "hp", AllowPrivate: true},
	}}, staticNext(&served))
	req := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader(url.Values{"a": {"1"}, "b": {"2"}, "hp": {""}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("all-fields ok = %d %q", rec.Code, rec.Body.String())
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("all fields not forwarded: %v", got)
	}
	if _, leaked := got["hp"]; leaked {
		t.Errorf("honeypot forwarded in all-fields mode")
	}

	// Delivery rejected (webhook 500) → 502.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	h2 := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/f", Type: "form", To: bad.URL, AllowPrivate: true},
	}}, staticNext(&served))
	req2 := httptest.NewRequest(http.MethodPost, "/f", strings.NewReader("x=1"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadGateway {
		t.Errorf("rejected delivery = %d, want 502", rec2.Code)
	}
}

// TestAuthGuard covers the Basic-auth prefix guard (#63): no/wrong credentials
// under the prefix are 401'd; correct credentials pass through to static; paths
// outside the prefix are untouched.
func TestAuthGuard(t *testing.T) {
	t.Setenv("MEMBERS_PW", "s3cret")
	var served bool
	h := endpointHandler(&config.Config{Quiet: true, Endpoints: []config.Endpoint{
		{Path: "/members/", Type: "auth", User: "ada", Password: "$MEMBERS_PW"},
	}}, staticNext(&served))

	// No credentials → 401 with a challenge.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/members/secret.html", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("guarded path without creds = %d, challenge %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	if served {
		t.Error("unauthorized request must not reach static")
	}

	// Wrong password → 401.
	served = false
	wrong := httptest.NewRequest(http.MethodGet, "/members/secret.html", nil)
	wrong.SetBasicAuth("ada", "nope")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, wrong)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", rec.Code)
	}

	// Correct credentials → served.
	served = false
	okReq := httptest.NewRequest(http.MethodGet, "/members/secret.html", nil)
	okReq.SetBasicAuth("ada", "s3cret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, okReq)
	if !served || rec.Body.String() != "static" {
		t.Errorf("authorized request must reach static, got %d %q", rec.Code, rec.Body.String())
	}

	// Outside the prefix → not gated.
	served = false
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/", nil))
	if !served {
		t.Error("path outside the guarded prefix must not require auth")
	}
}

// TestBuildAuthGuardErrors: missing user/password or an unset env var are rejected.
func TestBuildAuthGuardErrors(t *testing.T) {
	if _, err := buildAuthGuard(config.Endpoint{Path: "/m/", Type: "auth"}); err == nil {
		t.Error("auth without user must error")
	}
	if _, err := buildAuthGuard(config.Endpoint{Path: "/m/", Type: "auth", User: "u"}); err == nil {
		t.Error("auth without password must error")
	}
	if _, err := buildAuthGuard(config.Endpoint{Path: "/m/", Type: "auth", User: "u", Password: "$UNSET_PW_XYZ"}); err == nil {
		t.Error("auth with an unset env var must error")
	}
	// A literal password (discouraged but allowed) resolves.
	g, err := buildAuthGuard(config.Endpoint{Path: "/m/", Type: "auth", User: "u", Password: "lit"})
	if err != nil || g.pass != "lit" {
		t.Errorf("literal password guard = %+v, %v", g, err)
	}
}
