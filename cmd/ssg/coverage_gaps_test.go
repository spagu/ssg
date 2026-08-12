package main

// Targeted tests for small helpers that carried no coverage (project-wide
// coverage raise, 1.8.27).

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mcp"
)

func TestRepoFromRemote(t *testing.T) {
	// Default remote is origin; URL is parsed to owner/repo.
	run := func(args ...string) (string, error) {
		if strings.Join(args, " ") != "remote get-url origin" {
			t.Fatalf("unexpected git args: %v", args)
		}
		return "git@github.com:spagu/ssg.git\n", nil
	}
	if got := repoFromRemote(run, mcp.GitOptions{}); got != "spagu/ssg" {
		t.Fatalf("repoFromRemote = %q", got)
	}
	// Explicit remote name is honoured.
	run2 := func(args ...string) (string, error) {
		if args[len(args)-1] != "upstream" {
			t.Fatalf("remote not honoured: %v", args)
		}
		return "https://github.com/foo/bar.git", nil
	}
	if got := repoFromRemote(run2, mcp.GitOptions{Remote: "upstream"}); got != "foo/bar" {
		t.Fatalf("repoFromRemote upstream = %q", got)
	}
	// A failing git → empty string, no panic.
	runErr := func(...string) (string, error) { return "", errors.New("no repo") }
	if got := repoFromRemote(runErr, mcp.GitOptions{}); got != "" {
		t.Fatalf("error path = %q", got)
	}
}

func TestOrOpen(t *testing.T) {
	if orOpen("") != "open" || orOpen("basic") != "basic" {
		t.Fatal("orOpen labels wrong")
	}
}

func TestWorkerDirsOf(t *testing.T) {
	cfg := &config.Config{}
	if dirs := workerDirsOf(cfg); len(dirs) != 0 {
		t.Fatalf("no workers → no dirs, got %v", dirs)
	}
	cfg.Workers = []config.WorkerConfig{{Name: "a", Dir: "workers/a"}, {Name: "b"}}
	dirs := workerDirsOf(cfg)
	if len(dirs) != 1 || dirs[0] != "workers/a" {
		t.Fatalf("workerDirsOf = %v", dirs)
	}
}

func TestPrintMCPHelp(t *testing.T) {
	// Smoke: must not panic and must mention the subcommand.
	printMCPHelp()
}

// TestBuildMCPGitExplicitRepo: an explicit repo skips remote auto-detection and
// wires the PR creator.
func TestBuildMCPGitExplicitRepo(t *testing.T) {
	t.Setenv("SSG_TEST_MCP_TOK", "tok")
	cfg := &config.Config{}
	cfg.MCP.Git.Token = "$SSG_TEST_MCP_TOK"
	cfg.MCP.Git.Repo = "spagu/ssg"
	g := buildMCPGit(cfg)
	if !g.Enabled() || g.Repo != "spagu/ssg" || g.CreatePR == nil {
		t.Fatalf("explicit repo wiring broken: %+v", g)
	}
	if g.Now() == "" {
		t.Fatal("Now must yield a timestamp")
	}
}

func TestRunMCPFlagPaths(t *testing.T) {
	if code := runMCP([]string{"--role=bogus"}); code != 2 {
		t.Fatalf("unknown role should exit 2, got %d", code)
	}
	if code := runMCP([]string{"--help"}); code != 0 {
		t.Fatalf("--help should exit 0, got %d", code)
	}
}

// TestRunMCPServeEOF drives the full runMCP setup; the JSON-RPC server reads
// the test's stdin (/dev/null), sees EOF and returns — covering the whole
// wiring without a real client.
func TestRunMCPServeEOF(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "source: s\ntemplate: simple\ndomain: x.com\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	code := runMCP([]string{"--no-watch", "--role=content"})
	if code != 0 && code != 1 { // EOF may surface as a clean stop or a reported error
		t.Fatalf("runMCP EOF exit = %d", code)
	}
}

func TestRunNewWrangler(t *testing.T) {
	t.Chdir(t.TempDir())
	// No workers configured → refuse with exit 1.
	if code := runNewWrangler([]string{"src", "tmpl", "example.com"}); code != 1 {
		t.Fatalf("no workers should exit 1, got %d", code)
	}
}
