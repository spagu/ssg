package main

// The last paths through main.go that no other test walked: the ones that end
// the process (main itself, a refused --deploy provider, a signal reaching a
// watch runner), the ones that hand the process to a loop for good (--watch,
// --http, an MDDB watch), and the ones that depend on something outside the
// build going wrong at an awkward moment (an encoder that vanishes mid-pass, an
// archive deleted before its size is reported).
//
// Process-ending paths re-exec the test binary, the pattern main_test.go uses
// for --version and --help: the child runs the code, the parent judges how it
// ended.

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// cov100ChildEnv names the variable that tells a re-executed test binary it is
// the child half of a process-ending test.
const cov100ChildEnv = "SSG_COV100_CHILD"

// cov100IsChild reports whether this process is the child half of testName.
func cov100IsChild(testName string) bool {
	return os.Getenv(cov100ChildEnv) == testName
}

// cov100RunChild re-executes the test binary for exactly testName and returns
// what it printed and its exit code.
func cov100RunChild(t *testing.T, testName string) (string, int) {
	t.Helper()
	// #nosec G204 -- re-executing this very test binary is the point
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), cov100ChildEnv+"="+testName)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exitErr):
		return string(out), exitErr.ExitCode()
	default:
		t.Fatalf("re-exec %s: %v", testName, err)
		return "", -1
	}
}

