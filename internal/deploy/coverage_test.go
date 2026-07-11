package deploy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// ── Cloudflare error cascades ───────────────────────────────────────────────

func newCF(t *testing.T, base string) *CloudflarePages {
	t.Helper()
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Quiet: true})
	c.baseURL = base
	return c
}

func TestCloudflareConnRefused(t *testing.T) {
	c := newCF(t, "http://127.0.0.1:1")
	if _, err := c.Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a connection error")
	}
}

func TestCloudflareBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{ not json")) // decode failure in do()
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a decode error")
	}
}

func TestCloudflareCheckMissingFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/a/pages/projects/p/upload-token" {
			_, _ = w.Write([]byte(`{"success":true,"result":{"jwt":"j"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"nope"}]}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a check-missing error")
	}
}

func TestCloudflareCreateDeployFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/a/pages/projects/p/upload-token":
			_, _ = w.Write([]byte(`{"success":true,"result":{"jwt":"j"}}`))
		case "/pages/assets/check-missing":
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`)) // nothing to upload
		default: // /deployments → malformed
			_, _ = w.Write([]byte("nope"))
		}
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a create-deployment error")
	}
}

func TestCloudflareDeployVerbose(t *testing.T) {
	srv := cloudflareMock(t, new(int))
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Quiet: false})
	c.baseURL = srv.URL
	if _, err := c.Deploy(context.Background(), writeSite(t)); err != nil {
		t.Fatalf("verbose deploy: %v", err)
	}
}

// ── FTP error paths ─────────────────────────────────────────────────────────

func TestDeployFTPDialError(t *testing.T) {
	_, err := deployFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "ftp://user@127.0.0.1:1/p", Quiet: true,
		Env: func(k string) string { return map[string]string{"FTP_PASSWORD": "x"}[k] },
	})
	if err == nil {
		t.Error("expected a dial error to a closed port")
	}
}

func TestDeployFTPLoginError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = conn.Write([]byte("220 ready\r\n"))
		buf := make([]byte, 256)
		_, _ = conn.Read(buf) // USER
		_, _ = conn.Write([]byte("331 need pass\r\n"))
		_, _ = conn.Read(buf) // PASS
		_, _ = conn.Write([]byte("530 login incorrect\r\n"))
	}()
	_, err = deployFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "ftp://user@" + ln.Addr().String() + "/p", Quiet: true,
		Env: func(k string) string { return map[string]string{"FTP_PASSWORD": "bad"}[k] },
	})
	if err == nil {
		t.Error("expected a login error")
	}
}

// ── SFTP error paths ────────────────────────────────────────────────────────

func TestDeploySFTPDialError(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_ = os.WriteFile(kh, []byte(""), 0o600)
	_, err := deploySFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "sftp://user@127.0.0.1:1/p", Quiet: true,
		Env: func(k string) string {
			return map[string]string{"SSH_PASSWORD": "x", "SSH_KNOWN_HOSTS": kh}[k]
		},
	})
	if err == nil {
		t.Error("expected an ssh dial error")
	}
}

func TestSSHAuthMethodsPassphraseKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_enc")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		return map[string]string{"SSH_KEY_FILE": path, "SSH_KEY_PASSPHRASE": "hunter2"}[k]
	}
	methods, err := sshAuthMethods(Options{Env: env}, "")
	if err != nil || len(methods) != 1 {
		t.Fatalf("passphrase key auth = %v, %v", methods, err)
	}
	// Wrong passphrase → parse error.
	bad := func(k string) string {
		return map[string]string{"SSH_KEY_FILE": path, "SSH_KEY_PASSPHRASE": "wrong"}[k]
	}
	if _, err := sshAuthMethods(Options{Env: bad}, ""); err == nil {
		t.Error("expected a parse error with the wrong passphrase")
	}
}

// ── GitHub Pages: refuse a pre-existing .git ────────────────────────────────

func TestDeployGitHubPagesExistingGit(t *testing.T) {
	site := t.TempDir()
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(site, ".git"), 0o755)
	_, err := deployGitHubPages(context.Background(), Options{
		Dir: site, Target: "https://example.com/r.git", Quiet: true, Env: func(string) string { return "" },
	})
	if err == nil {
		t.Error("expected refusal when the output dir already has a .git")
	}
}

// ── deploy.go helpers ───────────────────────────────────────────────────────

func TestParseDeployURLNoHost(t *testing.T) {
	if _, err := parseDeployURL("ftp:///path", "ftp", 21); err == nil {
		t.Error("expected an error for a URL with no host")
	}
}
