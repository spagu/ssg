package main

// The dev server's port is a preference, not a requirement (#135).
//
// A second `ssg --watch --http` in another checkout, yesterday's `wrangler dev`
// still holding 8888, a migration re-run started before the last one was
// stopped — any of them used to end the run with "bind: address already in
// use", after the build had already succeeded. The server now walks forward
// (8888, 8889, 8890, …) and announces the port it actually landed on.

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/spagu/ssg/internal/config"
)

// portSearchSpan bounds the walk. A machine with 64 consecutive dev servers is
// not a machine that wants a 65th on an unpredictable port — it wants to be
// told something is wrong.
const portSearchSpan = 64

// claimPort binds the dev server's listener, stepping past ports already in
// use, and records the port it settled on in cfg — so the address the caller
// announces is the address being served, with no window for another process to
// take it in between (which a probe-then-bind check would leave open).
//
// Port 0 is an explicit "any free port": the kernel picks one and there is
// nothing to walk.
func claimPort(cfg *config.Config) (net.Listener, error) {
	first := cfg.Port

	for offset := 0; offset <= portSearchSpan; offset++ {
		port := first + offset
		if first == 0 && offset > 0 {
			break // the kernel already refused an ephemeral port; walking cannot help
		}

		addr, _, _ := resolveListenAddr(cfg.Host, port)

		ln, err := newServerListener(addr, cfg.MaxConns)
		if err == nil {
			cfg.Port = listenerPort(ln, port)
			announcePortShift(cfg, first)

			return ln, nil
		}

		if !isAddrInUse(err) {
			// A bad host, a privileged port, an exhausted file-descriptor
			// limit: walking the port cannot fix any of them.
			return nil, fmt.Errorf("server: %w", err)
		}
	}

	return nil, fmt.Errorf("server: ports %d-%d are all in use — free one, or pass --port=<free port>",
		first, first+portSearchSpan)
}

// listenerPort reads the port the kernel actually assigned, which is the whole
// point of --port=0. A listener whose address does not parse keeps the
// requested port rather than reporting nonsense.
func listenerPort(ln net.Listener, requested int) int {
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		return tcp.Port
	}
	if _, portStr, err := net.SplitHostPort(ln.Addr().String()); err == nil {
		if port, convErr := strconv.Atoi(portStr); convErr == nil {
			return port
		}
	}

	return requested
}

// announcePortShift says so when the server did not get the port that was
// asked for. Silently serving somewhere else is how a browser ends up pointed
// at a stale server from an earlier run.
func announcePortShift(cfg *config.Config, requested int) {
	if cfg.Quiet || requested == 0 || cfg.Port == requested {
		return
	}

	fmt.Printf("ℹ️  Port %d is in use — serving on %d instead\n", requested, cfg.Port)
}

// isAddrInUse reports whether a bind failed because something else holds the
// port — the one failure a different port fixes.
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
