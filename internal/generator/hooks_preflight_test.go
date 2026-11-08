package generator

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSnap makes the process look like the confined snap build: $SNAP set and
// the executable under it, which is what snapenv.Root checks.
func fakeSnap(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	root := filepath.Dir(exe)
	t.Setenv("SNAP", root)
	return root
}

// TestCheckHookCommands covers #397: a hook whose program cannot be found stops
// the build before anything renders, for both fatal phases, while post_page
// (non-fatal at run time) and blank entries are not checked.
func TestCheckHookCommands(t *testing.T) {
	t.Setenv("SNAP", "")
	g := newTestGen(t, "")
	g.config.Hooks = map[string][]string{
		"pre_build":  {"  ", "true"},
		"post_page":  {"no-such-program-397"},
		"post_build": {"true --flag"},
	}
	if err := g.checkHookCommands(); err != nil {
		t.Fatalf("resolvable hooks rejected: %v", err)
	}
	for _, phase := range []string{"pre_build", "post_build"} {
		g.config.Hooks = map[string][]string{phase: {"no-such-program-397 scripts/x.mjs"}}
		err := g.checkHookCommands()
		if !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("%s: want exec.ErrNotFound, got %v", phase, err)
		}
		if !strings.Contains(err.Error(), phase) || strings.Contains(err.Error(), "snap") {
			t.Errorf("%s: unexpected message outside a snap: %v", phase, err)
		}
	}
}

// TestHookErrorExplainsSnap: inside the snap the error says why a host
// program is missing and what to do; other errors are passed through as is.
func TestHookErrorExplainsSnap(t *testing.T) {
	root := fakeSnap(t)
	err := hookError("node scripts/a.mjs", "post_build", exec.ErrNotFound)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("the cause must stay inspectable: %v", err)
	}
	for _, want := range []string{"snap", root, "strict confinement", "install.sh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q: %v", want, err)
		}
	}
	plain := hookError("false", "post_build", errors.New("exit status 1"))
	if strings.Contains(plain.Error(), "snap") {
		t.Errorf("a failing hook is not a confinement problem: %v", plain)
	}
}

// TestRunHooksNotFoundInSnap: the post_page path (checked only at run time)
// carries the same explanation.
func TestRunHooksNotFoundInSnap(t *testing.T) {
	fakeSnap(t)
	g := newTestGen(t, "")
	g.config.Hooks = map[string][]string{"post_page": {"no-such-program-397"}}
	err := g.runHooks("post_page", nil)
	if err == nil || !strings.Contains(err.Error(), "strict confinement") {
		t.Fatalf("want the snap explanation, got %v", err)
	}
}

// TestGenerateFailsBeforeRenderingOnMissingHook: the build stops before
// pre_build runs and before a single page is written (#397 reported the
// failure arriving only after every page had rendered).
func TestGenerateFailsBeforeRenderingOnMissingHook(t *testing.T) {
	t.Setenv("SNAP", "")
	tmp := t.TempDir()
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	marker := filepath.Join(tmp, "pre-build-ran")
	cfg := Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Quiet:        true,
		Hooks: map[string][]string{
			"pre_build":  {"touch " + marker},
			"post_build": {"no-such-program-397"},
		},
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gen.Generate(); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("want exec.ErrNotFound, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("pre_build ran although a later hook cannot")
	}
	if treeHasFile(t, filepath.Join(tmp, "output"), "index.html") {
		t.Error("pages were rendered before the hook check")
	}
}
