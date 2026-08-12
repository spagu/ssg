package main

// `ssg mcp --http` preview server (1.8.30): the flag was parsed into the config
// but nothing served it, so an assistant reworked a theme nobody could look at.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
)

func TestServeMCPPreviewOffByDefault(t *testing.T) {
	old := reloadHub
	t.Cleanup(func() { reloadHub = old })
	reloadHub = nil
	var logged []string
	serveMCPPreview(&config.Config{}, func(f string, a ...any) { logged = append(logged, f) })
	if reloadHub != nil || len(logged) != 0 {
		t.Fatal("without --http nothing may start and nothing may be announced")
	}
}

// TestServeMCPPreviewServes: with --http the site is actually reachable, the
// address is announced on stderr (stdout belongs to JSON-RPC) and the live
// reload hub exists so each MCP rebuild refreshes the tab.
func TestServeMCPPreviewServes(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("out", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("out", "index.html"),
		[]byte("<html><body>preview</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := reloadHub
	t.Cleanup(func() { reloadHub = old })

	cfg := &config.Config{HTTP: true, OutputDir: "out", Host: "127.0.0.1", Port: 8899, Quiet: true}
	var logged string
	serveMCPPreview(cfg, func(f string, a ...any) { logged = f })
	if reloadHub == nil {
		t.Fatal("--http must arm live reload for MCP rebuilds")
	}
	if !strings.Contains(logged, "preview") {
		t.Fatalf("address must be announced, got %q", logged)
	}

	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ { // the server starts in a goroutine
		resp, err = http.Get("http://127.0.0.1:8899/")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("preview not reachable: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d", resp.StatusCode)
	}
}

// TestPrintMCPWiring: the help must hand over a registration line, not "run
// ssg mcp" — the server speaks stdio, so the assistant spawns it.
func TestPrintMCPWiring(t *testing.T) {
	out, err := captureStdout(func() error {
		printMCPWiring()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"claude mcp add ssg", "mcpServers", `"command":"ssg"`} {
		if !strings.Contains(out, want) {
			t.Errorf("wiring missing %q in:\n%s", want, out)
		}
	}
}

func TestWorkingDirOrDot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if got := workingDirOrDot(); got == "" || got == "." {
		t.Fatalf("must name a real directory for a pasted config, got %q", got)
	}
}
