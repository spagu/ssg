// Package generator - tests for generator
package generator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewGenerator(t *testing.T) {
	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if gen == nil {
		t.Fatal("New() returned nil generator")
	}

	if gen.config.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", gen.config.Domain)
	}

	if gen.siteData.Domain != "example.com" {
		t.Errorf("Expected siteData domain 'example.com', got '%s'", gen.siteData.Domain)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy file
	dstPath := filepath.Join(tmpDir, "dest.txt")
	gen := &Generator{}
	if err := gen.copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify copy
	copiedContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}

	if string(copiedContent) != string(content) {
		t.Errorf("Copied content mismatch: expected '%s', got '%s'", content, copiedContent)
	}
}

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory structure
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("file2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	gen := &Generator{}
	if err := gen.copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); err != nil {
		t.Error("file1.txt not copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "subdir", "file2.txt")); err != nil {
		t.Error("subdir/file2.txt not copied")
	}
}

func TestEnsureTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "templates", "test")

	cfg := Config{}
	gen := &Generator{config: cfg}

	if err := gen.ensureTemplates(templatePath); err != nil {
		t.Fatalf("ensureTemplates failed: %v", err)
	}

	// Check if templates were created
	expectedFiles := []string{"base.html", "index.html", "page.html", "post.html", "category.html"}
	for _, f := range expectedFiles {
		path := filepath.Join(templatePath, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Template %s not created: %v", f, err)
		}
	}
}
