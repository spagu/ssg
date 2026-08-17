package daemon

// Running a project as a child process.
//
// Each project is a plain `ssg --watch` in its own directory. That is a
// deliberate choice over running four builds in one process: the single-project
// code paths keep their global state and stay untouched, a project that panics
// takes only itself down, and a port that a watch loop is holding is released by
// the operating system rather than by hoping a goroutine unwound.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// ExecRunner starts each project by re-invoking the ssg binary.
type ExecRunner struct {
	// Binary is the ssg executable to run. Empty means this one.
	Binary string
	// Out receives each project's own output, prefixed with its name so four
	// interleaved builds stay readable.
	Out io.Writer
}

// Start launches one project's watch loop.
func (r ExecRunner) Start(p Project) (Process, error) {
	bin := r.Binary
	if bin == "" {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locating the ssg binary: %w", err)
		}
		bin = self
	}
	if info, err := os.Stat(p.Dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", p.Dir)
	}

	// #nosec G204 -- bin is this executable and the arguments are built from the
	// operator's own projects file; nothing passes through a shell.
	cmd := exec.Command(bin, p.Command()...)
	cmd.Dir = p.Dir
	// Its own process group where the platform has them, so stopping one project
	// cannot signal the daemon or its siblings.
	isolateProcess(cmd)

	out := r.Out
	if out == nil {
		out = io.Discard
	}
	prefixed := &prefixWriter{prefix: "[" + p.Name + "] ", w: out}
	cmd.Stdout, cmd.Stderr = prefixed, prefixed

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	proc := &execProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(proc.done)
	}()
	return proc, nil
}

// execProcess is one running project.
type execProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *execProcess) Done() <-chan struct{} { return p.done }

// Stop asks the project to leave, then insists. Where the platform has process
// groups the whole group is signalled: a watch loop with `watch_runner` has a
// child of its own, and signalling only the parent leaves that one holding the
// port.
func (p *execProcess) Stop(grace time.Duration) error {
	if p.cmd.Process == nil {
		return nil
	}
	terminateProcess(p.cmd)

	select {
	case <-p.done:
		return nil
	case <-time.After(grace):
	}
	if err := killProcess(p.cmd); err != nil {
		return fmt.Errorf("killing pid %d: %w", p.cmd.Process.Pid, err)
	}
	<-p.done
	return nil
}

// prefixWriter tags each line with the project it came from, so one terminal
// carrying four builds stays readable.
type prefixWriter struct {
	prefix  string
	w       io.Writer
	pending bool // mid-line: the last write did not end with a newline
}

func (w *prefixWriter) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 {
		if !w.pending {
			if _, err := io.WriteString(w.w, w.prefix); err != nil {
				return 0, err
			}
			w.pending = true
		}
		i := indexByte(b, '\n')
		if i < 0 {
			if _, err := w.w.Write(b); err != nil {
				return 0, err
			}
			break
		}
		if _, err := w.w.Write(b[:i+1]); err != nil {
			return 0, err
		}
		w.pending = false
		b = b[i+1:]
	}
	return n, nil
}

// indexByte is bytes.IndexByte, kept local so this file imports nothing for one
// call.
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
