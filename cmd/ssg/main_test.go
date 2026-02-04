package main

import (
	"io"
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

func TestParseFlagsSetsRelativeLinks(t *testing.T) {
	// Config with RelativeLinks=false
	cfg := &config.Config{
		Source:        "test-source",
		Template:      "test-template",
		Domain:        "example.com",
		ContentDir:    "content",
		TemplatesDir:  "templates",
		OutputDir:     "output",
		RelativeLinks: false,
	}

	// Parse flags WITH --relative-links flag - should set RelativeLinks to true
	args := []string{"test-source", "test-template", "example.com", "--relative-links"}
	parseFlags(args, cfg)

	if !cfg.RelativeLinks {
		t.Error("parseFlags should set RelativeLinks to true when --relative-links flag is passed")
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

// Test parseBoolFlags with all boolean flags
func TestParseBoolFlags(t *testing.T) {
	tests := []struct {
		flag     string
		check    func(*config.Config) bool
		expected bool
	}{
		{"--zip", func(c *config.Config) bool { return c.Zip }, true},
		{"-zip", func(c *config.Config) bool { return c.Zip }, true},
		{"--webp", func(c *config.Config) bool { return c.WebP }, true},
		{"-webp", func(c *config.Config) bool { return c.WebP }, true},
		{"--watch", func(c *config.Config) bool { return c.Watch }, true},
		{"-watch", func(c *config.Config) bool { return c.Watch }, true},
		{"--http", func(c *config.Config) bool { return c.HTTP }, true},
		{"-http", func(c *config.Config) bool { return c.HTTP }, true},
		{"--sitemap-off", func(c *config.Config) bool { return c.SitemapOff }, true},
		{"--robots-off", func(c *config.Config) bool { return c.RobotsOff }, true},
		{"--pretty-html", func(c *config.Config) bool { return c.PrettyHTML }, true},
		{"--pretty", func(c *config.Config) bool { return c.PrettyHTML }, true},
		{"--minify-all", func(c *config.Config) bool { return c.MinifyAll }, true},
		{"--minify-html", func(c *config.Config) bool { return c.MinifyHTML }, true},
		{"--minify-css", func(c *config.Config) bool { return c.MinifyCSS }, true},
		{"--minify-js", func(c *config.Config) bool { return c.MinifyJS }, true},
		{"--sourcemap", func(c *config.Config) bool { return c.SourceMap }, true},
		{"--clean", func(c *config.Config) bool { return c.Clean }, true},
		{"--quiet", func(c *config.Config) bool { return c.Quiet }, true},
		{"-q", func(c *config.Config) bool { return c.Quiet }, true},
		{"--unknown-flag", func(c *config.Config) bool { return false }, false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			cfg := &config.Config{}
			result := parseBoolFlags(tt.flag, cfg)
			if result != tt.expected {
				t.Errorf("parseBoolFlags(%q) returned %v, expected %v", tt.flag, result, tt.expected)
			}
			if tt.expected && !tt.check(cfg) {
				t.Errorf("parseBoolFlags(%q) did not set the expected config field", tt.flag)
			}
		})
	}
}

// Test parseEqualFlags with all --flag=value formats
func TestParseEqualFlags(t *testing.T) {
	tests := []struct {
		flag     string
		check    func(*config.Config) interface{}
		expected interface{}
	}{
		{"--webp-quality=80", func(c *config.Config) interface{} { return c.WebPQuality }, 80},
		{"--webp-quality=0", func(c *config.Config) interface{} { return c.WebPQuality }, 0},   // invalid, stays default
		{"--webp-quality=101", func(c *config.Config) interface{} { return c.WebPQuality }, 0}, // invalid, stays default
		{"--port=9000", func(c *config.Config) interface{} { return c.Port }, 9000},
		{"--content-dir=my-content", func(c *config.Config) interface{} { return c.ContentDir }, "my-content"},
		{"--templates-dir=my-templates", func(c *config.Config) interface{} { return c.TemplatesDir }, "my-templates"},
		{"--output-dir=my-output", func(c *config.Config) interface{} { return c.OutputDir }, "my-output"},
		{"--engine=pongo2", func(c *config.Config) interface{} { return c.Engine }, "pongo2"},
		{"--online-theme=https://example.com/theme", func(c *config.Config) interface{} { return c.OnlineTheme }, "https://example.com/theme"},
		{"--post-url-format=slug", func(c *config.Config) interface{} { return c.PostURLFormat }, "slug"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			cfg := &config.Config{}
			parseEqualFlags(tt.flag, cfg)
			if tt.check(cfg) != tt.expected {
				t.Errorf("parseEqualFlags(%q): got %v, expected %v", tt.flag, tt.check(cfg), tt.expected)
			}
		})
	}
}

// Test parseSeparateValueFlags with all --flag value formats
func TestParseSeparateValueFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		index    int
		check    func(*config.Config) interface{}
		expected interface{}
		skip     int
	}{
		{"webp-quality", []string{"--webp-quality", "75"}, 0, func(c *config.Config) interface{} { return c.WebPQuality }, 75, 1},
		{"port", []string{"--port", "3000"}, 0, func(c *config.Config) interface{} { return c.Port }, 3000, 1},
		{"content-dir", []string{"--content-dir", "custom"}, 0, func(c *config.Config) interface{} { return c.ContentDir }, "custom", 1},
		{"templates-dir", []string{"--templates-dir", "tmpl"}, 0, func(c *config.Config) interface{} { return c.TemplatesDir }, "tmpl", 1},
		{"output-dir", []string{"--output-dir", "dist"}, 0, func(c *config.Config) interface{} { return c.OutputDir }, "dist", 1},
		{"engine", []string{"--engine", "mustache"}, 0, func(c *config.Config) interface{} { return c.Engine }, "mustache", 1},
		{"online-theme", []string{"--online-theme", "http://theme.com"}, 0, func(c *config.Config) interface{} { return c.OnlineTheme }, "http://theme.com", 1},
		{"post-url-format", []string{"--post-url-format", "date"}, 0, func(c *config.Config) interface{} { return c.PostURLFormat }, "date", 1},
		{"config", []string{"--config", "myconfig.yaml"}, 0, func(c *config.Config) interface{} { return "" }, "", 1},
		{"unknown", []string{"--unknown", "value"}, 0, func(c *config.Config) interface{} { return "" }, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			skip := parseSeparateValueFlags(tt.args, tt.index, cfg)
			if skip != tt.skip {
				t.Errorf("parseSeparateValueFlags skip: got %d, expected %d", skip, tt.skip)
			}
			if tt.expected != "" && tt.check(cfg) != tt.expected {
				t.Errorf("parseSeparateValueFlags: got %v, expected %v", tt.check(cfg), tt.expected)
			}
		})
	}
}

