package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/daemon"
	"github.com/spagu/ssg/internal/migrate"
	"github.com/spagu/ssg/internal/resources"
)

// keepRuntime restores what applying a limit changes, so one test's ceiling
// does not slow the rest of the package.
func keepRuntime(t *testing.T) {
	t.Helper()
	procs := runtime.GOMAXPROCS(0)
	mem := debug.SetMemoryLimit(-1)
	gc := debug.SetGCPercent(100)
	debug.SetGCPercent(gc)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(procs)
		debug.SetMemoryLimit(mem)
		debug.SetGCPercent(gc)
	})
}

// TestResourceRequest: the block wins; mem_limit is the alias for an unset
// memory, and an error names whichever key was used.
func TestResourceRequest(t *testing.T) {
	gc := 50
	cfg := &config.Config{MemLimit: "256MiB", Resources: config.ResourcesConfig{CPU: 2, GCPercent: &gc}}
	r := resourceRequest(cfg)
	if r.Memory != "256MiB" || r.MemoryField != "mem_limit" || r.CPU != 2 || r.GCPercent != &gc {
		t.Errorf("alias: %+v", r)
	}
	cfg.Resources.Memory = "1GiB"
	if r := resourceRequest(cfg); r.Memory != "1GiB" || r.MemoryField != "resources.memory" {
		t.Errorf("the block wins: %+v", r)
	}
}

func TestApplyResources(t *testing.T) {
	keepRuntime(t)
	var logged []string
	logf := func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }

	if !applyResources(resources.Request{CPU: 1, Memory: "512MiB"}, false, logf) {
		t.Fatal("a valid request was refused")
	}
	if runtime.GOMAXPROCS(0) != 1 || debug.SetMemoryLimit(-1) != 512<<20 {
		t.Error("limits not applied")
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "memory limit 512 MiB") || !strings.Contains(logged[0], "1 CPU(s)") {
		t.Errorf("logged %q", logged)
	}
	logged = nil
	applyResources(resources.Request{CPU: 1, Memory: "none"}, true, logf)
	if len(logged) != 0 {
		t.Errorf("quiet still logged %q", logged)
	}
	out := captureStderr(t, func() {
		if applyResources(resources.Request{Memory: "lots", MemoryField: "mem_limit"}, false, logf) {
			t.Error("an invalid size was applied")
		}
	})
	if !strings.Contains(out, `mem_limit: "lots"`) {
		t.Errorf("stderr = %q", out)
	}
}

// TestBuildWorkersFollowTheCPULimit: cpu: 1 means one worker, so pages render
// one at a time; without the block, build_workers is as given.
func TestBuildWorkersFollowTheCPULimit(t *testing.T) {
	eight := 8
	if n := buildWorkers(&config.Config{BuildWorkers: &eight, Resources: config.ResourcesConfig{CPU: 1}}); n != 1 {
		t.Errorf("cpu 1 → %d workers", n)
	}
	if n := buildWorkers(&config.Config{BuildWorkers: &eight}); n != 8 {
		t.Errorf("no cap → %d workers", n)
	}
	if n := buildWorkers(&config.Config{}); n != runtime.GOMAXPROCS(0) {
		t.Errorf("default → %d workers", n)
	}
}

func TestParseResourceFlag(t *testing.T) {
	var r resources.Request
	for _, arg := range []string{"--mem-limit=1GiB", "--cpu-limit=3", "--gc-percent=40"} {
		if !isResourceFlag(arg, &r) {
			t.Errorf("%s not taken", arg)
		}
	}
	if r.Memory != "1GiB" || r.MemoryField != "--mem-limit" || r.CPU != 3 || *r.GCPercent != 40 {
		t.Errorf("%+v", r)
	}
	for _, arg := range []string{"--cpu-limit=x", "--gc-percent=", "--other=1"} {
		if isResourceFlag(arg, &r) {
			t.Errorf("%s taken", arg)
		}
	}
}

