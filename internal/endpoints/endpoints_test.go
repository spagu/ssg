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
