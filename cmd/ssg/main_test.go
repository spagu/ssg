package main

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

func TestParseBoolFlags(t *testing.T) {
	tests := []struct {
		flag    string
		check   func(*config.Config) bool
		handled bool
	}{
		{"--zip", func(c *config.Config) bool { return c.Zip }, true},
		{"-zip", func(c *config.Config) bool { return c.Zip }, true},
		{"--webp", func(c *config.Config) bool { return c.WebP }, true},
		{"-webp", func(c *config.Config) bool { return c.WebP }, true},
		{"--reconvert-images", func(c *config.Config) bool { return c.ReconvertImages }, true},
		{"--watch", func(c *config.Config) bool { return c.Watch }, true},
		{"-watch", func(c *config.Config) bool { return c.Watch }, true},
		{"--http", func(c *config.Config) bool { return c.HTTP }, true},
		{"-http", func(c *config.Config) bool { return c.HTTP }, true},
		{"--sitemap-off", func(c *config.Config) bool { return c.SitemapOff }, true},
		{"--robots-off", func(c *config.Config) bool { return c.RobotsOff }, true},
		{"--pretty-html", func(c *config.Config) bool { return c.PrettyHTML }, true},
		{"--pretty", func(c *config.Config) bool { return c.PrettyHTML }, true},
		{"--relative-links", func(c *config.Config) bool { return c.RelativeLinks }, true},
		{"--minify-all", func(c *config.Config) bool { return c.MinifyAll }, true},
		{"--minify-html", func(c *config.Config) bool { return c.MinifyHTML }, true},
		{"--minify-css", func(c *config.Config) bool { return c.MinifyCSS }, true},
		{"--minify-js", func(c *config.Config) bool { return c.MinifyJS }, true},
		{"--sourcemap", func(c *config.Config) bool { return c.SourceMap }, true},
		{"--clean", func(c *config.Config) bool { return c.Clean }, true},
		{"--quiet", func(c *config.Config) bool { return c.Quiet }, true},
		{"-q", func(c *config.Config) bool { return c.Quiet }, true},
		{"--unknown", func(c *config.Config) bool { return false }, false},
		{"positional-arg", func(c *config.Config) bool { return false }, false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			cfg := &config.Config{}
			got := parseBoolFlags(tt.flag, cfg)
			if got != tt.handled {
				t.Errorf("parseBoolFlags(%q) = %v, want %v", tt.flag, got, tt.handled)
			}
			if tt.handled && !tt.check(cfg) {
				t.Errorf("parseBoolFlags(%q) did not set expected field", tt.flag)
			}
		})
	}
}

func TestParseSpecialFlagsUnknown(t *testing.T) {
	tests := []string{"--unknown", "-x", "positional", ""}
	for _, arg := range tests {
		t.Run(arg, func(t *testing.T) {
			if parseSpecialFlags(arg) {
				t.Errorf("parseSpecialFlags(%q) should return false", arg)
			}
		})
	}
}

func TestParseSpecialFlagsVersion(t *testing.T) {
	if os.Getenv("TEST_SPECIAL_FLAG") == "1" {
		parseSpecialFlags(os.Getenv("TEST_FLAG_ARG"))
		return
	}

	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestParseSpecialFlagsVersion")
			cmd.Env = append(os.Environ(), "TEST_SPECIAL_FLAG=1", "TEST_FLAG_ARG="+flag)
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), "ssg version") {
				t.Errorf("expected version output, got: %s", out)
			}
		})
	}
}

func TestParseSpecialFlagsHelp(t *testing.T) {
	if os.Getenv("TEST_SPECIAL_FLAG") == "1" {
		parseSpecialFlags(os.Getenv("TEST_FLAG_ARG"))
		return
	}

	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestParseSpecialFlagsHelp")
			cmd.Env = append(os.Environ(), "TEST_SPECIAL_FLAG=1", "TEST_FLAG_ARG="+flag)
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), "SSG - Static Site Generator") {
				t.Errorf("expected usage output, got: %s", out)
			}
		})
	}
}

