package config

import (
	"os"
	"path/filepath"
	"testing"
)

// GO-059: the deprecated seo_off key must force SEO off (was a silent no-op).
func TestLoadSEOOffForcesOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ssg.yaml")
	if err := os.WriteFile(path, []byte("source: c\ntemplate: t\ndomain: e.com\nseo: true\nseo_off: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SEO {
		t.Error("seo_off: true must force SEO off")
	}
}

func TestLoadSEOWithoutOffStays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ssg.yaml")
	if err := os.WriteFile(path, []byte("source: c\ntemplate: t\ndomain: e.com\nseo: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SEO {
		t.Error("seo: true without seo_off should stay on")
	}
}
