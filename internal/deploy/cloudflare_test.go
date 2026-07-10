package deploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cloudflareMock serves the four Direct Upload endpoints and records what it saw.
func cloudflareMock(t *testing.T, uploaded *int) *httptest.Server {
	t.Helper()
	ok := func(w http.ResponseWriter, result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/upload-token"):
			ok(w, map[string]string{"jwt": "test-jwt"})
		case strings.HasSuffix(r.URL.Path, "/pages/assets/check-missing"):
			var body struct {
				Hashes []string `json:"hashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ok(w, body.Hashes) // everything missing → must be uploaded
		case strings.HasSuffix(r.URL.Path, "/pages/assets/upload"):
			var batch []map[string]any
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &batch)
			*uploaded += len(batch)
			ok(w, true)
		case strings.HasSuffix(r.URL.Path, "/deployments"):
			ok(w, map[string]string{"id": "dep1", "url": "https://test.pages.dev"})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "css"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>hi</html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "css", "app.css"), []byte("body{}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "_headers"), []byte("/*\n  X-Frame-Options: DENY"), 0o644)
	return dir
}

func TestCloudflareDeploy(t *testing.T) {
	srv := cloudflareMock(t, new(int))
	c := NewCloudflarePages(CloudflareConfig{AccountID: "acc", APIToken: "tok", Project: "proj", Quiet: true})
	c.baseURL = srv.URL

	url, err := c.Deploy(context.Background(), writeSite(t))
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if url != "https://test.pages.dev" {
		t.Errorf("deploy url = %q", url)
	}
}

func TestCloudflareValidate(t *testing.T) {
	cases := []CloudflareConfig{
		{APIToken: "", AccountID: "a", Project: "p"},
		{APIToken: "t", AccountID: "", Project: "p"},
		{APIToken: "t", AccountID: "a", Project: ""},
	}
	for i, cfg := range cases {
		if err := NewCloudflarePages(cfg).Validate(); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
	if err := NewCloudflarePages(CloudflareConfig{APIToken: "t", AccountID: "a", Project: "p"}).Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestCloudflareAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 10000, "message": "bad token"}},
		})
	}))
	t.Cleanup(srv.Close)
	c := NewCloudflarePages(CloudflareConfig{AccountID: "a", APIToken: "t", Project: "p", Quiet: true})
	c.baseURL = srv.URL
	if _, err := c.Deploy(context.Background(), writeSite(t)); err == nil {
		t.Error("expected error from failing API")
	}
}

func TestDeployCloudflareViaDispatcher(t *testing.T) {
	srv := cloudflareMock(t, new(int))
	// deployCloudflare reads creds from Env and the package default base; point that
	// at the mock for the duration of the test.
	oldBase := defaultAPIBase
	defaultAPIBase = srv.URL
	t.Cleanup(func() { defaultAPIBase = oldBase })

	url, err := Run(context.Background(), Options{
		Provider: "cloudflare",
		Dir:      writeSite(t),
		Project:  "proj",
		Quiet:    true,
		Env: func(k string) string {
			return map[string]string{"CLOUDFLARE_ACCOUNT_ID": "acc", "CLOUDFLARE_API_TOKEN": "tok"}[k]
		},
	})
	if err != nil {
		t.Fatalf("Run cloudflare: %v", err)
	}
	if url == "" {
		t.Error("expected a deploy URL")
	}
}
