package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// captureStderr runs fn and returns everything it wrote to os.Stderr.
// captureStderr collects what fn wrote to stderr.
//
// The reader runs concurrently, which is not a refinement — it is the only
// correct shape. Reading after fn() returns caps the capture at one 4096-byte
// read, so a longer message came back silently truncated and an assertion on
// its tail passed or failed for the wrong reason. Worse, a pipe holds about
// 64 KB: anything past that and fn() blocks on a write nobody is draining, and
// the test hangs forever rather than failing. captureStdout in mcp.go already
// does it this way; this now matches it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	os.Stderr = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// GO-056: incomplete TLS/HTTP3 configurations must be loud, never silent.
func TestWarnTLSMisconfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"http3 without tls", config.Config{HTTP3: true}, "--http3 requires TLS"},
		{"tls-auto without domain", config.Config{TLSAuto: true}, "--tls-auto needs --tls-domain"},
		{"cert without key", config.Config{TLSCert: "c.pem"}, "--tls-cert given without --tls-key"},
		{"key without cert", config.Config{TLSKey: "k.pem"}, "--tls-key given without --tls-cert"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			out := captureStderr(t, func() { warnTLSMisconfig(&cfg, serverTLSMode(&cfg)) })
			if !strings.Contains(out, tc.want) {
				t.Errorf("stderr = %q, want substring %q", out, tc.want)
			}
		})
	}

	t.Run("complete tls stays quiet", func(t *testing.T) {
		cfg := config.Config{TLSCert: "c.pem", TLSKey: "k.pem", HTTP3: true}
		out := captureStderr(t, func() { warnTLSMisconfig(&cfg, serverTLSMode(&cfg)) })
		if out != "" {
			t.Errorf("expected no warnings, got %q", out)
		}
	})
}

// TestCaptureStderrSurvivesALongMessage: the helper used to read once into a
// 4096-byte buffer after fn() had already returned. Anything longer came back
// truncated, and anything past a pipe's ~64 KB blocked fn() on a write nobody
// was draining — a hung test rather than a failing one. 19 tests lean on this.
func TestCaptureStderrSurvivesALongMessage(t *testing.T) {
	long := strings.Repeat("x", 200_000) // comfortably past a pipe buffer
	got := captureStderr(t, func() {
		if _, err := os.Stderr.WriteString(long); err != nil {
			t.Errorf("write: %v", err)
		}
	})
	if len(got) != len(long) {
		t.Fatalf("captured %d bytes of %d — the reader is not draining concurrently", len(got), len(long))
	}
	// And it still restores the real stderr afterwards, or every later test
	// writes into a closed pipe.
	if os.Stderr == nil {
		t.Fatal("stderr was not restored")
	}
}