// Test parseSeparateValueFlags at end of args (no next arg)
func TestParseSeparateValueFlagsNoNextArg(t *testing.T) {
	cfg := &config.Config{}
	skip := parseSeparateValueFlags([]string{"--port"}, 0, cfg)
	if skip != 0 {
		t.Errorf("expected skip=0 when no next arg, got %d", skip)
	}
}

// Test applyMinifyAll sets all minify flags
func TestApplyMinifyAll(t *testing.T) {
	cfg := &config.Config{MinifyAll: true}
	applyMinifyAll(cfg)

	if !cfg.MinifyHTML {
		t.Error("MinifyHTML should be true when MinifyAll is true")
	}
	if !cfg.MinifyCSS {
		t.Error("MinifyCSS should be true when MinifyAll is true")
	}
	if !cfg.MinifyJS {
		t.Error("MinifyJS should be true when MinifyAll is true")
	}
}

// Test applyMinifyAll does nothing when MinifyAll is false
func TestApplyMinifyAllFalse(t *testing.T) {
	cfg := &config.Config{MinifyAll: false}
	applyMinifyAll(cfg)

	if cfg.MinifyHTML {
		t.Error("MinifyHTML should remain false when MinifyAll is false")
	}
	if cfg.MinifyCSS {
		t.Error("MinifyCSS should remain false when MinifyAll is false")
	}
	if cfg.MinifyJS {
		t.Error("MinifyJS should remain false when MinifyAll is false")
	}
}

// Test validateRequiredFields with positional args
func TestValidateRequiredFieldsPositionalArgs(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"my-source", "my-template", "my-domain.com", "--http"}

	validateRequiredFields(args, cfg)

	if cfg.Source != "my-source" {
		t.Errorf("Source: got %q, expected %q", cfg.Source, "my-source")
	}
	if cfg.Template != "my-template" {
		t.Errorf("Template: got %q, expected %q", cfg.Template, "my-template")
	}
	if cfg.Domain != "my-domain.com" {
		t.Errorf("Domain: got %q, expected %q", cfg.Domain, "my-domain.com")
	}
}

// Test validateRequiredFields with config already set
func TestValidateRequiredFieldsAlreadySet(t *testing.T) {
	cfg := &config.Config{
		Source:   "config-source",
		Template: "config-template",
		Domain:   "config-domain.com",
	}
	args := []string{"--http"}

	validateRequiredFields(args, cfg)

	// Should not override from positional args since already set
	if cfg.Source != "config-source" {
		t.Errorf("Source should not be overridden: got %q", cfg.Source)
	}
}

// Test loadConfig with explicit --config flag
func TestLoadConfigWithConfigFlag(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom.yaml")

	yamlContent := `
source: "from-custom-config"
template: "custom-template"
domain: "custom.com"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{"--config=" + configPath}
	cfg := loadConfig(args)

	if cfg.Source != "from-custom-config" {
		t.Errorf("Source: got %q, expected %q", cfg.Source, "from-custom-config")
	}
}

// Test loadConfig with --config value format
func TestLoadConfigWithConfigFlagSeparate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "separate.yaml")

	yamlContent := `
source: "from-separate"
template: "sep-template"
domain: "sep.com"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{"--config", configPath}
	cfg := loadConfig(args)

	if cfg.Source != "from-separate" {
		t.Errorf("Source: got %q, expected %q", cfg.Source, "from-separate")
	}
}

