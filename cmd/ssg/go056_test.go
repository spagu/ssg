package main

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// captureStderr collects what fn wrote to stderr.
//
// It swaps the command's diagnostic sink rather than os.Stderr (#188). The old
// shape assigned os.Stderr, a standard-library package variable that every
// background goroutine this command leaves running reads to write its own
// diagnostics — a data race -race caught roughly one CI run in ten, in
// whichever test happened to be running when an earlier test's goroutine
// finally spoke. Swapping a sink of our own under a lock has no such subject.
//
// It also retires the pipe the old shape needed. A pipe holds about 64 KB, so a
// capture that outgrew it blocked fn() on a write nobody was draining and hung
// the test rather than failing it; a buffer cannot.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	var buf syncBuffer
	saved := setStderrSink(&buf)
	defer setStderrSink(saved)
	fn()
	return buf.String()
}

// syncBuffer collects diagnostics under a lock. The command leaves background
// goroutines running — an HTTP/3 listener, a watch loop, a server reporting a
// dead listener — and any of them may write to the sink while the test that
// installed it is still reading (#188).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
// was draining — a hung test rather than a failing one. 19 tests lean on this,
// and a buffered sink has neither limit.
func TestCaptureStderrSurvivesALongMessage(t *testing.T) {
	long := strings.Repeat("x", 200_000) // comfortably past a pipe buffer
	got := captureStderr(t, func() { errf("%s", long) })
	if len(got) != len(long) {
		t.Fatalf("captured %d bytes of %d", len(got), len(long))
	}
	// And it restores the previous sink, or every later test writes into a
	// buffer nobody reads.
	if stderrWriter() != os.Stderr {
		t.Fatal("the sink was not restored")
	}
}

// TestCaptureStderrCollectsABackgroundGoroutine: the reason the sink exists.
// The command leaves goroutines running that report through the same path, and
// the old helper assigned os.Stderr — a variable those goroutines read (#188).
func TestCaptureStderrCollectsABackgroundGoroutine(t *testing.T) {
	got := captureStderr(t, func() {
		done := make(chan struct{})
		go func() { defer close(done); errf("from a goroutine\n") }()
		<-done
	})
	if !strings.Contains(got, "from a goroutine") {
		t.Errorf("stderr = %q", got)
	}
}

// TestErrlnGoesThroughTheSink, the other half of the drop-in pair.
func TestErrlnGoesThroughTheSink(t *testing.T) {
	if got := captureStderr(t, func() { errln("a", "b") }); got != "a b\n" {
		t.Errorf("errln wrote %q", got)
	}
	if got := captureStderr(t, func() { errln() }); got != "\n" {
		t.Errorf("bare errln wrote %q", got)
	}
}
