package main

// The built-in server's failure paths — the ones a user only ever meets when
// something is already wrong, which is exactly when a silent one is worst.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/spagu/ssg/internal/config"
)

// unbindable is a host no kernel will bind, so claimPort fails for a reason
// walking the port range cannot fix.
func unbindable() *config.Config {
	return &config.Config{Host: "256.256.256.256", Port: 8888, Quiet: true, OutputDir: "."}
}

// TestAPortThatCannotBeClaimedIsReported: both server entry points must say so
// rather than returning as though they had started something.
func TestAPortThatCannotBeClaimedIsReported(t *testing.T) {
	if out := captureStderr(t, func() { startServer(unbindable()) }); !strings.Contains(out, "❌") {
		t.Errorf("startServer said %q", out)
	}
	if out := captureStderr(t, func() { startServerAsync(unbindable()) }); !strings.Contains(out, "❌") {
		t.Errorf("startServerAsync said %q", out)
	}
}

// collectServerErrors redirects the server's failure reporter into a channel
// for the duration of a test. A channel rather than a captured slice because
// the reports arrive on goroutines the test does not own, and a receive is what
// gives the restore something to happen after.
func collectServerErrors(t *testing.T) <-chan string {
	t.Helper()
	reports := make(chan string, 8)
	saved := setServerErrf(func(format string, a ...any) { reports <- sprintfLine(format, a...) })
	t.Cleanup(func() { setServerErrf(saved) })
	return reports
}

// awaitReport waits for a report containing want, so a test never sleeps for a
// background listener and never restores the reporter while one is still using
// it.
func awaitReport(t *testing.T, reports <-chan string, want string) {
	t.Helper()
	awaitReports(t, reports, want)
}

// awaitReports waits until every wanted substring has been reported, in any
// order. A test that starts more than one background listener has to drain all
// of them before it returns: the reporter it restores on the way out is the
// default one, and that closure reads os.Stderr at call time. A goroutine still
// holding it races the next test's captureStderr, which writes os.Stderr — a
// race the swap under serverErrMu cannot cover, because the contended variable
// is os.Stderr itself (#188).
func awaitReports(t *testing.T, reports <-chan string, want ...string) {
	t.Helper()
	pending := make(map[string]bool, len(want))
	for _, w := range want {
		pending[w] = true
	}
	deadline := time.After(10 * time.Second)
	for len(pending) > 0 {
		select {
		case got := <-reports:
			for w := range pending {
				if strings.Contains(got, w) {
					delete(pending, w)
				}
			}
		case <-deadline:
			t.Fatalf("these reports never arrived: %v", keysOf(pending))
		}
	}
}

// keysOf names what a timed-out wait was still missing.
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestAnHTTP3ListenerThatCannotStartSaysSo: HTTP/3 runs in the background, so
// its failure is the easiest one to lose. GO-056 made TLS misconfiguration
// loud; the same has to hold once the listener is actually trying to run.
func TestAnHTTP3ListenerThatCannotStartSaysSo(t *testing.T) {
	reports := collectServerErrors(t)

	startHTTP3(&http3.Server{Addr: "127.0.0.1:0"},
		&config.Config{TLSCert: "no-such.pem", TLSKey: "no-such.key"}, "manual")
	awaitReport(t, reports, "HTTP/3 server error")

	// The Let's Encrypt path reports too. A server with no TLS configuration
	// refuses immediately, which is the deterministic way to reach it: an
	// address chosen to be unbindable would go through a DNS lookup first, and
	// a resolver that takes ten seconds to say no is how this test would start
	// failing on somebody else's machine.
	startHTTP3(&http3.Server{Addr: "127.0.0.1:0"}, &config.Config{}, "auto")
	awaitReport(t, reports, "HTTP/3 server error")
}

// TestServingOnADeadListenerIsReported drives serveOnClaimedPort with TLS and
// HTTP/3 both on, and asserts the one thing that matters when the listener
// underneath it is gone: it is not silent.
//
// Manual TLS rather than Let's Encrypt on purpose. The autocert path starts an
// ACME HTTP-01 helper on :80 from its own goroutine, and that goroutine writes
// to os.Stderr — which several tests in this package swap out from under it.
// Adding another one would have made an existing intermittent race commoner,
// and a test that widens a flake is not worth the two statements it covers.
func TestServingOnADeadListenerIsReported(t *testing.T) {
	reports := collectServerErrors(t)
	cfg := &config.Config{
		Host: "127.0.0.1", Port: 0, OutputDir: t.TempDir(), Quiet: true,
		TLSCert: "no-such.pem", TLSKey: "no-such.key", HTTP3: true, MemLimit: "64MiB",
	}
	ln, err := newServerListener("127.0.0.1:0", 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // serving on it must fail immediately

	serveOnClaimedPort(cfg, ln)
	// Both listeners must be drained, not just the one under test: the HTTP/3
	// goroutine started alongside it reports from its own goroutine, and one
	// still running after this test restores the default reporter races the
	// next test's os.Stderr swap.
	awaitReports(t, reports, "Server error", "HTTP/3 server error")
}

// TestAMemoryLimitIsAnnouncedOrRefused: a limit that does not parse must not be
// applied silently, and one that does must say what it set.
func TestAMemoryLimitIsAnnouncedOrRefused(t *testing.T) {
	if out := captureStderr(t, func() { applyMemLimit("not-a-size", false) }); !strings.Contains(out, "invalid --mem-limit") {
		t.Errorf("stderr = %q", out)
	}
	out, err := captureStdout(func() error {
		applyMemLimit("256MiB", false)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Soft memory limit") {
		t.Errorf("stdout = %q", out)
	}
}

// TestAByteSizeWithANonNumericValueIsAnError: "MiB" is a unit, not a number.
func TestAByteSizeWithANonNumericValueIsAnError(t *testing.T) {
	if _, err := parseByteSize("lots-of-MiB"); err == nil {
		t.Error("a non-numeric size must be an error")
	}
}

// TestAGzipWriterWritesItsHeaderOnce: a second WriteHeader is a no-op, or the
// stripped Content-Length comes back and desynchronises the connection.
func TestAGzipWriterWritesItsHeaderOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	g := &gzipResponseWriter{ResponseWriter: rec}
	g.WriteHeader(http.StatusNotFound)
	g.WriteHeader(http.StatusOK)
	if rec.Code != http.StatusNotFound {
		t.Errorf("code = %d, want the first one written", rec.Code)
	}
}

// statFailingFS hands back a file whose Stat fails — a directory removed
// between the open and the stat, or a filesystem that went away.
type statFailingFS struct{}

func (statFailingFS) Open(string) (http.File, error) { return statFailingFile{}, nil }

type statFailingFile struct{ http.File }

func (statFailingFile) Stat() (os.FileInfo, error) { return nil, errors.New("stat failed") }
func (statFailingFile) Close() error               { return nil }

// TestAFileThatCannotBeStattedIsAnError: noDirListing has to decide whether it
// is looking at a directory, and it cannot answer 200 without knowing.
func TestAFileThatCannotBeStattedIsAnError(t *testing.T) {
	if _, err := (noDirListing{fs: statFailingFS{}}).Open("/x"); err == nil {
		t.Error("an unstattable file must not be served")
	}
}
