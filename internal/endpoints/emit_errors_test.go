package endpoints

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestFormBodyJSONDoneAndCollectAll: without a redirect the generated handler
// answers with a JSON {ok:true} body, and without declared fields it collects
// every form entry, skipping the honeypot key inside the loop.
func TestFormBodyJSONDoneAndCollectAll(t *testing.T) {
	src, err := formBodyJS(config.Endpoint{Type: "form", To: "https://hooks.test/x", Honeypot: "hp"})
	if err != nil {
		t.Fatalf("formBodyJS: %v", err)
	}
	for _, want := range []string{
		`new Response(JSON.stringify({ ok: true })`,
		"for (const [k, v] of form.entries())",
		`if (k === "hp") continue;`,
		"payload[k] = v.toString();",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("collect-all form missing %q in:\n%s", want, src)
		}
	}
	// Without a honeypot the collect-all loop carries no skip line.
	plain, err := formBodyJS(config.Endpoint{Type: "form", To: "https://hooks.test/x"})
	if err != nil {
		t.Fatalf("formBodyJS: %v", err)
	}
	if strings.Contains(plain, "continue;") {
		t.Errorf("honeypot-free form must not skip keys:\n%s", plain)
	}
}

// TestFormBodyHoneypotInDeclaredFields: a honeypot listed among the declared
// fields is never collected into the delivery payload.
func TestFormBodyHoneypotInDeclaredFields(t *testing.T) {
	src, err := formBodyJS(config.Endpoint{
		Type: "form", To: "https://hooks.test/x",
		Fields: []string{"name", "hp"}, Honeypot: "hp",
	})
	if err != nil {
		t.Fatalf("formBodyJS: %v", err)
	}
	if !strings.Contains(src, `payload["name"]`) {
		t.Errorf("declared field must be collected:\n%s", src)
	}
	if strings.Contains(src, `payload["hp"]`) {
		t.Errorf("honeypot field must never reach the payload:\n%s", src)
	}
}

// TestMethodsLitBlankEntries: entries that trim to nothing yield no literal at
// all, so the generated handler applies no method restriction.
func TestMethodsLitBlankEntries(t *testing.T) {
	if got := methodsLit([]string{"  ", ""}); got != "" {
		t.Errorf("methodsLit(blank) = %q, want empty", got)
	}
}

// TestEmitAdapterErrorPropagation: every adapter wraps a per-endpoint compile
// error with the endpoint path.
func TestEmitAdapterErrorPropagation(t *testing.T) {
	bad := []config.Endpoint{{Path: "/x", Type: "mystery"}}
	for _, platform := range []string{"cloudflare", "netlify", "vercel"} {
		if _, err := Emit(platform, bad, t.TempDir()); err == nil ||
			!strings.Contains(err.Error(), `endpoint "/x"`) {
			t.Errorf("%s: want wrapped endpoint error, got %v", platform, err)
		}
	}
}

// TestEmitMkdirFailure: an output "directory" that is a regular file fails the
// function-directory creation on every platform.
func TestEmitMkdirFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "outfile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	eps := []config.Endpoint{{Path: "/go/latest", Type: "redirect", To: "/new/"}}
	for _, platform := range []string{"cloudflare", "netlify", "vercel"} {
		if _, err := Emit(platform, eps, blocker); err == nil {
			t.Errorf("%s: emit into a file path must error", platform)
		}
	}
}

// TestEmitWriteFailure: a directory squatting the target function file name
// fails the module write on every platform.
func TestEmitWriteFailure(t *testing.T) {
	cases := map[string]string{
		"cloudflare": filepath.Join("functions", "go.js"),
		"netlify":    filepath.Join("netlify", "functions", "go.mjs"),
		"vercel":     filepath.Join("api", "go.js"),
	}
	eps := []config.Endpoint{{Path: "/go", Type: "redirect", To: "/new/"}}
	for platform, rel := range cases {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Emit(platform, eps, dir); err == nil {
			t.Errorf("%s: writing onto a directory must error", platform)
		}
	}
}

// TestVercelConfigWriteFailure: the functions emit fine but vercel.json cannot
// be written because a directory occupies its name.
func TestVercelConfigWriteFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vercel.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	eps := []config.Endpoint{{Path: "/go", Type: "redirect", To: "/new/"}}
	if _, err := Emit("vercel", eps, dir); err == nil {
		t.Error("vercel.json blocked by a directory must error")
	}
}
