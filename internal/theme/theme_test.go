package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertToArchiveURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "github repo",
			url:      "https://github.com/user/repo",
			expected: "https://github.com/user/repo/archive/refs/heads/main.zip",
		},
		{
			name:     "github repo with trailing slash",
			url:      "https://github.com/user/repo/",
			expected: "https://github.com/user/repo/archive/refs/heads/main.zip",
		},
		{
			name:     "github repo with .git",
			url:      "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo/archive/refs/heads/main.zip",
		},
		{
			name:     "gitlab repo",
			url:      "https://gitlab.com/user/repo",
			expected: "https://gitlab.com/user/repo/-/archive/main/archive.zip",
		},
		{
			name:     "direct zip url",
			url:      "https://example.com/theme.zip",
			expected: "https://example.com/theme.zip",
		},
		{
			name:     "direct tar.gz url",
			url:      "https://example.com/theme.tar.gz",
			expected: "https://example.com/theme.tar.gz",
		},
		{
			name:     "other url passthrough",
			url:      "https://example.com/download",
			expected: "https://example.com/download",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToArchiveURL(tt.url)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	// Write source file
	content := []byte("test content")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// Copy file
	if err := copyFile(srcFile, dstFile); err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	// Verify destination
	dst, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}

	if string(dst) != string(content) {
		t.Errorf("content mismatch: expected %s, got %s", content, dst)
	}
}

func TestCopyDir(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source structure
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	// Copy directory
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("failed to copy dir: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); err != nil {
		t.Error("file1.txt not copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "subdir", "file2.txt")); err != nil {
		t.Error("subdir/file2.txt not copied")
	}
}
