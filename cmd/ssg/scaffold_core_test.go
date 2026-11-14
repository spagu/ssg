package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/modules"
)

// withCore puts a core worker's manifest in ./workers/core (GO-144).
func withCore(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("workers", "core"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("workers", "core", "module.json"), []byte(`{"id": "core"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestScaffoldWithCoreAddsOneNote: with core in the project, a Pages template
// gets one more line after the unchanged text, a standalone Worker its
// modules: entry.
func TestScaffoldWithCoreAddsOneNote(t *testing.T) {
	withCore(t)
	out, code := captureGraphOutput(t, func() int { return runNewWorker([]string{"comments"}) })
	want := fmt.Sprintf("✅ Scaffolded worker %q into workers/%s/\n\n", "comments", "comments") + fmt.Sprintf(pagesSnippetBefore123, "comments", "comments") +
		"\nworkers/core is in this project: once workers/comments is in workers:, the next build adds it to the core worker's module registry.\n"
	if code != 0 || out != want {
		t.Errorf("exit %d, output:\n%s\nwant:\n%s", code, out, want)
	}

	out, code = captureGraphOutput(t, func() int { return runNewWorker([]string{"newsletter-ui"}) })
	if code != 0 || !strings.HasSuffix(out, "\nworkers/core is in this project: to register newsletter-ui with it, add this to .ssg.yaml:\n\n"+
		"modules:\n  - id: newsletter-ui\n    url: /newsletter-admin\n    dir: workers/newsletter-ui\n") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
}

func TestCoreRegistrationNoteFallsBackToTheName(t *testing.T) {
	if coreRegistrationNote(false, "x", nil) != "" {
		t.Error("no core, no note")
	}
	for _, toml := range []string{"name = \"x\"\n", "routes = [ { pattern = \"example.com/*\" } ]\n"} {
		if note := coreRegistrationNote(true, "x", []byte(toml)); !strings.Contains(note, "url: /x\n") {
			t.Errorf("%q: %s", toml, note)
		}
	}
}

// TestModulesOf: modules: reaches the generator unchanged; none stays nil.
func TestModulesOf(t *testing.T) {
	if modulesOf(&config.Config{}) != nil {
		t.Error("no modules → nil")
	}
	got := modulesOf(&config.Config{Modules: []config.ModuleConfig{{ID: "a", URL: "/a", Dir: "w/a"}}})
	if len(got) != 1 || got[0] != (modules.Ref{ID: "a", URL: "/a", Dir: "w/a"}) {
		t.Errorf("%+v", got)
	}
}
