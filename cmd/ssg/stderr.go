package main

// One diagnostic sink for the whole command (#188).
//
// Every warning and error here used to reach `fmt.Fprintf(os.Stderr, …)`
// directly, and the tests that assert on those messages captured them by
// assigning `os.Stderr` — a package variable of the standard library, written
// by one goroutine while background goroutines this command leaves running (the
// HTTP/3 listener, the watch loop, a server reporting a dead listener) read it
// to write their own diagnostics. That is a data race, and `-race` caught it
// intermittently: roughly one CI run in ten, always in whichever test happened
// to be running when an earlier test's goroutine finally spoke.
//
// The fix is the seam that `serverErrf` already established for server errors,
// applied to the command as a whole: writers go through a sink read under a
// lock, and a test swaps the sink instead of the standard library's variable.
// `os.Stderr` is then written by nobody and the race has no subject.
//
// Only diagnostics move. `cmd.Stderr = os.Stderr` still hands a child process
// the real file, which is what a child needs.

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	stderrMu   sync.RWMutex
	stderrSink io.Writer = os.Stderr
)

// stderrWriter returns the sink in force right now.
func stderrWriter() io.Writer {
	stderrMu.RLock()
	defer stderrMu.RUnlock()
	return stderrSink
}

// setStderrSink redirects diagnostics and returns the previous sink, so a caller
// can restore it. Safe to call while background goroutines are writing.
func setStderrSink(w io.Writer) io.Writer {
	stderrMu.Lock()
	defer stderrMu.Unlock()
	previous := stderrSink
	stderrSink = w
	return previous
}

// errf writes one formatted diagnostic. The drop-in for fmt.Fprintf(os.Stderr…).
func errf(format string, a ...any) {
	_, _ = fmt.Fprintf(stderrWriter(), format, a...)
}

// errln writes one diagnostic followed by a newline.
func errln(a ...any) {
	_, _ = fmt.Fprintln(stderrWriter(), a...)
}
