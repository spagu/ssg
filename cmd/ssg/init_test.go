package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdirTemp switches to a fresh temp dir for the duration of a test.
func chdirTemp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	return tmp
}

func TestRunInit_CreatesScaffold(t *testing.T) {
	tmp := chdirTemp(t)
	if code := runInit([]string{"myblog", "--domain", "blog.example.com"}); code != 0 {
		t.Fatalf("runInit exit %d", code)
	}
	for _, rel := range []string{
		".ssg.yaml",
		filepath.Join("content", "myblog", "metadata.json"),
		filepath.Join("content", "myblog", "pages", "index.md"),
		filepath.Join("content", "myblog", "posts", "hello-world.md"),
		filepath.Join("static", ".gitkeep"),
		".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(tmp, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	cfg, _ := os.ReadFile(filepath.Join(tmp, ".ssg.yaml"))
	if !strings.Contains(string(cfg), "source: myblog") || !strings.Contains(string(cfg), "domain: blog.example.com") {
		t.Fatalf("config not parameterised: %s", cfg)
	}
}

func TestRunInit_DoesNotOverwrite(t *testing.T) {
	tmp := chdirTemp(t)
	// Pre-create the config with custom content.
	custom := "source: mine\n# do not touch\n"
	if err := os.WriteFile(filepath.Join(tmp, ".ssg.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runInit(nil); code != 0 {
		t.Fatalf("runInit exit %d", code)
	}
	got, _ := os.ReadFile(filepath.Join(tmp, ".ssg.yaml"))
	if string(got) != custom {
		t.Fatalf("existing .ssg.yaml was overwritten: %q", got)
	}
	// The rest of the scaffold should still have been created.
	if _, err := os.Stat(filepath.Join(tmp, "content", "site", "metadata.json")); err != nil {
		t.Fatalf("expected the default source scaffold: %v", err)
	}
}

func TestRunInit_DefaultSourceName(t *testing.T) {
	tmp := chdirTemp(t)
	if code := runInit(nil); code != 0 {
		t.Fatalf("runInit exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(tmp, "content", "site", "pages", "index.md")); err != nil {
		t.Fatalf("default source should be 'site': %v", err)
	}
}

func TestRunInit_RejectsBadSourceAndFlags(t *testing.T) {
	chdirTemp(t)
	if code := runInit([]string{"../escape"}); code != 1 {
		t.Fatalf("path-escaping source should exit 1, got %d", code)
	}
	if code := runInit([]string{"--bogus"}); code != 2 {
		t.Fatalf("unknown flag should exit 2, got %d", code)
	}
}

func TestDispatchSingleVerb(t *testing.T) {
	if _, handled := dispatchSingleVerb([]string{"build"}); handled {
		t.Fatal("non-init verb should not be handled")
	}
	chdirTemp(t)
	if code, handled := dispatchSingleVerb([]string{"init"}); !handled || code != 0 {
		t.Fatalf("init should be handled with exit 0, got (%d,%v)", code, handled)
	}
}
