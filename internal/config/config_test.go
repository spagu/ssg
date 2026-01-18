package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ContentDir != "content" {
		t.Errorf("expected content_dir 'content', got '%s'", cfg.ContentDir)
	}
	if cfg.TemplatesDir != "templates" {
		t.Errorf("expected templates_dir 'templates', got '%s'", cfg.TemplatesDir)
	}
	if cfg.OutputDir != "output" {
		t.Errorf("expected output_dir 'output', got '%s'", cfg.OutputDir)
	}
	if cfg.Port != 8888 {
		t.Errorf("expected port 8888, got %d", cfg.Port)
	}
	if cfg.WebPQuality != 60 {
		t.Errorf("expected webp_quality 60, got %d", cfg.WebPQuality)
	}
}

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
source: "test-source"
template: "test-template"
domain: "test.com"
engine: "pongo2"
http: true
port: 3000
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source != "test-source" {
		t.Errorf("expected source 'test-source', got '%s'", cfg.Source)
	}
	if cfg.Template != "test-template" {
		t.Errorf("expected template 'test-template', got '%s'", cfg.Template)
	}
	if cfg.Domain != "test.com" {
		t.Errorf("expected domain 'test.com', got '%s'", cfg.Domain)
	}
	if cfg.Engine != "pongo2" {
		t.Errorf("expected engine 'pongo2', got '%s'", cfg.Engine)
	}
	if !cfg.HTTP {
		t.Error("expected http true")
	}
	if cfg.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Port)
	}
}

func TestLoadTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `
source = "toml-source"
template = "toml-template"
domain = "toml.com"
webp = true
webp_quality = 80
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source != "toml-source" {
		t.Errorf("expected source 'toml-source', got '%s'", cfg.Source)
	}
	if !cfg.WebP {
		t.Error("expected webp true")
	}
	if cfg.WebPQuality != 80 {
		t.Errorf("expected webp_quality 80, got %d", cfg.WebPQuality)
	}
}

func TestLoadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
  "source": "json-source",
  "template": "json-template",
  "domain": "json.com",
  "minify_all": true
}`
	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source != "json-source" {
		t.Errorf("expected source 'json-source', got '%s'", cfg.Source)
	}
	if !cfg.MinifyAll {
		t.Error("expected minify_all true")
	}
	// MinifyAll should also set individual flags
	if !cfg.MinifyHTML || !cfg.MinifyCSS || !cfg.MinifyJS {
		t.Error("expected minify_all to set individual minify flags")
	}
}

func TestLoadUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.xml")

	if err := os.WriteFile(configPath, []byte("<config></config>"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestFindConfigFile(t *testing.T) {
	// Just test the function runs without panicking
	// Actual file finding depends on the filesystem state
	_ = FindConfigFile()
}
