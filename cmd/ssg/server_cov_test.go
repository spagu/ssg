package main

// The last server.go paths: the default error reporter, the start line of a
// site served under a base path, and a broken access-control config.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestServerErrfWritesToStderr(t *testing.T) {
	serverErrMu.RLock()
	report := serverErrf
	serverErrMu.RUnlock()
	if out := captureStderr(t, func() { report("boom %d\n", 1) }); out != "boom 1\n" {
		t.Errorf("stderr = %q", out)
	}
}

// TestTheStartLineNamesTheBasePath: the site lives under /site/, so that is
// the address to open (#306).
func TestTheStartLineNamesTheBasePath(t *testing.T) {
	out, _ := captureStdout(func() error {
		logServerStart(&config.Config{Domain: "example.com/site", OutputDir: "out"}, "http://localhost:8888/", "", false)
		return nil
	})
	if !strings.Contains(out, "http://localhost:8888/site/") {
		t.Errorf("stdout = %q", out)
	}
}

// TestABrokenAccessControlConfigStopsTheServer: serving the site open when
// its owner asked for a login would be worse than not serving it.
func TestABrokenAccessControlConfigStopsTheServer(t *testing.T) {
	if cov100IsChild(t.Name()) {
		buildServerHandler(&config.Config{ServerAuth: "sso", OutputDir: t.TempDir()}, false)
		return
	}
	out, code := cov100RunChild(t, t.Name())
	if code != 1 || !strings.Contains(out, "Server access-control config") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
}
