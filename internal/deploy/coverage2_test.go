package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ── Cloudflare low-level branches ───────────────────────────────────────────

func TestCloudflareDoNewRequestError(t *testing.T) {
	c := newCF(t, "http://example.com")
	if _, err := c.do(context.Background(), "BAD METHOD", c.baseURL, "tok", "", nil); err == nil {
		t.Error("expected NewRequest error for an invalid method")
	}
}

func TestCloudflareValidateViaDeploy(t *testing.T) {
	c := NewCloudflarePages(CloudflareConfig{APIToken: "", AccountID: "a", Project: "p", Quiet: true})
	if _, err := c.Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("Deploy must fail validation with no token")
	}
}

func TestCloudflareCheckMissingBadResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts/a/pages/projects/p/upload-token" {
			_, _ = w.Write([]byte(`{"success":true,"result":{"jwt":"j"}}`))
			return
		}
		// check-missing: result is not an array → unmarshal into []string fails.
		_, _ = w.Write([]byte(`{"success":true,"result":{"unexpected":"object"}}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a check-missing decode error")
	}
}

func TestCloudflareCreateDeployBadResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/a/pages/projects/p/upload-token":
			_, _ = w.Write([]byte(`{"success":true,"result":{"jwt":"j"}}`))
		case "/pages/assets/check-missing":
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		default: // /deployments: result is a string, not the expected object
			_, _ = w.Write([]byte(`{"success":true,"result":"scalar"}`))
		}
	}))
	t.Cleanup(srv.Close)
	if _, err := newCF(t, srv.URL).Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected a create-deployment decode error")
	}
}

// ── Netlify low-level branches ──────────────────────────────────────────────

func TestNetlifyCreateDeployConnRefused(t *testing.T) {
	old := netlifyAPIBase
	netlifyAPIBase = "http://127.0.0.1:1"
	t.Cleanup(func() { netlifyAPIBase = old })
	_, err := deployNetlify(context.Background(), Options{
		Dir: writeSite(t), Project: "s", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected connection error creating the Netlify deploy")
	}
}

func TestNetlifyCreateDeployHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest) // create-deploy returns >= 300
	}))
	t.Cleanup(srv.Close)
	old := netlifyAPIBase
	netlifyAPIBase = srv.URL
	t.Cleanup(func() { netlifyAPIBase = old })
	_, err := deployNetlify(context.Background(), Options{
		Dir: writeSite(t), Project: "s", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected an HTTP error creating the Netlify deploy")
	}
}

// ── Vercel low-level branches ───────────────────────────────────────────────

func TestVercelConnRefused(t *testing.T) {
	old := vercelAPIBase
	vercelAPIBase = "http://127.0.0.1:1"
	t.Cleanup(func() { vercelAPIBase = old })
	_, err := deployVercel(context.Background(), Options{
		Dir: writeSite(t), Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected connection error uploading to Vercel")
	}
}

func TestVercelCreateDeploymentHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/files" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "bad", http.StatusBadRequest) // deployment creation fails
	}))
	t.Cleanup(srv.Close)
	old := vercelAPIBase
	vercelAPIBase = srv.URL
	t.Cleanup(func() { vercelAPIBase = old })
	_, err := deployVercel(context.Background(), Options{
		Dir: writeSite(t), Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected an HTTP error creating the Vercel deployment")
	}
}

// ── GitHub Pages: token-header push path ────────────────────────────────────

func TestDeployGitHubPagesTokenHeader(t *testing.T) {
	site := t.TempDir()
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("x"), 0o644)
	// https remote + GITHUB_TOKEN → push uses an auth header (then fails: bad remote).
	_, err := deployGitHubPages(context.Background(), Options{
		Dir: site, Target: "https://127.0.0.1:1/nope.git", Quiet: true,
		Env: func(k string) string { return map[string]string{"GITHUB_TOKEN": "secret"}[k] },
	})
	if err == nil {
		t.Error("expected push to fail against an unreachable https remote")
	}
}

// ── FTP: anonymous default user ─────────────────────────────────────────────

func TestDeployFTPAnonymousUser(t *testing.T) {
	s := newFTPTestServer(t)
	// No user in the URL and no FTP_USERNAME → defaults to "anonymous".
	url, err := deployFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "ftp://" + s.addr() + "/pub", Quiet: true,
		Env: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("anonymous ftp: %v", err)
	}
	if url == "" {
		t.Error("expected a URL")
	}
}

// ── SFTP: auth and known_hosts error branches + home defaults ───────────────

func TestDeploySFTPAuthError(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_ = os.WriteFile(kh, []byte(""), 0o600)
	// No password and a missing key file → sshAuthMethods fails before dialing.
	_, err := deploySFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "sftp://user@127.0.0.1:22/p", Quiet: true,
		Env: func(k string) string {
			return map[string]string{"SSH_KEY_FILE": filepath.Join(t.TempDir(), "absent"), "SSH_KNOWN_HOSTS": kh}[k]
		},
	})
	if err == nil {
		t.Error("expected an auth error")
	}
}

func TestDeploySFTPKnownHostsError(t *testing.T) {
	// Valid password auth but a missing known_hosts file → knownHostsCallback fails.
	_, err := deploySFTP(context.Background(), Options{
		Dir: writeSite(t), Target: "sftp://user@127.0.0.1:22/p", Quiet: true,
		Env: func(k string) string {
			return map[string]string{"SSH_PASSWORD": "x", "SSH_KNOWN_HOSTS": filepath.Join(t.TempDir(), "absent")}[k]
		},
	})
	if err == nil {
		t.Error("expected a known_hosts error")
	}
}

func TestSSHAuthMethodsHomeDefault(t *testing.T) {
	// No password, no SSH_KEY_FILE → falls back to ~/.ssh/id_rsa (present or not,
	// the default-path branch runs).
	_, _ = sshAuthMethods(Options{Env: func(string) string { return "" }}, "")
}

func TestKnownHostsCallbackHomeDefault(t *testing.T) {
	// No SSH_KNOWN_HOSTS → falls back to ~/.ssh/known_hosts (default-path branch).
	_, _ = knownHostsCallback(Options{Env: func(string) string { return "" }})
}

// ── deploy.go + manifest.go helpers ─────────────────────────────────────────

func TestParseDeployURLInvalid(t *testing.T) {
	if _, err := parseDeployURL("://bad url", "ftp", 21); err == nil {
		t.Error("expected a parse error for a malformed URL")
	}
}

func TestWalkFilesMissingDir(t *testing.T) {
	if _, err := walkFiles(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("expected an error walking a missing directory")
	}
}

func TestCollectSiteFilesMissingDir(t *testing.T) {
	if _, err := collectSiteFiles(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("expected an error collecting from a missing directory")
	}
}