// TestMainParserTakesResourceFlags: --cpu-limit and --gc-percent in both
// spellings, and --mem-limit into the block, so a flag beats the config.
func TestMainParserTakesResourceFlags(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Resources.Memory = "2GiB"
	parseFlags([]string{"--cpu-limit", "2", "--gc-percent=-1", "--mem-limit=128MiB"}, cfg)
	if cfg.Resources.CPU != 2 || cfg.Resources.GCPercent == nil || *cfg.Resources.GCPercent != -1 || cfg.Resources.Memory != "128MiB" {
		t.Errorf("%+v", cfg.Resources)
	}
}

// TestCommandsRefuseABadLimit: each command that applies limits stops on a
// value that is not one, before doing any work.
func TestCommandsRefuseABadLimit(t *testing.T) {
	keepRuntime(t)
	dir := t.TempDir()
	projects := filepath.Join(dir, daemon.DefaultConfigFile)
	if err := os.WriteFile(projects, []byte("projects:\n  - {name: blog, dir: .}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = captureStderr(t, func() {
		if code := runDaemon([]string{"--config", projects, "--mem-limit=lots", "--once"}); code != 2 {
			t.Errorf("daemon: %d", code)
		}
		if code := runDaemon([]string{"--config", projects, "--cpu-limit=1", "--quiet", "--once"}); code != 0 {
			t.Errorf("daemon with a limit: %d", code)
		}
		if code := runMigrate([]string{"wordpress", "https://e.example.com", "--mem-limit", "lots"}); code != 2 {
			t.Errorf("migrate: %d", code)
		}
		if code := runMigrate([]string{"wordpress", "https://e.example.com", "--cpu-limit=x"}); code != 2 {
			t.Errorf("migrate, not a number: %d", code)
		}
		if code := runMCP([]string{"--no-watch", "--mem-limit=lots"}); code != 2 {
			t.Errorf("mcp: %d", code)
		}
	})
	// The build stops before writing anything.
	args := append(rcovProject(t), "--mem-limit=lots")
	_ = captureStderr(t, func() {
		if code, done := run(args); !done || code != 2 {
			t.Errorf("build: %d %v", code, done)
		}
	})
	if _, err := os.Stat("output"); err == nil {
		t.Error("the build ran with a limit it could not apply")
	}
}

// TestTheLimitsAreNamedAtStartup: every command that applies a limit says so
// once — on stdout, or on stderr for `ssg mcp`, whose stdout is the protocol.
func TestTheLimitsAreNamedAtStartup(t *testing.T) {
	keepRuntime(t)
	const line = "1 CPU(s)"

	args := rcovProject(t)
	args = append(args[:len(args)-1:len(args)-1], "--cpu-limit=1") // drop --quiet
	out, _ := captureStdout(func() error {
		if code, done := run(args); !done || code != 0 {
			t.Errorf("build: %d %v", code, done)
		}
		return nil
	})
	if !strings.Contains(out, line) {
		t.Errorf("build output lacks the limits:\n%s", out)
	}

	projects := filepath.Join(t.TempDir(), daemon.DefaultConfigFile)
	if err := os.WriteFile(projects, []byte("projects:\n  - {name: blog, dir: .}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ = captureStdout(func() error {
		runDaemon([]string{"--config", projects, "--cpu-limit=1", "--once"})
		return nil
	})
	if !strings.Contains(out, line) {
		t.Errorf("daemon output lacks the limits:\n%s", out)
	}

	t.Chdir(t.TempDir())
	stubMigrate(t, func(migrate.Options) (*migrate.Report, error) { return nil, errors.New("engine down") })
	out, _ = captureStdout(func() error {
		runMigrate([]string{"wordpress", "https://e.example.com", "--cpu-limit=1"})
		return nil
	})
	if !strings.Contains(out, line) {
		t.Errorf("migrate output lacks the limits:\n%s", out)
	}

	if err := os.WriteFile(".ssg.yaml", []byte("source: s\ntemplate: simple\ndomain: example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		// --no-stdio without --listen stops before serving anything.
		if code := runMCP([]string{"--no-watch", "--no-stdio", "--cpu-limit=1"}); code != 2 {
			t.Errorf("mcp: %d", code)
		}
	})
	if !strings.Contains(errOut, line) {
		t.Errorf("mcp stderr lacks the limits:\n%s", errOut)
	}
}
