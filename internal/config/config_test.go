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

func TestFindConfigFileWithFiles(t *testing.T) {
	// Save current dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp dir: %v", err)
	}

	// No config files - should return empty string
	result := FindConfigFile()
	if result != "" {
		t.Errorf("expected empty string when no config files, got %s", result)
	}

	// Create .ssg.yaml
	if err := os.WriteFile(".ssg.yaml", []byte("source: test"), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	result = FindConfigFile()
	if result != ".ssg.yaml" {
		t.Errorf("expected '.ssg.yaml', got '%s'", result)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `
source: "test
  invalid: yaml
`
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	invalidTOML := `
source = "test
invalid toml
`
	if err := os.WriteFile(configPath, []byte(invalidTOML), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	invalidJSON := `{"source": "test", invalid json}`
	if err := os.WriteFile(configPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadYMLExtension(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	yamlContent := `
source: "yml-source"
template: "yml-template"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source != "yml-source" {
		t.Errorf("expected source 'yml-source', got '%s'", cfg.Source)
	}
}

func TestLoadPrettyHTMLFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Test case 1: pretty_html set to true
	yamlContent := `
source: "test-source"
template: "test-template"
domain: "test.com"
pretty_html: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.PrettyHTML {
		t.Error("expected pretty_html to be true from config file, got false")
	}

	// Test case 2: pretty_html not set (should default to false)
	yamlContent2 := `
source: "test-source"
template: "test-template"
domain: "test.com"
`
	configPath2 := filepath.Join(tmpDir, "config2.yaml")
	if err := os.WriteFile(configPath2, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg2, err := Load(configPath2)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg2.PrettyHTML {
		t.Error("expected pretty_html to default to false, got true")
	}

	// Test case 3: pretty_html set to false explicitly
	yamlContent3 := `
source: "test-source"
template: "test-template"
domain: "test.com"
pretty_html: false
`
	configPath3 := filepath.Join(tmpDir, "config3.yaml")
	if err := os.WriteFile(configPath3, []byte(yamlContent3), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg3, err := Load(configPath3)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg3.PrettyHTML {
		t.Error("expected pretty_html to be false when explicitly set, got true")
	}
}

func TestLoadPrettyHTMLFromTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `
source = "test-source"
template = "test-template"
domain = "test.com"
pretty_html = true
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.PrettyHTML {
		t.Error("expected pretty_html to be true from TOML config file, got false")
	}
}

func TestLoadPrettyHTMLFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
  "source": "test-source",
  "template": "test-template",
  "domain": "test.com",
  "pretty_html": true
}`
	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.PrettyHTML {
		t.Error("expected pretty_html to be true from JSON config file, got false")
	}
}

func TestLoadAllOptions(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
source: "test-source"
template: "test-template"
domain: "example.com"
content_dir: "custom-content"
templates_dir: "custom-templates"
output_dir: "custom-output"
engine: "pongo2"
online_theme: "https://example.com/theme.zip"
http: true
port: 9000
watch: true
clean: true
sitemap_off: true
robots_off: true
pretty_html: true
minify_html: false
minify_css: false
minify_js: false
sourcemap: true
webp: true
webp_quality: 75
zip: true
quiet: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source != "test-source" {
		t.Errorf("source mismatch")
	}
	if cfg.ContentDir != "custom-content" {
		t.Errorf("content_dir mismatch: got %s", cfg.ContentDir)
	}
	if cfg.TemplatesDir != "custom-templates" {
		t.Errorf("templates_dir mismatch: got %s", cfg.TemplatesDir)
	}
	if cfg.OutputDir != "custom-output" {
		t.Errorf("output_dir mismatch: got %s", cfg.OutputDir)
	}
	if cfg.Port != 9000 {
		t.Errorf("port mismatch: got %d", cfg.Port)
	}
	if !cfg.Watch {
		t.Error("watch should be true")
	}
	if !cfg.Clean {
		t.Error("clean should be true")
	}
	if !cfg.SitemapOff {
		t.Error("sitemap_off should be true")
	}
	if !cfg.RobotsOff {
		t.Error("robots_off should be true")
	}
	if !cfg.PrettyHTML {
		t.Error("pretty_html should be true")
	}
	if !cfg.WebP {
		t.Error("webp should be true")
	}
	if cfg.WebPQuality != 75 {
		t.Errorf("webp_quality mismatch: got %d", cfg.WebPQuality)
	}
	if !cfg.Zip {
		t.Error("zip should be true")
	}
	if !cfg.Quiet {
		t.Error("quiet should be true")
	}
}
