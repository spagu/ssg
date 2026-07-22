package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasPagesFunctions(t *testing.T) {
	dir := t.TempDir()
	if hasPagesFunctions(dir) {
		t.Fatal("empty dir should have no functions")
	}
	if err := os.MkdirAll(filepath.Join(dir, "functions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasPagesFunctions(dir) {
		t.Fatal("functions dir should be detected")
	}
}

func TestDeployCloudflareWrangler_ArgvAndEnvPassthrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "functions"), 0o755); err != nil {
		t.Fatal(err)
	}
	var gotName string
	var gotArgs []string
	o := Options{
		Provider: ProviderCloudflare,
		Dir:      dir,
		Project:  "my-site",
		Branch:   "main",
		Exec: func(_ context.Context, name string, args ...string) error {
			gotName = name
			gotArgs = args
			return nil
		},
	}
	if _, err := deployCloudflare(context.Background(), o); err != nil {
		t.Fatalf("deployCloudflare: %v", err)
	}
	if gotName != "npx" {
		t.Fatalf("expected npx, got %q", gotName)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"wrangler pages deploy", "--project-name=my-site", "--branch=main"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in argv: %s", want, joined)
		}
	}
}

func TestDeployCloudflareWrangler_MissingProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "functions"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := Options{Provider: ProviderCloudflare, Dir: dir, Exec: func(context.Context, string, ...string) error { return nil }}
	if _, err := deployCloudflare(context.Background(), o); err == nil {
		t.Fatal("expected a missing-project error")
	}
}

func TestRunNPXCommand(t *testing.T) {
	// `true` exits 0 on any POSIX system — exercises the real exec path.
	if err := runNPXCommand(context.Background(), "true"); err != nil {
		t.Fatalf("runNPXCommand(true): %v", err)
	}
	if err := runNPXCommand(context.Background(), "false"); err == nil {
		t.Fatal("runNPXCommand(false) should return the non-zero exit as an error")
	}
}

func TestDeployCloudflareWrangler_ExecError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "functions"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := Options{
		Provider: ProviderCloudflare,
		Dir:      dir,
		Project:  "p",
		Exec:     func(context.Context, string, ...string) error { return context.DeadlineExceeded },
	}
	_, err := deployCloudflare(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "wrangler pages deploy failed") {
		t.Fatalf("expected a wrapped wrangler error, got %v", err)
	}
}
