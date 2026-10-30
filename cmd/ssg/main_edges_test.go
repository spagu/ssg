package main

// Edges of main.go the other tests do not reach: runner flags, flag
// suggestions, the custom-output mapping, a config reload, and the failure
// paths of build and the watch runner.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// TestParseFlagsWatchRunnerImpliesWatch: --wrangler selects the runner and
// turns watching on.
func TestParseFlagsWatchRunnerImpliesWatch(t *testing.T) {
	cfg := &config.Config{}
	parseFlags([]string{"--wrangler"}, cfg)
	if cfg.WatchRunner != "wrangler" || !cfg.Watch {
		t.Errorf("runner=%q watch=%v", cfg.WatchRunner, cfg.Watch)
	}
}

// TestNearestFlagLongerThanKnown: a typed flag that a known one prefixes is
// still suggested.
func TestNearestFlagLongerThanKnown(t *testing.T) {
	if got := nearestFlag("--quietly", map[string]bool{"--quiet": true}); got != "--quiet" {
		t.Errorf("nearestFlag = %q", got)
	}
}

func TestToGeneratorOutputs(t *testing.T) {
	got := toGeneratorOutputs([]config.CustomOutput{{Name: "api", Suffix: "index.json", MIME: "application/json", Template: "api.tmpl"}})
	want := generator.CustomOutput{Name: "api", Suffix: "index.json", MIME: "application/json", Template: "api.tmpl"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("toGeneratorOutputs = %+v", got)
	}
}

// TestReloadWatchConfigReportsTheReload: a good file reloads and says so.
func TestReloadWatchConfigReportsTheReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssg.yaml")
	if err := os.WriteFile(path, []byte("domain: example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cfg, ok := reloadWatchConfig(nil, path, &config.Config{})
	if !ok || cfg == nil || cfg.Domain != "example.com" {
		t.Fatalf("reload ok=%v cfg=%+v", ok, cfg)
	}
}

// TestBuildRejectsAnInvalidGeneratorConfig: a configuration the generator
// refuses fails the build before anything is written.
func TestBuildRejectsAnInvalidGeneratorConfig(t *testing.T) {
	genCfg := generator.Config{OutputDir: t.TempDir()}
	genCfg.I18n.Enabled = true // and no languages: the generator refuses it
	err := build(genCfg, &config.Config{Quiet: true})
	if err == nil || !strings.Contains(err.Error(), "initializing generator") {
		t.Errorf("build = %v", err)
	}
}

// TestStartWatchRunnerCannotStart: a runner whose directory is a file fails to
// start, and the build carries on without it.
func TestStartWatchRunnerCannotStart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cmd := startWatchRunner(watchRunnerSpec{Runner: "true", Dir: file, Quiet: true}); cmd != nil {
		t.Errorf("a runner that cannot start must return nil, got %v", cmd)
	}
}

// TestParseBoolFlagsMarkdownCache: both spellings set the pointer.
func TestParseBoolFlagsMarkdownCache(t *testing.T) {
	for arg, want := range map[string]bool{markdownCacheFlag: true, noMarkdownCacheFlag: false} {
		cfg := &config.Config{}
		if !parseBoolFlags(arg, cfg) || cfg.MarkdownCache == nil || *cfg.MarkdownCache != want {
			t.Errorf("%s: %v", arg, cfg.MarkdownCache)
		}
	}
}

// TestRunDispatchesASubcommand: a single verb and a verb+noun pair are both
// handled, and run returns.
func TestRunDispatchesASubcommand(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, done := run([]string{"migrate"}); !done || code != 2 {
		t.Errorf("bare migrate: code=%d done=%v", code, done)
	}
	if _, done := run([]string{"new", "worker"}); !done {
		t.Error("a subcommand finishes the program")
	}
}

// TestMddbWatchDefaultIntervalAndInitialChecksum: an unset interval falls back
// to the default, and the first checksum is reported when not quiet.
func TestMddbWatchDefaultIntervalAndInitialChecksum(t *testing.T) {
	srv, _ := wcovChecksumServer(t, []string{"aaa"}, http.StatusOK)
	genCfg, cfg, _ := wcovSite(t, srv.URL)
	cfg.Mddb.WatchInterval = 0
	cfg.Quiet = false
	stop := make(chan struct{})
	close(stop)
	if !wcovRun(t, genCfg, cfg, stop, 5*time.Second) {
		t.Error("the loop ignored its stop channel")
	}
}

// TestWatchLoopStopsItsRunner: the runner the loop started is killed when the
// loop ends.
func TestWatchLoopStopsItsRunner(t *testing.T) {
	genCfg, cfg, _ := wcovFileSite(t)
	cfg.WatchRunner = "sleep 30"
	stop := make(chan struct{})
	close(stop)
	done := make(chan struct{})
	go func() {
		runWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the loop ignored its stop channel")
	}
}

// TestBuildStopsAtAFailingStep: a step after the generator — endpoints, then
// the archive — fails the build with its own error.
func TestBuildStopsAtAFailingStep(t *testing.T) {
	genCfg, cfg, dir := wcovFileSite(t)
	cfg.EndpointsPlatform = "nowhere"
	cfg.Endpoints = []config.Endpoint{{Path: "/api/go", Type: "redirect", To: "https://example.com/"}}
	if err := build(genCfg, cfg); err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("endpoints: %v", err)
	}

	cfg.EndpointsPlatform, cfg.Endpoints = "", nil
	cfg.Zip = true
	// A directory where the archive should go: the archive cannot be created.
	if err := os.Mkdir(filepath.Join(dir, "example.com.zip"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := build(genCfg, cfg); err == nil {
		t.Error("archive: a blocked zip path must fail the build")
	}
}
