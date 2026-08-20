package main

// One process for preview, MCP and the filesystem (#184).

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// countingRebuilder is a rebuilder whose build does nothing but count, so a
// test can watch the loop's decisions rather than a site generator's.
func countingRebuilder(cfg *config.Config, count *atomic.Int32) *mcpRebuilder {
	r := newMCPRebuilder(generator.Config{}, cfg)
	r.buildFn = func(generator.Config, *config.Config) error {
		count.Add(1)
		return nil
	}
	return r
}

// TestTwoRebuildsNeverOverlap is the race the report did not mention and that
// exists without any watcher: the Streamable HTTP transport gives every request
// its own goroutine, so two concurrent tools/call already rebuilt one output
// tree at the same time — and captureStdout swaps the process-wide os.Stdout
// while doing it. Run under -race, this is the assertion that matters.
func TestTwoRebuildsNeverOverlap(t *testing.T) {
	var inFlight, highWater atomic.Int32
	r := newMCPRebuilder(generator.Config{}, &config.Config{Quiet: true})
	r.buildFn = func(generator.Config, *config.Config) error {
		n := inFlight.Add(1)
		for {
			seen := highWater.Load()
			if n <= seen || highWater.CompareAndSwap(seen, n) {
				break
			}
		}
		time.Sleep(200 * time.Microsecond) // wide enough for an overlap to show
		inFlight.Add(-1)
		return nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ { // MCP mutations and the watcher, arriving together
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 12; j++ {
				if _, err := r.rebuild(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if got := highWater.Load(); got != 1 {
		t.Fatalf("%d rebuilds ran at once over one output tree, want 1", got)
	}
}

// TestAReconfiguredRebuilderBuildsFromTheNewConfig: a config reload during the
// watch loop must change what a later rebuild is made of, or the watcher keeps
// building from the settings the process started with (#70's rule, here).
func TestAReconfiguredRebuilderBuildsFromTheNewConfig(t *testing.T) {
	r := newMCPRebuilder(generator.Config{Source: "before"}, &config.Config{Quiet: true})
	var sawSource string
	r.buildFn = func(g generator.Config, _ *config.Config) error {
		sawSource = g.Source
		return nil
	}
	if _, err := r.rebuild(); err != nil || sawSource != "before" {
		t.Fatalf("source = %q err = %v", sawSource, err)
	}
	r.reconfigure(generator.Config{Source: "after"}, &config.Config{Quiet: true, ContentDir: "docs"})
	if _, err := r.rebuild(); err != nil || sawSource != "after" {
		t.Fatalf("source = %q err = %v", sawSource, err)
	}
	if r.config().ContentDir != "docs" {
		t.Errorf("config() = %q, want the reloaded one", r.config().ContentDir)
	}
}

// TestAFailedRebuildIsReported: the model reads what the build said, so the
// error must survive the stdout capture rather than being swallowed by it.
func TestAFailedRebuildIsReported(t *testing.T) {
	r := newMCPRebuilder(generator.Config{}, &config.Config{Quiet: true})
	r.buildFn = func(generator.Config, *config.Config) error {
		_, _ = os.Stdout.WriteString("what the build said\n")
		return os.ErrPermission
	}
	out, err := r.rebuild()
	if err == nil {
		t.Fatal("a failed build must report its error")
	}
	if out == "" {
		t.Error("the build's own output must reach the caller")
	}
}

// watchedSite lays out a minimal project and returns its config path.
func watchedSite(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, sub := range []string{"content", "templates"} {
		if err := os.MkdirAll(sub, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join("content", "page.md"), []byte("# One\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".ssg.yaml")
	if err := os.WriteFile(configPath, []byte("domain: example.com\nquiet: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.Domain = "example.com"
	cfg.Watch = true
	return cfg, configPath
}

// TestAnEditMadeOutsideMCPRebuildsTheSite is the reported gap: a human editor
// beside the agent, an rsync, a CMS export. None of them went through MCP, and
// none of them used to rebuild anything.
func TestAnEditMadeOutsideMCPRebuildsTheSite(t *testing.T) {
	cfg, configPath := watchedSite(t)
	var builds atomic.Int32
	w := newMCPWatcher(countingRebuilder(cfg, &builds), nil, configPath, func(string, ...any) {})

	if w.poll() {
		t.Fatal("a quiet tree must not rebuild")
	}
	if err := os.WriteFile(filepath.Join("content", "page.md"), []byte("# Two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.poll() {
		t.Fatal("an edit from outside MCP must rebuild")
	}
	if builds.Load() != 1 {
		t.Fatalf("builds = %d, want exactly one", builds.Load())
	}
	if w.poll() {
		t.Error("the same edit must not rebuild twice")
	}
}

// TestATouchDoesNotRebuild: the mcp path inherits the serve path's
// content-signature gate, so mtime churn does not cost a build (PLAT-006).
func TestATouchDoesNotRebuild(t *testing.T) {
	cfg, configPath := watchedSite(t)
	var builds atomic.Int32
	w := newMCPWatcher(countingRebuilder(cfg, &builds), nil, configPath, func(string, ...any) {})

	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join("content", "page.md"), later, later); err != nil {
		t.Fatal(err)
	}
	if w.poll() || builds.Load() != 0 {
		t.Fatalf("mtime moved but bytes did not: builds = %d", builds.Load())
	}
}

// TestAConfigEditReachesTheRunningPreview: the config file is an input of its
// own here too, and the endpoints it declares are republished onto the server
// that is already listening (#180) rather than waiting for a restart.
func TestAConfigEditReachesTheRunningPreview(t *testing.T) {
	resetLiveEndpoints(t)
	cfg, configPath := watchedSite(t)
	static := staticSaying("static")
	publishEndpoints(cfg, static) // a server is up, as it would be under --http
	h := liveEndpointHandler(static)

	var builds atomic.Int32
	w := newMCPWatcher(countingRebuilder(cfg, &builds), nil, configPath, func(string, ...any) {})

	if rec := getPath(h, "/go/latest"); rec.Body.String() != "static" {
		t.Fatalf("before: %q", rec.Body)
	}
	edited := "domain: example.com\nquiet: true\ncontent_dir: docs\n" +
		"endpoints:\n  - {path: /go/latest, type: redirect, to: /blog/}\n"
	if err := os.WriteFile(configPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	if !w.poll() {
		t.Fatal("a config edit must rebuild")
	}
	if got := getPath(h, "/go/latest").Header().Get("Location"); got != "/blog/" {
		t.Errorf("Location = %q, want the endpoint the reloaded config declares", got)
	}
	if w.rebuilder.config().ContentDir != "docs" {
		t.Errorf("content_dir = %q, want the reloaded value", w.rebuilder.config().ContentDir)
	}
	if len(w.dirs) == 0 || w.dirs[0] != "docs" {
		t.Errorf("the watcher must follow the new content dir, watching %v", w.dirs)
	}
}

// TestAHalfSavedConfigKeepsTheLastGoodOne: an editor writing a file in pieces
// must not take the watcher down with it.
func TestAHalfSavedConfigKeepsTheLastGoodOne(t *testing.T) {
	cfg, configPath := watchedSite(t)
	var builds atomic.Int32
	w := newMCPWatcher(countingRebuilder(cfg, &builds), nil, configPath, func(string, ...any) {})

	if err := os.WriteFile(configPath, []byte("content_dir: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.poll() {
		t.Fatal("a config change must still be acted on")
	}
	if got := w.rebuilder.config().ContentDir; got != "content" {
		t.Errorf("content_dir = %q, want the last good value", got)
	}
}

// TestWatchIsOnlyStartedWhenAsked: a plain `ssg mcp` must not begin polling the
// filesystem — MCP mutations rebuild through the same path either way.
func TestWatchIsOnlyStartedWhenAsked(t *testing.T) {
	cfg, configPath := watchedSite(t)
	cfg.Watch = false
	var builds atomic.Int32
	var lines []string
	logf := func(f string, a ...any) { lines = append(lines, sprintfLine(f, a...)) }

	if w := startMCPWatch(countingRebuilder(cfg, &builds), nil, configPath, logf); w != nil {
		t.Fatal("no --watch, no watcher")
	}
	if len(lines) != 0 {
		t.Errorf("nothing to announce: %v", lines)
	}

	cfg.Watch = true
	w := startMCPWatch(countingRebuilder(cfg, &builds), nil, configPath, logf)
	if w == nil {
		t.Fatal("--watch must start the loop")
	}
	t.Cleanup(w.stopWatching)
	if len(lines) != 1 || !contains(lines[0], "content") || !contains(lines[0], configPath) {
		t.Errorf("the startup line must name what is watched: %v", lines)
	}
}

// TestTheLoopKeepsPolling: run() is a goroutine in production, so the only
// thing worth asserting is that it does poll — driven here on a short interval
// rather than a sleeping test.
func TestTheLoopKeepsPolling(t *testing.T) {
	cfg, configPath := watchedSite(t)
	var builds atomic.Int32
	w := newMCPWatcher(countingRebuilder(cfg, &builds), nil, configPath, func(string, ...any) {})
	w.interval = time.Millisecond // set before the goroutine reads it
	t.Cleanup(w.stopWatching)
	go w.run()

	if err := os.WriteFile(filepath.Join("content", "page.md"), []byte("# Three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for builds.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if builds.Load() == 0 {
		t.Fatal("the loop never picked the edit up")
	}
}

// contains is strings.Contains without importing strings for one call.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
