package main

import (
	"net"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestClaimPortWalksPastABusyPort: a port someone else holds shifts the server
// forward instead of ending the run, and cfg records where it landed so the
// announced address is the served one (#135).
func TestClaimPortWalksPastABusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close() //nolint:errcheck // test cleanup

	taken := busy.Addr().(*net.TCPAddr).Port
	cfg := &config.Config{Host: "127.0.0.1", Port: taken, Quiet: true}

	ln, err := claimPort(cfg)
	if err != nil {
		t.Fatalf("a busy port must not end the run: %v", err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup

	if cfg.Port <= taken {
		t.Fatalf("port = %d, want something after the busy %d", cfg.Port, taken)
	}
	if got := ln.Addr().(*net.TCPAddr).Port; got != cfg.Port {
		t.Fatalf("cfg says %d, listener is on %d", cfg.Port, got)
	}
}

// TestClaimPortKeepsAFreePort: the requested port is used as-is when nothing
// holds it — the walk is a fallback, not a policy.
func TestClaimPortKeepsAFreePort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Host: "127.0.0.1", Port: free, Quiet: true}

	ln, err := claimPort(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup

	if cfg.Port != free {
		t.Fatalf("port = %d, want the requested %d", cfg.Port, free)
	}
}

// TestClaimPortEphemeral: --port=0 asks the kernel for any free port, and the
// one it hands back is recorded rather than left as 0.
func TestClaimPortEphemeral(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: 0, Quiet: true}

	ln, err := claimPort(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup

	if cfg.Port == 0 {
		t.Fatal("the assigned port must be recorded")
	}
	if got := ln.Addr().(*net.TCPAddr).Port; got != cfg.Port {
		t.Fatalf("cfg says %d, listener is on %d", cfg.Port, got)
	}
}

// TestClaimPortReportsUnwalkableFailures: a bad host is not a busy port, and
// walking 64 addresses that cannot exist would only delay the message.
func TestClaimPortReportsUnwalkableFailures(t *testing.T) {
	cfg := &config.Config{Host: "256.256.256.256", Port: 8888, Quiet: true}

	if _, err := claimPort(cfg); err == nil {
		t.Fatal("an unresolvable host must fail")
	} else if !strings.Contains(err.Error(), "server:") {
		t.Fatalf("error must name the server: %v", err)
	}
}

// TestListenerPortFallsBackToRequested: an address the standard library cannot
// split keeps the requested port rather than reporting nonsense.
func TestListenerPortFallsBackToRequested(t *testing.T) {
	if got := listenerPort(oddAddrListener{}, 8888); got != 8888 {
		t.Fatalf("port = %d, want the requested 8888", got)
	}
}

// oddAddrListener is a listener whose address is neither a *net.TCPAddr nor a
// host:port string — the shape listenerPort must survive.
type oddAddrListener struct{ net.Listener }

func (oddAddrListener) Addr() net.Addr { return oddAddr{} }

type oddAddr struct{}

func (oddAddr) Network() string { return "unix" }
func (oddAddr) String() string  { return "/tmp/ssg.sock" }