// Test loadConfig returns default when no config file
func TestLoadConfigDefault(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	cfg := loadConfig([]string{})

	if cfg.ContentDir != "content" {
		t.Errorf("ContentDir should be default 'content', got %q", cfg.ContentDir)
	}
	if cfg.OutputDir != "output" {
		t.Errorf("OutputDir should be default 'output', got %q", cfg.OutputDir)
	}
}

// Test parseValueFlags handles equal format
func TestParseValueFlagsEqualFormat(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"--port=5000"}
	skip := parseValueFlags(args, 0, cfg)

	if skip != 0 {
		t.Errorf("parseValueFlags with = should return 0, got %d", skip)
	}
	if cfg.Port != 5000 {
		t.Errorf("Port should be 5000, got %d", cfg.Port)
	}
}

// Test parseValueFlags handles separate format
func TestParseValueFlagsSeparateFormat(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"--port", "6000"}
	skip := parseValueFlags(args, 0, cfg)

	if skip != 1 {
		t.Errorf("parseValueFlags with separate value should return 1, got %d", skip)
	}
	if cfg.Port != 6000 {
		t.Errorf("Port should be 6000, got %d", cfg.Port)
	}
}

// Test handleConfigSkip
func TestHandleConfigSkip(t *testing.T) {
	result := handleConfigSkip("--config")
	if result != 0 {
		t.Errorf("handleConfigSkip(--config) should return 0, got %d", result)
	}

	result = handleConfigSkip("--other")
	if result != 0 {
		t.Errorf("handleConfigSkip(--other) should return 0, got %d", result)
	}
}

// Test createGeneratorConfig passes all fields
func TestCreateGeneratorConfigAllFields(t *testing.T) {
	cfg := &config.Config{
		Source:        "src",
		Template:      "tmpl",
		Domain:        "example.com",
		ContentDir:    "content",
		TemplatesDir:  "templates",
		OutputDir:     "output",
		SitemapOff:    true,
		RobotsOff:     true,
		PrettyHTML:    true,
		PostURLFormat: "slug",
		MinifyHTML:    true,
		MinifyCSS:     true,
		MinifyJS:      true,
		SourceMap:     true,
		Clean:         true,
		Quiet:         true,
		Engine:        "pongo2",
	}

	genCfg := createGeneratorConfig(cfg)

	if genCfg.Source != cfg.Source {
		t.Error("Source mismatch")
	}
	if genCfg.SitemapOff != cfg.SitemapOff {
		t.Error("SitemapOff mismatch")
	}
	if genCfg.PostURLFormat != cfg.PostURLFormat {
		t.Error("PostURLFormat mismatch")
	}
	if genCfg.Engine != cfg.Engine {
		t.Error("Engine mismatch")
	}
}

// Test parseSpecialFlags returns false for unknown args
func TestParseSpecialFlagsUnknown(t *testing.T) {
	tests := []string{
		"--unknown",
		"-x",
		"--foo",
		"source",
		"",
	}

	for _, arg := range tests {
		t.Run(arg, func(t *testing.T) {
			result := parseSpecialFlags(arg)
			if result {
				t.Errorf("parseSpecialFlags(%q) should return false", arg)
			}
		})
	}
}

// Test printUsage outputs expected content
func TestPrintUsage(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	_ = w.Close()
	os.Stdout = oldStdout

	outputBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	output := string(outputBytes)

	// Verify key parts of usage output
	expectedParts := []string{
		"SSG - Static Site Generator",
		"Usage:",
		"source",
		"template",
		"domain",
		"--config",
		"--http",
		"--watch",
		"--webp",
		"--help",
		"--version",
		"--post-url-format",
	}

	for _, part := range expectedParts {
		if !strings.Contains(output, part) {
			t.Errorf("printUsage output missing %q", part)
		}
	}
}

// Test parseFlags full flow
func TestParseFlagsFullFlow(t *testing.T) {
	cfg := &config.Config{}
	args := []string{
		"source", "template", "domain.com",
		"--http", "--watch", "--zip",
		"--port=9999",
		"--engine", "pongo2",
	}

	parseFlags(args, cfg)

	if !cfg.HTTP {
		t.Error("HTTP should be true")
	}
	if !cfg.Watch {
		t.Error("Watch should be true")
	}
	if !cfg.Zip {
		t.Error("Zip should be true")
	}
	if cfg.Port != 9999 {
		t.Errorf("Port: got %d, expected 9999", cfg.Port)
	}
	if cfg.Engine != "pongo2" {
		t.Errorf("Engine: got %q, expected 'pongo2'", cfg.Engine)
	}
}
