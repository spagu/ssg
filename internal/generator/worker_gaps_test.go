package generator

// Worker integration gaps (GO-065/GO-076): config validation, the vendored-dir
// reuse contract (a build must not be gated on the network), and write
// failures surfacing instead of a half-deployed Functions tree.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/fetch"
)

// TestResolveWorkerDirGuards: dir/source validation and the cached-vendor
// reuse path — an already-populated vendor dir is used as-is, no fetch.
func TestResolveWorkerDirGuards(t *testing.T) {
	t.Chdir(t.TempDir())
	g := &Generator{config: Config{OutputDir: "out", Quiet: true}}

	if _, err := g.resolveWorkerDir(WorkerConfig{}, "w"); err == nil ||
		!strings.Contains(err.Error(), "neither dir nor source") {
		t.Fatalf("empty worker config must error, got %v", err)
	}
	// A remote source without a name has no stable vendor directory: two such
	// workers would silently share files (GO-081).
	if _, err := g.resolveWorkerDir(WorkerConfig{Source: "https://example.com/w.zip"}, "w"); err == nil ||
		!strings.Contains(err.Error(), "needs a name") {
		t.Fatalf("unnamed remote source must error, got %v", err)
	}
	// A populated derived vendor dir is reused — never re-fetched.
	mustWrite(t, filepath.Join("workers", "consent", "functions", "api.js"), "export default {}")
	dir, err := g.resolveWorkerDir(WorkerConfig{Name: "consent", Source: "https://example.com/w.zip"}, "w")
	if err != nil || dir != filepath.Join("workers", "consent") {
		t.Fatalf("vendored dir must be reused without fetching: %q, %v", dir, err)
	}
}

// TestResolveWorkerDirFetchErrors: a literal secret and an unsupported archive
// shape are rejected before any network I/O happens.
func TestResolveWorkerDirFetchErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	g := &Generator{config: Config{OutputDir: "out", Quiet: true}}
	// Auth secrets must be env references — a literal in config is the leak
	// the design forbids.
	_, err := g.resolveWorkerDir(WorkerConfig{Name: "a", Source: "https://example.com/w.zip",
		Auth: fetch.Auth{Type: "bearer", Token: "literal-secret"}}, "w")
	if err == nil || !strings.Contains(err.Error(), "environment variable") {
		t.Fatalf("literal secret must error, got %v", err)
	}
	// .tar.gz is rejected up front by the fetcher — still no network.
	_, err = g.resolveWorkerDir(WorkerConfig{Name: "b", Source: "https://example.com/w.tar.gz"}, "w")
	if err == nil || !strings.Contains(err.Error(), "unsupported worker archive") {
		t.Fatalf("unsupported archive must error, got %v", err)
	}
}

// TestGenerateWorkerFilesWriteErrors: rule-cap overflow and blocked output
// paths are build errors, not truncated deployments.
func TestGenerateWorkerFilesWriteErrors(t *testing.T) {
	newWorkerGen := func(t *testing.T, w WorkerConfig) *Generator {
		t.Helper()
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "functions", "api.js"), "export default {}")
		w.Dir = dir
		return &Generator{config: Config{OutputDir: t.TempDir(), Quiet: true, Workers: []WorkerConfig{w}}}
	}

	// 101 routes exceed the Cloudflare Pages cap: fail, do not silently trim.
	over := make([]string, cfMaxRoutesRules+1)
	for i := range over {
		over[i] = "/api/" + string(rune('a'+i%26)) + "/" + string(rune('0'+i%10)) + string(rune('0'+i/10))
	}
	g := newWorkerGen(t, WorkerConfig{RoutesInclude: over})
	if err := g.generateWorkerFiles(); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("rule-cap overflow must error, got %v", err)
	}

	// A directory squatting on _routes.json blocks the manifest write.
	g = newWorkerGen(t, WorkerConfig{})
	mustMkdir(t, filepath.Join(g.config.OutputDir, "_routes.json"))
	if err := g.generateWorkerFiles(); err == nil || !strings.Contains(err.Error(), "_routes.json") {
		t.Fatalf("blocked _routes.json must error, got %v", err)
	}

	// A file squatting on output/functions blocks the tree copy.
	g = newWorkerGen(t, WorkerConfig{})
	mustWrite(t, filepath.Join(g.config.OutputDir, "functions"), "not a dir")
	if err := g.generateWorkerFiles(); err == nil || !strings.Contains(err.Error(), "copying functions") {
		t.Fatalf("blocked functions dir must error, got %v", err)
	}
}

// TestCopyWorkerPublicCopyError: a worker's public/ assets failing to copy is
// an error naming the worker, not a site quietly missing its banner JS.
func TestCopyWorkerPublicCopyError(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "public", "banner.js"), "x")
	// OutputDir is a plain file, so copying into it fails.
	outFile := filepath.Join(t.TempDir(), "out")
	mustWrite(t, outFile, "not a dir")
	g := &Generator{config: Config{OutputDir: outFile, Quiet: true}}
	err := g.copyWorkerPublic(src, `"w"`, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "copying public assets") {
		t.Fatalf("blocked public copy must error, got %v", err)
	}
}

// TestCopyPrebuiltWorkerWriteError: mode "worker" with a blocked output
// _worker.js is a build error.
func TestCopyPrebuiltWorkerWriteError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "_worker.js"), "export default {}")
	out := t.TempDir()
	mustMkdir(t, filepath.Join(out, "_worker.js"))
	g := &Generator{config: Config{OutputDir: out, Quiet: true}}
	if err := g.copyPrebuiltWorker(dir); err == nil || !strings.Contains(err.Error(), "writing _worker.js") {
		t.Fatalf("blocked _worker.js must error, got %v", err)
	}
}

// TestWarnBareImportsUnreadable: the import scan is best-effort — an
// unreadable source never fails (or stalls) the build.
func TestWarnBareImportsUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything; the unreadable-file skip cannot trigger")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.js")
	mustWrite(t, locked, `import pkg from "left-pad"`)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })              // #nosec G302 -- restoring perms on a test temp file
	(&Generator{config: Config{Quiet: true}}).warnBareImports(dir) // must not panic
}

// TestIsBareModuleSpecEmpty: an empty specifier is not a package.
func TestIsBareModuleSpecEmpty(t *testing.T) {
	if isBareModuleSpec("") {
		t.Fatal("empty specifier must not count as a bare module")
	}
}
