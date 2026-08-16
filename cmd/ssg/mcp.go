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
	"path/filepath"
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
		ConfigPath:   configPathOf(rest),
		ValidateConfig: func(path string) error {
			_, err := loadConfigFile(path)
			return err
		},
		Rebuild: func() (string, error) {
			// Rebuild quietly, capturing anything printed so build noise cannot
			// leak into the JSON-RPC stdout channel and errors flow to the model.
			q := *cfg
			q.Quiet = true
			out, err := captureStdout(func() error { return build(genCfg, &q) })
			// Push the result to an open --http preview: a reload on success,
			// the error overlay otherwise (GO-090). No-op without --http.
			if err != nil {
				notifyBuildError(err.Error())
			} else {
				notifyReload()
			}
			return out, err
		},
	}

	logf("🔌 ssg mcp %s — designer/content development server (stdio)", Version)
	logf("   roles: %s · watch: %v · git PR flow: %v", roleNames(opts.Roles), watch, opts.Git.Enabled())
	// --http was parsed into cfg but nothing served it, so the agent edited a
	// site nobody could look at. Serve it here: an assistant reworking a theme
	// is exactly when a human wants the preview open. Live reload needs the
	// rebuild signal, which MCP's Rebuild provides, so it is on with --http.
	serveMCPPreview(cfg, logf)
	if err := mcp.NewServer(opts).Serve(os.Stdin, os.Stdout); err != nil {
		logf("❌ mcp server: %v", err)
		return 1
	}
	return 0
}

// serveMCPPreview starts the preview server when `ssg mcp --http` was given,
// announcing the address on stderr (stdout belongs to JSON-RPC). A no-op
// without --http, so the plain stdio server is unchanged.
func serveMCPPreview(cfg *config.Config, logf func(string, ...any)) {
	if !cfg.HTTP {
		return
	}
	reloadHub = newLiveReloadHub() // each MCP rebuild refreshes the open tab
	// The port is claimed before the address is logged, so a busy 8888 shifts
	// the announcement too instead of pointing the agent at someone else's
	// server (#135).
	startServerAsync(cfg)
	_, url, _ := resolveListenAddr(cfg.Host, cfg.Port)
	logf("   👁️  preview: %s (serving %s/)", url, cfg.OutputDir)
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

// gitRunner returns a runner that executes git through an absolute path resolved
// once from PATH, so a writable or attacker-controlled PATH entry cannot swap the
// binary out between calls (S4036). A git that is missing, or resolves to a
// relative path, yields a runner that refuses to execute anything.
func gitRunner() func(args ...string) (string, error) {
	bin, err := exec.LookPath("git")
	if err == nil && !filepath.IsAbs(bin) {
		err = fmt.Errorf("git resolved to the non-absolute path %q", bin)
	}
	if err != nil {
		return func(...string) (string, error) {
			return "", fmt.Errorf("git is unavailable: %w", err)
		}
	}
	return func(args ...string) (string, error) {
		out, cmdErr := exec.Command(bin, args...).CombinedOutput() // #nosec G204 -- absolute binary resolved once; args are built by the mcp package
		return string(out), cmdErr
	}
}

// buildMCPGit wires the git write-back flow from config.mcp.git: token from $ENV,
// repo derived from the remote when unset, PRs opened via the GitHub REST API.
func buildMCPGit(cfg *config.Config) mcp.GitOptions {
	gc := cfg.MCP.Git
	token := expandEnvValue(gc.Token)
	run := gitRunner()
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

// githubAPIBase is where the PR is opened. A variable rather than a literal so
// the request, the auth header and every failure path can be exercised against
// a local server — the alternative is shipping this code untested and finding
// out during someone's approve-then-PR flow.
var githubAPIBase = "https://api.github.com"

// openGitHubPR opens a pull request via the REST API and returns its html_url.
func openGitHubPR(token, repo, base, head, title, body string) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("no repository configured (set mcp.git.repo or a github remote)")
	}
	if base == "" {
		base = "main"
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "head": head, "base": base})
	req, err := http.NewRequest(http.MethodPost, githubAPIBase+"/repos/"+repo+"/pulls", bytes.NewReader(payload))
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

// printMCPWiring prints the copy-pasteable way to connect this project's MCP
// server to an assistant. `ssg mcp` speaks stdio, so the CLIENT spawns it —
// telling people to "run ssg mcp" leaves them with a server nobody talks to;
// what they need is the registration line.
func printMCPWiring() {
	fmt.Println("   Claude Code (registers for this project):")
	fmt.Println("      claude mcp add ssg -- ssg mcp")
	fmt.Println("   Claude Desktop — add to claude_desktop_config.json:")
	fmt.Println(`      {"mcpServers":{"ssg":{"command":"ssg","args":["mcp"],"cwd":"` + workingDirOrDot() + `"}}}`)
	fmt.Println("   Then ask it to study the original site and rebuild the theme")
	fmt.Println("   (designer_* tools). Add --http to `ssg mcp` for a live preview.")
}

// workingDirOrDot names the project directory for a config snippet the user
// pastes elsewhere, where a relative path would be meaningless.
func workingDirOrDot() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
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
	fmt.Println("  --http [--port=N]        - also serve the site so you can watch it change")
	fmt.Println()
	fmt.Println("Register it with your assistant (this server speaks stdio, so the client")
	fmt.Println("launches it — you do not run it yourself):")
	printMCPWiring()
}
