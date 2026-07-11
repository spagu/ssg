package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNetlifyWalkError(t *testing.T) {
	_, err := deployNetlify(context.Background(), Options{
		Dir: filepath.Join(t.TempDir(), "absent"), Project: "s", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected a scan error for a missing directory")
	}
}

func TestVercelWalkError(t *testing.T) {
	_, err := deployVercel(context.Background(), Options{
		Dir: filepath.Join(t.TempDir(), "absent"), Project: "p", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "t"}[k] },
	})
	if err == nil {
		t.Error("expected a scan error for a missing directory")
	}
}

func TestNetlifyCreateDeployBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json")) // 200 but unparseable → decode error
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
		t.Error("expected a decode error from the Netlify create-deploy response")
	}
}

func TestVercelCreateDeploymentBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/files" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("not json")) // deployment response unparseable
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
		t.Error("expected a decode error from the Vercel deployment response")
	}
}

// TestGitHubPagesOriginLookup runs from a non-git directory with no explicit target,
// so the origin lookup returns empty and deployGitHubPages reports "no git remote".
func TestGitHubPagesOriginLookup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	_, err = deployGitHubPages(context.Background(), Options{
		Dir: tmp, Quiet: true, Env: func(string) string { return "" },
	})
	if err == nil {
		t.Error("expected a 'no git remote' error outside any repository")
	}
}
