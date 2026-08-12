package main

// Tests for build-step runners and watch helpers (second coverage raise, 1.8.27).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

func TestRunArchives(t *testing.T) {
	t.Chdir(t.TempDir())
	out := "site-out"
	if err := os.MkdirAll(filepath.Join(out, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("<html/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Domain: "example.com", OutputDir: out, Quiet: true,
		Zip: true, TarGz: true, TarXz: true}
	if err := runArchives(cfg); err != nil {
		t.Fatalf("runArchives: %v", err)
	}
	for _, name := range []string{"example.com.zip", "example.com.tar.gz", "example.com.tar.xz"} {
		if info, err := os.Stat(name); err != nil || info.Size() == 0 {
			t.Errorf("archive %s missing/empty: %v", name, err)
		}
	}
	// All formats off → no-op, no error.
	if err := runArchives(&config.Config{Domain: "x", OutputDir: out}); err != nil {
		t.Fatalf("no formats: %v", err)
	}
}

func TestRunImagesGC(t *testing.T) {
	t.Chdir(t.TempDir())
	gen, err := generator.New(generator.Config{
		Source: "s", Template: "simple", Domain: "x",
		ContentDir: "content", TemplatesDir: "templates", OutputDir: "out", Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Off → silent no-op.
	runImagesGC(gen, &config.Config{})
	// Dry run against an empty cache → zero, no error output that matters.
	runImagesGC(gen, &config.Config{ImagesGCDry: true, Quiet: false})
	// Real run, quiet.
	runImagesGC(gen, &config.Config{ImagesGC: true, Quiet: true})
}

// TestBuildEndToEnd drives the full main-level build wrapper: generate →
// endpoints → images GC → WebP → archives (deploy off). One call, whole seam.
func TestBuildEndToEnd(t *testing.T) {
	t.Chdir(t.TempDir())
	postsDir := filepath.Join("content", "site", "posts", "news")
	if err := os.MkdirAll(postsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	post := "---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(postsDir, "hello.md"), []byte(post), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("content", "site", "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Source: "site", Template: "simple", Domain: "e2e.example.com",
		ContentDir: "content", TemplatesDir: "templates", OutputDir: "out",
		StaticDir: "static", DataDir: "data", PostURLFormat: "slug",
		Quiet: true, Zip: true, ImagesGCDry: true,
	}
	if err := build(createGeneratorConfig(cfg), cfg); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(filepath.Join("out", "hello", "index.html")); err != nil {
		t.Fatalf("post not generated: %v", err)
	}
	if info, err := os.Stat("e2e.example.com.zip"); err != nil || info.Size() == 0 {
		t.Fatalf("zip archive missing: %v", err)
	}
}

// TestEnsureWranglerForWorkers: with a worker configured, a starter
// wrangler.toml is generated once; the second call leaves it untouched.
func TestEnsureWranglerForWorkers(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{Domain: "x.com", OutputDir: "out", Quiet: true}
	// No workers → no file.
	ensureWranglerForWorkers(cfg)
	if _, err := os.Stat("wrangler.toml"); err == nil {
		t.Fatal("no workers must not generate wrangler.toml")
	}
	cfg.Workers = []config.WorkerConfig{{Name: "w", Dir: "workers/w"}}
	ensureWranglerForWorkers(cfg)
	if _, err := os.Stat("wrangler.toml"); err != nil {
		t.Fatalf("wrangler.toml not generated: %v", err)
	}
	before, _ := os.ReadFile("wrangler.toml")
	ensureWranglerForWorkers(cfg) // idempotent
	after, _ := os.ReadFile("wrangler.toml")
	if string(before) != string(after) {
		t.Fatal("second call must not rewrite the file")
	}
	// runNewWrangler loads ITS OWN config from disk; give it one with a worker —
	// the wrangler.toml already exists, so it reports that and exits 0.
	yaml := "source: s\ntemplate: simple\ndomain: x.com\nworkers:\n  - name: w\n    dir: workers/w\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runNewWrangler(nil); code != 0 {
		t.Fatalf("runNewWrangler existing = %d", code)
	}
}

// TestExplicitWranglerConfig: the first worker naming its own config wins.
func TestExplicitWranglerConfig(t *testing.T) {
	cfg := &config.Config{Workers: []config.WorkerConfig{{Name: "a"}, {Name: "b", WranglerConfig: "own.toml"}}}
	if got := explicitWranglerConfig(cfg); got != "own.toml" {
		t.Fatalf("explicitWranglerConfig = %q", got)
	}
	if got := explicitWranglerConfig(&config.Config{}); got != "" {
		t.Fatalf("no workers = %q", got)
	}
}

// TestLogServerStartAndListener: banner variants print without panic; the
// listener binds an ephemeral port and applies the max-conns cap.
func TestLogServerStartAndListener(t *testing.T) {
	logServerStart(&config.Config{Quiet: true}, "http://x", "", false)
	logServerStart(&config.Config{OutputDir: "out"}, "http://127.0.0.1:1", "", false)
	logServerStart(&config.Config{OutputDir: "out", TLSDomain: "x.com"}, "http://127.0.0.1:1", "auto", true)

	ln, err := newServerListener("127.0.0.1:0", 2)
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	defer func() { _ = ln.Close() }()
	if ln.Addr().String() == "" {
		t.Fatal("no bound address")
	}
	// Bad address errors.
	if _, err := newServerListener("256.256.256.256:0", 0); err == nil {
		t.Fatal("bad address must error")
	}
}

// TestReportMissingSettings: both branches (config file present/absent) print
// without panicking and name every missing field.
func TestReportMissingSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	reportMissingSettings(&config.Config{}, false)           // no config file
	reportMissingSettings(&config.Config{Source: "s"}, true) // sourceOptional
	if err := os.WriteFile(".ssg.yaml", []byte("template: simple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reportMissingSettings(&config.Config{Template: "simple"}, false) // config file present
}

func TestWatchDirsContentSources(t *testing.T) {
	cfg := &config.Config{ContentDir: "content", TemplatesDir: "templates", DataDir: "data"}
	cfg.ContentSources = []config.ContentSource{{Path: "docs"}, {Path: "  "}, {Path: "blog"}}
	dirs := watchDirs(cfg)
	want := []string{"content", "templates", "data", "docs", "blog"}
	if len(dirs) != len(want) {
		t.Fatalf("watchDirs = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Fatalf("watchDirs[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
	// No data dir → skipped.
	if dirs := watchDirs(&config.Config{ContentDir: "c", TemplatesDir: "t"}); len(dirs) != 2 {
		t.Fatalf("no data dir: %v", dirs)
	}
}
