package migrate

// What a failed engine run tells the operator (#289). Every failure used to end
// with "upgrade wpexporter", because the likeliest cause of an immediate
// failure was once an engine too old for --ssg-sections. Since the minimum
// version is enforced before the run, that is now the rarest cause: a DNS
// failure, a refused connection or an HTTP/2 reset ended the run with advice to
// upgrade an engine newer than required, and the real error sat a screen
// earlier, above cobra's usage dump.

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// engineOutputLimit caps what a run keeps of the engine's output. The reason
// for a failure is in the last lines; a full export log can run to megabytes.
const engineOutputLimit = 64 << 10

// tailBuffer keeps the last engineOutputLimit bytes written to it. The engine
// writes stdout and stderr from separate goroutines, so writes are serialised.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - engineOutputLimit; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// engineRunError is a failed run together with what the engine printed.
type engineRunError struct {
	err    error
	output string
}

func (e *engineRunError) Error() string { return e.err.Error() }
func (e *engineRunError) Unwrap() error { return e.err }

// engineOutput returns what a failed run printed, or "" when the runner did
// not capture it (an injected Run, or a failure before the engine started).
func engineOutput(err error) string {
	var runErr *engineRunError
	if errors.As(err, &runErr) {
		return runErr.output
	}
	return ""
}

// engineReason is the engine's own statement of what went wrong: the first
// "Error:" line cobra prints, without the prefix. Empty when there is none.
func engineReason(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if reason, ok := strings.CutPrefix(strings.TrimSpace(line), "Error:"); ok {
			if reason = strings.TrimSpace(reason); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// namesUnknownFlag reports whether a failure is the one an old engine causes:
// cobra rejecting a flag it does not know ("unknown flag", "unknown shorthand
// flag"). Only then is upgrading the engine the answer.
func namesUnknownFlag(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "unknown flag") || strings.Contains(text, "unknown shorthand flag")
}

// engineFailure builds the error a failed run ends with: the exit status, then
// the engine's own reason as the last line, so a tool that reports only the
// final line of a run reports the cause. The upgrade hint is added only when
// the failure names an unknown flag, the one failure an upgrade fixes.
func engineFailure(runErr error, engine string) error {
	output := engineOutput(runErr)
	var b strings.Builder
	if reason := engineReason(output); reason != "" {
		fmt.Fprintf(&b, "\n   %s", reason)
	}
	if namesUnknownFlag(output) || namesUnknownFlag(runErr.Error()) {
		fmt.Fprintf(&b, "\n   The engine reported %s; ssg migrate is built against %s or newer.\n"+
			"   It rejected a flag, so it is older than it reports or not wpexporter; upgrade it:\n"+
			"   snap refresh static-site-generator, or go install .../cmd/wpexporter@latest",
			engine, engineVersionString(minimumEngine))
	}
	return fmt.Errorf("wpexporter failed: %w%s", runErr, b.String())
}
