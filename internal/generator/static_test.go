// Package generator - tests for the static/ passthrough directory (issue #8).
package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStaticFile is a small helper that creates parent dirs and writes a file.
func writeStaticFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// assertFileContent fails unless the file exists with exactly the wanted bytes.
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path) // #nosec G304 -- test reads files it just wrote
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s content = %q, want %q", path, string(got), want)
	}
}

// TestCopyStaticDirCopiesEverything reproduces issue #8: every file and
// subdirectory under static/ must reach the output, not only a fixed subset.
func TestCopyStaticDirCopiesEverything(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	outputDir := filepath.Join(tmpDir, "output")

	// The previously-skipped entries from the bug report plus a nested tree.
	files := map[string]string{
		filepath.Join(staticDir, "downloads", "guide.pdf"):      "PDF",
		filepath.Join(staticDir, "assets", "app.css"):           "body{}",
		filepath.Join(staticDir, "scripts", "app.js"):           "console.log(1)",
		filepath.Join(staticDir, "styles", "theme.css"):         ".a{color:#123}",
		filepath.Join(staticDir, "manifest.json"):               `{"name":"ssg"}`,
		filepath.Join(staticDir, "images", "logo.svg"):          "<svg/>",
		filepath.Join(staticDir, "downloads", "v1", "note.txt"): "deep",
	}
	for path, content := range files {
		writeStaticFile(t, path, content)
	}

	gen := &Generator{config: Config{StaticDir: staticDir, OutputDir: outputDir, Quiet: true}}
	if err := gen.copyStaticDir(); err != nil {
		t.Fatalf("copyStaticDir failed: %v", err)
	}

	// Every source file must appear at the mirrored output path with same bytes.
	for path, content := range files {
		rel, err := filepath.Rel(staticDir, path)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		assertFileContent(t, filepath.Join(outputDir, rel), content)
	}
}

// TestCopyStaticDirMissingIsNoop ensures sites without a static/ directory are
// unaffected: the step returns nil and creates nothing.
func TestCopyStaticDirMissingIsNoop(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	gen := &Generator{config: Config{
		StaticDir: filepath.Join(tmpDir, "does-not-exist"),
		OutputDir: outputDir,
		Quiet:     true,
	}}
	if err := gen.copyStaticDir(); err != nil {
		t.Fatalf("copyStaticDir on missing dir should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Errorf("output dir should not be created when static/ is absent")
	}
}

// TestCopyStaticDirFileNotDirIsNoop ensures a plain file named like the static
// dir is ignored rather than treated as a directory.
func TestCopyStaticDirFileNotDirIsNoop(t *testing.T) {
	tmpDir := t.TempDir()
	staticPath := filepath.Join(tmpDir, "static")
	if err := os.WriteFile(staticPath, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	outputDir := filepath.Join(tmpDir, "output")

	gen := &Generator{config: Config{StaticDir: staticPath, OutputDir: outputDir, Quiet: true}}
	if err := gen.copyStaticDir(); err != nil {
		t.Fatalf("copyStaticDir on a file should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Errorf("output dir should not be created when static is a file")
	}
}

// TestCopyStaticDirDefaultsToStatic verifies that an empty StaticDir falls back
// to the conventional "static" directory relative to the working directory.
func TestCopyStaticDirDefaultsToStatic(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir) // run generation as if invoked from the project root

	writeStaticFile(t, filepath.Join(tmpDir, "static", "robots-extra.txt"), "ok")
	outputDir := filepath.Join(tmpDir, "output")

	gen := &Generator{config: Config{StaticDir: "", OutputDir: outputDir, Quiet: true}}
	if err := gen.copyStaticDir(); err != nil {
		t.Fatalf("copyStaticDir failed: %v", err)
	}
	assertFileContent(t, filepath.Join(outputDir, "robots-extra.txt"), "ok")
}

// TestCopyStaticDirNonQuietPrints exercises the informational branch so it is
// covered and does not panic when Quiet is false.
func TestCopyStaticDirNonQuietPrints(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	outputDir := filepath.Join(tmpDir, "output")
	writeStaticFile(t, filepath.Join(staticDir, "file.txt"), "x")

	gen := &Generator{config: Config{StaticDir: staticDir, OutputDir: outputDir, Quiet: false}}
	if err := gen.copyStaticDir(); err != nil {
		t.Fatalf("copyStaticDir failed: %v", err)
	}
	assertFileContent(t, filepath.Join(outputDir, "file.txt"), "x")
}
