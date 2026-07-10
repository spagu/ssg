package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// deployGitHubPages publishes o.Dir by force-pushing it as a single commit to the
// target branch (default "gh-pages") of a git remote. The remote is o.Target or the
// current repo's "origin". A GITHUB_TOKEN, if present, is passed via an auth header
// (never embedded in the remote URL — OPS-009).
func deployGitHubPages(ctx context.Context, o Options) (string, error) {
	branch := o.Branch
	if branch == "" {
		branch = "gh-pages"
	}
	remote := o.Target
	if remote == "" {
		remote = gitOriginURL(ctx)
	}
	if remote == "" {
		return "", fmt.Errorf("no git remote: pass --deploy-target=<git-url> or run inside a repo with an 'origin' remote")
	}

	gitPath, err := exec.LookPath("git") // NOSONAR S4036: git is intentionally resolved from PATH (portable)
	if err != nil {
		return "", fmt.Errorf("git not found in PATH: %w", err)
	}
	// An isolated repo inside the output dir; removed afterwards so re-deploys are clean.
	gitDir := filepath.Join(o.Dir, ".git")
	if _, statErr := os.Stat(gitDir); statErr == nil {
		return "", fmt.Errorf("%q already contains a .git directory; refusing to overwrite", o.Dir)
	}
	defer func() { _ = os.RemoveAll(gitDir) }()

	run := func(extra []string, args ...string) error {
		full := append(extra, args...) //nolint:gocritic // deliberately builds a fresh argv per call
		// #nosec G204 -- git resolved via LookPath; argv is deploy config (branch/remote/token header), never page content
		cmd := exec.CommandContext(ctx, gitPath, full...)
		cmd.Dir = o.Dir
		if out, e := cmd.CombinedOutput(); e != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), e, strings.TrimSpace(string(out)))
		}
		return nil
	}

	commitMsg := "Deploy via ssg at " + time.Now().UTC().Format(time.RFC3339)
	// Pin identity and disable GPG signing so the throwaway deploy commit never depends
	// on the operator's global git signing config.
	identity := []string{
		"-c", "user.email=deploy@ssg.local",
		"-c", "user.name=ssg",
		"-c", "commit.gpgsign=false",
	}
	if err := run(nil, "init", "-q", "-b", branch); err != nil {
		return "", err
	}
	if err := run(nil, "add", "-A"); err != nil {
		return "", err
	}
	if err := run(identity, "commit", "-q", "-m", commitMsg); err != nil {
		return "", err
	}

	pushExtra := []string(nil)
	if token := o.env("GITHUB_TOKEN"); token != "" && strings.HasPrefix(remote, "https://") {
		pushExtra = []string{"-c", "http.extraheader=AUTHORIZATION: bearer " + token}
	}
	o.logf("☁️  Pushing %s to %s (%s)…", o.Dir, remote, branch)
	if err := run(pushExtra, "push", "--force", remote, "HEAD:"+branch); err != nil {
		return "", err
	}
	return githubPagesURL(remote), nil
}

// gitOriginURL returns the current repository's origin URL, or "" if unavailable.
func gitOriginURL(ctx context.Context) string {
	gitPath, err := exec.LookPath("git") // NOSONAR S4036: git is intentionally resolved from PATH (portable)
	if err != nil {
		return ""
	}
	// #nosec G204 -- fixed git subcommand with constant args; reads the local repo's origin
	out, err := exec.CommandContext(ctx, gitPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// githubURLRe extracts owner/repo from an https or ssh GitHub remote.
var githubURLRe = regexp.MustCompile(`github\.com[:/]([^/]+)/(.+?)(?:\.git)?/?$`)

// githubPagesURL derives the default *.github.io Pages URL from a GitHub remote,
// falling back to the remote itself when it cannot be parsed.
func githubPagesURL(remote string) string {
	m := githubURLRe.FindStringSubmatch(remote)
	if m == nil {
		return remote
	}
	owner, repo := m[1], m[2]
	if strings.EqualFold(repo, owner+".github.io") {
		return fmt.Sprintf("https://%s.github.io/", owner)
	}
	return fmt.Sprintf("https://%s.github.io/%s/", owner, repo)
}