// cov100ExitWhenServing ends a child process once its preview server answers
// on port, and a moment more so the watch loop is entered. It waits on the
// server, not on a clock: under -race the first build alone can take seconds.
func cov100ExitWhenServing(port int, marker string) {
	go func() {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if conn, err := net.Dial("tcp", addr); err == nil {
				_ = conn.Close()
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
		_, _ = os.Stdout.WriteString(marker + "\n")
		os.Exit(0)
	}()
}

// cov100FreePort returns a loopback port nobody is listening on right now.
func cov100FreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// TestMainExitsWithTheCodeRunReturns: main is run plus os.Exit, and the exit
// code is the one run decided on — a bare `ssg migrate` is a usage error.
func TestMainExitsWithTheCodeRunReturns(t *testing.T) {
	if cov100IsChild(t.Name()) {
		t.Chdir(t.TempDir())
		os.Args = []string{"ssg", "migrate"}
		main()
		return
	}
	out, code := cov100RunChild(t, t.Name())
	if code != 2 {
		t.Errorf("exit code = %d, want 2; output:\n%s", code, out)
	}
}

// TestRunHandsAWatchedPreviewToItsLoop: --watch with --http builds once, starts
// the server with live reload, and then never returns — the watch loop owns the
// process until something ends it.
func TestRunHandsAWatchedPreviewToItsLoop(t *testing.T) {
	if cov100IsChild(t.Name()) {
		port := cov100FreePort(t)
		args := append(rcovProject(t), "--watch", "--http",
			"--port="+strconv.Itoa(port), "--host=127.0.0.1")
		cov100ExitWhenServing(port, "cov100: still watching")
		code, done := run(args)
		_, _ = os.Stdout.WriteString("cov100: run returned " + strconv.Itoa(code) + " " + strconv.FormatBool(done) + "\n")
		os.Exit(3)
	}
	out, code := cov100RunChild(t, t.Name())
	if code != 0 || !strings.Contains(out, "cov100: still watching") {
		t.Errorf("run must keep the process for its loop; exit %d, output:\n%s", code, out)
	}
	if strings.Contains(out, "cov100: run returned") {
		t.Errorf("run returned while a watcher should own the process:\n%s", out)
	}
}

// TestRunWatchOrServeHandsMddbWatchItsOwnLoop: an MDDB watch takes precedence
// over the file watcher. A client that cannot be built ends the loop at once,
// which is what lets this run in-process.
func TestRunWatchOrServeHandsMddbWatchItsOwnLoop(t *testing.T) {
	cfg := &config.Config{Watch: true}
	cfg.Mddb.Enabled = true
	cfg.Mddb.Watch = true
	cfg.Mddb.Protocol = "grpc"
	cfg.Mddb.URL = "grpc://mddb.example.com:9000"
	cfg.Mddb.APIKey = "secret"
	out := captureStderr(t, func() { runWatchOrServe(generator.Config{}, cfg) })
	if !strings.Contains(out, "Error creating MDDB client") {
		t.Errorf("the MDDB loop was not the one started; stderr:\n%s", out)
	}
}

// TestMddbWatchAnnouncesAChangedChecksum: outside --quiet the operator is told
// which checksum moved and how many documents the collection now holds.
func TestMddbWatchAnnouncesAChangedChecksum(t *testing.T) {
	srv, _ := wcovChecksumServer(t, []string{"first", "second"}, 200)
	genCfg, cfg, _ := wcovSite(t, srv.URL)
	cfg.Quiet = false

	buf, settle := dcovStdoutTap(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runMddbWatchLoop(genCfg, cfg, stop)
		close(done)
	}()
	deadline := time.After(15 * time.Second)
	for !strings.Contains(buf.String(), "MDDB content changed") {
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatalf("no change announced; stdout:\n%s", settle())
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(stop)
	<-done
	if out := settle(); !strings.Contains(out, "first → second (3 docs)") {
		t.Errorf("announcement lacks the checksums and count:\n%s", out)
	}
}

// TestSetupTemplateEngineRejectsAnUnknownDeployProvider: a typo in --deploy
// must stop the run before anything is built, naming what is supported.
func TestSetupTemplateEngineRejectsAnUnknownDeployProvider(t *testing.T) {
	if cov100IsChild(t.Name()) {
		setupTemplateEngine(&config.Config{Deploy: "carrier-pigeon", Quiet: true})
		return
	}
	out, code := cov100RunChild(t, t.Name())
	if code != 1 || !strings.Contains(out, `unknown --deploy provider "carrier-pigeon"`) {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
}

// TestBuildStopsWhenTheImagesPhaseFails: --webp without the encoder fails the
// whole build, not just a log line, so a site referencing missing .webp files
// is never reported as built.
func TestBuildStopsWhenTheImagesPhaseFails(t *testing.T) {
	genCfg, cfg, _ := wcovSite(t, "")
	cfg.Mddb = config.MddbConfig{}
	cfg.WebP = true
	icovNoTools(t)
	err := build(genCfg, cfg)
	if err == nil || !strings.Contains(err.Error(), "converting to WebP") {
		t.Fatalf("build = %v, want the images phase's error", err)
	}
}

// TestRunWebPReportsWhatItConverted: with an encoder present and no AVIF asked
// for, the pass converts, says how many images it did, and succeeds.
func TestRunWebPReportsWhatItConverted(t *testing.T) {
	out := t.TempDir()
	icovPNG(t, filepath.Join(out, "hero.png"), 24, 16)
	icovWrite(t, filepath.Join(out, "index.html"), `<img src="hero.png">`, 0o644)
	icovStubTools(t, "cwebp")
	cfg := &config.Config{WebP: true, OutputDir: out}
	var err error
	printed := icovCaptureStdout(t, func() {
		err = runWebP(cfg, generator.NewProfile(generator.ProfileText))
	})
	if err != nil {
		t.Fatalf("runWebP: %v", err)
	}
	if !strings.Contains(printed, "Converted 1 images") {
		t.Errorf("no conversion summary, got %q", printed)
	}
}

// TestRunDeployAnnouncesWhereTheSiteWent: a provider that reports a URL gets
// it printed, so the operator does not have to work out where it went.
func TestRunDeployAnnouncesWhereTheSiteWent(t *testing.T) {
	icovStubTools(t, "git")
	out := t.TempDir()
	icovWrite(t, filepath.Join(out, "index.html"), "<html></html>", 0o644)
	cfg := &config.Config{
		Deploy: "github-pages", OutputDir: out,
		DeployTarget: "https://github.com/example/site.git",
	}
	var err error
	printed := icovCaptureStdout(t, func() { err = runDeploy(cfg) })
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if !strings.Contains(printed, "Deployed to https://example.github.io/site/") {
		t.Errorf("deploy URL not announced, got %q", printed)
	}
}

// TestCreateZipArchiveSaysNothingAboutAnArchiveThatVanished: the size report
// stats the archive after writing it. If something removed it in between,
// there is no size to report, and inventing one would be worse than silence.
//
// A FIFO in the output tree pauses the archiver mid-walk — after the archive
// file exists, before its size is read — which is where it is deleted.
func TestCreateZipArchiveSaysNothingAboutAnArchiveThatVanished(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	out := filepath.Join(dir, "output")
	icovWrite(t, filepath.Join(out, "index.html"), "<html></html>", 0o644)
	fifo := filepath.Join(out, "pause.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no FIFOs here: %v", err)
	}
	zipName := "example.com.zip"

	released := make(chan error, 1)
	go func() {
		// Opening for write blocks until the archiver opens the FIFO to read it.
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0) // #nosec G304 -- test fixture
		if err != nil {
			released <- err
			return
		}
		removeErr := os.Remove(zipName)
		_ = w.Close()
		released <- removeErr
	}()

	cfg := &config.Config{Zip: true, Domain: "example.com", OutputDir: out}
	var err error
	printed := icovCaptureStdout(t, func() { err = createZipArchive(cfg) })
	if err != nil {
		t.Fatalf("createZipArchive: %v", err)
	}
	if removeErr := <-released; removeErr != nil {
		t.Fatalf("removing the archive mid-write: %v", removeErr)
	}
	if strings.Contains(printed, "Created deployment package") {
		t.Errorf("reported an archive that no longer exists: %q", printed)
	}
}

// TestWatchRunnerIsKilledWhenTheWatcherIsSignalled: Ctrl-C on `ssg --watch`
// must not leave the runner it started (wrangler, workerd) holding its port.
func TestWatchRunnerIsKilledWhenTheWatcherIsSignalled(t *testing.T) {
	if cov100IsChild(t.Name()) {
		cmd := startWatchRunner(watchRunnerSpec{Runner: "sleep 30", Quiet: true})
		if cmd == nil {
			_, _ = os.Stdout.WriteString("cov100: runner did not start\n")
			os.Exit(4)
		}
		_, _ = os.Stdout.WriteString("cov100: runner pid " + strconv.Itoa(cmd.Process.Pid) + "\n")
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			os.Exit(5)
		}
		time.Sleep(10 * time.Second)
		os.Exit(6) // the signal handler should have exited first
	}
	out, code := cov100RunChild(t, t.Name())
	if code != 0 {
		t.Fatalf("exit %d, want 0 from the signal handler; output:\n%s", code, out)
	}
	pidText := ""
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "cov100: runner pid "); ok {
			pidText = strings.TrimSpace(rest)
		}
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatalf("no runner pid in output:\n%s", out)
	}
	// The runner was killed; once reaped by init it no longer exists. Give
	// the reaper a moment.
	for range 50 {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("runner %d outlived the watcher", pid)
}

// cov100FailingCloser is a file whose Close reports a deferred write error,
// the way NFS or an exhausted quota does.
type cov100FailingCloser struct{}

func (cov100FailingCloser) Close() error { return errors.New("disk quota exceeded") }

// TestCloseZipFileReportsAFailedClose: Close is where a deferred write error
// surfaces, and an archive whose close failed must not count as built.
func TestCloseZipFileReportsAFailedClose(t *testing.T) {
	err := closeZipFile(cov100FailingCloser{})
	if err == nil || !strings.Contains(err.Error(), "closing zip file: disk quota exceeded") {
		t.Errorf("closeZipFile = %v", err)
	}
}
