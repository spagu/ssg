package main

// `ssg daemon` flag parsing and the projects-file watcher (#169).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/daemon"
)

// TestParseDaemonFlags: the defaults, both spellings of --config, and an
// unknown flag stopping the run rather than being ignored.
func TestParseDaemonFlags(t *testing.T) {
	f, code := parseDaemonFlags(nil)
	if code >= 0 {
		t.Fatalf("no flags must be valid, got code %d", code)
	}
	if filepath.Base(f.config) != daemon.DefaultConfigFile {
		t.Errorf("default config = %q", f.config)
	}
	if !filepath.IsAbs(f.config) {
		t.Errorf("the config path must be absolute so a project's own dir cannot change it: %q", f.config)
	}

	for _, args := range [][]string{{"--config", "other.yml"}, {"--config=other.yml"}} {
		f, code := parseDaemonFlags(args)
		if code >= 0 || filepath.Base(f.config) != "other.yml" {
			t.Errorf("%v → %q, code %d", args, f.config, code)
		}
	}
	if f, _ := parseDaemonFlags([]string{"--quiet", "--once"}); !f.quiet || !f.once {
		t.Errorf("flags = %+v", f)
	}
	if _, code := parseDaemonFlags([]string{"--check"}); code >= 0 {
		t.Error("--check is --once")
	}

	out := captureStderr(t, func() {
		if _, code := parseDaemonFlags([]string{"--nope"}); code != 2 {
			t.Errorf("an unknown flag must stop the run, got %d", code)
		}
	})
	if !strings.Contains(out, "--nope") {
		t.Errorf("the message must name the flag: %q", out)
	}
	if _, code := parseDaemonFlags([]string{"--help"}); code != 0 {
		t.Error("--help exits 0")
	}
}

// TestRunDaemonOnce: a projects file check that leaves nothing running, which
// is what makes it usable in CI.
func TestRunDaemonOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, daemon.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(
		"projects:\n  - {name: blog, dir: .}\n  - {name: off, dir: ., disabled: true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runDaemon([]string{"--config", path, "--once"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	// A file that cannot be read, and one with nothing to run, both stop it.
	if code := runDaemon([]string{"--config", filepath.Join(dir, "missing"), "--once"}); code == 0 {
		t.Error("a missing projects file must fail")
	}
	empty := filepath.Join(dir, "empty.yml")
	if err := os.WriteFile(empty, []byte("projects: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runDaemon([]string{"--config", empty, "--once"}); code == 0 {
		t.Error("a file listing no project must fail rather than idle")
	}
}

// TestProjectsWatcherNoticesAnEdit: the reload trigger. A file being rewritten
// is momentarily absent, and that must not read as a change — a half-written
// file would reload the fleet from nothing.
func TestProjectsWatcherNoticesAnEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.yml")
	if err := os.WriteFile(path, []byte("projects: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := newProjectsWatcher(path)
	if w.changed() {
		t.Error("an untouched file has not changed")
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("projects:\n  - {name: a, dir: .}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.changed() {
		t.Error("an edited file must be noticed")
	}
	if w.changed() {
		t.Error("the same edit must not be reported twice")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if w.changed() {
		t.Error("a file being rewritten is momentarily absent — that is not a change")
	}
}
