package daemon

// Running and stopping a real child process (#169). These tests spawn actual
// processes — the point of the code is what happens to one — but only /bin/sh
// and /bin/sleep, never a build.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is an io.Writer a test can read while a child process is writing.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// shellProject writes a script and returns a project that runs it.
func shellProject(t *testing.T, name, script string) (Project, ExecRunner, *syncBuffer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-ssg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out := &syncBuffer{}
	return Project{Name: name, Dir: dir}, ExecRunner{Binary: path, Out: out}, out
}

// TestStartRunsInTheProjectDirectory: a project builds in its own directory,
// which is the whole reason the daemon can run four of them.
func TestStartRunsInTheProjectDirectory(t *testing.T) {
	p, r, out := shellProject(t, "blog", `pwd; echo "args: $*"`)
	proc, err := r.Start(p)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the process never finished")
	}

	got := out.String()
	// Its output is tagged, so one terminal carrying four builds stays readable.
	if !strings.Contains(got, "[blog] ") {
		t.Errorf("output must be prefixed with the project name:\n%s", got)
	}
	// macOS resolves /var through a symlink, so compare the resolved forms.
	want, _ := filepath.EvalSymlinks(p.Dir)
	if resolved, _ := filepath.EvalSymlinks(firstPath(got)); resolved != want {
		t.Errorf("ran in %q, want the project dir %q", resolved, want)
	}
	if !strings.Contains(got, "--watch") {
		t.Errorf("the project must be started as an ordinary watch:\n%s", got)
	}
}

// firstPath pulls the first absolute path out of prefixed output.
func firstPath(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "/"); i >= 0 {
			return strings.TrimSpace(line[i:])
		}
	}
	return ""
}

// TestStopEndsALongRunningProject: a watch loop never exits on its own, so
// stopping it is the only way a reload can move it to another port.
func TestStopEndsALongRunningProject(t *testing.T) {
	p, r, _ := shellProject(t, "shop", "sleep 120")
	proc, err := r.Start(p)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := proc.Stop(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Stop returned while the process was still running")
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("stopping took %v — the grace period should not be waited out for a process that leaves", elapsed)
	}
	// Stopping twice is a no-op, not a panic: shutdown may race a reload.
	if err := proc.Stop(time.Second); err != nil {
		t.Errorf("second Stop = %v", err)
	}
}

// TestStopKillsWhatWillNotLeave: a project ignoring SIGTERM still has to
// release its port, or the next start finds it busy.
func TestStopKillsWhatWillNotLeave(t *testing.T) {
	// The trap has to survive the signal, so the shell itself must stay alive:
	// `sleep 120` alone would be killed as part of the group and the shell would
	// exit with it, never reaching the kill path this test is for.
	p, r, out := shellProject(t, "stubborn", "trap \"\" TERM\necho ready\nwhile :; do sleep 0.2; done")
	proc, err := r.Start(p)
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the trap to be installed: signalling a shell that is still
	// starting up kills it, and the test would pass without ever reaching the
	// path it exists for.
	waitFor(t, func() bool { return strings.Contains(out.String(), "ready") })

	if err := proc.Stop(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a process that ignores SIGTERM must still be ended")
	}
}

// waitFor blocks until cond holds, or fails the test.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the process to be ready")
}

// TestStartRefusesAMissingDirectory: naming a project that is not there is a
// mistake worth reporting rather than a process that fails obscurely.
func TestStartRefusesAMissingDirectory(t *testing.T) {
	r := ExecRunner{Binary: "/bin/sh"}
	_, err := r.Start(Project{Name: "gone", Dir: filepath.Join(t.TempDir(), "nowhere")})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("err = %v", err)
	}
	// A file is not a project directory either.
	file := filepath.Join(t.TempDir(), "f")
	if writeErr := os.WriteFile(file, []byte("x"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if _, err := r.Start(Project{Name: "f", Dir: file}); err == nil {
		t.Error("a file must not pass as a project directory")
	}
}

// TestStartReportsAMissingBinary.
func TestStartReportsAMissingBinary(t *testing.T) {
	r := ExecRunner{Binary: filepath.Join(t.TempDir(), "no-such-ssg")}
	if _, err := r.Start(Project{Name: "x", Dir: t.TempDir()}); err == nil {
		t.Fatal("a binary that is not there must be an error")
	}
}

// TestPrefixWriterTagsLines: four interleaved builds in one terminal are only
// readable if every line says which project it came from — including a line
// delivered in pieces, which is how a progress line arrives.
func TestPrefixWriterTagsLines(t *testing.T) {
	var out strings.Builder
	w := &prefixWriter{prefix: "[a] ", w: &out}

	if _, err := w.Write([]byte("one\ntwo\n")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[a] one\n[a] two\n" {
		t.Errorf("whole lines = %q", got)
	}

	// A line split across writes is prefixed once, not per fragment.
	out.Reset()
	w = &prefixWriter{prefix: "[b] ", w: &out}
	for _, chunk := range []string{"par", "tial", " line\n", "next\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if got := out.String(); got != "[b] partial line\n[b] next\n" {
		t.Errorf("split line = %q", got)
	}

	// An empty write says nothing at all.
	out.Reset()
	w = &prefixWriter{prefix: "[c] ", w: &out}
	if n, err := w.Write(nil); n != 0 || err != nil {
		t.Fatalf("empty write = %d, %v", n, err)
	}
	if out.String() != "" {
		t.Errorf("an empty write must produce nothing, got %q", out.String())
	}
}

// TestIndexByte covers the small helper the writer leans on.
func TestIndexByte(t *testing.T) {
	if got := indexByte([]byte("abc\ndef"), '\n'); got != 3 {
		t.Errorf("indexByte = %d, want 3", got)
	}
	if got := indexByte([]byte("abc"), '\n'); got != -1 {
		t.Errorf("indexByte with no match = %d, want -1", got)
	}
	if got := indexByte(nil, '\n'); got != -1 {
		t.Errorf("indexByte(nil) = %d, want -1", got)
	}
}
