package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A config with no include: must pass through byte-for-byte.
func TestResolveIncludesNoOp(t *testing.T) {
	dir := t.TempDir()
	body := "template: simple\ndomain: example.com\n"
	p := writeYAML(t, dir, ".ssg.yaml", body)
	out, err := resolveIncludes(p, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != body {
		t.Errorf("no-include config was rewritten:\n%s", out)
	}
}

func TestIncludesSplitAndMerge(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "base.yaml", "template: base-theme\ndomain: base.example\nsearch_index: true\n")
	main := writeYAML(t, dir, ".ssg.yaml",
		"include:\n  - base.yaml\ntemplate: ssgtheme\n") // main overrides template, inherits the rest

	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Template != "ssgtheme" {
		t.Errorf("Template = %q, want the main file to win", cfg.Template)
	}
	if cfg.Domain != "base.example" {
		t.Errorf("Domain = %q, want it inherited from the include", cfg.Domain)
	}
	if !cfg.SearchIndex {
		t.Error("search_index from the include was lost")
	}
}

// Each worker's own config file contributes one entry to a `content_sources:`
// (a name-keyed list). Includes must concatenate them, not clobber.
func TestIncludesMergeNamedLists(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "a.yaml", "content_sources:\n  - name: docs\n    path: docs\n    type: page\n")
	writeYAML(t, dir, "b.yaml", "content_sources:\n  - name: blog\n    path: blog\n    type: post\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - a.yaml\n  - b.yaml\ntemplate: ssgtheme\ndomain: x\n")

	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ContentSources) != 2 {
		t.Fatalf("content_sources = %+v, want both entries merged", cfg.ContentSources)
	}
	got := map[string]string{}
	for _, s := range cfg.ContentSources {
		got[s.Path] = s.Type
	}
	if got["docs"] != "page" || got["blog"] != "post" {
		t.Errorf("merged sources = %+v", cfg.ContentSources)
	}
}

// Same name in two files → deep-merged, overlay (the later include) wins per key.
func TestIncludesNamedListOverrideByName(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "a.yaml", "content_sources:\n  - name: docs\n    path: docs\n    type: page\n    category: Old\n")
	writeYAML(t, dir, "b.yaml", "content_sources:\n  - name: docs\n    category: New\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - a.yaml\n  - b.yaml\ntemplate: t\ndomain: x\n")

	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ContentSources) != 1 {
		t.Fatalf("want one merged entry, got %+v", cfg.ContentSources)
	}
	s := cfg.ContentSources[0]
	if s.Path != "docs" || s.Category != "New" {
		t.Errorf("merged-by-name = %+v, want path kept from a.yaml and category from b.yaml", s)
	}
}

func TestIncludeFromURLWithAuth(t *testing.T) {
	t.Setenv("INC_TOKEN", "s3cret")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("domain: remote.example\nsearch_index: true\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	main := writeYAML(t, dir, ".ssg.yaml", fmt.Sprintf(
		"include:\n  - url: %s/base.yaml\n    auth:\n      type: bearer\n      token: $INC_TOKEN\ntemplate: t\n", srv.URL))

	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if gotAuth != "Bearer s3cret" {
		t.Errorf("include did not send auth: %q", gotAuth)
	}
	if cfg.Domain != "remote.example" || !cfg.SearchIndex {
		t.Errorf("remote include not merged: domain=%q search=%v", cfg.Domain, cfg.SearchIndex)
	}
}

func TestIncludeCycleDetected(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "a.yaml", "include:\n  - b.yaml\n")
	writeYAML(t, dir, "b.yaml", "include:\n  - a.yaml\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - a.yaml\ntemplate: t\ndomain: x\n")

	_, err := Load(main)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle not detected: %v", err)
	}
}

// A diamond (two includes pulling the same base) is legal, not a cycle.
func TestIncludeDiamondAllowed(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "base.yaml", "search_index: true\n")
	writeYAML(t, dir, "a.yaml", "include:\n  - base.yaml\n")
	writeYAML(t, dir, "b.yaml", "include:\n  - base.yaml\n")
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - a.yaml\n  - b.yaml\ntemplate: t\ndomain: x\n")

	cfg, err := Load(main)
	if err != nil {
		t.Fatalf("diamond include rejected: %v", err)
	}
	if !cfg.SearchIndex {
		t.Error("shared base not applied through a diamond")
	}
}

func TestIncludeMissingFile(t *testing.T) {
	dir := t.TempDir()
	main := writeYAML(t, dir, ".ssg.yaml", "include:\n  - nope.yaml\ntemplate: t\ndomain: x\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "nope.yaml") {
		t.Fatalf("missing include not reported: %v", err)
	}
}

// on_error: warn lets a build continue without an unreachable remote include,
// keeping the main file's own content; the default (fail) surfaces the error.
func TestIncludeOnErrorWarnAndFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	dir := t.TempDir()

	// warn + retries:0 → no fail, no delay; main content preserved.
	warn := writeYAML(t, dir, "warn.ssg.yaml",
		"include:\n  - url: "+srv.URL+"/missing.yaml\n    on_error: warn\n    retries: 0\ntemplate: t\ndomain: x\n")
	cfg, err := Load(warn)
	if err != nil {
		t.Fatalf("on_error: warn should not fail the build: %v", err)
	}
	if cfg.Template != "t" {
		t.Errorf("main content lost through a warned include: %+v", cfg.Template)
	}

	// default (fail) → error.
	fail := writeYAML(t, dir, "fail.ssg.yaml",
		"include:\n  - url: "+srv.URL+"/missing.yaml\n    retries: 0\ntemplate: t\ndomain: x\n")
	if _, err := Load(fail); err == nil {
		t.Fatal("a failing include with default on_error should fail the build")
	}
}

// on_error must be fail or warn; anything else is a config error.
func TestIncludeOnErrorInvalid(t *testing.T) {
	dir := t.TempDir()
	main := writeYAML(t, dir, ".ssg.yaml",
		"include:\n  - url: https://example.com/x.yaml\n    on_error: maybe\ntemplate: t\ndomain: x\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "on_error") {
		t.Fatalf("invalid on_error not reported: %v", err)
	}
}
