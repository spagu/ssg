package main

// Remaining branches of the port walk (#135) and of the migration's reporting.

import (
	"net"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/migrate"
)

// fakeAddr is a listener address that is not a TCPAddr and does not parse as
// host:port — the shape a non-TCP listener would have.
type fakeAddr struct{ s string }

func (f fakeAddr) Network() string { return "fake" }
func (f fakeAddr) String() string  { return f.s }

type fakeListener struct {
	net.Listener
	addr net.Addr
}

func (f fakeListener) Addr() net.Addr { return f.addr }

// TestListenerPortFallbacks: the port is read from the listener so --port=0
// reports what the kernel chose; an address that cannot be read keeps the
// requested number rather than reporting nonsense.
func TestListenerPortFallbacks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if got := listenerPort(ln, 0); got != ln.Addr().(*net.TCPAddr).Port || got == 0 {
		t.Fatalf("kernel-assigned port = %d", got)
	}
	// host:port string, not a TCPAddr → parsed from the string.
	if got := listenerPort(fakeListener{addr: fakeAddr{s: "127.0.0.1:4321"}}, 9); got != 4321 {
		t.Fatalf("parsed port = %d, want 4321", got)
	}
	// Unparseable → the requested port survives.
	if got := listenerPort(fakeListener{addr: fakeAddr{s: "unix-socket"}}, 8888); got != 8888 {
		t.Fatalf("fallback port = %d, want 8888", got)
	}
	// A port that is not a number is treated the same way.
	if got := listenerPort(fakeListener{addr: fakeAddr{s: "host:notaport"}}, 7777); got != 7777 {
		t.Fatalf("non-numeric port = %d, want 7777", got)
	}
}

// TestAnnouncePortShiftSilence: the shift is announced only when it happened
// and only when the run is not quiet — an unchanged port must print nothing.
func TestAnnouncePortShiftSilence(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.Config
		requested int
	}{
		{"quiet run", config.Config{Quiet: true, Port: 8890}, 8888},
		{"ephemeral request", config.Config{Port: 41234}, 0},
		{"port unchanged", config.Config{Port: 8888}, 8888},
	}
	for _, c := range cases {
		out, err := captureStdout(func() error {
			announcePortShift(&c.cfg, c.requested)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if out != "" {
			t.Errorf("%s must stay silent, printed %q", c.name, out)
		}
	}
	// The one case that must speak.
	cfg := config.Config{Port: 8890}
	out, err := captureStdout(func() error {
		announcePortShift(&cfg, 8888)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8888") || !strings.Contains(out, "8890") {
		t.Fatalf("the shift must name both ports, got %q", out)
	}
}

// TestClaimPortRefusesUnfixableErrors: walking the port helps only when the
// port is taken. A host that cannot be bound is reported immediately.
func TestClaimPortRefusesUnfixableErrors(t *testing.T) {
	cfg := &config.Config{Host: "256.256.256.256", Port: 8888, Quiet: true}
	ln, err := claimPort(cfg)
	if err == nil {
		_ = ln.Close()
		t.Fatal("an unusable host must fail immediately, not walk 64 ports")
	}
	if !strings.Contains(err.Error(), "server:") {
		t.Fatalf("error must name the phase: %v", err)
	}
}

// TestPrintMigrateReportComments: what the run actually brought back is named,
// including reader comments (#134) and anything the engine could not deliver.
func TestPrintMigrateReportComments(t *testing.T) {
	out, err := captureStdout(func() error {
		printMigrateReport(&migrate.Report{
			Provider: "wordpress@1.1.0", Pages: 3, Posts: 7, Media: 12, Comments: 40,
			Warnings: []string{"menus: not readable without authentication"},
		}, "https://example.com")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"3", "7", "12", "40", "not readable without authentication"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
}
