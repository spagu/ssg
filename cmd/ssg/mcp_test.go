package main

import (
	"fmt"
	"os"
	"path/filepath"
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

// TestGitRunner: git is resolved to an absolute path once and actually runs; when
// it cannot be resolved, the runner refuses instead of executing anything.
func TestGitRunner(t *testing.T) {
	if out, err := gitRunner()("--version"); err != nil || !strings.Contains(out, "git version") {
		t.Errorf("git --version = %q, %v", out, err)
	}
	t.Setenv("PATH", t.TempDir()) // no git on PATH
	out, err := gitRunner()("status")
	if err == nil || !strings.Contains(err.Error(), "git is unavailable") || out != "" {
		t.Errorf("missing git must refuse: %q, %v", out, err)
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

// TestParseMCPArgsSplitsItsOwnFlagsFromTheBuildFlags: everything `ssg mcp` owns
// is consumed here, and everything else reaches the ordinary build parser — a
// directory named like a flag is the case that used to be easy to break.
func TestParseMCPArgsSplitsItsOwnFlagsFromTheBuildFlags(t *testing.T) {
	p := parseMCPArgs([]string{
		"--listen=127.0.0.1:7823", "--token=s3cret", "--allow-origin=https://example.com",
		"--allow-origin=https://cms.example.com", "--no-stdio", "--no-watch",
		"--role=designer", "--role=content", "site", "--http",
	})
	if p.done {
		t.Fatalf("a well-formed command line must run, code = %d", p.code)
	}
	if p.netCfg.listen != "127.0.0.1:7823" || p.netCfg.token != "s3cret" || !p.netCfg.noStdio {
		t.Errorf("network flags = %+v", p.netCfg)
	}
	if len(p.netCfg.origins) != 2 {
		t.Errorf("--allow-origin must be repeatable, got %v", p.netCfg.origins)
	}
	if p.watch {
		t.Error("--no-watch must turn rebuilds off")
	}
	if !p.roles["designer"] || !p.roles["content"] {
		t.Errorf("roles = %v", p.roles)
	}
	if len(p.rest) != 2 || p.rest[0] != "site" || p.rest[1] != "--http" {
		t.Errorf("unrecognised arguments must pass through, got %v", p.rest)
	}
}

// TestParseMCPArgsStopsOnHelpAndOnABadRole: both end the command rather than
// starting a server nobody asked for.
func TestParseMCPArgsStopsOnHelpAndOnABadRole(t *testing.T) {
	if _, err := captureStdout(func() error {
		if p := parseMCPArgs([]string{"--help"}); !p.done || p.code != 0 {
			t.Errorf("--help: done = %v code = %d", p.done, p.code)
		}
		if p := parseMCPArgs([]string{"-h"}); !p.done || p.code != 0 {
			t.Errorf("-h: done = %v code = %d", p.done, p.code)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if p := parseMCPArgs([]string{"--role=editor"}); !p.done || p.code != 2 {
		t.Errorf("an unknown role: done = %v code = %d", p.done, p.code)
	}
	// Defaults, for the plain `ssg mcp` everybody actually types.
	p := parseMCPArgs(nil)
	if !p.watch || len(p.roles) != 0 || p.netCfg.listen != "" {
		t.Errorf("defaults = %+v", p)
	}
}

// TestGitRunnerRefusesARelativeBinary: a git resolved from a relative PATH entry
// can be swapped between calls, so the runner refuses rather than executing it.
func TestGitRunnerRefusesARelativeBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"), 0o755); err != nil { // #nosec G306 -- test fixture must be executable
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("PATH", ".")
	if _, err := gitRunner()("status"); err == nil {
		t.Fatal("a relative git must not be executed")
	}
}

// TestBuildMCPGitDerivesTheRepoFromTheRemote: with a token but no configured
// repo, the remote is the only place the answer can come from.
func TestBuildMCPGitDerivesTheRepoFromTheRemote(t *testing.T) {
	t.Setenv("MCP_GH_REMOTE", "tok")
	cfg := &config.Config{}
	cfg.MCP.Git.Token = "$MCP_GH_REMOTE"
	g := buildMCPGit(cfg) // repo unset: derived from `git remote get-url`
	if !g.Enabled() || g.CreatePR == nil {
		t.Fatalf("git flow must be enabled by a token alone: %+v", g)
	}
}
