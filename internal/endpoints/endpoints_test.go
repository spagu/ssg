package endpoints

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestRegistry: the Cloudflare adapter self-registers and is discoverable.
func TestRegistry(t *testing.T) {
	if _, ok := For("cloudflare"); !ok {
		t.Fatal("cloudflare adapter not registered")
	}
	if _, ok := For("nope"); ok {
		t.Error("unknown platform must not resolve")
	}
	found := false
	for _, p := range Platforms() {
		if p == "cloudflare" {
			found = true
		}
	}
	if !found {
		t.Errorf("Platforms() missing cloudflare: %v", Platforms())
	}
}

// TestEmitDispatch: Emit is a no-op with no endpoints, errors on an unknown
// platform, and otherwise delegates to the adapter.
func TestEmitDispatch(t *testing.T) {
	if files, err := Emit("cloudflare", nil, t.TempDir()); err != nil || files != nil {
		t.Errorf("no endpoints must be a no-op, got %v %v", files, err)
	}
	eps := []config.Endpoint{{Path: "/api/x", Type: "redirect", To: "/y"}}
	if _, err := Emit("mystery", eps, t.TempDir()); err == nil {
		t.Error("unknown platform must error")
	}
	if _, err := Emit("cloudflare", eps, t.TempDir()); err != nil {
		t.Errorf("known platform must emit: %v", err)
	}
}

// TestCloudflareEmitWritesFunctions: endpoints compile to functions/<path>.js
// with the expected module bodies.
func TestCloudflareEmitWritesFunctions(t *testing.T) {
	dir := t.TempDir()
	eps := []config.Endpoint{
		{Path: "/go/latest", Type: "redirect", To: "/releases/", Status: 301},
		{Path: "/api/quote", Type: "proxy", Target: "https://api.example.com/v1/quote", Methods: []string{"get", "post"}},
	}
	written, err := Emit("cloudflare", eps, dir)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %d files, want 2: %v", len(written), written)
	}

	redir := mustRead(t, filepath.Join(dir, "functions", "go", "latest.js"))
	if !strings.Contains(redir, "Response.redirect(to, 301)") || !strings.Contains(redir, `"/releases/"`) {
		t.Errorf("redirect module wrong:\n%s", redir)
	}
	if !strings.Contains(redir, "export function onRequest") {
		t.Errorf("redirect must export onRequest:\n%s", redir)
	}

	proxy := mustRead(t, filepath.Join(dir, "functions", "api", "quote.js"))
	for _, want := range []string{
		"export async function onRequest",
		`new URL("https://api.example.com/v1/quote")`,
		`const allowed = ["GET", "POST"]`,
		"status: 405",
		"return fetch(new Request(target, request))",
	} {
		if !strings.Contains(proxy, want) {
			t.Errorf("proxy module missing %q in:\n%s", want, proxy)
		}
	}
}

// TestCloudflareSourceErrors: malformed endpoints are rejected.
func TestCloudflareSourceErrors(t *testing.T) {
	bad := []config.Endpoint{
		{Path: "/x", Type: "redirect"},                        // no 'to'
		{Path: "/x", Type: "redirect", To: "/y", Status: 200}, // not 3xx
		{Path: "/x", Type: "proxy"},                           // no target
		{Path: "/x", Type: "mystery"},
	}
	for _, ep := range bad {
		if _, err := cloudflareSource(ep); err == nil {
			t.Errorf("expected error for %+v", ep)
		}
	}
}

