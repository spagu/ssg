package main

// The daemon's supervisor loop, the migration flags that reach the engine, and
// the wrangler/worker scaffolding — the paths that only run when a signal
// arrives, a file changes underfoot, or a write fails.

import (
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/daemon"
)

// dcovLimit bounds every wait in this file. Generous on purpose: the supervisor
// polls once a second, so a loaded CI box needs room, and a test that hangs is
// worse than one that fails.
const dcovLimit = 30 * time.Second

// dcovStdoutTap redirects os.Stdout for the duration of the test. The buffer it
// returns is readable *while* the code under test is still running, which is the
// only way to drive a loop that reacts to what it printed; the settle function
// restores stdout and waits for the last byte to arrive, so an assertion never
// races the goroutine draining the pipe.
func dcovStdoutTap(t *testing.T) (*syncBuffer, func() string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	var buf syncBuffer
	saved := os.Stdout
	os.Stdout = w
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		_, _ = io.Copy(&buf, r)
	}()
	var once sync.Once
	settle := func() string {
		once.Do(func() {
			os.Stdout = saved
			_ = w.Close()
			<-copied
			_ = r.Close()
		})
		return buf.String()
	}
	t.Cleanup(func() { settle() })
	return &buf, settle
}

// dcovStderrTap does the same for the diagnostic sink.
func dcovStderrTap(t *testing.T) *syncBuffer {
	t.Helper()
	var buf syncBuffer
	saved := setStderrSink(&buf)
	t.Cleanup(func() { setStderrSink(saved) })
	return &buf
}