func TestParseEqualFlags(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		check    func(*config.Config) interface{}
		expected interface{}
	}{
		{"webp-quality valid", "--webp-quality=80", func(c *config.Config) interface{} { return c.WebPQuality }, 80},
		{"webp-quality min", "--webp-quality=1", func(c *config.Config) interface{} { return c.WebPQuality }, 1},
		{"webp-quality max", "--webp-quality=100", func(c *config.Config) interface{} { return c.WebPQuality }, 100},
		{"webp-quality too low", "--webp-quality=0", func(c *config.Config) interface{} { return c.WebPQuality }, 0},
		{"webp-quality too high", "--webp-quality=101", func(c *config.Config) interface{} { return c.WebPQuality }, 0},
		{"webp-quality non-numeric", "--webp-quality=abc", func(c *config.Config) interface{} { return c.WebPQuality }, 0},
		{"port", "--port=9000", func(c *config.Config) interface{} { return c.Port }, 9000},
		{"port non-numeric", "--port=abc", func(c *config.Config) interface{} { return c.Port }, 0},
		{"content-dir", "--content-dir=my-content", func(c *config.Config) interface{} { return c.ContentDir }, "my-content"},
		{"templates-dir", "--templates-dir=my-templates", func(c *config.Config) interface{} { return c.TemplatesDir }, "my-templates"},
		{"output-dir", "--output-dir=my-output", func(c *config.Config) interface{} { return c.OutputDir }, "my-output"},
		{"static-dir", "--static-dir=my-static", func(c *config.Config) interface{} { return c.StaticDir }, "my-static"},
		{"host", "--host=0.0.0.0", func(c *config.Config) interface{} { return c.Host }, "0.0.0.0"},
		{"engine", "--engine=pongo2", func(c *config.Config) interface{} { return c.Engine }, "pongo2"},
		{"online-theme", "--online-theme=https://example.com/t", func(c *config.Config) interface{} { return c.OnlineTheme }, "https://example.com/t"},
		{"post-url-format", "--post-url-format=slug", func(c *config.Config) interface{} { return c.PostURLFormat }, "slug"},
		{"mddb-url", "--mddb-url=http://localhost:11023", func(c *config.Config) interface{} { return c.Mddb.URL }, "http://localhost:11023"},
		{"mddb-url enables mddb", "--mddb-url=http://localhost:11023", func(c *config.Config) interface{} { return c.Mddb.Enabled }, true},
		{"mddb-key", "--mddb-key=secret123", func(c *config.Config) interface{} { return c.Mddb.APIKey }, "secret123"},
		{"mddb-collection", "--mddb-collection=blog", func(c *config.Config) interface{} { return c.Mddb.Collection }, "blog"},
		{"mddb-lang", "--mddb-lang=en_US", func(c *config.Config) interface{} { return c.Mddb.Lang }, "en_US"},
		{"mddb-timeout valid", "--mddb-timeout=60", func(c *config.Config) interface{} { return c.Mddb.Timeout }, 60},
		{"mddb-timeout zero", "--mddb-timeout=0", func(c *config.Config) interface{} { return c.Mddb.Timeout }, 0},
		{"mddb-timeout negative", "--mddb-timeout=-1", func(c *config.Config) interface{} { return c.Mddb.Timeout }, 0},
		{"mddb-batch-size valid", "--mddb-batch-size=500", func(c *config.Config) interface{} { return c.Mddb.BatchSize }, 500},
		{"mddb-batch-size zero", "--mddb-batch-size=0", func(c *config.Config) interface{} { return c.Mddb.BatchSize }, 0},
		{"mddb-protocol http", "--mddb-protocol=http", func(c *config.Config) interface{} { return c.Mddb.Protocol }, "http"},
		{"mddb-protocol grpc", "--mddb-protocol=grpc", func(c *config.Config) interface{} { return c.Mddb.Protocol }, "grpc"},
		{"mddb-protocol invalid", "--mddb-protocol=websocket", func(c *config.Config) interface{} { return c.Mddb.Protocol }, ""},
		{"mddb-watch", "--mddb-watch", func(c *config.Config) interface{} { return c.Mddb.Watch }, true},
		{"mddb-watch-interval", "--mddb-watch-interval=10", func(c *config.Config) interface{} { return c.Mddb.WatchInterval }, 10},
		{"mddb-watch-interval zero", "--mddb-watch-interval=0", func(c *config.Config) interface{} { return c.Mddb.WatchInterval }, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			parseEqualFlags(tt.flag, cfg)
			got := tt.check(cfg)
			if got != tt.expected {
				t.Errorf("parseEqualFlags(%q): got %v, want %v", tt.flag, got, tt.expected)
			}
		})
	}
}

func TestParseSeparateValueFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		idx      int
		check    func(*config.Config) interface{}
		expected interface{}
		wantSkip int
	}{
		{"webp-quality", []string{"--webp-quality", "75"}, 0, func(c *config.Config) interface{} { return c.WebPQuality }, 75, 1},
		{"webp-quality invalid", []string{"--webp-quality", "abc"}, 0, func(c *config.Config) interface{} { return c.WebPQuality }, 0, 1},
		{"webp-quality out of range", []string{"--webp-quality", "200"}, 0, func(c *config.Config) interface{} { return c.WebPQuality }, 0, 1},
		{"port", []string{"--port", "3000"}, 0, func(c *config.Config) interface{} { return c.Port }, 3000, 1},
		{"port invalid", []string{"--port", "xyz"}, 0, func(c *config.Config) interface{} { return c.Port }, 0, 1},
		{"content-dir", []string{"--content-dir", "custom"}, 0, func(c *config.Config) interface{} { return c.ContentDir }, "custom", 1},
		{"templates-dir", []string{"--templates-dir", "tmpl"}, 0, func(c *config.Config) interface{} { return c.TemplatesDir }, "tmpl", 1},
		{"output-dir", []string{"--output-dir", "dist"}, 0, func(c *config.Config) interface{} { return c.OutputDir }, "dist", 1},
		{"engine", []string{"--engine", "mustache"}, 0, func(c *config.Config) interface{} { return c.Engine }, "mustache", 1},
		{"online-theme", []string{"--online-theme", "http://t.com"}, 0, func(c *config.Config) interface{} { return c.OnlineTheme }, "http://t.com", 1},
		{"post-url-format", []string{"--post-url-format", "date"}, 0, func(c *config.Config) interface{} { return c.PostURLFormat }, "date", 1},
		{"config skip", []string{"--config", "myconfig.yaml"}, 0, func(c *config.Config) interface{} { return "" }, "", 1},
		{"mddb-url", []string{"--mddb-url", "http://localhost:11023"}, 0, func(c *config.Config) interface{} { return c.Mddb.URL }, "http://localhost:11023", 1},
		{"mddb-url enables", []string{"--mddb-url", "http://localhost:11023"}, 0, func(c *config.Config) interface{} { return c.Mddb.Enabled }, true, 1},
		{"mddb-key", []string{"--mddb-key", "secret"}, 0, func(c *config.Config) interface{} { return c.Mddb.APIKey }, "secret", 1},
		{"mddb-collection", []string{"--mddb-collection", "posts"}, 0, func(c *config.Config) interface{} { return c.Mddb.Collection }, "posts", 1},
		{"mddb-lang", []string{"--mddb-lang", "pl_PL"}, 0, func(c *config.Config) interface{} { return c.Mddb.Lang }, "pl_PL", 1},
		{"mddb-timeout", []string{"--mddb-timeout", "45"}, 0, func(c *config.Config) interface{} { return c.Mddb.Timeout }, 45, 1},
		{"mddb-timeout invalid", []string{"--mddb-timeout", "abc"}, 0, func(c *config.Config) interface{} { return c.Mddb.Timeout }, 0, 1},
		{"mddb-timeout zero", []string{"--mddb-timeout", "0"}, 0, func(c *config.Config) interface{} { return c.Mddb.Timeout }, 0, 1},
		{"mddb-batch-size", []string{"--mddb-batch-size", "200"}, 0, func(c *config.Config) interface{} { return c.Mddb.BatchSize }, 200, 1},
		{"mddb-batch-size invalid", []string{"--mddb-batch-size", "abc"}, 0, func(c *config.Config) interface{} { return c.Mddb.BatchSize }, 0, 1},
		{"mddb-protocol http", []string{"--mddb-protocol", "http"}, 0, func(c *config.Config) interface{} { return c.Mddb.Protocol }, "http", 1},
		{"mddb-protocol grpc", []string{"--mddb-protocol", "grpc"}, 0, func(c *config.Config) interface{} { return c.Mddb.Protocol }, "grpc", 1},
		{"mddb-protocol invalid", []string{"--mddb-protocol", "ws"}, 0, func(c *config.Config) interface{} { return c.Mddb.Protocol }, "", 1},
		{"mddb-watch-interval", []string{"--mddb-watch-interval", "15"}, 0, func(c *config.Config) interface{} { return c.Mddb.WatchInterval }, 15, 1},
		{"mddb-watch-interval invalid", []string{"--mddb-watch-interval", "abc"}, 0, func(c *config.Config) interface{} { return c.Mddb.WatchInterval }, 0, 1},
		{"unknown flag", []string{"--unknown", "value"}, 0, func(c *config.Config) interface{} { return "" }, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			gotSkip := parseSeparateValueFlags(tt.args, tt.idx, cfg)
			if gotSkip != tt.wantSkip {
				t.Errorf("skip: got %d, want %d", gotSkip, tt.wantSkip)
			}
			got := tt.check(cfg)
			if got != tt.expected {
				t.Errorf("value: got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseSeparateValueFlagsNoNextArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"config at end", []string{"--config"}},
		{"port at end", []string{"--port"}},
		{"unknown at end", []string{"--unknown"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			skip := parseSeparateValueFlags(tt.args, 0, cfg)
			if skip != 0 {
				t.Errorf("expected skip=0 for last arg, got %d", skip)
			}
		})
	}
}

func TestHandleConfigSkip(t *testing.T) {
	tests := []struct {
		arg      string
		expected int
	}{
		{"--config", 0},
		{"--other", 0},
		{"--port", 0},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got := handleConfigSkip(tt.arg)
			if got != tt.expected {
				t.Errorf("handleConfigSkip(%q) = %d, want %d", tt.arg, got, tt.expected)
			}
		})
	}
}

func TestLoadConfigWithEqualFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "custom.yaml")
	if err := os.WriteFile(cfgPath, []byte("source: from-equal\ntemplate: t\ndomain: d.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig([]string{"--config=" + cfgPath})
	if cfg.Source != "from-equal" {
		t.Errorf("Source: got %q, want %q", cfg.Source, "from-equal")
	}
}

func TestLoadConfigWithSeparateFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "separate.yaml")
	if err := os.WriteFile(cfgPath, []byte("source: from-sep\ntemplate: t\ndomain: d.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig([]string{"--config", cfgPath})
	if cfg.Source != "from-sep" {
		t.Errorf("Source: got %q, want %q", cfg.Source, "from-sep")
	}
}

func TestLoadConfigDefault(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cfg := loadConfig([]string{})
	if cfg.ContentDir != "content" {
		t.Errorf("ContentDir: got %q, want %q", cfg.ContentDir, "content")
	}
	if cfg.OutputDir != "output" {
		t.Errorf("OutputDir: got %q, want %q", cfg.OutputDir, "output")
	}
	if cfg.Port != 8888 {
		t.Errorf("Port: got %d, want 8888", cfg.Port)
	}
}

func TestLoadConfigAutoDetect(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".ssg.yaml")
	if err := os.WriteFile(cfgPath, []byte("source: auto-detect\ntemplate: t\ndomain: d.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cfg := loadConfig([]string{})
	if cfg.Source != "auto-detect" {
		t.Errorf("Source: got %q, want %q", cfg.Source, "auto-detect")
	}
}

func TestValidateRequiredFieldsFromPositional(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"my-source", "my-template", "my-domain.com", "--http"}
	validateRequiredFields(args, cfg)

	if cfg.Source != "my-source" {
		t.Errorf("Source: got %q, want %q", cfg.Source, "my-source")
	}
	if cfg.Template != "my-template" {
		t.Errorf("Template: got %q, want %q", cfg.Template, "my-template")
	}
	if cfg.Domain != "my-domain.com" {
		t.Errorf("Domain: got %q, want %q", cfg.Domain, "my-domain.com")
	}
}

func TestValidateRequiredFieldsAlreadySet(t *testing.T) {
	cfg := &config.Config{
		Source:   "cfg-src",
		Template: "cfg-tmpl",
		Domain:   "cfg.com",
	}
	validateRequiredFields([]string{"--http"}, cfg)

	if cfg.Source != "cfg-src" {
		t.Errorf("Source should not be overridden: got %q", cfg.Source)
	}
}

func TestValidateRequiredFieldsMissingExits(t *testing.T) {
	if os.Getenv("TEST_VALIDATE_EXIT") == "1" {
		cfg := &config.Config{}
		validateRequiredFields([]string{"only-one"}, cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestValidateRequiredFieldsMissingExits")
	cmd.Env = append(os.Environ(), "TEST_VALIDATE_EXIT=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected process to exit with error")
	}
}

func TestApplyMinifyAllTrue(t *testing.T) {
	cfg := &config.Config{MinifyAll: true}
	applyMinifyAll(cfg)

	if !cfg.MinifyHTML || !cfg.MinifyCSS || !cfg.MinifyJS {
		t.Error("all minify flags should be true when MinifyAll is true")
	}
}

func TestApplyMinifyAllFalse(t *testing.T) {
	cfg := &config.Config{MinifyAll: false}
	applyMinifyAll(cfg)

	if cfg.MinifyHTML || cfg.MinifyCSS || cfg.MinifyJS {
		t.Error("minify flags should remain false when MinifyAll is false")
	}
}

func TestApplyMinifyAllPreservesExisting(t *testing.T) {
	cfg := &config.Config{MinifyAll: false, MinifyHTML: true}
	applyMinifyAll(cfg)

	if !cfg.MinifyHTML {
		t.Error("existing MinifyHTML=true should be preserved")
	}
	if cfg.MinifyCSS {
		t.Error("MinifyCSS should remain false")
	}
}

func TestHasChangesDetectsModified(t *testing.T) {
	tmpDir := t.TempDir()
	pastTime := time.Now().Add(-2 * time.Second)

	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if !hasChanges([]string{tmpDir}, pastTime) {
		t.Error("expected hasChanges to return true for recently modified file")
	}
}

func TestHasChangesNoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	futureTime := time.Now().Add(10 * time.Second)
	if hasChanges([]string{tmpDir}, futureTime) {
		t.Error("expected hasChanges to return false when no files modified after lastBuild")
	}
}

func TestHasChangesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	if hasChanges([]string{tmpDir}, time.Now().Add(-1*time.Second)) {
		t.Error("expected hasChanges to return false for empty directory")
	}
}

