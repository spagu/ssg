package main

// `ssg --http --watch --edit` (GO-102, phase 1).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/editui"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/mcp"
)

// TestEditPreconditions: the combinations that would make edit mode unsafe or
// useless are refused, and each says which one it is.
func TestEditPreconditions(t *testing.T) {
	cases := []struct {
		name  string
		cfg   config.Config
		token string
		wants string
	}{
		{"no server", config.Config{Watch: true}, "", "--http"},
		{"no watch", config.Config{HTTP: true}, "", "--watch"},
		{"public without a token", config.Config{HTTP: true, Watch: true, Host: "0.0.0.0"}, "", "network"},
	}
	for _, c := range cases {
		err := checkEditPreconditions(&c.cfg, c.token)
		if err == nil || !strings.Contains(err.Error(), c.wants) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	ok := []struct {
		name  string
		cfg   config.Config
		token string
	}{
		{"loopback", config.Config{HTTP: true, Watch: true, Host: "127.0.0.1"}, ""},
		{"no host set", config.Config{HTTP: true, Watch: true}, ""},
		{"public with a token", config.Config{HTTP: true, Watch: true, Host: "0.0.0.0"}, "chosen"},
	}
	for _, c := range ok {
		if err := checkEditPreconditions(&c.cfg, c.token); err != nil {
			t.Errorf("%s should be allowed: %v", c.name, err)
		}
	}
}

// TestStartEditModeIsOffByDefault, and refuses rather than half-starting.
func TestStartEditModeIsOffByDefault(t *testing.T) {
	t.Cleanup(stopEditMode)
	stopEditMode()
	if !startEditMode(generator.Config{}, &config.Config{}) {
		t.Error("without --edit there is nothing to start and nothing to refuse")
	}
	if editModeEnabled() {
		t.Error("edit mode must be off by default")
	}
	if startEditMode(generator.Config{}, &config.Config{Edit: true, Quiet: true}) {
		t.Error("--edit without --http must stop the run")
	}
	if editModeEnabled() {
		t.Error("a refused start must not leave an editor running")
	}
}

// TestStartEditModeRuns: with the right flags an editor is running, its token
// is set, and the middleware serves it.
func TestStartEditModeRuns(t *testing.T) {
	t.Cleanup(stopEditMode)
	stopEditMode()
	t.Setenv("SSG_EDIT_TOKEN", "chosen")
	t.Chdir(t.TempDir())
	cfg := &config.Config{Edit: true, HTTP: true, Watch: true, Host: "127.0.0.1", Quiet: true,
		OutputDir: ".out", ContentDir: "content", TemplatesDir: "templates", StaticDir: "static"}
	if !startEditMode(generator.Config{}, cfg) {
		t.Fatal("edit mode should have started")
	}
	if !editModeEnabled() {
		t.Fatal("no editor is running")
	}

	served := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><h1>Hi</h1></body></html>"))
	})
	h := editMiddleware(served)

	// The panel is injected into HTML.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "ssg-edit-bar") {
		t.Errorf("the panel was not injected:\n%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `var TOKEN="chosen"`) {
		t.Error("the session token was not carried into the page")
	}

	// The editing endpoints answer, and refuse without the token.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, editui.Path+"status", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated endpoint: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, editui.Path+"status", nil)
	r.Header.Set(editui.TokenHeader, "chosen")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("authenticated endpoint: %d — %s", w.Code, w.Body.String())
	}
}

// TestEditMiddlewareLeavesNonHTMLAlone, and is a no-op when no editor runs.
func TestEditMiddlewareLeavesNonHTMLAlone(t *testing.T) {
	t.Cleanup(stopEditMode)
	stopEditMode()
	plain := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1}`))
	})
	// No editor: the handler is returned untouched.
	w := httptest.NewRecorder()
	editMiddleware(plain).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x.json", nil))
	if w.Body.String() != `{"a":1}` {
		t.Errorf("body = %q", w.Body.String())
	}

	t.Setenv("SSG_EDIT_TOKEN", "chosen")
	t.Chdir(t.TempDir())
	if !startEditMode(generator.Config{}, &config.Config{Edit: true, HTTP: true, Watch: true, Quiet: true}) {
		t.Fatal("setup")
	}
	w = httptest.NewRecorder()
	editMiddleware(plain).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x.json", nil))
	if w.Body.String() != `{"a":1}` {
		t.Errorf("JSON must pass through untouched: %q", w.Body.String())
	}
	// An HTML page with no </body> still gets the panel, and a status other
	// than 200 is preserved.
	odd := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<h1>gone</h1>"))
	})
	w = httptest.NewRecorder()
	editMiddleware(odd).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope/", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ssg-edit-bar") {
		t.Error("a page without </body> should still get the panel")
	}
	// Compressed output is left alone: injecting into it would corrupt it.
	gz := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write([]byte("not really gzip"))
	})
	w = httptest.NewRecorder()
	editMiddleware(gz).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(w.Body.String(), "ssg-edit-bar") {
		t.Error("an encoded response must not be rewritten")
	}
}

// TestLocalGitStagesContentOnly: a project whose theme lives outside the
// repository must still be able to commit an edit to its own content.
func TestLocalGitStagesContentOnly(t *testing.T) {
	g := localGit(mcp.GitOptions{}, []string{"content", "posts"})
	if !g.Local {
		t.Error("an editing session needs the local git tools")
	}
	if g.BranchPrefix != "edit/" {
		t.Errorf("prefix = %q", g.BranchPrefix)
	}
	if len(g.StageDirs) != 2 || g.StageDirs[0] != "content" {
		t.Errorf("stage dirs = %v", g.StageDirs)
	}
	// A configured prefix is respected.
	if got := localGit(mcp.GitOptions{BranchPrefix: "cms/"}, nil); got.BranchPrefix != "cms/" {
		t.Errorf("prefix = %q", got.BranchPrefix)
	}
}

// TestEditFlagParses and is known to the unknown-flag check.
func TestEditFlagParses(t *testing.T) {
	cfg := &config.Config{}
	parseFlags([]string{"--edit"}, cfg)
	if !cfg.Edit {
		t.Error("--edit did not parse")
	}
	if !knownFlagNames(&config.Config{})["--edit"] {
		t.Error("--edit must be a known flag")
	}
}

// TestStartEditModeMintsATokenAndSaysSo when the operator did not choose one.
func TestStartEditModeMintsATokenAndSaysSo(t *testing.T) {
	t.Cleanup(stopEditMode)
	stopEditMode()
	t.Setenv("SSG_EDIT_TOKEN", "")
	t.Chdir(t.TempDir())
	out, err := captureStdout(func() error {
		if !startEditMode(generator.Config{}, &config.Config{Edit: true, HTTP: true, Watch: true}) {
			t.Error("edit mode should have started")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Token: ") || !strings.Contains(out, "branch of their own") {
		t.Errorf("the start-up banner should carry the token and the git rule:\n%s", out)
	}
	if strings.Contains(out, "Token: \n") {
		t.Error("the minted token is empty")
	}
}
