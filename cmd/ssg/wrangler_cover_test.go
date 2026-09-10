package main

// Scaffolding for Workers: the starter wrangler.toml a watch build generates
// when a project configures a Worker without one, and the template extractor
// that must never write outside the workers directory.

import (
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// dcovBlockWranglerWrite makes wrangler.toml impossible to create in the current
// directory without depending on file permissions (a test run as root would
// write straight through a read-only directory). A symlink pointing at itself is
// refused by the kernel for every user.
func dcovBlockWranglerWrite(t *testing.T) {
	t.Helper()
	if err := os.Symlink("wrangler.toml", "wrangler.toml"); err != nil {
		t.Skipf("this filesystem cannot hold a symlink: %v", err)
	}
}

// TestRunNewWranglerWritesAStarterConfigForAConfiguredWorker (GO-077): a Pages
// project with workers needs a wrangler.toml before `wrangler pages dev` can
// read any binding, and hand-writing one is the step people skip.
func TestRunNewWranglerWritesAStarterConfigForAConfiguredWorker(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "source: s\ntemplate: simple\ndomain: shop.example.com\noutput_dir: out\n" +
		"workers:\n  - name: w\n    dir: workers/w\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(func() error {
		if code := runNewWrangler(nil); code != 0 {
			t.Errorf("runNewWrangler = %d, want 0", code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Wrote wrangler.toml") {
		t.Errorf("the generated file must be announced:\n%s", out)
	}
	written, readErr := os.ReadFile("wrangler.toml")
	if readErr != nil {
		t.Fatalf("wrangler.toml not written: %v", readErr)
	}
	if !strings.Contains(string(written), `pages_build_output_dir = "./out"`) {
		t.Errorf("the config must point wrangler at the site's output dir:\n%s", written)
	}
}

// TestWranglerGenerationReportsAConfigItCannotWrite: generating is best-effort
// during a build and fatal for the explicit command, but either way the failure
// must be visible — a silent one leaves `wrangler pages dev` with no bindings
// and no clue why.
func TestWranglerGenerationReportsAConfigItCannotWrite(t *testing.T) {
	t.Chdir(t.TempDir())
	dcovBlockWranglerWrite(t)

	cfg := &config.Config{Domain: "shop.example.com", OutputDir: "out",
		Workers: []config.WorkerConfig{{Name: "w", Dir: "workers/w"}}}
	diag := captureStderr(t, func() { ensureWranglerForWorkers(cfg) })
	if !strings.Contains(diag, "could not generate wrangler.toml") {
		t.Errorf("a build must warn and carry on: %q", diag)
	}

	yaml := "source: s\ntemplate: simple\ndomain: shop.example.com\noutput_dir: out\n" +
		"workers:\n  - name: w\n    dir: workers/w\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	diag = captureStderr(t, func() {
		if code := runNewWrangler(nil); code != 1 {
			t.Errorf("`ssg new wrangler` that wrote nothing must exit 1, got %d", code)
		}
	})
	if !strings.Contains(diag, "wrangler.toml") {
		t.Errorf("the command must name the file it could not write: %q", diag)
	}
}

// TestEnsureWranglerForWorkersAnnouncesWhatItGeneratedUnlessQuiet: a file that
// appears in the project during a build has to be accounted for; --quiet is the
// only reason to stay silent about it.
func TestEnsureWranglerForWorkersAnnouncesWhatItGeneratedUnlessQuiet(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Domain: "shop.example.com", OutputDir: "out",
		Workers: []config.WorkerConfig{{Name: "w", Dir: "workers/w"}}}
	out, err := captureStdout(func() error { ensureWranglerForWorkers(cfg); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Generated wrangler.toml") {
		t.Errorf("a generated wrangler.toml must be announced:\n%s", out)
	}
}

// TestRunNewWorkerRefusesATemplateNameThatCouldEscapeTheWorkersDirectory: the
// name becomes a path segment under ./workers, so anything that could climb out
// of it must be refused before a single file is written.
func TestRunNewWorkerRefusesATemplateNameThatCouldEscapeTheWorkersDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"../evil", "sub/dir", `back\slash`, ".."} {
		var diag string
		_, _ = captureStdout(func() error {
			diag = captureStderr(t, func() {
				if code := runNewWorker([]string{name}); code != 1 {
					t.Errorf("runNewWorker(%q) = %d, want 1", name, code)
				}
			})
			return nil
		})
		if !strings.Contains(diag, "invalid template name") {
			t.Errorf("%q must be refused as a name, not looked up: %q", name, diag)
		}
	}
	if entries, err := os.ReadDir("."); err != nil || len(entries) != 0 {
		t.Errorf("a refused name must leave the project untouched: %v (%v)", entries, err)
	}
}

// TestRunNewWorkerReportsAScaffoldItCouldNotWrite: an exit 0 with nothing on
// disk would send the operator looking for files that were never created.
func TestRunNewWorkerReportsAScaffoldItCouldNotWrite(t *testing.T) {
	t.Chdir(t.TempDir())
	// `workers` is a regular file, so nothing can be created beneath it.
	if err := os.WriteFile("workers", []byte("in the way\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var diag string
	_, _ = captureStdout(func() error {
		diag = captureStderr(t, func() {
			if code := runNewWorker([]string{"dynamic-price"}); code != 1 {
				t.Errorf("a scaffold that wrote nothing must exit 1, got %d", code)
			}
		})
		return nil
	})
	if !strings.Contains(diag, "workers") {
		t.Errorf("the failure must name the path it could not create: %q", diag)
	}
}
