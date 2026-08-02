package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmptyListings: fresh dirs report "no files yet" instead of an empty string.
func TestEmptyListings(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := call(t, s, "designer_list", nil); !strings.Contains(text(r), "No template") {
		t.Errorf("empty designer list: %q", text(r))
	}
	if r := call(t, s, "content_list", nil); !strings.Contains(text(r), "No content") {
		t.Errorf("empty content list: %q", text(r))
	}
}

// TestReadMissing: reading nonexistent files is a readable tool error in both
// sections, and content args must be present.
func TestReadMissing(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := call(t, s, "designer_read", map[string]any{"path": "templates/none.html"}); !r.IsError {
		t.Error("missing template read must error")
	}
	if r := call(t, s, "content_read", map[string]any{"path": "content/none.md"}); !r.IsError {
		t.Error("missing content read must error")
	}
	for _, tool := range []string{"content_create", "content_update"} {
		if r := call(t, s, tool, map[string]any{"path": "content/x.md"}); !r.IsError || !strings.Contains(text(r), "content") {
			t.Errorf("%s without content must error: %s", tool, text(r))
		}
	}
	if r := call(t, s, "content_delete", map[string]any{"path": "../x.md"}); !r.IsError {
		t.Error("delete traversal must error")
	}
}

// TestGitDefaultsAndFailures: remote/base/prefix defaults, status variants, and
// every git failure branch surfaces as a tool error.
func TestGitDefaultsAndFailures(t *testing.T) {
	if (GitOptions{}).remote() != "origin" || (GitOptions{}).base() != "main" {
		t.Error("git defaults")
	}
	if (GitOptions{Token: "t"}).Enabled() {
		t.Error("token without runner must not enable")
	}

	fg := &fakeGit{branch: "main", fail: map[string]bool{}}
	s, _ := newTestServer(t, func(o *Options) {
		o.Git = GitOptions{Token: "tok", Run: fg.run, StageDirs: []string{"content"}}
	})
	// StageDirs override is honoured.
	if got := s.stageTargets(); len(got) != 1 || got[0] != "content" {
		t.Errorf("stageTargets = %v", got)
	}
	// branchName without Now falls back to "change"; default prefix is mcp/.
	if got := s.branchName(""); got != "mcp/change" {
		t.Errorf("branchName = %q", got)
	}

	fg.fail["status"] = true
	if r := call(t, s, "git_status", nil); !r.IsError {
		t.Error("status failure must error")
	}
	fg.fail["checkout"] = true
	if r := call(t, s, "git_new_branch", map[string]any{"name": "x"}); !r.IsError {
		t.Error("checkout failure must error")
	}
	fg.fail["add"] = true
	if r := call(t, s, "git_commit", map[string]any{"message": "m"}); !r.IsError || !strings.Contains(text(r), "git add failed") {
		t.Errorf("add failure: %s", text(r))
	}
	delete(fg.fail, "add")
	fg.fail["commit"] = true
	if r := call(t, s, "git_commit", map[string]any{"message": "m"}); !r.IsError || !strings.Contains(text(r), "git commit failed") {
		t.Errorf("commit failure: %s", text(r))
	}
	fg.fail["rev-parse"] = true
	if r := call(t, s, "git_open_pr", map[string]any{"title": "t"}); !r.IsError || !strings.Contains(text(r), "current branch") {
		t.Errorf("rev-parse failure: %s", text(r))
	}
	delete(fg.fail, "rev-parse")
	if r := call(t, s, "git_open_pr", nil); !r.IsError {
		t.Error("pr without title must error")
	}

	// No CreatePR configured: push succeeds, model is told to open the PR by hand.
	fg.branch = "mcp/x"
	if r := call(t, s, "git_open_pr", map[string]any{"title": "t"}); r.IsError || !strings.Contains(text(r), "no PR opener") {
		t.Errorf("push-only flow: %s", text(r))
	}

	// A clean tree reports as such.
	clean := &fakeGit{branch: "main", fail: map[string]bool{}}
	s2, _ := newTestServer(t, func(o *Options) {
		o.Git = GitOptions{Token: "tok", Run: func(args ...string) (string, error) {
			if args[0] == "status" {
				return "  \n", nil
			}
			return clean.run(args...)
		}}
	})
	if r := call(t, s2, "git_status", nil); text(r) != "working tree clean" {
		t.Errorf("clean status: %q", text(r))
	}
}

// TestCreatePRFailure: a PR-opener error is reported after the push succeeded.
func TestCreatePRFailure(t *testing.T) {
	fg := &fakeGit{branch: "mcp/x", fail: map[string]bool{}}
	s, _ := newTestServer(t, func(o *Options) {
		o.Git = GitOptions{Token: "tok", Run: fg.run,
			CreatePR: func(string, string, string) (string, error) { return "", fmt.Errorf("403") }}
	})
	if r := call(t, s, "git_open_pr", map[string]any{"title": "t"}); !r.IsError || !strings.Contains(text(r), "opening the PR failed") {
		t.Errorf("pr failure: %s", text(r))
	}
}

// TestInstructionsRoleGating: a content-only server's instructions and help omit
// the designer section, and defaults fill in (Logf, both roles when empty).
func TestInstructionsRoleGating(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) { o.Roles = map[string]bool{"content": true} })
	ins := s.instructions()
	if strings.Contains(ins, "DESIGNER") || !strings.Contains(ins, "CONTENT MANAGER") {
		t.Errorf("content-only instructions: %s", ins)
	}
	// Watch note appears only when watching.
	sw, _ := newTestServer(t, func(o *Options) { o.Watch = true })
	if !strings.Contains(sw.instructions(), "rebuilds after every change") {
		t.Error("watch note missing")
	}
	// Defaulted Logf must be callable.
	sw.opts.Logf("noop %d", 1)
}

// TestAfterMutateNoWatch: without watch the mutation summary is returned as-is.
func TestAfterMutateNoWatch(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := s.afterMutate("done"); text(r) != "done" || r.IsError {
		t.Errorf("afterMutate = %q", text(r))
	}
}

// TestResolveInEdges: empty path and path equal to the base itself.
func TestResolveInEdges(t *testing.T) {
	if _, _, err := resolveIn(".", []string{"content"}, ""); err == nil {
		t.Error("empty path must error")
	}
	if _, base, err := resolveIn(".", []string{"content"}, "content"); err != nil || base != "content" {
		t.Errorf("base itself must resolve: %v %q", err, base)
	}
}

func TestFirstSentence(t *testing.T) {
	if got := firstSentence("One. Two."); got != "One." {
		t.Errorf("firstSentence = %q", got)
	}
	if got := firstSentence("No period"); got != "No period" {
		t.Errorf("firstSentence = %q", got)
	}
}
