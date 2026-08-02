package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

// GitOptions drives the optional git write-back flow. It is enabled only when a
// token and a command runner are present. Run executes a git command in the repo;
// CreatePR pushes the branch (done by the caller) is separate — CreatePR only
// opens the pull request and returns its URL. Injecting Run/CreatePR/Now keeps the
// flow fully testable and decoupled from the git/gh binaries.
type GitOptions struct {
	Token         string
	Repo          string   // owner/name, for the PR
	Remote        string   // default "origin"
	DefaultBranch string   // PR base, default "main"
	BranchPrefix  string   // default "mcp/"
	StageDirs     []string // directories staged on commit
	Run           func(args ...string) (string, error)
	CreatePR      func(head, title, body string) (string, error)
	Now           func() string
}

// Enabled reports whether the git tools should be exposed.
func (g GitOptions) Enabled() bool { return g.Token != "" && g.Run != nil }

func (g GitOptions) remote() string {
	if g.Remote != "" {
		return g.Remote
	}
	return "origin"
}

func (g GitOptions) base() string {
	if g.DefaultBranch != "" {
		return g.DefaultBranch
	}
	return "main"
}

// gitTools is the git write-back section (only exposed when configured): branch →
// commit → open PR. The PR is the explicit, human-approved final step.
func (s *Server) gitTools() []tool {
	return []tool{
		{
			name: "git_status",
			description: "GIT · Show the working-tree status (branch + changed files) so you and the " +
				"human can see exactly what will be committed before you commit.",
			schema:  objectSchema(nil),
			handler: s.gitStatus,
		},
		{
			name: "git_new_branch",
			description: "GIT · Create and switch to a working branch for this set of changes (prefix " +
				"\"" + s.opts.Git.BranchPrefix + "\"). Always start a change here so edits never land " +
				"directly on the base branch. Optional `name` describes the change; omitted ⇒ a " +
				"timestamped name.",
			schema:  objectSchema(map[string]any{"name": stringProp("Short description of the change, e.g. \"hero-redesign\" (optional)")}),
			handler: s.gitNewBranch,
		},
		{
			name: "git_commit",
			description: "GIT · Stage the content/template changes and commit them on the current " +
				"working branch with `message`. Run after your edits build cleanly. Does nothing to the " +
				"base branch.",
			schema:  objectSchema(map[string]any{"message": stringProp("Commit message describing the change")}, "message"),
			handler: s.gitCommit,
		},
		{
			name: "git_open_pr",
			description: "GIT · Push the working branch and open a pull request against \"" + s.opts.Git.base() +
				"\". THIS IS THE FINAL, HUMAN-APPROVED STEP — only call it after the person has reviewed " +
				"the changes and explicitly asked to open the PR. Returns the PR URL.",
			schema: objectSchema(map[string]any{
				"title": stringProp("Pull-request title"),
				"body":  stringProp("Pull-request description (optional)"),
			}, "title"),
			handler: s.gitOpenPR,
		},
	}
}

func (s *Server) gitStatus(map[string]any) toolResult {
	out, err := s.opts.Git.Run("status", "--porcelain=v1", "-b")
	if err != nil {
		return errResult("git status failed: " + err.Error())
	}
	if strings.TrimSpace(out) == "" {
		return textResult("working tree clean")
	}
	return textResult(out)
}

func (s *Server) gitNewBranch(args map[string]any) toolResult {
	name, _ := strArg(args, "name")
	branch := s.branchName(name)
	if out, err := s.opts.Git.Run("checkout", "-b", branch); err != nil {
		return errResult("could not create branch: " + strings.TrimSpace(out) + " " + err.Error())
	}
	return textResult("created and switched to branch " + branch +
		"\nMake your edits with the designer_/content_ tools, then git_commit, then (after the human approves) git_open_pr.")
}

func (s *Server) gitCommit(args map[string]any) toolResult {
	msg, ok := strArg(args, "message")
	if !ok || msg == "" {
		return errResult("`message` is required")
	}
	stage := append([]string{"add", "--"}, s.stageTargets()...)
	if out, err := s.opts.Git.Run(stage...); err != nil {
		return errResult("git add failed: " + strings.TrimSpace(out) + " " + err.Error())
	}
	if out, err := s.opts.Git.Run("commit", "-m", msg); err != nil {
		return errResult("git commit failed (nothing to commit, or an error): " + strings.TrimSpace(out))
	}
	return textResult("committed: " + msg)
}

func (s *Server) gitOpenPR(args map[string]any) toolResult {
	title, ok := strArg(args, "title")
	if !ok || title == "" {
		return errResult("`title` is required")
	}
	body, _ := strArg(args, "body")
	branch, err := s.currentBranch()
	if err != nil {
		return errResult(err.Error())
	}
	if branch == s.opts.Git.base() {
		return errResult("you are on the base branch " + branch + " — call git_new_branch first")
	}
	if out, err := s.opts.Git.Run("push", "-u", s.opts.Git.remote(), branch); err != nil {
		return errResult("git push failed: " + strings.TrimSpace(out) + " " + err.Error())
	}
	if s.opts.Git.CreatePR == nil {
		return textResult("pushed " + branch + ". Open the pull request in your git host (no PR opener configured).")
	}
	url, err := s.opts.Git.CreatePR(branch, title, body)
	if err != nil {
		return errResult("pushed " + branch + " but opening the PR failed: " + err.Error())
	}
	return textResult("opened pull request: " + url)
}

func (s *Server) currentBranch() (string, error) {
	out, err := s.opts.Git.Run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not read current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// stageTargets is the set of directories git add stages — the content and template
// sections only, so a commit never sweeps in build output.
func (s *Server) stageTargets() []string {
	if len(s.opts.Git.StageDirs) > 0 {
		return s.opts.Git.StageDirs
	}
	return append(append([]string{}, s.opts.ContentDirs...), s.opts.TemplateDirs...)
}

func (s *Server) branchName(name string) string {
	prefix := s.opts.Git.BranchPrefix
	if prefix == "" {
		prefix = "mcp/"
	}
	if name == "" {
		if s.opts.Git.Now != nil {
			name = s.opts.Git.Now()
		} else {
			name = "change"
		}
	}
	return prefix + slugifyBranch(name)
}

var branchInvalid = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// slugifyBranch turns a description into a safe git branch component.
func slugifyBranch(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = branchInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return "change"
	}
	return s
}
