package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer builds a server over a temp project with both roles and a
// recording rebuild hook.
func newTestServer(t *testing.T, opts func(*Options)) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"templates", "static", "content/posts"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o := Options{
		Root:         root,
		TemplateDirs: []string{"templates"},
		StaticDirs:   []string{"static"},
		ContentDirs:  []string{"content"},
	}
	if opts != nil {
		opts(&o)
	}
	return NewServer(o), root
}

func call(t *testing.T, s *Server, name string, args map[string]any) toolResult {
	t.Helper()
	raw, _ := json.Marshal(args)
	params, _ := json.Marshal(callParams{Name: name, Arguments: raw})
	return s.callTool(params)
}

func text(r toolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	return r.Content[0].Text
}

// TestServeHandshake drives the stdio loop end to end: initialize, tools/list,
// a tools/call, an unknown method, a parse error, and a notification (no reply).
func TestServeHandshake(t *testing.T) {
	s, _ := newTestServer(t, nil)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-01-01"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, // notification → no response
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"help"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
		`not json`,
		`{"jsonrpc":"2.0","id":5,"method":"ping"}`,
	}, "\n") + "\n"
	var out strings.Builder
	if err := s.Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 { // 7 inputs - 1 notification
		t.Fatalf("want 6 responses, got %d:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"2025-01-01"`) || !strings.Contains(lines[0], `"ssg"`) {
		t.Errorf("initialize should echo version + server info: %s", lines[0])
	}
	if !strings.Contains(lines[1], "designer_write") || !strings.Contains(lines[1], "content_update") {
		t.Errorf("tools/list should include both sections: %s", lines[1])
	}
	if !strings.Contains(lines[2], "DESIGNER") || !strings.Contains(lines[2], "CONTENT MANAGER") {
		t.Errorf("help should describe both roles: %s", lines[2])
	}
	if !strings.Contains(lines[3], "unknown method") {
		t.Errorf("unknown method should error: %s", lines[3])
	}
	if !strings.Contains(lines[4], "parse error") {
		t.Errorf("bad json should be a parse error: %s", lines[4])
	}
}

// TestRoleFiltering: a designer-only server exposes no content tools, and git
// tools appear only when configured.
func TestRoleFiltering(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) { o.Roles = map[string]bool{"designer": true} })
	names := make([]string, 0, len(s.tools))
	for _, tl := range s.tools {
		names = append(names, tl.name)
	}
	joined := strings.Join(names, " ")
	if strings.Contains(joined, "content_") || strings.Contains(joined, "git_") {
		t.Errorf("designer-only server leaked tools: %s", joined)
	}
	if !strings.Contains(joined, "designer_write") || !strings.Contains(joined, "help") {
		t.Errorf("missing designer tools: %s", joined)
	}
}

