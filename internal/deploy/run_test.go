package deploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRunDispatchesAllProviders drives Run through every provider's entry point. With
// no credentials/target each fails fast, but the dispatch switch and each provider's
// validation branch are exercised.
func TestRunDispatchesAllProviders(t *testing.T) {
	dir := writeSite(t)
	noenv := func(string) string { return "" }
	for _, provider := range []string{"cloudflare", "netlify", "vercel", "ftp", "sftp"} {
		if _, err := Run(context.Background(), Options{Provider: provider, Dir: dir, Env: noenv}); err == nil {
			t.Errorf("Run(%q) without credentials should error", provider)
		}
	}
	// github-pages with an invalid explicit remote fails at push, not origin lookup.
	if _, err := Run(context.Background(), Options{
		Provider: "github-pages", Dir: dir, Target: t.TempDir() + "/nope", Env: noenv,
	}); err == nil {
		t.Error("Run(github-pages) with a bad remote should error")
	}
}

func TestCloudflareEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Report success but an empty jwt → uploadToken must reject it.
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]string{"jwt": ""}})
	}))
	t.Cleanup(srv.Close)
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Quiet: true})
	c.baseURL = srv.URL
	if _, err := c.Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected error when upload token is empty")
	}
}

func TestCloudflareEmptyDir(t *testing.T) {
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Quiet: true})
	if _, err := c.Deploy(context.Background(), t.TempDir()); err == nil {
		t.Error("expected error deploying an empty directory")
	}
}

func TestOptionsEnvDefault(t *testing.T) {
	// With no injected Env, env() must fall back to os.Getenv without panicking.
	o := Options{}
	_ = o.env("PATH")
	// Quiet logf is a no-op; non-quiet exercises the print branch.
	Options{Quiet: true}.logf("x %d", 1)
	Options{Quiet: false}.logf("y %d", 2)
}

func TestCloudflareDeployWithBranch(t *testing.T) {
	srv := cloudflareMock(t, new(int))
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Branch: "preview", Quiet: true})
	c.baseURL = srv.URL
	if _, err := c.Deploy(context.Background(), writeSite(t)); err != nil {
		t.Fatalf("Deploy with branch: %v", err)
	}
}

func TestNetlifyUploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { // create deploy → everything required
			var body struct {
				Files map[string]string `json:"files"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			req := make([]string, 0, len(body.Files))
			for _, s := range body.Files {
				req = append(req, s)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d", "required": req, "ssl_url": "https://x"})
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError) // PUT upload fails
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
		t.Error("expected error when Netlify upload fails")
	}
}

func TestVercelUploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError) // /v2/files fails
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
		t.Error("expected error when Vercel upload fails")
	}
}