func TestHasChangesNonExistentDir(t *testing.T) {
	if hasChanges([]string{"/nonexistent/dir"}, time.Now().Add(-1*time.Second)) {
		t.Error("expected hasChanges to return false for nonexistent directory")
	}
}

func TestHasChangesMultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	pastTime := time.Now().Add(-2 * time.Second)
	if err := os.WriteFile(filepath.Join(dir2, "changed.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if !hasChanges([]string{dir1, dir2}, pastTime) {
		t.Error("expected hasChanges to detect change in second directory")
	}
}

func TestCreateZip(t *testing.T) {
	srcDir := t.TempDir()

	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	if err := createZip(srcDir, zipPath); err != nil {
		t.Fatalf("createZip failed: %v", err)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer func() { _ = r.Close() }()

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	if !fileNames["file1.txt"] {
		t.Error("zip missing file1.txt")
	}
	if !fileNames["subdir/"] {
		t.Error("zip missing subdir/")
	}
	if !fileNames["subdir/file2.txt"] {
		t.Error("zip missing subdir/file2.txt")
	}
}

func TestCreateZipFileContent(t *testing.T) {
	srcDir := t.TempDir()
	expected := "test content here"
	if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "content.zip")
	if err := createZip(srcDir, zipPath); err != nil {
		t.Fatal(err)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if f.Name == "data.txt" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != expected {
				t.Errorf("zip content: got %q, want %q", string(content), expected)
			}
			return
		}
	}
	t.Error("data.txt not found in zip")
}

func TestCreateZipInvalidSource(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "fail.zip")
	err := createZip("/nonexistent/source/dir", zipPath)
	if err == nil {
		t.Error("expected error for nonexistent source directory")
	}
}

func TestCreateZipInvalidDest(t *testing.T) {
	srcDir := t.TempDir()
	err := createZip(srcDir, "/nonexistent/path/fail.zip")
	if err == nil {
		t.Error("expected error for invalid destination path")
	}
}

func TestPrintUsage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	_ = w.Close()
	os.Stdout = oldStdout

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	output := string(out)

	requiredParts := []string{
		"SSG - Static Site Generator",
		"Usage:",
		"--config",
		"--http",
		"--watch",
		"--webp",
		"--help",
		"--version",
		"--mddb-url",
		"--mddb-collection",
		"--post-url-format",
		"--engine",
		"--online-theme",
		"--minify-all",
		"--quiet",
		"--clean",
	}

	for _, part := range requiredParts {
		if !strings.Contains(output, part) {
			t.Errorf("printUsage output missing %q", part)
		}
	}
}

