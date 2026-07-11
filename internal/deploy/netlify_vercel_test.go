package deploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNetlifyDeploy(t *testing.T) {
	uploads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/deploys") && r.Method == http.MethodPost && !strings.Contains(r.URL.Path, "/files/"):
			var body struct {
				Files map[string]string `json:"files"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			required := make([]string, 0, len(body.Files))
			for _, sha := range body.Files {
				required = append(required, sha)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "required": required, "ssl_url": "https://site.netlify.app"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/files/"):
			uploads++
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	oldBase := netlifyAPIBase
	netlifyAPIBase = srv.URL
	t.Cleanup(func() { netlifyAPIBase = oldBase })

	url, err := deployNetlify(context.Background(), Options{
		Provider: "netlify", Dir: writeSite(t), Project: "site-id", Quiet: true,
		Env: func(k string) string { return map[string]string{"NETLIFY_AUTH_TOKEN": "tok"}[k] },
	})
	if err != nil {
		t.Fatalf("deployNetlify: %v", err)
	}
	if url != "https://site.netlify.app" {
		t.Errorf("url = %q", url)
	}
	if uploads == 0 {
		t.Error("expected at least one file upload")
	}
}

func TestNetlifyMissingCreds(t *testing.T) {
	_, err := deployNetlify(context.Background(), Options{Dir: writeSite(t), Env: func(string) string { return "" }})
	if err == nil {
		t.Error("expected error when site/token missing")
	}
}

func TestVercelDeploy(t *testing.T) {
	uploads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/files"):
			if r.Header.Get("x-vercel-digest") == "" {
				http.Error(w, "missing digest", http.StatusBadRequest)
				return
			}
			uploads++
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/v13/deployments"):
			_ = json.NewEncoder(w).Encode(map[string]any{"url": "proj-abc.vercel.app"})
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	oldBase := vercelAPIBase
	vercelAPIBase = srv.URL
	t.Cleanup(func() { vercelAPIBase = oldBase })

	url, err := deployVercel(context.Background(), Options{
		Provider: "vercel", Dir: writeSite(t), Project: "proj", Quiet: true,
		Env: func(k string) string { return map[string]string{"VERCEL_TOKEN": "tok"}[k] },
	})
	if err != nil {
		t.Fatalf("deployVercel: %v", err)
	}
	if url != "https://proj-abc.vercel.app" {
		t.Errorf("url = %q", url)
	}
	if uploads == 0 {
		t.Error("expected file uploads")
	}
}

func TestVercelMissingCreds(t *testing.T) {
	_, err := deployVercel(context.Background(), Options{Dir: writeSite(t), Env: func(string) string { return "" }})
	if err == nil {
		t.Error("expected error when project/token missing")
	}
}