// TestDesignerFlow: list → write (create + update, rebuild hook fires) → read;
// traversal and absolute paths are rejected.
func TestDesignerFlow(t *testing.T) {
	rebuilds := 0
	s, root := newTestServer(t, func(o *Options) {
		o.Watch = true
		o.Rebuild = func() (string, error) { rebuilds++; return "built 3 pages", nil }
	})

	if r := call(t, s, "designer_write", map[string]any{"path": "templates/post.html", "content": "<html>v1</html>"}); r.IsError {
		t.Fatalf("write: %s", text(r))
	} else if !strings.Contains(text(r), "created") || !strings.Contains(text(r), "rebuilt cleanly") {
		t.Errorf("create result: %s", text(r))
	}
	if r := call(t, s, "designer_write", map[string]any{"path": "templates/post.html", "content": "<html>v2</html>"}); !strings.Contains(text(r), "updated") {
		t.Errorf("update result: %s", text(r))
	}
	if rebuilds != 2 {
		t.Errorf("rebuild hook fired %d times, want 2", rebuilds)
	}
	if r := call(t, s, "designer_read", map[string]any{"path": "templates/post.html"}); text(r) != "<html>v2</html>" {
		t.Errorf("read = %q", text(r))
	}
	if r := call(t, s, "designer_list", nil); !strings.Contains(text(r), "templates/post.html") {
		t.Errorf("list = %q", text(r))
	}

	// Confinement: traversal, absolute path, and out-of-section writes fail.
	for _, bad := range []string{"../evil.html", "/etc/passwd", "content/posts/x.md"} {
		if r := call(t, s, "designer_write", map[string]any{"path": bad, "content": "x"}); !r.IsError {
			t.Errorf("designer_write %q must be rejected", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "..", "evil.html")); err == nil {
		t.Error("traversal write escaped the project")
	}
}

// TestContentFlow: create (no overwrite) → update (must exist) → delete; only
// Markdown allowed; content cannot touch templates.
func TestContentFlow(t *testing.T) {
	s, _ := newTestServer(t, nil)
	doc := "---\ntitle: Hi\n---\nbody"

	if r := call(t, s, "content_create", map[string]any{"path": "content/posts/hi.md", "content": doc}); r.IsError {
		t.Fatalf("create: %s", text(r))
	}
	if r := call(t, s, "content_create", map[string]any{"path": "content/posts/hi.md", "content": doc}); !r.IsError || !strings.Contains(text(r), "content_update") {
		t.Errorf("re-create must point at content_update: %s", text(r))
	}
	if r := call(t, s, "content_update", map[string]any{"path": "content/posts/hi.md", "content": doc + "!"}); r.IsError {
		t.Errorf("update: %s", text(r))
	}
	if r := call(t, s, "content_read", map[string]any{"path": "content/posts/hi.md"}); text(r) != doc+"!" {
		t.Errorf("read = %q", text(r))
	}
	if r := call(t, s, "content_update", map[string]any{"path": "content/posts/none.md", "content": doc}); !r.IsError || !strings.Contains(text(r), "content_create") {
		t.Errorf("update-missing must point at content_create: %s", text(r))
	}
	if r := call(t, s, "content_create", map[string]any{"path": "content/notes.txt", "content": "x"}); !r.IsError {
		t.Error("non-markdown must be rejected")
	}
	if r := call(t, s, "content_create", map[string]any{"path": "templates/post.md", "content": "x"}); !r.IsError {
		t.Error("content must not write into templates")
	}
	if r := call(t, s, "content_list", nil); !strings.Contains(text(r), "content/posts/hi.md") {
		t.Errorf("list = %q", text(r))
	}
	if r := call(t, s, "content_delete", map[string]any{"path": "content/posts/hi.md"}); r.IsError {
		t.Errorf("delete: %s", text(r))
	}
	if r := call(t, s, "content_delete", map[string]any{"path": "content/posts/hi.md"}); !r.IsError {
		t.Error("double delete must error")
	}
}

// TestRebuildErrorSurfaces: a failing rebuild turns a successful write into an
// error result carrying the build output, so the model must fix it.
func TestRebuildErrorSurfaces(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) {
		o.Watch = true
		o.Rebuild = func() (string, error) { return "template: post.html:3: bad pipeline", fmt.Errorf("build failed") }
	})
	r := call(t, s, "designer_write", map[string]any{"path": "templates/post.html", "content": "{{bad"})
	if !r.IsError || !strings.Contains(text(r), "bad pipeline") || !strings.Contains(text(r), "did NOT rebuild") {
		t.Errorf("rebuild failure must surface: %v %s", r.IsError, text(r))
	}
}

// fakeGit records git invocations and returns canned output per subcommand.
type fakeGit struct {
	calls  [][]string
	branch string
	fail   map[string]bool
}

func (f *fakeGit) run(args ...string) (string, error) {
	f.calls = append(f.calls, args)
	if f.fail[args[0]] {
		return "boom", fmt.Errorf("exit 1")
	}
	if args[0] == "rev-parse" {
		return f.branch + "\n", nil
	}
	if args[0] == "status" {
		return "## " + f.branch + "\n M content/posts/hi.md\n", nil
	}
	return "", nil
}