func TestParseFlagsFullFlow(t *testing.T) {
	cfg := &config.Config{}
	args := []string{
		"source", "template", "domain.com",
		"--http", "--watch", "--zip", "--webp",
		"--port=9999",
		"--engine", "pongo2",
		"--content-dir=custom-content",
		"--minify-all",
		"--quiet",
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
	if !cfg.WebP {
		t.Error("WebP should be true")
	}
	if cfg.Port != 9999 {
		t.Errorf("Port: got %d, want 9999", cfg.Port)
	}
	if cfg.Engine != "pongo2" {
		t.Errorf("Engine: got %q, want pongo2", cfg.Engine)
	}
	if cfg.ContentDir != "custom-content" {
		t.Errorf("ContentDir: got %q, want custom-content", cfg.ContentDir)
	}
	if !cfg.MinifyAll {
		t.Error("MinifyAll should be true")
	}
	if !cfg.Quiet {
		t.Error("Quiet should be true")
	}
}

func TestParseValueFlagsEqualFormat(t *testing.T) {
	cfg := &config.Config{}
	skip := parseValueFlags([]string{"--port=5000"}, 0, cfg)
	if skip != 0 {
		t.Errorf("skip: got %d, want 0", skip)
	}
	if cfg.Port != 5000 {
		t.Errorf("Port: got %d, want 5000", cfg.Port)
	}
}

func TestParseValueFlagsSeparateFormat(t *testing.T) {
	cfg := &config.Config{}
	skip := parseValueFlags([]string{"--port", "6000"}, 0, cfg)
	if skip != 1 {
		t.Errorf("skip: got %d, want 1", skip)
	}
	if cfg.Port != 6000 {
		t.Errorf("Port: got %d, want 6000", cfg.Port)
	}
}

func TestCreateGeneratorConfigAllFields(t *testing.T) {
	cfg := &config.Config{
		Source:        "src",
		Template:      "tmpl",
		Domain:        "example.com",
		ContentDir:    "content",
		TemplatesDir:  "templates",
		OutputDir:     "output",
		StaticDir:     "static",
		SitemapOff:    true,
		RobotsOff:     true,
		PrettyHTML:    true,
		PostURLFormat: "slug",
		RelativeLinks: true,
		MinifyHTML:    true,
		MinifyCSS:     true,
		MinifyJS:      true,
		SourceMap:     true,
		Clean:         true,
		Quiet:         true,
		Engine:        "pongo2",
		Mddb: config.MddbConfig{
			Enabled:       true,
			URL:           "http://localhost:11023",
			Protocol:      "grpc",
			APIKey:        "key123",
			Collection:    "blog",
			Lang:          "en_US",
			Timeout:       60,
			BatchSize:     500,
			Watch:         true,
			WatchInterval: 15,
		},
	}

	genCfg := createGeneratorConfig(cfg)

	if genCfg.Source != "src" {
		t.Error("Source mismatch")
	}
	if genCfg.Template != "tmpl" {
		t.Error("Template mismatch")
	}
	if genCfg.Domain != "example.com" {
		t.Error("Domain mismatch")
	}
	if genCfg.ContentDir != "content" {
		t.Error("ContentDir mismatch")
	}
	if genCfg.TemplatesDir != "templates" {
		t.Error("TemplatesDir mismatch")
	}
	if genCfg.OutputDir != "output" {
		t.Error("OutputDir mismatch")
	}
	if genCfg.StaticDir != "static" {
		t.Error("StaticDir mismatch")
	}
	if !genCfg.SitemapOff {
		t.Error("SitemapOff mismatch")
	}
	if !genCfg.RobotsOff {
		t.Error("RobotsOff mismatch")
	}
	if !genCfg.PrettyHTML {
		t.Error("PrettyHTML mismatch")
	}
	if genCfg.PostURLFormat != "slug" {
		t.Error("PostURLFormat mismatch")
	}
	if !genCfg.RelativeLinks {
		t.Error("RelativeLinks mismatch")
	}
	if !genCfg.MinifyHTML {
		t.Error("MinifyHTML mismatch")
	}
	if !genCfg.MinifyCSS {
		t.Error("MinifyCSS mismatch")
	}
	if !genCfg.MinifyJS {
		t.Error("MinifyJS mismatch")
	}
	if !genCfg.SourceMap {
		t.Error("SourceMap mismatch")
	}
	if !genCfg.Clean {
		t.Error("Clean mismatch")
	}
	if !genCfg.Quiet {
		t.Error("Quiet mismatch")
	}
	if genCfg.Engine != "pongo2" {
		t.Error("Engine mismatch")
	}
	if !genCfg.Mddb.Enabled {
		t.Error("Mddb.Enabled mismatch")
	}
	if genCfg.Mddb.URL != "http://localhost:11023" {
		t.Error("Mddb.URL mismatch")
	}
	if genCfg.Mddb.Protocol != "grpc" {
		t.Error("Mddb.Protocol mismatch")
	}
	if genCfg.Mddb.APIKey != "key123" {
		t.Error("Mddb.APIKey mismatch")
	}
	if genCfg.Mddb.Collection != "blog" {
		t.Error("Mddb.Collection mismatch")
	}
	if genCfg.Mddb.Lang != "en_US" {
		t.Error("Mddb.Lang mismatch")
	}
	if genCfg.Mddb.Timeout != 60 {
		t.Error("Mddb.Timeout mismatch")
	}
	if genCfg.Mddb.BatchSize != 500 {
		t.Error("Mddb.BatchSize mismatch")
	}
	if !genCfg.Mddb.Watch {
		t.Error("Mddb.Watch mismatch")
	}
	if genCfg.Mddb.WatchInterval != 15 {
		t.Error("Mddb.WatchInterval mismatch")
	}
}

func TestCreateGeneratorConfigShortcodes(t *testing.T) {
	cfg := &config.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		Shortcodes: []config.Shortcode{
			{
				Name:     "banner",
				Type:     "ad",
				Template: "banner.html",
				Title:    "Ad Title",
				Text:     "Ad Text",
				Url:      "https://example.com",
				Logo:     "logo.png",
				Legal:    "18+",
				Ranking:  4.5,
				Tags:     []string{"game", "public"},
				Data:     map[string]string{"key": "val"},
			},
			{
				Name:     "simple",
				Template: "simple.html",
			},
		},
	}

	genCfg := createGeneratorConfig(cfg)

	if len(genCfg.Shortcodes) != 2 {
		t.Fatalf("expected 2 shortcodes, got %d", len(genCfg.Shortcodes))
	}

	sc := genCfg.Shortcodes[0]
	if sc.Name != "banner" {
		t.Error("shortcode Name mismatch")
	}
	if sc.Type != "ad" {
		t.Error("shortcode Type mismatch")
	}
	if sc.Template != "banner.html" {
		t.Error("shortcode Template mismatch")
	}
	if sc.Title != "Ad Title" {
		t.Error("shortcode Title mismatch")
	}
	if sc.Text != "Ad Text" {
		t.Error("shortcode Text mismatch")
	}
	if sc.Url != "https://example.com" {
		t.Error("shortcode Url mismatch")
	}
	if sc.Logo != "logo.png" {
		t.Error("shortcode Logo mismatch")
	}
	if sc.Legal != "18+" {
		t.Error("shortcode Legal mismatch")
	}
	if sc.Ranking != 4.5 {
		t.Error("shortcode Ranking mismatch")
	}
	if len(sc.Tags) != 2 || sc.Tags[0] != "game" {
		t.Error("shortcode Tags mismatch")
	}
	if sc.Data["key"] != "val" {
		t.Error("shortcode Data mismatch")
	}
}

