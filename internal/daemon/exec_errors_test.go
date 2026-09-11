package daemon

// What happens when running a project goes wrong (#169): the binary the daemon
// re-invokes, a handle whose process never started, a kill that cannot be
// delivered, and a terminal that goes away mid-line. Same rule as the happy
// paths — real processes, but only /bin/sh and this test binary.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStartWithoutABinaryReinvokesThisExecutable: an empty Binary means "the
// ssg that is already running", not "whatever ssg is on PATH" — a stale copy
// earlier in PATH would build the fleet with a different version than the
// daemon the operator started.
func TestStartWithoutABinaryReinvokesThisExecutable(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("this platform cannot name its own executable: %v", err)
	}
	// This test binary stands in for ssg: it does not understand --watch, so it
	// says so and exits at once instead of watching anything.
	out := &syncBuffer{}
	proc, err := ExecRunner{Out: out}.Start(Project{Name: "self", Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(30 * time.Second):
		_ = proc.Stop(time.Second)
		t.Fatal("the re-invoked executable never finished")
	}

	got := out.String()
	if !strings.Contains(got, "[self] ") {
		t.Errorf("output must be prefixed with the project name:\n%s", got)
	}
	// The child names itself when it rejects the flag, which is how this test
	// knows Start reached for this executable rather than some other ssg. Base
	// names only: /proc/self/exe is symlink-resolved, argv[0] is not.
	if !strings.Contains(got, filepath.Base(self)) {
		t.Errorf("Start must re-invoke %s, output was:\n%s", filepath.Base(self), got)
	}
	if !strings.Contains(got, "watch") {
		t.Errorf("the project must still be started as an ordinary watch:\n%s", got)
	}
}

// TestStopIsANoOpForAProcessThatNeverStarted: shutdown walks every handle it
// holds, and one whose process is not there must be a quiet no-op rather than a
// nil dereference that takes the whole fleet down on the way out.
func TestStopIsANoOpForAProcessThatNeverStarted(t *testing.T) {
	// #nosec G204 -- a fixed name, never started; only cmd.Process is read.
	proc := &execProcess{cmd: exec.Command("does-not-matter"), done: make(chan struct{})}

	returned := make(chan error, 1)
	go func() { returned <- proc.Stop(time.Hour) }()
	select {
	case err := <-returned:
		if err != nil {
			t.Errorf("Stop of an unstarted process = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop waited out the grace period for a process that does not exist")
	}
}

// TestStopReportsAKillItCouldNotDeliver: Stop waits for the process to be
// reaped after killing it, so a kill that never landed must come back as an
// error naming the pid — returning nothing would leave the daemon blocked
// forever on a project that can no longer exit.
func TestStopReportsAKillItCouldNotDeliver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this fabricates a POSIX process group that cannot exist")
	}
	// Far above any pid_max, so both signals are certain to find nothing and
	// no real process on the machine can be hit.
	const absentPid = 1 << 30
	proc := &execProcess{
		cmd:  &exec.Cmd{Process: &os.Process{Pid: absentPid}},
		done: make(chan struct{}), // never closed: nothing is there to be reaped
	}

	returned := make(chan error, 1)
	go func() { returned <- proc.Stop(time.Millisecond) }()
	select {
	case err := <-returned:
		if err == nil {
			t.Fatal("a kill that was not delivered must be reported")
		}
		if !strings.Contains(err.Error(), "1073741824") {
			t.Errorf("err = %v, want the pid named so the operator can look for it", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stop blocked waiting for a process that will never be reaped")
	}
}

// errTerminalGone is what a writer returns once nobody is reading it.
var errTerminalGone = errors.New("terminal went away")

// failingWriter fails from its failAt'th write onwards, standing in for a
// terminal or pipe that closed while a build was talking to it.
type failingWriter struct {
	failAt int // 1 = the very first write fails
	writes int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errTerminalGone
	}
	return len(p), nil
}

// TestPrefixWriterReportsAWriteThatFailed: the prefix, a whole line and the
// tail of a line are three separate writes, and a failure in any of them has to
// reach the caller — a daemon that swallowed a broken pipe would go on building
// into nothing and report success.
func TestPrefixWriterReportsAWriteThatFailed(t *testing.T) {
	cases := []struct {
		what   string
		failAt int
		input  string
	}{
		{"the prefix", 1, "one\n"},
		{"a complete line", 2, "one\n"},
		{"the tail of an unfinished line", 2, "progress: 40%"},
	}
	for _, c := range cases {
		w := &prefixWriter{prefix: "[a] ", w: &failingWriter{failAt: c.failAt}}
		n, err := w.Write([]byte(c.input))
		if !errors.Is(err, errTerminalGone) {
			t.Errorf("%s: err = %v, want the writer's own error", c.what, err)
		}
		if n != 0 {
			t.Errorf("%s: n = %d, want 0 — a failed write must not be reported as bytes delivered", c.what, n)
		}
	}
}
