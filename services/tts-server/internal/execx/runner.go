// Package execx runs external programs (espeak-ng, piper, lame) safely:
// argv only, never a shell, input on stdin, bounded by a context deadline.
package execx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner executes a program with the given arguments, feeding stdin and
// returning stdout. Engines and the encoder depend on this interface so tests
// can substitute a fake.
type Runner interface {
	Run(ctx context.Context, bin string, args []string, stdin []byte) ([]byte, error)
}

// stderrLimit caps how much of a failing program's stderr is kept for the
// error message; the message is logged, never sent to clients.
const stderrLimit = 2048

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct{}

// Run starts bin with args, writes stdin to it and waits for it to finish.
// The process is killed when ctx is done.
func (ExecRunner) Run(ctx context.Context, bin string, args []string, stdin []byte) ([]byte, error) {
	// #nosec G204 -- bin comes from operator configuration and args are built
	// by the engines from validated values; no shell is involved and request
	// text is passed on stdin, never as an argument.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(bin), err, tail(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Available reports whether bin can be found (as a path or on $PATH).
func Available(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// tail keeps the last stderrLimit bytes of s on a single line.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > stderrLimit {
		s = s[len(s)-stderrLimit:]
	}
	return strings.ReplaceAll(s, "\n", " | ")
}
