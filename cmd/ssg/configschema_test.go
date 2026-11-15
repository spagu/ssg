package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestConfigSchemaCommand(t *testing.T) {
	t.Chdir(t.TempDir()) // no config anywhere: the schema needs none
	if !isConfigSubcommand("schema") {
		t.Fatal("schema is a config verb")
	}
	out, err := captureStdout(func() error {
		if code := runConfig([]string{"schema"}); code != 0 {
			t.Errorf("exit %d", code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc config.Schema
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Keys) == 0 {
		t.Fatalf("not the schema: %v", err)
	}
	if out != string(config.SchemaJSON()) {
		t.Error("the command prints SchemaJSON exactly")
	}
	if code := runConfig([]string{"schema", "extra"}); code != 2 {
		t.Errorf("extra argument: exit %d, want 2", code)
	}
}

func TestConfigViewEffectiveNamesOverlayKeys(t *testing.T) {
	path := inProject(t, ".ssg.yaml", "source: s\ndomain: example.com\ntitle: File\n")
	writeOverlay(t, path, `{"title": "Overlay"}`)
	var out string
	errOut := captureStderr(t, func() {
		out, _ = captureStdout(func() error {
			if runConfig([]string{"view", "--effective", "title"}) != 0 {
				t.Error("view failed")
			}
			return nil
		})
	})
	if strings.TrimSpace(out) != "Overlay" {
		t.Errorf("stdout %q: the overlay wins over the file", out)
	}
	if !strings.Contains(errOut, config.OverlayFileName+": title") {
		t.Errorf("stderr %q does not name the overlay key", errOut)
	}
	// Without an overlay nothing is said.
	inProject(t, ".ssg.yaml", "source: s\n")
	if errOut := captureStderr(t, func() { _, _ = captureStdout(func() error { runConfig([]string{"view", "--effective"}); return nil }) }); strings.Contains(errOut, config.OverlayFileName) {
		t.Errorf("unexpected note: %q", errOut)
	}
}

// TestConfigSignatureSeesTheOverlay: the watcher rebuilds on an overlay edit.
func TestConfigSignatureSeesTheOverlay(t *testing.T) {
	path := inProject(t, ".ssg.yaml", "title: x\n")
	before := configSignature(path)
	writeOverlay(t, path, `{"title": "y"}`)
	if configSignature(path) == before {
		t.Error("an overlay edit must change the config signature")
	}
	if configSignature("") != "|" {
		t.Error("no config, empty signature")
	}
}

// TestConfigSourcePrecedence pins the order, lowest first: defaults, include
// files, the config file, the overlay, command-line flags.
func TestConfigSourcePrecedence(t *testing.T) {
	path := inProject(t, ".ssg.yaml", "include: [base.yaml]\nport: 9001\ntitle: File\ntoc_depth: 5\n")
	dir := filepath.Dir(path)
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte("port: 9000\nhost: 0.0.0.0\nfeed_items: 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOverlay(t, path, `{"port": 9002, "title": "Overlay"}`)
	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parseFlags([]string{"--port=9003"}, cfg)
	checks := map[string][2]any{
		"default (webp_quality)": {cfg.WebPQuality, 60},
		"include (host)":         {cfg.Host, "0.0.0.0"},
		"include (feed_items)":   {cfg.FeedItems, 7},
		"file (toc_depth)":       {cfg.TOCDepth, 5},
		"overlay (title)":        {cfg.Title, "Overlay"},
		"flag (port)":            {cfg.Port, 9003},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
}

func writeOverlay(t *testing.T, configPath, body string) {
	t.Helper()
	if err := os.WriteFile(config.OverlayPath(configPath), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
