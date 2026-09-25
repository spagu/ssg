package migrate

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// dnsFailure is what wpexporter printed for a domain with no WordPress behind
// it: the reason, then cobra's usage dump (#289).
const dnsFailure = `Error: failed to get site info: Get "https://example.com/wp-json": dial tcp: lookup example.com on 127.0.0.11:53: no such host
Usage:
  wpexporter export [flags]

Flags:
  -h, --help   help for export
`

// TestEngineFailureNamesTheRealReason: a failure that is not an old engine
// ends with the engine's own reason and no advice to upgrade it (#289).
func TestEngineFailureNamesTheRealReason(t *testing.T) {
	err := engineFailure(&engineRunError{err: errors.New("exit status 1"), output: dnsFailure},
		"wpexporter 1.8.19 (/tools/wpexporter)")
	msg := err.Error()
	lines := strings.Split(msg, "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); !strings.HasSuffix(last, "no such host") {
		t.Fatalf("the last line must be the engine's reason, got %q", last)
	}
	if strings.Contains(msg, "upgrade") {
		t.Fatalf("a DNS failure must not advise an upgrade:\n%s", msg)
	}
	if !strings.HasPrefix(msg, "wpexporter failed: exit status 1") {
		t.Fatalf("the exit status must stay, got %q", msg)
	}
}

// TestEngineFailureUpgradeHintOnlyForUnknownFlags: an engine that rejects a
// flag is the one case where upgrading is the answer.
func TestEngineFailureUpgradeHintOnlyForUnknownFlags(t *testing.T) {
	for _, output := range []string{
		"Error: unknown flag: --ssg-sections\n",
		"Error: unknown shorthand flag: 'q' in -q\n",
	} {
		msg := engineFailure(&engineRunError{err: errors.New("exit status 1"), output: output},
			"wpexporter 1.8.19 (/tools/wpexporter)").Error()
		if !strings.Contains(msg, "go install .../cmd/wpexporter@latest") || !strings.Contains(msg, "1.8.19") {
			t.Errorf("an unknown flag must earn the upgrade hint:\n%s", msg)
		}
	}
	// An injected runner captures nothing; its error text is all there is.
	msg := engineFailure(errors.New("unknown flag: --x"), "wpexporter").Error()
	if !strings.Contains(msg, "upgrade it") {
		t.Errorf("an unknown flag in the error itself must earn the hint:\n%s", msg)
	}
}

// TestEngineFailureWithoutOutput: a bare error stays a bare error, wrapped.
func TestEngineFailureWithoutOutput(t *testing.T) {
	boom := errors.New("boom")
	err := engineFailure(boom, "wpexporter")
	if err.Error() != "wpexporter failed: boom" {
		t.Fatalf("got %q", err.Error())
	}
	if !errors.Is(err, boom) {
		t.Fatal("the run error must stay in the chain")
	}
}

func TestEngineReason(t *testing.T) {
	for output, want := range map[string]string{
		dnsFailure:                  `failed to get site info: Get "https://example.com/wp-json": dial tcp: lookup example.com on 127.0.0.11:53: no such host`,
		"progress\n  Error:   x \n": "x",
		"Error:\nError: second\n":   "second",
		"no error line\n":           "",
		"":                          "",
	} {
		if got := engineReason(output); got != want {
			t.Errorf("engineReason(%q) = %q, want %q", output, got, want)
		}
	}
}

// TestTailBufferKeepsTheEnd: only the last engineOutputLimit bytes are kept,
// because the reason is at the end and an export log can be megabytes.
func TestTailBufferKeepsTheEnd(t *testing.T) {
	var tail tailBuffer
	if _, err := tail.Write([]byte(strings.Repeat("a", engineOutputLimit))); err != nil {
		t.Fatal(err)
	}
	n, err := tail.Write([]byte("Error: last"))
	if err != nil || n != len("Error: last") {
		t.Fatalf("Write = %d, %v", n, err)
	}
	got := tail.String()
	if len(got) != engineOutputLimit || !strings.HasSuffix(got, "Error: last") {
		t.Fatalf("kept %d bytes ending %q", len(got), got[len(got)-12:])
	}
}

// TestRunStreamingCapturesOutput: the default runner keeps what the engine
// printed, quiet or not, so the failure can quote it.
func TestRunStreamingCapturesOutput(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh in PATH: %v", err)
	}
	for _, quiet := range []bool{true, false} {
		err := runStreaming(sh, []string{"-c", "echo 'Error: no such host' >&2; exit 1"}, quiet)
		if got := engineReason(engineOutput(err)); got != "no such host" {
			t.Errorf("quiet=%v: captured reason %q, want %q", quiet, got, "no such host")
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("quiet=%v: the exit error must stay in the chain, got %T", quiet, err)
		}
	}
	if engineOutput(errors.New("plain")) != "" {
		t.Error("an uncaptured error has no output")
	}
}
