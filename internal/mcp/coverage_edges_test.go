package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// failWriter always fails, to surface Serve's response-encoding error path.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("sink closed") }

// TestServeBlankLineAndEncodeFailure: blank input lines are skipped without a
// response, and a write error on the output stream aborts Serve with it.
func TestServeBlankLineAndEncodeFailure(t *testing.T) {
	s, _ := newTestServer(t, nil)
	in := "\n   \n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	if err := s.Serve(strings.NewReader(in), failWriter{}); err == nil {
		t.Error("a failing output writer must abort Serve with its error")
	}
}

// TestInstructionsConfigAndGitSections: with a config path the designer contract
// names the config tools, and configured git adds its workflow section.
func TestInstructionsConfigAndGitSections(t *testing.T) {
	s, _ := newTestServer(t, func(o *Options) {
		o.ConfigPath = "ssg.yaml"
		o.Git = GitOptions{Token: "tok", Run: func(...string) (string, error) { return "", nil }}
	})
	ins := s.instructions()
	for _, want := range []string{
		"designer_config_read",
		"CANNOT: change any other configuration key",
		"GIT (git_*)",
		"git_new_branch",
	} {
		if !strings.Contains(ins, want) {
			t.Errorf("instructions missing %q in:\n%s", want, ins)
		}
	}
}

// TestGitRemoteOverride: an explicit remote replaces the origin default.
func TestGitRemoteOverride(t *testing.T) {
	if got := (GitOptions{Remote: "upstream"}).remote(); got != "upstream" {
		t.Errorf("remote() = %q, want upstream", got)
	}
}

// TestContentListSkipsNonMarkdown: the extension filter keeps stray files out
// of the content listing.
func TestContentListSkipsNonMarkdown(t *testing.T) {
	s, root := newTestServer(t, nil)
	if err := os.WriteFile(filepath.Join(root, "content", "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "content", "posts", "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := call(t, s, "content_list", nil)
	if !strings.Contains(text(r), "content/posts/a.md") || strings.Contains(text(r), "note.txt") {
		t.Errorf("content list must filter by extension: %q", text(r))
	}
}

// TestAbsRootFailure: with an unresolvable working directory every relative
// root fails fast — in the path resolvers and through the list tools.
func TestAbsRootFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove the working directory on this platform: %v", err)
	}
	if _, _, err := resolveIn("proj", []string{"content"}, "content/x.md"); err == nil {
		t.Error("resolveIn with an unresolvable cwd must error")
	}
	if _, err := listFiles("proj", []string{"content"}); err == nil {
		t.Error("listFiles with an unresolvable cwd must error")
	}
	s := NewServer(Options{Root: "proj", ContentDirs: []string{"content"},
		TemplateDirs: []string{"templates"}, StaticDirs: []string{"static"}})
	if r := s.contentList(nil); !r.IsError {
		t.Error("content list must surface the resolution error")
	}
	if r := s.designerList(nil); !r.IsError {
		t.Error("designer list must surface the resolution error")
	}
}

// TestContentErrorBranches: traversal rejections and filesystem write/delete
// failures all come back as readable tool errors.
func TestContentErrorBranches(t *testing.T) {
	s, root := newTestServer(t, nil)
	if r := call(t, s, "content_read", map[string]any{"path": "../secret.md"}); !r.IsError {
		t.Error("content_read traversal must error")
	}
	if r := call(t, s, "content_update", map[string]any{"path": "../x.md", "content": "x"}); !r.IsError {
		t.Error("content_update traversal must error")
	}
	// create: the parent path is blocked by a regular file.
	if err := os.WriteFile(filepath.Join(root, "content", "block"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := call(t, s, "content_create", map[string]any{"path": "content/block/new.md", "content": "x"}); !r.IsError ||
		!strings.Contains(text(r), "create failed") {
		t.Errorf("blocked parent must fail the create: %s", text(r))
	}
	// update/delete: a non-empty directory squats the target name.
	nested := filepath.Join(root, "content", "dir.md")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "keep.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := call(t, s, "content_update", map[string]any{"path": "content/dir.md", "content": "x"}); !r.IsError ||
		!strings.Contains(text(r), "update failed") {
		t.Errorf("directory target must fail the update: %s", text(r))
	}
	if r := call(t, s, "content_delete", map[string]any{"path": "content/dir.md"}); !r.IsError ||
		!strings.Contains(text(r), "delete failed") {
		t.Errorf("non-empty directory must fail the delete: %s", text(r))
	}
}

// TestDesignerWriteFailure: a blocked parent path fails the designer write.
func TestDesignerWriteFailure(t *testing.T) {
	s, root := newTestServer(t, nil)
	if err := os.WriteFile(filepath.Join(root, "templates", "block"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := call(t, s, "designer_write", map[string]any{"path": "templates/block/x.html", "content": "x"}); !r.IsError ||
		!strings.Contains(text(r), "write failed") {
		t.Errorf("blocked parent must fail the write: %s", text(r))
	}
}

// TestConfigNonMappingFile: a config file that is valid YAML but not a mapping
// fails both the read (values cannot be extracted) and the set (no mapping to
// edit) with readable errors.
func TestConfigNonMappingFile(t *testing.T) {
	s, _ := configServer(t, "- a\n- b\n", nil)
	if r := call(t, s, "designer_config_read", nil); !r.IsError {
		t.Error("reading a non-mapping config must error")
	}
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc", "value": "true"}); !r.IsError ||
		!strings.Contains(text(r), "could not update") {
		t.Errorf("setting into a non-mapping config must error: %s", text(r))
	}
}

// TestConfigSetReadOnlyFile: a write-protected config file fails the set after
// a successful read.
func TestConfigSetReadOnlyFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	s, path := configServer(t, "toc: false\n", nil)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if r := call(t, s, "designer_config_set", map[string]any{"key": "toc", "value": "true"}); !r.IsError ||
		!strings.Contains(text(r), "could not write") {
		t.Errorf("read-only config must fail the write: %s", text(r))
	}
}

// TestDocumentMappingEmptyDocument: a document node without content unwraps to
// no mapping at all.
func TestDocumentMappingEmptyDocument(t *testing.T) {
	if documentMapping(&yaml.Node{Kind: yaml.DocumentNode}) != nil {
		t.Error("an empty document must yield no mapping")
	}
}
