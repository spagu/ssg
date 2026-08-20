package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestConfigPathOf: an explicit --config wins in both forms; otherwise the
// auto-detected default is used.
func TestConfigPathOf(t *testing.T) {
	if got := configPathOf([]string{"--config=custom.yaml"}); got != "custom.yaml" {
		t.Errorf("--config= form = %q", got)
	}
	if got := configPathOf([]string{"--config", "other.yaml"}); got != "other.yaml" {
		t.Errorf("--config space form = %q", got)
	}
	// No flag: whatever FindConfigFile resolves in an empty dir (nothing).
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if got := configPathOf(nil); got != "" {
		t.Errorf("no config anywhere = %q, want empty", got)
	}
}

// TestLoadConfigFile: an empty path yields defaults, a good file loads, and a
// broken one returns an error instead of exiting (so the watcher survives).
func TestLoadConfigFile(t *testing.T) {
	cfg, err := loadConfigFile("")
	if err != nil || cfg == nil || cfg.ContentDir != "content" {
		t.Fatalf("defaults = %v, %v", cfg, err)
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "ok.yaml")
	if err := os.WriteFile(good, []byte("content_dir: docs\ntemplates_dir: themes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigFile(good)
	if err != nil || cfg.ContentDir != "docs" || cfg.TemplatesDir != "themes" {
		t.Fatalf("loaded = %v, %v", cfg, err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("content_dir: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFile(bad); err == nil {
		t.Error("a broken config must return an error, not exit")
	}
}

// TestFileSignature covers #70's change detection: content changes move the
// signature, a touch does not, and a missing file hashes to empty.
func TestFileSignature(t *testing.T) {
	if fileSignature("") != "" || fileSignature(filepath.Join(t.TempDir(), "nope.yaml")) != "" {
		t.Error("missing file must hash to empty")
	}
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := fileSignature(p)
	if first == "" {
		t.Fatal("existing file must hash")
	}
	// Rewriting identical bytes keeps the signature (no spurious reload).
	if err := os.WriteFile(p, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fileSignature(p) != first {
		t.Error("identical content must keep the same signature")
	}
	// A real edit moves it.
	if err := os.WriteFile(p, []byte("a: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fileSignature(p) == first {
		t.Error("edited content must change the signature")
	}
}

// TestWatchedInputs: the startup line names the config file, so nobody debugs a
// watcher that silently ignored it (#70).
func TestWatchedInputs(t *testing.T) {
	cfg := &config.Config{}
	got := strings.Join(watchedInputs(cfg, ""), ", ")
	if got != "content, templates" {
		t.Errorf("minimal = %q", got)
	}
	cfg.DataDir = "data"
	got = strings.Join(watchedInputs(cfg, ".ssg.yaml"), ", ")
	if !strings.Contains(got, "data") || !strings.Contains(got, "config (.ssg.yaml)") {
		t.Errorf("full = %q", got)
	}
}

// TestReloadWatchConfig: a valid edit produces refreshed config; a broken one
// keeps the previous configuration and reports ok=false.
func TestReloadWatchConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("content_dir: docs\nquiet: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := &config.Config{Quiet: true, ContentDir: "content"}
	_, cfg, ok := reloadWatchConfig(nil, p, old)
	if !ok || cfg.ContentDir != "docs" {
		t.Fatalf("reload = %v, ok=%v", cfg, ok)
	}

	if err := os.WriteFile(p, []byte("content_dir: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := reloadWatchConfig(nil, p, old); ok {
		t.Error("a broken config must not be adopted")
	}
}

// TestABrokenConfigEditIsReportedToAWatcherThatIsNotQuiet: the watcher keeps
// the last good settings either way, but a half-saved file that changes nothing
// and says nothing is a watcher the author will assume is broken (#70).
func TestABrokenConfigEditIsReportedToAWatcherThatIsNotQuiet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("content_dir: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loud := &config.Config{ContentDir: "content"} // Quiet is false
	out := captureStderr(t, func() {
		if _, _, ok := reloadWatchConfig(nil, p, loud); ok {
			t.Error("a broken config must not be adopted")
		}
	})
	if !strings.Contains(out, "Config error") {
		t.Errorf("stderr = %q", out)
	}
}
