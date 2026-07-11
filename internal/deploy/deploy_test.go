package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalProvider(t *testing.T) {
	cases := map[string]string{
		"cloudflare":       ProviderCloudflare,
		"cloudflare-pages": ProviderCloudflare,
		"github-pages":     ProviderGitHubPages,
		"gh-pages":         ProviderGitHubPages,
		"GitHub":           ProviderGitHubPages,
		"netlify":          ProviderNetlify,
		"vercel":           ProviderVercel,
		"ftp":              ProviderFTP,
		"sftp":             ProviderSFTP,
		"ssh":              ProviderSFTP,
		"  Vercel  ":       ProviderVercel,
		"nope":             "",
	}
	for in, want := range cases {
		if got := canonicalProvider(in); got != want {
			t.Errorf("canonicalProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSupported(t *testing.T) {
	if !Supported("cloudflare") || !Supported("ssh") {
		t.Error("expected known providers to be supported")
	}
	if Supported("dropbox") {
		t.Error("dropbox should not be supported")
	}
	if len(SupportedProviders()) != 6 {
		t.Errorf("expected 6 providers, got %d", len(SupportedProviders()))
	}
}

func TestRunUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(context.Background(), Options{Provider: "s3", Dir: dir}); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestRunMissingDir(t *testing.T) {
	if _, err := Run(context.Background(), Options{Provider: "cloudflare", Dir: ""}); err == nil {
		t.Error("expected error for empty dir")
	}
	if _, err := Run(context.Background(), Options{Provider: "cloudflare", Dir: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Error("expected error for missing dir")
	}
}

func TestParseDeployURL(t *testing.T) {
	u, err := parseDeployURL("ftp://user@host/path", "ftp", 21)
	if err != nil {
		t.Fatalf("parseDeployURL: %v", err)
	}
	if u.Host != "host:21" {
		t.Errorf("default port not applied: %q", u.Host)
	}
	if u, _ := parseDeployURL("sftp://h:2222/p", "sftp", 22); u.Host != "h:2222" {
		t.Errorf("explicit port lost: %q", u.Host)
	}
	// Errors: empty, wrong scheme, no host.
	if _, err := parseDeployURL("", "ftp", 21); err == nil {
		t.Error("empty target should error")
	}
	if _, err := parseDeployURL("http://h/p", "ftp", 21); err == nil {
		t.Error("wrong scheme should error")
	}
}

func TestCredentials(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"U": "envuser", "P": "envpass"}[k]
	}
	o := Options{Env: env}
	// URL userinfo wins.
	u, _ := parseDeployURL("ftp://alice:secret@host/p", "ftp", 21)
	if user, pass := o.credentials(u, "U", "P"); user != "alice" || pass != "secret" {
		t.Errorf("url creds = %q/%q", user, pass)
	}
	// Falls back to env when URL has none.
	u2, _ := parseDeployURL("ftp://host/p", "ftp", 21)
	if user, pass := o.credentials(u2, "U", "P"); user != "envuser" || pass != "envpass" {
		t.Errorf("env creds = %q/%q", user, pass)
	}
}

func TestWalkFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "sub", "b.css"), []byte("B"), 0o644)
	files, err := walkFiles(dir)
	if err != nil {
		t.Fatalf("walkFiles: %v", err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.Rel] = string(f.Data)
	}
	if got["a.txt"] != "A" || got["sub/b.css"] != "B" {
		t.Errorf("walkFiles = %#v", got)
	}
}
