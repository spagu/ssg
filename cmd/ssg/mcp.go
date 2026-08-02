package main

// `ssg mcp` runs the development MCP server (#1.8.16): a Model Context Protocol
// server over stdio that lets an AI assistant work on the site in two roles —
// designer (templates/theme) and content manager (Markdown) — with live rebuilds
// in watch mode and an optional git write-back flow (branch → commit → PR after
// human approval). Wire it into an MCP-capable assistant as a stdio server:
//
//	ssg mcp                     # both roles, rebuild on every change
//	ssg mcp --role=designer     # designer only
//	ssg mcp --role=content      # content manager only
//	ssg mcp --no-watch          # edit only, no rebuilds
//
// All human-facing logs go to stderr; stdout is the JSON-RPC channel.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mcp"
)

// runMCP implements `ssg mcp [flags] [dir]`.
func runMCP(args []string) int {
	roles := map[string]bool{}
	watch := true
	rest := []string{}
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--role="):
			r := strings.TrimPrefix(a, "--role=")
			if r != "designer" && r != "content" {
				fmt.Fprintf(os.Stderr, "❌ unknown --role=%s (designer | content). See 'ssg --help'.\n", r)
				return 2
			}
			roles[r] = true
		case a == "--no-watch":
			watch = false
		case a == "--help" || a == "-h":
			printMCPHelp()
			return 0
		default:
			rest = append(rest, a)
		}
	}

	cfg := loadConfig(rest)
	parseFlags(rest, cfg)
	applyMinifyAll(cfg)
	setupTemplateEngine(cfg)
	downloadOnlineTheme(cfg)
	genCfg := createGeneratorConfig(cfg)
	logf := func(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }

	opts := mcp.Options{
		Root:         ".",
		TemplateDirs: []string{cfg.TemplatesDir},
		StaticDirs:   []string{cfg.StaticDir},
		ContentDirs:  contentRoots(cfg),
		Roles:        roles,
		Watch:        watch,
		Version:      Version,
		Git:          buildMCPGit(cfg),
		Logf:         logf,
		Rebuild: func() (string, error) {
			// Rebuild quietly, capturing anything printed so build noise cannot
			// leak into the JSON-RPC stdout channel and errors flow to the model.
			q := *cfg
			q.Quiet = true
			return captureStdout(func() error { return build(genCfg, &q) })
		},
	}

	logf("🔌 ssg mcp %s — designer/content development server (stdio)", Version)
	logf("   roles: %s · watch: %v · git PR flow: %v", roleNames(opts.Roles), watch, opts.Git.Enabled())
	if err := mcp.NewServer(opts).Serve(os.Stdin, os.Stdout); err != nil {
		logf("❌ mcp server: %v", err)
		return 1
	}
	return 0
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns whatever
// was printed. The MCP transport owns the real stdout (JSON-RPC), so build noise
// must never reach it; the server handles one request at a time, so the temporary
// process-wide swap is safe here.
func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", fn() // cannot capture — still run the build
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	ferr := fn()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, ferr
}

// contentRoots lists every local Markdown root: content_dir plus content_sources.
func contentRoots(cfg *config.Config) []string {
	roots := []string{cfg.ContentDir}
	for _, s := range cfg.ContentSources {
		if s.Path != "" {
			roots = append(roots, s.Path)
		}
	}
	return roots
}

func roleNames(roles map[string]bool) string {
	if len(roles) == 0 {
		return "designer+content"
	}
	names := make([]string, 0, 2)
	for _, r := range []string{"designer", "content"} {
		if roles[r] {
			names = append(names, r)
		}
	}
	return strings.Join(names, "+")
}

// buildMCPGit wires the git write-back flow from config.mcp.git: token from $ENV,
// repo derived from the remote when unset, PRs opened via the GitHub REST API.
func buildMCPGit(cfg *config.Config) mcp.GitOptions {
	gc := cfg.MCP.Git
	token := expandEnvValue(gc.Token)
	run := func(args ...string) (string, error) {
		out, err := exec.Command("git", args...).CombinedOutput() // #nosec G204 -- fixed binary, args built by the mcp package
		return string(out), err
	}
	g := mcp.GitOptions{
		Token:         token,
		Repo:          gc.Repo,
		Remote:        gc.Remote,
		DefaultBranch: gc.DefaultBranch,
		BranchPrefix:  gc.BranchPrefix,
		Run:           run,
		Now:           func() string { return time.Now().Format("20060102-150405") },
	}
	if token == "" {
		return g // git tools stay hidden
	}
	if g.Repo == "" {
		g.Repo = repoFromRemote(run, g)
	}
	repo := g.Repo
	g.CreatePR = func(head, title, body string) (string, error) {
		return openGitHubPR(token, repo, g.DefaultBranch, head, title, body)
	}
	return g
}

// expandEnvValue resolves a $VAR reference to its environment value; literals are
// returned as-is (config docs say: always use $ENV for the token).
func expandEnvValue(v string) string {
	if strings.HasPrefix(v, "$") {
		return os.Getenv(strings.TrimPrefix(v, "$"))
	}
	return v
}

// repoFromRemote derives "owner/name" from the configured remote's URL.
func repoFromRemote(run func(...string) (string, error), g mcp.GitOptions) string {
	remote := g.Remote
	if remote == "" {
		remote = "origin"
	}
	out, err := run("remote", "get-url", remote)
	if err != nil {
		return ""
	}
	return parseRepoURL(strings.TrimSpace(out))
}

// parseRepoURL extracts owner/name from https or ssh GitHub-style remote URLs.
func parseRepoURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	if i := strings.Index(url, "github.com/"); i >= 0 {
		return url[i+len("github.com/"):]
	}
	if i := strings.Index(url, "github.com:"); i >= 0 {
		return url[i+len("github.com:"):]
	}
	return ""
}

// openGitHubPR opens a pull request via the REST API and returns its html_url.
func openGitHubPR(token, repo, base, head, title, body string) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("no repository configured (set mcp.git.repo or a github remote)")
	}
	if base == "" {
		base = "main"
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "head": head, "base": base})
	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/repos/"+repo+"/pulls", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	var pr struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(rb, &pr); err != nil || pr.HTMLURL == "" {
		return "", fmt.Errorf("unexpected github response")
	}
	return pr.HTMLURL, nil
}

func printMCPHelp() {
	fmt.Println("ssg mcp — development MCP server (stdio) for AI-assisted editing")
	fmt.Println()
	fmt.Println("Two roles, each a section of tools with a described contract:")
	fmt.Println("  designer          - edits templates and theme assets (how the site looks)")
	fmt.Println("  content manager   - creates/updates/fixes/removes Markdown (what the site says)")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --role=designer|content  - expose one role only (default: both)")
	fmt.Println("  --no-watch               - do not rebuild the site after each change")
	fmt.Println("  --config=FILE            - site config (default: .ssg.yaml)")
	fmt.Println()
	fmt.Println("Git write-back (optional, config `mcp.git`): with an account + $ENV token the")
	fmt.Println("assistant gets git_new_branch / git_commit / git_open_pr — edits land on a")
	fmt.Println("working branch and a PR is opened only after explicit human approval.")
	fmt.Println()
	fmt.Println("Register in an MCP-capable assistant as a stdio server, e.g.:")
	fmt.Println(`  {"command": "ssg", "args": ["mcp"]}`)
}
