package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

func TestCreateGeneratorConfigPrettyHTML(t *testing.T) {
	// Test that PrettyHTML is correctly passed from config to generator config
	cfg := &config.Config{
		Source:       "test-source",
		Template:     "test-template",
		Domain:       "example.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		PrettyHTML:   true,
	}

	genCfg := createGeneratorConfig(cfg)

	if !genCfg.PrettyHTML {
		t.Error("expected PrettyHTML to be true in generator config, got false")
	}
}

func TestCreateGeneratorConfigPrettyHTMLFalse(t *testing.T) {
	// Test that PrettyHTML false is correctly passed
	cfg := &config.Config{
		Source:       "test-source",
		Template:     "test-template",
		Domain:       "example.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		PrettyHTML:   false,
	}

	genCfg := createGeneratorConfig(cfg)

	if genCfg.PrettyHTML {
		t.Error("expected PrettyHTML to be false in generator config, got true")
	}
}

func TestLoadConfigPrettyHTMLFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".ssg.yaml")

	yamlContent := `
source: "test-source"
template: "test-template"
domain: "test.com"
pretty_html: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Test loadConfig function
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	cfg := loadConfig([]string{})

	if !cfg.PrettyHTML {
		t.Error("expected PrettyHTML to be true after loading from config file, got false")
	}
}

func TestParseFlagsDoesNotOverridePrettyHTML(t *testing.T) {
	// Config with PrettyHTML=true from config file
	cfg := &config.Config{
		Source:       "test-source",
		Template:     "test-template",
		Domain:       "example.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		PrettyHTML:   true,
	}

	// Parse flags without --pretty-html flag - should NOT change PrettyHTML
	args := []string{"test-source", "test-template", "example.com"}
	parseFlags(args, cfg)

	if !cfg.PrettyHTML {
		t.Error("parseFlags should NOT override PrettyHTML when --pretty-html flag is not passed")
	}
}

func TestParseFlagsSetsPrettyHTML(t *testing.T) {
	// Config with PrettyHTML=false
	cfg := &config.Config{
		Source:       "test-source",
		Template:     "test-template",
		Domain:       "example.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		PrettyHTML:   false,
	}

	// Parse flags WITH --pretty-html flag - should set PrettyHTML to true
	args := []string{"test-source", "test-template", "example.com", "--pretty-html"}
	parseFlags(args, cfg)

	if !cfg.PrettyHTML {
		t.Error("parseFlags should set PrettyHTML to true when --pretty-html flag is passed")
	}
}

func TestFullPipelineConfigToGenerator(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".ssg.yaml")

	yamlContent := `
source: "test-source"
template: "test-template"
domain: "test.com"
pretty_html: true
minify_html: false
quiet: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Change to temp dir to find the config file
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Step 1: Load config (simulating main.go)
	args := []string{}
	cfg := loadConfig(args)

	if !cfg.PrettyHTML {
		t.Fatal("Step 1 failed: PrettyHTML should be true after loadConfig")
	}

	// Step 2: Parse flags (simulating main.go)
	parseFlags(args, cfg)

	if !cfg.PrettyHTML {
		t.Fatal("Step 2 failed: PrettyHTML should still be true after parseFlags")
	}

	// Step 3: Apply minify all
	applyMinifyAll(cfg)

	if !cfg.PrettyHTML {
		t.Fatal("Step 3 failed: PrettyHTML should still be true after applyMinifyAll")
	}

	// Step 4: Create generator config (simulating main.go)
	genCfg := createGeneratorConfig(cfg)

	if !genCfg.PrettyHTML {
		t.Fatal("Step 4 failed: PrettyHTML should be true in generator config")
	}

	// All steps passed - the pipeline works correctly
}

func TestEndToEndPrettyHTMLFromConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup directory structure
	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	// Create metadata.json
	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("failed to create metadata: %v", err)
	}

	// Create templates with extra blank lines
	templates := map[string]string{
		"base.html": `<!DOCTYPE html>


<html>


<head></head>


<body>


{{.Content}}


</body>


</html>`,
		"index.html":    `{{define "content"}}Index content{{end}}`,
		"page.html":     `{{define "content"}}Page content{{end}}`,
		"post.html":     `{{define "content"}}Post content{{end}}`,
		"category.html": `{{define "content"}}Category content{{end}}`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to create template: %v", err)
		}
	}

	// Create config file with pretty_html: true
	configPath := filepath.Join(tmpDir, ".ssg.yaml")
	yamlContent := `
source: "test-source"
template: "simple"
domain: "test.com"
content_dir: "` + contentDir + `"
templates_dir: "` + templatesDir + `"
output_dir: "` + outputDir + `"
pretty_html: true
quiet: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Simulate the exact main.go flow
	args := []string{}
	cfg := loadConfig(args)

	if !cfg.PrettyHTML {
		t.Fatal("PrettyHTML should be true from config file")
	}

	parseFlags(args, cfg)
	applyMinifyAll(cfg)

	genCfg := createGeneratorConfig(cfg)

	if !genCfg.PrettyHTML {
		t.Fatal("Generator PrettyHTML should be true")
	}

	// Run the actual build
	gen, err := generator.New(genCfg)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("failed to generate: %v", err)
	}

	// Verify the index.html was prettified (no 3+ consecutive newlines)
	indexPath := filepath.Join(outputDir, "index.html")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Check for 3+ consecutive newlines (which should have been reduced to 2)
	if strings.Contains(string(content), "\n\n\n") {
		t.Errorf("HTML should be prettified - no 3+ consecutive newlines\nContent:\n%s", string(content))
	}
}
