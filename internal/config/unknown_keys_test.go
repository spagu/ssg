package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UX-002: an unknown key used to be ignored in silence, so a config written
// against a newer ssg behaved as if the key were absent — and the resulting
// "missing source" was impossible to trace back to its cause.

// captureStderr runs fn with os.Stderr redirected and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = orig

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	_ = r.Close()
	return string(buf[:n])
}

func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWarnsAboutUnknownKeys(t *testing.T) {
	path := writeConfig(t, ".ssg.yaml", "template: simple\ndomain: example.com\ncontent_sourcesX: []\nwhat_is_this: 1\n")

	var cfg *Config
	var err error
	out := captureStderr(t, func() { cfg, err = Load(path) })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The known keys still load: an unknown key is a warning, never a failure,
	// so a config from a newer ssg keeps working on an older one.
	if cfg.Template != "simple" || cfg.Domain != "example.com" {
		t.Errorf("known keys were lost: %+v", cfg)
	}
	for _, want := range []string{"content_sourcesX", "what_is_this", "unknown configuration key"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr %q does not mention %q", out, want)
		}
	}
}

func TestLoadSilentForKnownKeys(t *testing.T) {
	path := writeConfig(t, ".ssg.yaml", "template: simple\ndomain: example.com\ncontent_sources:\n  - path: docs\n    category: Docs\nauto_excerpt: true\nlink_rewrites:\n  \"../x/\": \"https://example.com/x/\"\n")

	var cfg *Config
	var err error
	out := captureStderr(t, func() { cfg, err = Load(path) })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(out, "unknown configuration key") {
		t.Errorf("valid config warned: %s", out)
	}
	if len(cfg.ContentSources) != 1 || cfg.ContentSources[0].Path != "docs" || cfg.ContentSources[0].Category != "Docs" {
		t.Errorf("content_sources = %+v", cfg.ContentSources)
	}
	if !cfg.AutoExcerpt {
		t.Error("auto_excerpt was not loaded")
	}
	if cfg.LinkRewrites["../x/"] != "https://example.com/x/" {
		t.Errorf("link_rewrites = %+v", cfg.LinkRewrites)
	}
}

// TOML and JSON keep the historical pass-through: strict decoding is
// YAML-only, and the warning must not claim otherwise by staying silent there.
func TestLoadUnknownKeysNonYAML(t *testing.T) {
	path := writeConfig(t, ".ssg.json", `{"template":"simple","domain":"example.com","nope":1}`)
	out := captureStderr(t, func() {
		if _, err := Load(path); err != nil {
			t.Errorf("Load: %v", err)
		}
	})
	if strings.Contains(out, "unknown configuration key") {
		t.Errorf("JSON config warned about unknown keys: %s", out)
	}
}

func TestUnknownFieldName(t *testing.T) {
	got, ok := unknownFieldName("line 12: field content_sources not found in type config.Config")
	if !ok || got != "content_sources" {
		t.Errorf("unknownFieldName = (%q, %v), want (\"content_sources\", true)", got, ok)
	}
	if _, ok := unknownFieldName("some unrelated error"); ok {
		t.Error("unrelated error parsed as an unknown-field complaint")
	}
}
