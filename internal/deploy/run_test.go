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
