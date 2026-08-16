package main

// The approve-then-PR flow's last step (#1.8.16). It runs when a human has
// already approved an assistant's work, which is the worst moment to discover
// the request was malformed — so every path is exercised against a local
// server instead of shipping it untested.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// githubStub answers like the PR endpoint and records what it was sent.
func githubStub(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *map[string]string) {
	t.Helper()
	var gotReq *http.Request
	payload := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r.Clone(r.Context())
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old; srv.Close() })
	return srv, gotReq, &payload
}

func TestOpenGitHubPRSuccess(t *testing.T) {
	_, _, payload := githubStub(t, http.StatusCreated,
		`{"html_url":"https://github.com/spagu/ssg/pull/7"}`)

	url, err := openGitHubPR("tok", "spagu/ssg", "", "feature", "Title", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/spagu/ssg/pull/7" {
		t.Fatalf("url = %q", url)
	}
	// An unset base means the project's default branch, not an empty one the
	// API would reject.
	if (*payload)["base"] != "main" {
		t.Errorf("base = %q, want main", (*payload)["base"])
	}
	if (*payload)["head"] != "feature" || (*payload)["title"] != "Title" || (*payload)["body"] != "Body" {
		t.Errorf("payload = %v", *payload)
	}
}

func TestOpenGitHubPRSendsAuth(t *testing.T) {
	var auth, accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, accept = r.Header.Get("Authorization"), r.Header.Get("Accept")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"html_url":"https://example.com/pr/1"}`))
	}))
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old; srv.Close() })

	if _, err := openGitHubPR("s3cret", "o/r", "trunk", "h", "t", "b"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer s3cret" {
		t.Errorf("Authorization = %q", auth)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", accept)
	}
}

// TestOpenGitHubPRFailures: every refusal names what happened, because the
// human on the other end has just approved something and needs to know why it
// did not land.
func TestOpenGitHubPRFailures(t *testing.T) {
	t.Run("api error carries the status and the body", func(t *testing.T) {
		githubStub(t, http.StatusUnprocessableEntity, `{"message":"No commits between main and feature"}`)
		_, err := openGitHubPR("tok", "o/r", "main", "feature", "t", "b")
		if err == nil || !strings.Contains(err.Error(), "422") ||
			!strings.Contains(err.Error(), "No commits") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a response without a URL is not a success", func(t *testing.T) {
		githubStub(t, http.StatusCreated, `{"html_url":""}`)
		if _, err := openGitHubPR("tok", "o/r", "", "h", "t", "b"); err == nil {
			t.Fatal("an empty html_url must not be reported as a PR")
		}
	})

	t.Run("unreadable response", func(t *testing.T) {
		githubStub(t, http.StatusCreated, `not json`)
		if _, err := openGitHubPR("tok", "o/r", "", "h", "t", "b"); err == nil {
			t.Fatal("unparseable body must fail")
		}
	})

	t.Run("unreachable API", func(t *testing.T) {
		old := githubAPIBase
		githubAPIBase = "http://127.0.0.1:1" // nothing listens there
		t.Cleanup(func() { githubAPIBase = old })
		if _, err := openGitHubPR("tok", "o/r", "", "h", "t", "b"); err == nil {
			t.Fatal("an unreachable API must fail")
		}
	})

	t.Run("unbuildable request", func(t *testing.T) {
		old := githubAPIBase
		githubAPIBase = "://not a url"
		t.Cleanup(func() { githubAPIBase = old })
		if _, err := openGitHubPR("tok", "o/r", "", "h", "t", "b"); err == nil {
			t.Fatal("a malformed base must fail before any request")
		}
	})
}

// TestBuildMCPGitCreatePR: the wiring hands the configured token, repo and
// default branch to the call — a PR opened against the wrong branch is a
// review nobody asked for.
func TestBuildMCPGitCreatePR(t *testing.T) {
	_, _, payload := githubStub(t, http.StatusCreated, `{"html_url":"https://example.com/pr/9"}`)
	t.Setenv("SSG_TEST_PR_TOKEN", "tok")

	cfg := &config.Config{}
	cfg.MCP.Git.Token = "$SSG_TEST_PR_TOKEN"
	cfg.MCP.Git.Repo = "spagu/ssg"
	cfg.MCP.Git.DefaultBranch = "trunk"
	g := buildMCPGit(cfg)
	if g.CreatePR == nil {
		t.Fatal("a configured token must expose the PR flow")
	}
	url, err := g.CreatePR("ssg/feature", "Title", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.com/pr/9" {
		t.Fatalf("url = %q", url)
	}
	if (*payload)["base"] != "trunk" {
		t.Errorf("configured default branch not used: %v", *payload)
	}
}
