package execx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunPipesStdin(t *testing.T) {
	out, err := ExecRunner{}.Run(context.Background(), "cat", nil, []byte("hello"))
	if err != nil || string(out) != "hello" {
		t.Fatal(err, string(out))
	}
}

func TestRunFailureKeepsStderrTail(t *testing.T) {
	_, err := ExecRunner{}.Run(context.Background(), "sh", []string{"-c", "echo line1 >&2; echo line2 >&2; exit 3"}, nil)
	if err == nil || !strings.Contains(err.Error(), "line1 | line2") || !strings.HasPrefix(err.Error(), "sh:") {
		t.Fatal(err)
	}
}

func TestRunTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ExecRunner{}.Run(ctx, "sleep", []string{"5"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestTailAndAvailable(t *testing.T) {
	if got := tail(strings.Repeat("x", stderrLimit+10)); len(got) != stderrLimit {
		t.Fatal(len(got))
	}
	if !Available("sh") || Available("no-such-binary-here") {
		t.Fatal("Available")
	}
}
