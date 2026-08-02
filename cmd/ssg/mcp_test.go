package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestParseRepoURL: owner/name extraction from https and ssh remotes.
func TestParseRepoURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/spagu/ssg.git": "spagu/ssg",
		"git@github.com:spagu/ssg.git":     "spagu/ssg",
		"https://example.com/x.git":        "",
	} {
		if got := parseRepoURL(in); got != want {
			t.Errorf("parseRepoURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExpandEnvValue: $VAR is read from the environment, literals pass through.
func TestExpandEnvValue(t *testing.T) {
	t.Setenv("MCP_TOK", "abc")
	if got := expandEnvValue("$MCP_TOK"); got != "abc" {
		t.Errorf("env expand = %q", got)
	}
	if got := expandEnvValue("literal"); got != "literal" {
		t.Errorf("literal = %q", got)
	}
}

// TestContentRoots: content_dir plus every content_sources path.
func TestContentRoots(t *testing.T) {
	cfg := &config.Config{ContentDir: "content"}
	cfg.ContentSources = []config.ContentSource{{Path: "extra"}, {Path: ""}}
	got := contentRoots(cfg)
	if len(got) != 2 || got[0] != "content" || got[1] != "extra" {
		t.Errorf("contentRoots = %v", got)
	}
}

// TestBuildMCPGit: without a token the git tools stay disabled; with one the repo
// is derived from config and CreatePR is wired.
func TestBuildMCPGit(t *testing.T) {
	cfg := &config.Config{}
	if g := buildMCPGit(cfg); g.Enabled() {
		t.Error("no token must not enable git")
	}
	t.Setenv("MCP_GH", "tok")
	cfg.MCP.Git.Token = "$MCP_GH"
	cfg.MCP.Git.Repo = "spagu/ssg"
	g := buildMCPGit(cfg)
	if !g.Enabled() || g.Repo != "spagu/ssg" || g.CreatePR == nil {
		t.Errorf("git flow not wired: enabled=%v repo=%q", g.Enabled(), g.Repo)
	}
}

// TestCaptureStdout: printed output is captured and the error passes through.
func TestCaptureStdout(t *testing.T) {
	out, err := captureStdout(func() error {
		fmt.Println("hello from build")
		return nil
	})
	if err != nil || !strings.Contains(out, "hello from build") {
		t.Errorf("capture = %q, %v", out, err)
	}
}

// TestRoleNames: display names for the role sets.
func TestRoleNames(t *testing.T) {
	if roleNames(nil) != "designer+content" {
		t.Error("empty roles must mean both")
	}
	if roleNames(map[string]bool{"content": true}) != "content" {
		t.Error("single role name")
	}
}