// TestGitFlow: branch → commit → open PR happy path, plus the guard rails
// (base-branch PR refused, push failure reported, no name ⇒ timestamped).
func TestGitFlow(t *testing.T) {
	fg := &fakeGit{branch: "mcp/hero-redesign", fail: map[string]bool{}}
	var prHead, prTitle string
	s, _ := newTestServer(t, func(o *Options) {
		o.Git = GitOptions{
			Token: "tok", Run: fg.run, BranchPrefix: "mcp/", DefaultBranch: "main",
			Now: func() string { return "20260802-120000" },
			CreatePR: func(head, title, body string) (string, error) {
				prHead, prTitle = head, title
				return "https://github.com/o/r/pull/7", nil
			},
		}
	})

	if r := call(t, s, "git_status", nil); !strings.Contains(text(r), "hi.md") {
		t.Errorf("status: %s", text(r))
	}
	if r := call(t, s, "git_new_branch", map[string]any{"name": "Hero Redesign!"}); !strings.Contains(text(r), "mcp/hero-redesign") {
		t.Errorf("branch: %s", text(r))
	}
	if r := call(t, s, "git_new_branch", nil); !strings.Contains(text(r), "mcp/20260802-120000") {
		t.Errorf("unnamed branch should be timestamped: %s", text(r))
	}
	if r := call(t, s, "git_commit", map[string]any{"message": "redesign hero"}); !strings.Contains(text(r), "committed") {
		t.Errorf("commit: %s", text(r))
	}
	// The commit staged only the content/template sections.
	var staged []string
	for _, c := range fg.calls {
		if c[0] == "add" {
			staged = c
		}
	}
	if len(staged) == 0 || !strings.Contains(strings.Join(staged, " "), "content") || !strings.Contains(strings.Join(staged, " "), "templates") {
		t.Errorf("git add should stage content+templates: %v", staged)
	}
	if r := call(t, s, "git_open_pr", map[string]any{"title": "Redesign hero"}); !strings.Contains(text(r), "pull/7") {
		t.Errorf("open pr: %s", text(r))
	}
	if prHead != "mcp/hero-redesign" || prTitle != "Redesign hero" {
		t.Errorf("pr args = %q %q", prHead, prTitle)
	}

	// On the base branch, a PR is refused before any push.
	fg.branch = "main"
	if r := call(t, s, "git_open_pr", map[string]any{"title": "x"}); !r.IsError || !strings.Contains(text(r), "git_new_branch") {
		t.Errorf("base-branch PR must be refused: %s", text(r))
	}
	// Push failure is reported.
	fg.branch = "mcp/x"
	fg.fail["push"] = true
	if r := call(t, s, "git_open_pr", map[string]any{"title": "x"}); !r.IsError || !strings.Contains(text(r), "push failed") {
		t.Errorf("push failure must surface: %s", text(r))
	}
	// Missing message.
	if r := call(t, s, "git_commit", nil); !r.IsError {
		t.Error("commit without message must error")
	}
}

// TestGitHidden: without a token the git tools are absent and help omits the git
// section.
func TestGitHidden(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := call(t, s, "git_status", nil); !r.IsError || !strings.Contains(text(r), "unknown tool") {
		t.Errorf("git tools must be hidden: %s", text(r))
	}
	if h := text(call(t, s, "help", nil)); strings.Contains(h, "git_open_pr") {
		t.Errorf("help must omit git when unconfigured: %s", h)
	}
}

// TestUnknownAndBadArgs: unknown tool and malformed params are readable tool
// errors, not protocol failures.
func TestUnknownAndBadArgs(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if r := s.callTool(json.RawMessage(`{"name":"designer_read","arguments":"nope"}`)); !r.IsError {
		t.Error("string arguments must error")
	}
	if r := s.callTool(json.RawMessage(`not json`)); !r.IsError {
		t.Error("bad params must error")
	}
	if r := call(t, s, "designer_read", map[string]any{}); !r.IsError {
		t.Error("missing path must error")
	}
	if r := call(t, s, "designer_write", map[string]any{"path": "templates/x.html"}); !r.IsError {
		t.Error("missing content must error")
	}
}

func TestSlugifyBranch(t *testing.T) {
	for in, want := range map[string]string{
		"Hero Redesign!": "hero-redesign",
		"  ":             "change",
		"fix/nav höme":   "fix-nav-h-me",
	} {
		if got := slugifyBranch(in); got != want {
			t.Errorf("slugifyBranch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTail(t *testing.T) {
	if got := tail("a\n\nb\nc\nd", 2); got != "c\nd" {
		t.Errorf("tail = %q", got)
	}
}