// TestFunctionFileAndMethods: path→file mapping and method-literal rendering.
func TestFunctionFileAndMethods(t *testing.T) {
	cases := map[string]string{
		"/api/quote": "api/quote.js",
		"/x/":        "x.js",
		"/":          "index.js",
		"":           "index.js",
	}
	for in, want := range cases {
		if got := functionFile(in, ".js"); got != want {
			t.Errorf("functionFile(%q) = %q, want %q", in, got, want)
		}
	}
	if methodsLit(nil) != "" {
		t.Error("no methods must yield empty literal")
	}
	if got := methodsLit([]string{" get ", "", "Post"}); got != `["GET", "POST"]` {
		t.Errorf("methodsLit = %q", got)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestNetlifyEmit: Netlify Functions v2 modules with self-declared routes.
func TestNetlifyEmit(t *testing.T) {
	dir := t.TempDir()
	eps := []config.Endpoint{
		{Path: "/api/quote", Type: "proxy", Target: "https://api.example.com/q", Methods: []string{"GET"}},
		{Path: "/go", Type: "redirect", To: "/new/", Status: 302},
	}
	if _, err := Emit("netlify", eps, dir); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	proxy := mustRead(t, filepath.Join(dir, "netlify", "functions", "api-quote.mjs"))
	for _, want := range []string{
		`export const config = { path: "/api/quote" }`,
		"export default async (request) =>",
		`const allowed = ["GET"]`,
		"return fetch(new Request(target, request))",
	} {
		if !strings.Contains(proxy, want) {
			t.Errorf("netlify proxy missing %q in:\n%s", want, proxy)
		}
	}
	redir := mustRead(t, filepath.Join(dir, "netlify", "functions", "go.mjs"))
	if !strings.Contains(redir, "Response.redirect(to, 302)") {
		t.Errorf("netlify redirect wrong:\n%s", redir)
	}
}

// TestVercelEmit: Vercel Edge Functions under api/ plus a vercel.json that
// rewrites each endpoint path to its function.
func TestVercelEmit(t *testing.T) {
	dir := t.TempDir()
	eps := []config.Endpoint{
		{Path: "/api/quote", Type: "proxy", Target: "https://api.example.com/q"},
		{Path: "/go/latest", Type: "redirect", To: "/new/", Status: 301},
	}
	written, err := Emit("vercel", eps, dir)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	// Two functions + one vercel.json.
	if len(written) != 3 {
		t.Fatalf("wrote %v, want 2 functions + vercel.json", written)
	}
	fn := mustRead(t, filepath.Join(dir, "api", "go-latest.js"))
	if !strings.Contains(fn, "runtime: 'edge'") || !strings.Contains(fn, "Response.redirect(to, 301)") {
		t.Errorf("vercel edge fn wrong:\n%s", fn)
	}
	vj := mustRead(t, filepath.Join(dir, "vercel.json"))
	for _, want := range []string{`"source": "/go/latest"`, `"destination": "/api/go-latest"`, `"source": "/api/quote"`} {
		if !strings.Contains(vj, want) {
			t.Errorf("vercel.json missing %q in:\n%s", want, vj)
		}
	}
}

// TestPathSlug: path → flat function-name segment.
func TestPathSlug(t *testing.T) {
	cases := map[string]string{"/api/quote": "api-quote", "/go/": "go", "/": "index", "": "index"}
	for in, want := range cases {
		if got := pathSlug(in); got != want {
			t.Errorf("pathSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNetlifyVercelSourceErrors: both adapters reject the same malformed inputs.
func TestNetlifyVercelSourceErrors(t *testing.T) {
	bad := []config.Endpoint{
		{Path: "/x", Type: "redirect"},                        // no 'to'
		{Path: "/x", Type: "redirect", To: "/y", Status: 299}, // not 3xx
		{Path: "/x", Type: "proxy"},                           // no target
		{Path: "/x", Type: "mystery"},
	}
	for _, ep := range bad {
		if _, err := netlifySource(ep); err == nil {
			t.Errorf("netlify: expected error for %+v", ep)
		}
		if _, err := vercelSource(ep); err == nil {
			t.Errorf("vercel: expected error for %+v", ep)
		}
	}
	// Well-formed redirect + proxy compile on both.
	for _, ep := range []config.Endpoint{
		{Path: "/x", Type: "redirect", To: "/y"},
		{Path: "/x", Type: "proxy", Target: "https://api.test/x", Methods: []string{"GET"}},
	} {
		if _, err := netlifySource(ep); err != nil {
			t.Errorf("netlify rejected valid %+v: %v", ep, err)
		}
		if _, err := vercelSource(ep); err != nil {
			t.Errorf("vercel rejected valid %+v: %v", ep, err)
		}
	}
}

// TestFormCodegen: all three adapters generate a form handler that POST-guards,
// honeypots, collects declared fields and delivers to the webhook.
func TestFormCodegen(t *testing.T) {
	ep := config.Endpoint{
		Path: "/api/contact", Type: "form", To: "https://hooks.example.com/mail",
		Fields: []string{"name", "email"}, Honeypot: "company", Redirect: "/thanks/",
	}
	for name, gen := range map[string]func(config.Endpoint) (string, error){
		"cloudflare": cloudflareSource, "netlify": netlifySource, "vercel": vercelSource,
	} {
		src, err := gen(ep)
		if err != nil {
			t.Fatalf("%s form: %v", name, err)
		}
		for _, want := range []string{
			`request.method !== "POST"`,
			"await request.formData()",
			`form.get("company")`, // honeypot
			`payload["name"]`,
			`fetch("https://hooks.example.com/mail"`,
			`Response.redirect(new URL("/thanks/", request.url).toString(), 303)`,
		} {
			if !strings.Contains(src, want) {
				t.Errorf("%s form missing %q in:\n%s", name, want, src)
			}
		}
		// A form without a delivery target is rejected.
		if _, err := gen(config.Endpoint{Path: "/f", Type: "form"}); err == nil {
			t.Errorf("%s: form without 'to' must error", name)
		}
	}
}

// TestEmitFiltersAuth: auth guards are self-hosted only, so adapters never
// compile them — an auth-only set emits nothing, and mixed sets skip the guard.
func TestEmitFiltersAuth(t *testing.T) {
	dir := t.TempDir()
	// Auth-only → no files.
	if files, err := Emit("cloudflare", []config.Endpoint{
		{Path: "/members/", Type: "auth", User: "u", Password: "p"},
	}, dir); err != nil || files != nil {
		t.Errorf("auth-only must emit nothing, got %v %v", files, err)
	}
	// Mixed → only the redirect compiles.
	files, err := Emit("cloudflare", []config.Endpoint{
		{Path: "/members/", Type: "auth", User: "u", Password: "p"},
		{Path: "/go", Type: "redirect", To: "/new/"},
	}, dir)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(files) != 1 || files[0] != filepath.Join("functions", "go.js") {
		t.Errorf("mixed set must emit only the redirect, got %v", files)
	}
}