func TestBuildWithRealGenerator(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><head></head><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}Index{{end}}`,
		"page.html":     `{{define "content"}}Page{{end}}`,
		"post.html":     `{{define "content"}}Post{{end}}`,
		"category.html": `{{define "content"}}Cat{{end}}`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	genCfg := generator.Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "test.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}

	cfg := &config.Config{
		OutputDir: outputDir,
		Quiet:     true,
	}

	if err := build(genCfg, cfg); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	indexPath := filepath.Join(outputDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("expected index.html to be generated")
	}
}

func TestBuildWithZip(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><head></head><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}Index{{end}}`,
		"page.html":     `{{define "content"}}Page{{end}}`,
		"post.html":     `{{define "content"}}Post{{end}}`,
		"category.html": `{{define "content"}}Cat{{end}}`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	genCfg := generator.Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "test.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}

	cfg := &config.Config{
		Domain:    "test.com",
		OutputDir: outputDir,
		Zip:       true,
		Quiet:     true,
	}

	if err := build(genCfg, cfg); err != nil {
		t.Fatalf("build with zip failed: %v", err)
	}

	zipPath := filepath.Join(tmpDir, "test.com.zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		t.Error("expected test.com.zip to be created")
	}
}

func TestBuildWithInvalidConfig(t *testing.T) {
	genCfg := generator.Config{
		Source:       "nonexistent",
		Template:     "nonexistent",
		Domain:       "test.com",
		ContentDir:   "/nonexistent/content",
		TemplatesDir: "/nonexistent/templates",
		OutputDir:    "/tmp/ssg-test-output",
		Quiet:        true,
	}

	cfg := &config.Config{Quiet: true}
	err := build(genCfg, cfg)
	if err == nil {
		t.Error("expected build to fail with invalid config")
	}
}

func TestRunInitialBuildSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}

	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}
	cfg := &config.Config{OutputDir: outputDir, Quiet: true}

	if !runInitialBuild(genCfg, cfg) {
		t.Error("expected runInitialBuild to succeed")
	}
}

func TestRunInitialBuildFailure(t *testing.T) {
	genCfg := generator.Config{
		Source:       "nope",
		Template:     "nope",
		Domain:       "d.com",
		ContentDir:   "/nonexistent",
		TemplatesDir: "/nonexistent",
		OutputDir:    "/tmp/ssg-fail-output",
		Quiet:        true,
	}
	cfg := &config.Config{Quiet: true}

	if runInitialBuild(genCfg, cfg) {
		t.Error("expected runInitialBuild to fail")
	}
}

func TestRunInitialBuildFailureVerbose(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	genCfg := generator.Config{
		Source:       "nope",
		Template:     "nope",
		Domain:       "d.com",
		ContentDir:   "/nonexistent",
		TemplatesDir: "/nonexistent",
		OutputDir:    "/tmp/ssg-fail-output",
	}
	cfg := &config.Config{Quiet: false}

	result := runInitialBuild(genCfg, cfg)

	_ = w.Close()
	os.Stderr = oldStderr
	out, _ := io.ReadAll(r)

	if result {
		t.Error("expected failure")
	}
	if !strings.Contains(string(out), "Error") {
		t.Error("expected error message on stderr")
	}
}

func TestSetupTemplateEngineEmpty(t *testing.T) {
	cfg := &config.Config{Engine: ""}
	setupTemplateEngine(cfg)
}

func TestSetupTemplateEngineValid(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config.Config{Engine: "go"}
	setupTemplateEngine(cfg)

	_ = w.Close()
	os.Stdout = oldStdout
}

func TestSetupTemplateEngineInvalid(t *testing.T) {
	if os.Getenv("TEST_ENGINE_EXIT") == "1" {
		cfg := &config.Config{Engine: "nonexistent-engine-xyz"}
		setupTemplateEngine(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSetupTemplateEngineInvalid")
	cmd.Env = append(os.Environ(), "TEST_ENGINE_EXIT=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected exit for invalid engine")
	}
}

func TestDownloadOnlineThemeEmpty(t *testing.T) {
	cfg := &config.Config{OnlineTheme: ""}
	downloadOnlineTheme(cfg)
}

func TestRebuildOnChange(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}
	cfg := &config.Config{OutputDir: outputDir, Quiet: true}

	result := rebuildOnChange(genCfg, cfg)
	if result.IsZero() {
		t.Error("expected non-zero time from rebuildOnChange")
	}
}

func TestRebuildOnChangeFailure(t *testing.T) {
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	genCfg := generator.Config{
		Source:       "nope",
		ContentDir:   "/nonexistent",
		TemplatesDir: "/nonexistent",
		OutputDir:    "/tmp/ssg-fail",
	}
	cfg := &config.Config{Quiet: false}

	result := rebuildOnChange(genCfg, cfg)

	_ = w.Close()
	os.Stderr = oldStderr

	if result.IsZero() {
		t.Error("expected non-zero time even on failure")
	}
}

func TestRunWatchOrServeNoAction(t *testing.T) {
	genCfg := generator.Config{}
	cfg := &config.Config{
		Watch: false,
		HTTP:  false,
		Mddb:  config.MddbConfig{Watch: false, Enabled: false},
	}
	runWatchOrServe(genCfg, cfg)
}

func TestLoadConfigFromYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".ssg.yaml")

	yamlContent := `
source: "yaml-source"
template: "yaml-template"
domain: "yaml.com"
pretty_html: true
quiet: true
webp_quality: 85
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cfg := loadConfig([]string{})

	if cfg.Source != "yaml-source" {
		t.Errorf("Source: got %q, want yaml-source", cfg.Source)
	}
	if !cfg.PrettyHTML {
		t.Error("PrettyHTML should be true")
	}
	if cfg.WebPQuality != 85 {
		t.Errorf("WebPQuality: got %d, want 85", cfg.WebPQuality)
	}
}

func TestStartServer(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	go startServer(tmpDir, "127.0.0.1", 0, true)
	time.Sleep(100 * time.Millisecond)
}

func TestBuildWithWebP(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not available")
	}
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "simple")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	genCfg := generator.Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "test.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}

	cfg := &config.Config{
		OutputDir:   outputDir,
		WebP:        true,
		WebPQuality: 60,
		Quiet:       true,
	}

	if err := build(genCfg, cfg); err != nil {
		t.Fatalf("build with webp failed: %v", err)
	}
}

func TestBuildWithZipVerbose(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        false,
	}
	cfg := &config.Config{
		Domain:    "d.com",
		OutputDir: outputDir,
		Zip:       true,
		Quiet:     false,
	}

	err := build(genCfg, cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
}

func TestRunInitialBuildSuccessVerbose(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
	}
	cfg := &config.Config{OutputDir: outputDir, Quiet: false}

	result := runInitialBuild(genCfg, cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	if !result {
		t.Error("expected success")
	}
}

func TestRebuildOnChangeVerbose(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
	}
	cfg := &config.Config{OutputDir: outputDir, Quiet: false}

	result := rebuildOnChange(genCfg, cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	if result.IsZero() {
		t.Error("expected non-zero time")
	}
}

func TestSetupTemplateEngineValidVerbose(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config.Config{Engine: "go", Quiet: false}
	setupTemplateEngine(cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "Using template engine") {
		t.Error("expected verbose output about template engine")
	}
}

func TestLoadConfigInvalidFileExits(t *testing.T) {
	if os.Getenv("TEST_LOAD_INVALID") == "1" {
		loadConfig([]string{"--config=/nonexistent/path/config.yaml"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestLoadConfigInvalidFileExits")
	cmd.Env = append(os.Environ(), "TEST_LOAD_INVALID=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected exit for invalid config file")
	}
}

func TestStartServerVerbose(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	go startServer(tmpDir, "127.0.0.1", 0, false)
	time.Sleep(100 * time.Millisecond)

	_ = w.Close()
	os.Stdout = oldStdout
}

func TestBuildWithWebPVerbose(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not available")
	}
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(sourceDir, "metadata.json"),
		[]byte(`{"categories":[],"media":[],"users":[]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
	}

	cfg := &config.Config{
		OutputDir:   outputDir,
		WebP:        true,
		WebPQuality: 60,
		Quiet:       false,
	}

	err := build(genCfg, cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("build with webp verbose failed: %v", err)
	}
}

func TestSetupTemplateEngineQuiet(t *testing.T) {
	cfg := &config.Config{Engine: "go", Quiet: true}
	setupTemplateEngine(cfg)
}

func TestParseFlagsWithSpecialFlagInMiddle(t *testing.T) {
	if os.Getenv("TEST_PARSE_SPECIAL") == "1" {
		cfg := &config.Config{}
		parseFlags([]string{"--http", "--version"}, cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestParseFlagsWithSpecialFlagInMiddle")
	cmd.Env = append(os.Environ(), "TEST_PARSE_SPECIAL=1")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "ssg version") {
		t.Errorf("expected version output, got: %s", out)
	}
}

func TestBuildGenerateError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte("invalid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}
	cfg := &config.Config{Quiet: true}

	err := build(genCfg, cfg)
	if err == nil {
		t.Error("expected build to fail with invalid metadata")
	}
	if !strings.Contains(err.Error(), "generating site") {
		t.Errorf("expected 'generating site' error, got: %v", err)
	}
}

func TestRunWatchOrServeHTTPOnly(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	genCfg := generator.Config{}
	cfg := &config.Config{
		Watch:     false,
		HTTP:      true,
		OutputDir: tmpDir,
		Port:      0,
		Quiet:     true,
		Mddb:      config.MddbConfig{Watch: false, Enabled: false},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runWatchOrServe(genCfg, cfg)
	}()

	time.Sleep(100 * time.Millisecond)
}

func TestCreateGeneratorConfigNoShortcodes(t *testing.T) {
	cfg := &config.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
	}

	genCfg := createGeneratorConfig(cfg)
	if len(genCfg.Shortcodes) != 0 {
		t.Errorf("expected 0 shortcodes, got %d", len(genCfg.Shortcodes))
	}
}

func TestDownloadOnlineThemeInvalidExits(t *testing.T) {
	if os.Getenv("TEST_DOWNLOAD_EXIT") == "1" {
		cfg := &config.Config{
			OnlineTheme:  "http://invalid-nonexistent-domain-xyz.test/theme.zip",
			TemplatesDir: t.TempDir(),
			Template:     "test",
			Quiet:        true,
		}
		downloadOnlineTheme(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestDownloadOnlineThemeInvalidExits")
	cmd.Env = append(os.Environ(), "TEST_DOWNLOAD_EXIT=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected exit for invalid theme URL")
	}
}

func TestBuildInitError(t *testing.T) {
	tmpDir := t.TempDir()
	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	if err := os.MkdirAll(filepath.Join(sourceDir, "pages"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "posts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(`{"categories":[],"media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "base.html"), []byte(`{{define "broken" }}`), 0644); err != nil {
		t.Fatal(err)
	}

	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}
	cfg := &config.Config{Quiet: true}

	err := build(genCfg, cfg)
	if err == nil {
		t.Error("expected build init error")
	}
}

func TestCreateZipEmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "empty.zip")

	if err := createZip(srcDir, zipPath); err != nil {
		t.Fatalf("createZip on empty dir failed: %v", err)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	if len(r.File) != 0 {
		t.Errorf("expected 0 entries in zip, got %d", len(r.File))
	}
}

func TestCreateZipNestedDirs(t *testing.T) {
	srcDir := t.TempDir()
	nested := filepath.Join(srcDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("deep"), 0644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "nested.zip")
	if err := createZip(srcDir, zipPath); err != nil {
		t.Fatal(err)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	found := false
	for _, f := range r.File {
		if f.Name == "a/b/c/deep.txt" {
			found = true
		}
	}
	if !found {
		t.Error("expected a/b/c/deep.txt in zip")
	}
}

func TestValidateRequiredFieldsSkipsFlags(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"--http", "src", "--watch", "tmpl", "dom.com", "--zip"}
	validateRequiredFields(args, cfg)

	if cfg.Source != "src" {
		t.Errorf("Source: got %q, want src", cfg.Source)
	}
	if cfg.Template != "tmpl" {
		t.Errorf("Template: got %q, want tmpl", cfg.Template)
	}
	if cfg.Domain != "dom.com" {
		t.Errorf("Domain: got %q, want dom.com", cfg.Domain)
	}
}

func TestBuildWithWebPAndImages(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not available")
	}
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "src")
	for _, dir := range []string{
		filepath.Join(sourceDir, "pages"),
		filepath.Join(sourceDir, "posts"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(`{"categories":[],"media":[],"users":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	templateDir := filepath.Join(templatesDir, "tmpl")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{.Content}}</body></html>`,
		"index.html":    `{{define "content"}}I{{end}}`,
		"page.html":     `{{define "content"}}P{{end}}`,
		"post.html":     `{{define "content"}}Po{{end}}`,
		"category.html": `{{define "content"}}C{{end}}`,
	} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	outputDir := filepath.Join(tmpDir, "output")
	genCfg := generator.Config{
		Source:       "src",
		Template:     "tmpl",
		Domain:       "d.com",
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
		Quiet:        true,
	}

	cfg := &config.Config{
		OutputDir:       outputDir,
		WebP:            true,
		WebPQuality:     80,
		ReconvertImages: true,
		Quiet:           false,
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	_, w1, _ := os.Pipe()
	_, w2, _ := os.Pipe()
	os.Stdout = w1
	os.Stderr = w2

	_ = build(genCfg, cfg)

	_ = w1.Close()
	_ = w2.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
}

func TestParseFlagsIgnoresPositional(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"source", "template", "domain.com"}
	parseFlags(args, cfg)

	if cfg.HTTP || cfg.Watch || cfg.Zip {
		t.Error("no bool flags should be set from positional args")
	}
}

func TestHasChangesSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	pastTime := time.Now().Add(-2 * time.Second)
	if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if !hasChanges([]string{tmpDir}, pastTime) {
		t.Error("expected hasChanges to detect file in subdirectory")
	}
}

func TestDownloadOnlineThemeVerbose(t *testing.T) {
	if os.Getenv("TEST_DOWNLOAD_VERBOSE") == "1" {
		cfg := &config.Config{
			OnlineTheme:  "http://invalid-nonexistent-domain-xyz.test/theme.zip",
			TemplatesDir: os.TempDir(),
			Template:     "test",
			Quiet:        false,
		}
		downloadOnlineTheme(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestDownloadOnlineThemeVerbose")
	cmd.Env = append(os.Environ(), "TEST_DOWNLOAD_VERBOSE=1")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Downloading theme") {
		t.Errorf("expected download message, got: %s", out)
	}
}

// TestWarnUnimplementedFlags verifies GO-004: --sourcemap is announced as a
// no-op instead of being silently ignored, and stays silent when unset/quiet.
func TestWarnUnimplementedFlags(t *testing.T) {
	capture := func(cfg *config.Config) string {
		old := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		warnUnimplementedFlags(cfg)
		_ = w.Close()
		os.Stderr = old
		out, _ := io.ReadAll(r)
		return string(out)
	}

	if got := capture(&config.Config{SourceMap: true}); !strings.Contains(got, "sourcemap") {
		t.Errorf("expected a sourcemap warning, got %q", got)
	}
	if got := capture(&config.Config{SourceMap: false}); got != "" {
		t.Errorf("expected no warning when --sourcemap unset, got %q", got)
	}
	if got := capture(&config.Config{SourceMap: true, Quiet: true}); got != "" {
		t.Errorf("expected no warning in quiet mode, got %q", got)
	}
}

// TestResolveListenAddr verifies SEC-012: the dev server defaults to loopback
// and only 0.0.0.0 is reported as exposing all interfaces.
func TestResolveListenAddr(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		port        int
		wantAddr    string
		wantURL     string
		wantExposed bool
	}{
		{"empty defaults to loopback", "", 8888, "127.0.0.1:8888", "http://127.0.0.1:8888", false},
		{"explicit loopback", "127.0.0.1", 3000, "127.0.0.1:3000", "http://127.0.0.1:3000", false},
		{"all interfaces flagged", "0.0.0.0", 8080, "0.0.0.0:8080", "http://127.0.0.1:8080", true},
		{"custom host", "192.168.1.5", 9000, "192.168.1.5:9000", "http://192.168.1.5:9000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, url, exposed := resolveListenAddr(tt.host, tt.port)
			if addr != tt.wantAddr || url != tt.wantURL || exposed != tt.wantExposed {
				t.Errorf("resolveListenAddr(%q,%d) = (%q,%q,%v), want (%q,%q,%v)",
					tt.host, tt.port, addr, url, exposed, tt.wantAddr, tt.wantURL, tt.wantExposed)
			}
		})
	}
}