// dcovWaitFor blocks until buf contains want, or the deadline passes. It never
// fails the test itself, so a driver goroutine may call it; the assertion stays
// on the main goroutine, against the same buffer.
func dcovWaitFor(buf *syncBuffer, want string) {
	deadline := time.Now().Add(dcovLimit)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// dcovFakeSignals replaces the signal seam with two unbuffered channels, so a
// test drives reload and shutdown in a fixed order rather than racing them.
func dcovFakeSignals(t *testing.T) (reload, quit chan os.Signal) {
	t.Helper()
	reload, quit = make(chan os.Signal), make(chan os.Signal)
	saved := daemonSignals
	daemonSignals = func() (<-chan os.Signal, <-chan os.Signal) { return reload, quit }
	t.Cleanup(func() { daemonSignals = saved })
	return reload, quit
}

// dcovSend hands one signal to the loop, giving up rather than blocking forever
// if the loop never reaches its select.
func dcovSend(ch chan os.Signal, sig os.Signal) bool {
	select {
	case ch <- sig:
		return true
	case <-time.After(dcovLimit):
		return false
	}
}

// dcovProjects writes a projects file and returns the loaded fleet plus the path.
func dcovProjects(t *testing.T, dir, body string) (*daemon.Config, daemonFlags) {
	t.Helper()
	path := filepath.Join(dir, daemon.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing projects file: %v", err)
	}
	cfg, err := daemon.Load(path)
	if err != nil {
		t.Fatalf("loading projects file: %v", err)
	}
	return cfg, daemonFlags{config: path}
}

// dcovRewrite replaces the projects file, guaranteeing the watcher sees a
// different size as well as a different mtime.
func dcovRewrite(t *testing.T, flags daemonFlags, body string) {
	t.Helper()
	if err := os.WriteFile(flags.config, []byte(body), 0o600); err != nil {
		t.Errorf("rewriting projects file: %v", err)
	}
}

// TestRunDaemonStopsOnTheFlagParsersVerdictBeforeTouchingTheProjectsFile:
// `--help` and a typo must answer for themselves. If runDaemon loaded the file
// first, `ssg daemon --help` on a machine with no .ssg_projects would print
// "reading .ssg_projects" instead of the usage the operator asked for.
func TestRunDaemonStopsOnTheFlagParsersVerdictBeforeTouchingTheProjectsFile(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.yml")

	out, err := captureStdout(func() error {
		if code := runDaemon([]string{"--config", absent, "--help"}); code != 0 {
			t.Errorf("--help = %d, want 0", code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ssg daemon") || strings.Contains(out, "absent.yml") {
		t.Errorf("--help must print the usage and never mention the missing file:\n%s", out)
	}

	var diag string
	if _, err = captureStdout(func() error {
		diag = captureStderr(t, func() {
			if code := runDaemon([]string{"--config", absent, "--nope"}); code != 2 {
				t.Errorf("unknown flag = %d, want 2", code)
			}
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag, "--nope") || strings.Contains(diag, "absent.yml") {
		t.Errorf("the typo must be the complaint, not the file: %q", diag)
	}
}

// TestRunDaemonStartsEveryActiveProjectAndStopsOnAQuitSignal: the daemon's whole
// promise — one process, the fleet up, and SIGTERM taking it all down with exit
// 0. A disabled project must stay out of the count, otherwise commenting a
// project out by `disabled: true` would still run it. Driven through runDaemon
// so the plain `ssg daemon` path really does reach the supervisor.
func TestRunDaemonStartsEveryActiveProjectAndStopsOnAQuitSignal(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, flags := dcovProjects(t, root,
		"projects:\n  - {name: blog, dir: blog}\n  - {name: off, dir: blog, disabled: true}\n")

	_, quit := dcovFakeSignals(t)
	_, settle := dcovStdoutTap(t)
	go func() { dcovSend(quit, syscall.SIGTERM) }()

	if code := runDaemon([]string{"--config", flags.config}); code != 0 {
		t.Errorf("a quit signal must end the daemon cleanly, got %d", code)
	}
	out := settle()
	if !strings.Contains(out, "Watching 1 project(s)") {
		t.Errorf("a disabled project must not be counted as watched:\n%s", out)
	}
	if !strings.Contains(out, "▶️  blog") {
		t.Errorf("the active project must be started:\n%s", out)
	}
	if strings.Contains(out, "▶️  off") {
		t.Errorf("a disabled project must not be started:\n%s", out)
	}
	if !strings.Contains(out, "Stopping every project") {
		t.Errorf("the shutdown must be announced:\n%s", out)
	}
}

// TestSuperviseProjectsWarnsAboutAProjectItCannotStartButKeepsWatching: three
// sites serving beats none. A project pointing at something that is not a
// directory must be named and skipped, not abort the daemon.
func TestSuperviseProjectsWarnsAboutAProjectItCannotStartButKeepsWatching(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "blog"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	_, quit := dcovFakeSignals(t)
	_, settle := dcovStdoutTap(t)
	diag := dcovStderrTap(t)
	go func() { dcovSend(quit, syscall.SIGTERM) }()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("a project that will not start must not fail the daemon, got %d", code)
	}
	if !strings.Contains(diag.String(), "blog") {
		t.Errorf("the project that would not start must be named: %q", diag.String())
	}
	if out := settle(); !strings.Contains(out, "Stopping every project") {
		t.Errorf("the daemon must still have reached its shutdown:\n%s", out)
	}
}

// TestSuperviseProjectsReloadsTheFleetOnSIGHUP: `kill -HUP` is the documented
// way to pick up an edited projects file. A project added to the file must join
// the fleet in place — without it, "reload on demand" is just a restart.
func TestSuperviseProjectsReloadsTheFleetOnSIGHUP(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"blog", "shop"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	reload, quit := dcovFakeSignals(t)
	tap, settle := dcovStdoutTap(t)
	go func() {
		dcovWaitFor(tap, "▶️  blog")
		dcovRewrite(t, flags, "projects:\n  - {name: blog, dir: blog}\n  - {name: shop, dir: shop}\n")
		if dcovSend(reload, syscall.SIGHUP) {
			dcovSend(quit, syscall.SIGTERM)
		}
	}()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("exit code = %d", code)
	}
	out := settle()
	if !strings.Contains(out, "Reloading (SIGHUP)") {
		t.Errorf("SIGHUP must be named as the reason for the reload:\n%s", out)
	}
	if !strings.Contains(out, "2 project(s) running: [blog shop]") {
		t.Errorf("the added project must have joined the fleet:\n%s", out)
	}
}

// TestSuperviseProjectsReportsAProjectAddedByAReloadThatWillNotStart: a reload
// reconciles the whole fleet, so one unusable entry in the edited file must be
// named and skipped — dropping the projects that were already serving over a
// mistyped directory would make every edit a gamble.
func TestSuperviseProjectsReportsAProjectAddedByAReloadThatWillNotStart(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shop"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	reload, quit := dcovFakeSignals(t)
	tap, settle := dcovStdoutTap(t)
	diag := dcovStderrTap(t)
	go func() {
		dcovWaitFor(tap, "▶️  blog")
		dcovRewrite(t, flags, "projects:\n  - {name: blog, dir: blog}\n  - {name: shop, dir: shop}\n")
		if dcovSend(reload, syscall.SIGHUP) {
			dcovSend(quit, syscall.SIGTERM)
		}
	}()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("exit code = %d", code)
	}
	if !strings.Contains(diag.String(), "starting shop") {
		t.Errorf("the project the reload could not start must be named: %q", diag.String())
	}
	if out := settle(); !strings.Contains(out, "1 project(s) running: [blog]") {
		t.Errorf("the project that was already serving must be left alone:\n%s", out)
	}
}

// TestSuperviseProjectsKeepsTheFleetWhenTheProjectsFileStopsParsing: the file is
// polled, so a half-saved or mistyped edit reaches the daemon on its own. It
// must keep what is running and say how to recover — stopping every site over a
// stray character would be the worse failure.
func TestSuperviseProjectsKeepsTheFleetWhenTheProjectsFileStopsParsing(t *testing.T) {
	root := t.TempDir()
	// A project that cannot start keeps the fleet still, so the only reload in
	// this test is the one the broken file provokes.
	if err := os.WriteFile(filepath.Join(root, "blog"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	_, quit := dcovFakeSignals(t)
	dcovStdoutTap(t)
	diag := dcovStderrTap(t)
	go func() {
		dcovWaitFor(diag, "blog")
		dcovRewrite(t, flags, "projects:\n  - {name: blog, dir: blog\n    broken: [\n")
		dcovWaitFor(diag, "Keeping the running projects")
		dcovSend(quit, syscall.SIGTERM)
	}()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("a broken projects file must not end the daemon, got %d", code)
	}
	if !strings.Contains(diag.String(), "Keeping the running projects — fix the file and save to retry.") {
		t.Errorf("the daemon must say what it kept and how to retry: %q", diag.String())
	}
}

// TestSuperviseProjectsRestartsAProjectThatExitedOnItsOwn: a build that gives up
// leaves a site unserved and nothing else notices. The poll must bring it back.
func TestSuperviseProjectsRestartsAProjectThatExitedOnItsOwn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	_, quit := dcovFakeSignals(t)
	tap, settle := dcovStdoutTap(t)
	go func() {
		dcovWaitFor(tap, "exited on its own")
		dcovSend(quit, syscall.SIGTERM)
	}()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("exit code = %d", code)
	}
	out := settle()
	if !strings.Contains(out, "♻️  blog exited on its own — restarting") {
		t.Errorf("the daemon must notice a project that died:\n%s", out)
	}
	if strings.Count(out, "▶️  blog") < 2 {
		t.Errorf("noticing is not enough — the project must actually be started again:\n%s", out)
	}
}

// TestSuperviseProjectsReportsAProjectItCannotRestart: when the restart itself
// fails the operator has to be told, otherwise a site quietly stays down while
// the daemon still looks healthy.
func TestSuperviseProjectsReportsAProjectItCannotRestart(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "blog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, flags := dcovProjects(t, root, "projects:\n  - {name: blog, dir: blog}\n")

	_, quit := dcovFakeSignals(t)
	tap, _ := dcovStdoutTap(t)
	diag := dcovStderrTap(t)
	go func() {
		dcovWaitFor(tap, "▶️  blog")
		// The project's directory disappears under it — a checkout deleted or a
		// mount that went away while the daemon was running.
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing the project dir: %v", err)
			dcovSend(quit, syscall.SIGTERM)
			return
		}
		if err := os.WriteFile(dir, []byte("gone\n"), 0o600); err != nil {
			t.Errorf("replacing the project dir: %v", err)
		}
		dcovWaitFor(diag, "starting blog")
		dcovSend(quit, syscall.SIGTERM)
	}()

	if code := superviseProjects(cfg, flags); code != 0 {
		t.Errorf("a failed restart must not end the daemon, got %d", code)
	}
	if !strings.Contains(diag.String(), "starting blog") || !strings.Contains(diag.String(), "not a directory") {
		t.Errorf("a restart that failed must be reported with its reason: %q", diag.String())
	}
}

// TestDaemonSignalsSubscribesToTheReloadSignalItDocuments: the usage promises
// "SIGHUP reloads on demand". Every other daemon test replaces this seam, so
// nothing else proves the real one ever calls signal.Notify — and a missing
// subscription would make `kill -HUP` a silent no-op.
func TestDaemonSignalsSubscribesToTheReloadSignalItDocuments(t *testing.T) {
	reload, quit := daemonSignals()
	t.Cleanup(func() { signal.Reset(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM) })

	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Skipf("cannot address this process: %v", err)
	}
	if err := self.Signal(syscall.SIGHUP); err != nil {
		t.Skipf("this platform cannot raise SIGHUP: %v", err)
	}
	select {
	case sig := <-reload:
		if sig != syscall.SIGHUP {
			t.Errorf("reload channel carried %v", sig)
		}
	case <-time.After(dcovLimit):
		t.Fatal("SIGHUP never reached the reload channel")
	}
	select {
	case sig := <-quit:
		t.Errorf("SIGHUP must not read as a stop request, got %v", sig)
	default:
	}
}
