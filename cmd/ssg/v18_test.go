package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/config"
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
