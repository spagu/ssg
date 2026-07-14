package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSass writes an executable stand-in for dart-sass that emits a fixed CSS
// body into the destination path (last argument), and returns its path.
func fakeSass(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sass")
	script := "#!/bin/sh\n# fake dart-sass: <flags> src dst\nfor last; do :; done\necho 'body{color:teal}' > \"$last\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { // #nosec G306 -- test executable
		t.Fatal(err)
	}
	return bin
}

func TestCompileSCSSIfRequested(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "css", "main.scss"), "$c: teal;\nbody{color:$c}")
	mustWrite(t, filepath.Join(out, "css", "_partial.scss"), "$c: teal;")

	g := newTestGen(t, "")
	g.config.OutputDir = out
	g.config.SCSS = true
	g.config.SassBinary = fakeSass(t)
	g.config.Quiet = true

	if err := g.compileSCSSIfRequested(); err != nil {
		t.Fatalf("compileSCSSIfRequested: %v", err)
	}

	// Entry point compiled to css; both .scss sources removed from the output.
	css, err := os.ReadFile(filepath.Join(out, "css", "main.css"))
	if err != nil || !strings.Contains(string(css), "teal") {
		t.Errorf("main.css = %q, %v", css, err)
	}
	if _, err := os.Stat(filepath.Join(out, "css", "main.scss")); !os.IsNotExist(err) {
		t.Error("main.scss must be removed from the output")
	}
	if _, err := os.Stat(filepath.Join(out, "css", "_partial.scss")); !os.IsNotExist(err) {
		t.Error("_partial.scss must be removed from the output")
	}
	// No stray _partial.css: partials are never compiled standalone.
	if _, err := os.Stat(filepath.Join(out, "css", "_partial.css")); !os.IsNotExist(err) {
		t.Error("partials must not be compiled to their own css")
	}
}

func TestCompileSCSSDisabledAndMissingTool(t *testing.T) {
	g := newTestGen(t, "")
	// Disabled → no-op even without any binary.
	if err := g.compileSCSSIfRequested(); err != nil {
		t.Errorf("disabled scss should be a no-op: %v", err)
	}
	// Enabled with an explicit, nonexistent binary → graceful skip, no error.
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "a.scss"), "body{}")
	g.config.OutputDir = out
	g.config.SCSS = true
	g.config.SassBinary = filepath.Join(t.TempDir(), "absent-sass")
	if err := g.compileSCSSIfRequested(); err != nil {
		t.Errorf("missing tool must skip gracefully, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "a.scss")); err != nil {
		t.Error("sources must stay untouched when the tool is missing")
	}
}

func TestCompileSCSSCompilerError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sass")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'syntax error' >&2\nexit 65\n"), 0o755); err != nil { // #nosec G306 -- test executable
		t.Fatal(err)
	}
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "bad.scss"), "body{")

	g := newTestGen(t, "")
	g.config.OutputDir = out
	g.config.SCSS = true
	g.config.SassBinary = bin
	err := g.compileSCSSIfRequested()
	if err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("expected a descriptive compiler error, got: %v", err)
	}
}

func TestSafeSassArg(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"/abs/a.scss": "/abs/a.scss",
		"./rel.scss":  "./rel.scss",
		"rel.scss":    "./rel.scss",
		"-flag.scss":  "./-flag.scss", // never parseable as an option (SEC-011)
	}
	for in, want := range cases {
		if got := safeSassArg(in); got != want {
			t.Errorf("safeSassArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSassBinaryPathLookup covers PATH resolution: found in PATH, and absent.
func TestSassBinaryPathLookup(t *testing.T) {
	g := newTestGen(t, "")
	// PATH containing our fake `sass` → resolved.
	fake := fakeSass(t)
	t.Setenv("PATH", filepath.Dir(fake))
	if got := g.sassBinary(); got == "" {
		t.Error("expected sass to resolve from PATH")
	}
	// Empty PATH → "" (skip).
	t.Setenv("PATH", t.TempDir())
	if got := g.sassBinary(); got != "" {
		t.Errorf("expected empty resolution, got %q", got)
	}
}

// TestCompileSCSSRemoveWarning covers the remove-failure warning path: the
// .scss source disappears between compile and cleanup.
func TestCompileSCSSRemoveWarning(t *testing.T) {
	out := t.TempDir()
	src := filepath.Join(out, "gone.scss")
	mustWrite(t, src, "body{}")

	dir := t.TempDir()
	bin := filepath.Join(dir, "sass")
	// Fake sass that also deletes its own source, so os.Remove later fails.
	script := "#!/bin/sh\nfor last; do :; done\necho 'x{}' > \"$last\"\nrm -f \"$2\" 2>/dev/null || rm -f \"$1\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { // #nosec G306 -- test executable
		t.Fatal(err)
	}

	g := newTestGen(t, "")
	g.config.OutputDir = out
	g.config.SCSS = true
	g.config.SassBinary = bin
	if err := g.compileSCSSIfRequested(); err != nil {
		t.Fatalf("compileSCSSIfRequested: %v", err)
	}
}
