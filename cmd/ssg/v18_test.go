package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

func TestParseIntListEdge(t *testing.T) {
	got := parseIntList("480, ,x,-5,0,960")
	if len(got) != 2 || got[0] != 480 || got[1] != 960 {
		t.Errorf("parseIntList = %v, want [480 960]", got)
	}
	if parseIntList("") != nil {
		t.Errorf("empty should yield nil")
	}
}

func TestSetPermalink(t *testing.T) {
	cfg := &config.Config{}
	setPermalink(cfg, "post", "") // empty is ignored
	if cfg.Permalinks != nil {
		t.Errorf("empty pattern should not init map")
	}
	setPermalink(cfg, "post", "/:slug/")
	if cfg.Permalinks["post"] != "/:slug/" {
		t.Errorf("permalink not set: %v", cfg.Permalinks)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV("pl, en ,, de")
	if len(got) != 3 || got[0] != "pl" || got[1] != "en" || got[2] != "de" {
		t.Errorf("splitCSV = %v, want [pl en de]", got)
	}
}

func TestWatchDirs(t *testing.T) {
	cfg := &config.Config{ContentDir: "c", TemplatesDir: "t", DataDir: "d"}
	dirs := watchDirs(cfg)
	if len(dirs) != 3 {
		t.Errorf("watchDirs = %v, want 3 entries", dirs)
	}
	cfg.DataDir = ""
	if len(watchDirs(cfg)) != 2 {
		t.Errorf("expected 2 dirs without data dir")
	}
}

func TestContentSignature(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.md"), []byte("hello"), 0644)
	sig1 := contentSignature([]string{dir})
	sig2 := contentSignature([]string{dir})
	if sig1 != sig2 {
		t.Errorf("signature not stable for identical content")
	}
	_ = os.WriteFile(filepath.Join(dir, "a.md"), []byte("changed"), 0644)
	if contentSignature([]string{dir}) == sig1 {
		t.Errorf("signature should change when content changes")
	}
}

func TestRunWatchOrServeNoop(t *testing.T) {
	// No watch, no http, no mddb-watch → returns immediately (no blocking).
	done := make(chan struct{})
	go func() {
		runWatchOrServe(generator.Config{}, &config.Config{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWatchOrServe blocked with no watch/http configured")
	}
}

func TestServerAndArchiveFlags(t *testing.T) {
	cfg := config.DefaultConfig()
	parseFlags([]string{
		"--tls-cert=c.pem", "--tls-key=k.pem", "--tls-auto", "--tls-domain=x.com",
		"--gzip", "--http3", "--sanitize-html", "--targz", "--tarxz",
		"--max-conns=50", "--mem-limit=256MiB",
	}, cfg)
	if cfg.TLSCert != "c.pem" || cfg.TLSKey != "k.pem" || !cfg.TLSAuto || cfg.TLSDomain != "x.com" {
		t.Errorf("TLS flags not parsed: %+v", cfg)
	}
	if !cfg.Gzip || !cfg.HTTP3 || !cfg.SanitizeHTML || !cfg.TarGz || !cfg.TarXz {
		t.Errorf("bool server/archive flags not parsed")
	}
	if cfg.MaxConns != 50 || cfg.MemLimit != "256MiB" {
		t.Errorf("MaxConns/MemLimit not parsed: %d %q", cfg.MaxConns, cfg.MemLimit)
	}
}

func TestMakeArchive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644)
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	defer func() { _ = os.Chdir(cwd) }()
	cfg := &config.Config{Domain: "example.com", OutputDir: dir, Quiet: true}
	if err := makeArchive(cfg, "tar.gz", createTarGz); err != nil {
		t.Fatalf("makeArchive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "example.com.tar.gz")); err != nil {
		t.Errorf("archive not created: %v", err)
	}
}

func TestMakeArchiveSuccessAndError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Domain: "example.com", OutputDir: dir, Quiet: false}

	// Success: fn writes a file at the requested name (non-quiet → stat/print path).
	got := ""
	err := makeArchive(cfg, "tar.gz", func(src, out string) error {
		got = out
		return os.WriteFile(out, []byte("payload"), 0o644)
	})
	if err != nil {
		t.Fatalf("makeArchive success: %v", err)
	}
	if got != "example.com.tar.gz" {
		t.Errorf("archive name = %q, want example.com.tar.gz", got)
	}
	_ = os.Remove(got)

	// Error: fn fails → wrapped error returned.
	err = makeArchive(cfg, "tar.xz", func(_, _ string) error {
		return os.ErrPermission
	})
	if err == nil {
		t.Error("makeArchive should propagate the fn error")
	}
}
