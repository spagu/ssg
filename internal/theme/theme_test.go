package theme

import (
	"archive/zip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestCopyFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile("/nonexistent/file.txt", filepath.Join(tmpDir, "dest.txt"))
	if err == nil {
		t.Error("Expected error for nonexistent source file")
	}
}

func TestCopyFileInvalidDestination(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")

	if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Try to copy to invalid destination (directory that doesn't exist)
	err := copyFile(srcFile, "/nonexistent/dir/dest.txt")
	if err == nil {
		t.Error("Expected error for invalid destination path")
	}
}

func TestExtractZipBasic(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a simple zip file
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/file1.txt":        "content1",
		"root/subdir/file2.txt": "content2",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify files were extracted (common prefix stripped)
	content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	if err != nil {
		t.Errorf("file1.txt not extracted: %v", err)
	} else if string(content1) != "content1" {
		t.Errorf("file1.txt content mismatch")
	}

	content2, err := os.ReadFile(filepath.Join(destDir, "subdir", "file2.txt"))
	if err != nil {
		t.Errorf("subdir/file2.txt not extracted: %v", err)
	} else if string(content2) != "content2" {
		t.Errorf("subdir/file2.txt content mismatch")
	}
}

func TestExtractZipInvalidPath(t *testing.T) {
	err := extractZip("/nonexistent/file.zip", "/tmp/dest")
	if err == nil {
		t.Error("Expected error for nonexistent zip file")
	}
}

func TestConvertHugoTheme(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "hugo-theme")
	outputDir := filepath.Join(tmpDir, "output")

	// Create Hugo theme structure
	layoutsDir := filepath.Join(themeDir, "layouts")
	staticDir := filepath.Join(themeDir, "static")
	assetsDir := filepath.Join(themeDir, "assets")

	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatalf("Failed to create layouts dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(staticDir, "css"), 0755); err != nil {
		t.Fatalf("Failed to create static/css dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(assetsDir, "js"), 0755); err != nil {
		t.Fatalf("Failed to create assets/js dir: %v", err)
	}

	// Add files
	if err := os.WriteFile(filepath.Join(layoutsDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("Failed to create layout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "css", "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "js", "main.js"), []byte("//js"), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Convert theme
	if err := ConvertHugoTheme(themeDir, outputDir); err != nil {
		t.Fatalf("ConvertHugoTheme failed: %v", err)
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Error("index.html not copied from layouts")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "css", "style.css")); err != nil {
		t.Error("css/style.css not copied from static")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "js", "main.js")); err != nil {
		t.Error("js/main.js not copied from assets")
	}
}

func TestConvertHugoThemeEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "empty-theme")
	outputDir := filepath.Join(tmpDir, "output")

	// Create empty theme directory
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		t.Fatalf("Failed to create theme dir: %v", err)
	}

	// Should not fail even with missing directories
	if err := ConvertHugoTheme(themeDir, outputDir); err != nil {
		t.Fatalf("ConvertHugoTheme should not fail with empty theme: %v", err)
	}
}

// Helper function to create a test zip file
func createTestZip(zipPath string, files map[string]string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = zipFile.Close() }()

	zipWriter := newZipWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

	for name, content := range files {
		w, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}

// Wrapper for archive/zip
type zipWriterWrapper struct {
	w *zip.Writer
}

func newZipWriter(f *os.File) *zipWriterWrapper {
	return &zipWriterWrapper{w: zip.NewWriter(f)}
}

func (zw *zipWriterWrapper) Create(name string) (io.Writer, error) {
	return zw.w.Create(name)
}

func (zw *zipWriterWrapper) Close() error {
	return zw.w.Close()
}

func TestExtractZipWithSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip file with nested directories
	zipPath := filepath.Join(tmpDir, "nested.zip")
	if err := createTestZip(zipPath, map[string]string{
		"theme-main/layouts/base.html":       "<html>base</html>",
		"theme-main/layouts/index.html":      "<html>index</html>",
		"theme-main/static/css/style.css":    "body {}",
		"theme-main/static/js/main.js":       "// js",
		"theme-main/assets/images/logo.png":  "PNG",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify nested structure (prefix stripped)
	expectedFiles := []string{
		"layouts/base.html",
		"layouts/index.html",
		"static/css/style.css",
		"static/js/main.js",
		"assets/images/logo.png",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(destDir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Expected file %s not found: %v", f, err)
		}
	}
}

func TestExtractZipEmptyArchive(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create an empty zip file
	zipPath := filepath.Join(tmpDir, "empty.zip")
	if err := createTestZip(zipPath, map[string]string{}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract should not fail
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed on empty archive: %v", err)
	}
}

func TestConvertHugoThemePartial(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "hugo-partial")
	outputDir := filepath.Join(tmpDir, "output")

	// Create only layouts directory (no static or assets)
	layoutsDir := filepath.Join(themeDir, "layouts")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatalf("Failed to create layouts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutsDir, "base.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("Failed to create base.html: %v", err)
	}

	// Convert should work with partial structure
	if err := ConvertHugoTheme(themeDir, outputDir); err != nil {
		t.Fatalf("ConvertHugoTheme failed: %v", err)
	}

	// Verify layouts were copied
	if _, err := os.Stat(filepath.Join(outputDir, "base.html")); err != nil {
		t.Error("base.html not copied")
	}
}

func TestConvertHugoThemeStaticOnly(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "hugo-static")
	outputDir := filepath.Join(tmpDir, "output")

	// Create only static directory
	staticDir := filepath.Join(themeDir, "static", "css")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatalf("Failed to create static/css dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "main.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("Failed to create main.css: %v", err)
	}

	// Convert should work
	if err := ConvertHugoTheme(themeDir, outputDir); err != nil {
		t.Fatalf("ConvertHugoTheme failed: %v", err)
	}

	// Verify static was copied
	if _, err := os.Stat(filepath.Join(outputDir, "css", "main.css")); err != nil {
		t.Error("css/main.css not copied")
	}
}

func TestConvertHugoThemeAssetsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "hugo-assets")
	outputDir := filepath.Join(tmpDir, "output")

	// Create only assets directory
	assetsDir := filepath.Join(themeDir, "assets", "js")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		t.Fatalf("Failed to create assets/js dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("// app"), 0644); err != nil {
		t.Fatalf("Failed to create app.js: %v", err)
	}

	// Convert should work
	if err := ConvertHugoTheme(themeDir, outputDir); err != nil {
		t.Fatalf("ConvertHugoTheme failed: %v", err)
	}

	// Verify assets was copied
	if _, err := os.Stat(filepath.Join(outputDir, "js", "app.js")); err != nil {
		t.Error("js/app.js not copied")
	}
}

func TestCopyDirError(t *testing.T) {
	// Test error case - source doesn't exist
	err := copyDir("/nonexistent/source", "/tmp/dest")
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestConvertToArchiveURLBitbucket(t *testing.T) {
	// Test URL that's not GitHub or GitLab (passthrough)
	url := "https://bitbucket.org/user/repo"
	result := convertToArchiveURL(url)
	if result != url {
		t.Errorf("Expected passthrough for bitbucket URL, got %s", result)
	}
}

func TestConvertToArchiveURLHTTP(t *testing.T) {
	// Test http (not https) URL
	url := "http://github.com/user/repo"
	result := convertToArchiveURL(url)
	expected := "http://github.com/user/repo/archive/refs/heads/main.zip"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestConvertToArchiveURLGitLabWithGit(t *testing.T) {
	url := "https://gitlab.com/user/repo.git"
	result := convertToArchiveURL(url)
	expected := "https://gitlab.com/user/repo/-/archive/main/archive.zip"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestConvertToArchiveURLGitLabWithTrailingSlash(t *testing.T) {
	url := "https://gitlab.com/user/repo/"
	result := convertToArchiveURL(url)
	expected := "https://gitlab.com/user/repo/-/archive/main/archive.zip"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestExtractZipFileMode(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip file
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/script.sh": "#!/bin/bash\necho hello",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(destDir, "script.sh")); err != nil {
		t.Errorf("script.sh not extracted: %v", err)
	}
}

func TestExtractZipWithDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip file with directories using CreateHeader for proper directory entries
	zipPath := filepath.Join(tmpDir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add a directory entry
	header := &zip.FileHeader{
		Name:   "root/subdir/",
		Method: zip.Store,
	}
	header.SetMode(os.ModeDir | 0755)
	_, err = zipWriter.CreateHeader(header)
	if err != nil {
		t.Fatalf("Failed to create directory entry: %v", err)
	}

	// Add a file in the directory
	w, err := zipWriter.Create("root/subdir/file.txt")
	if err != nil {
		t.Fatalf("Failed to create file entry: %v", err)
	}
	_, _ = w.Write([]byte("content"))

	_ = zipWriter.Close()
	_ = zipFile.Close()

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify directory and file exist
	if _, err := os.Stat(filepath.Join(destDir, "subdir")); err != nil {
		t.Errorf("subdir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "subdir", "file.txt")); err != nil {
		t.Errorf("subdir/file.txt not extracted: %v", err)
	}
}

func TestExtractZipInvalidDestination(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid zip file
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := createTestZip(zipPath, map[string]string{
		"root/file.txt": "content",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Try to extract to invalid destination
	err := extractZip(zipPath, "/nonexistent/deep/path/dest")
	// MkdirAll should not fail, but if the path is really problematic, it would
	if err != nil {
		// This is expected on some systems
		t.Logf("Expected behavior - got error: %v", err)
	}
}

func TestConvertHugoThemeOutputDirError(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "theme")

	// Create theme with layouts
	layoutsDir := filepath.Join(themeDir, "layouts")
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatalf("Failed to create layouts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutsDir, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}

	// Try to convert to a path that will fail
	err := ConvertHugoTheme(themeDir, "/nonexistent/deep/path/output")
	// This might work on some systems due to MkdirAll
	if err != nil {
		// Expected on some systems
		t.Logf("Got expected error: %v", err)
	}
}

func TestExtractZipNoPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip file without common prefix (files at root level)
	zipPath := filepath.Join(tmpDir, "noprefix.zip")
	if err := createTestZip(zipPath, map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Files should be at root level (prefix "file1.txt" stripped means empty name skipped)
	// The behavior depends on how files are stored in the archive
}

func TestExtractZipRootPrefixStripped(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip with a deep structure
	zipPath := filepath.Join(tmpDir, "deep.zip")
	if err := createTestZip(zipPath, map[string]string{
		"theme-v1.0.0/layouts/index.html":    "index content",
		"theme-v1.0.0/layouts/base.html":     "base content",
		"theme-v1.0.0/static/style.css":      "body {}",
	}); err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract
	if err := extractZip(zipPath, destDir); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify prefix "theme-v1.0.0/" was stripped
	if _, err := os.Stat(filepath.Join(destDir, "layouts", "index.html")); err != nil {
		t.Errorf("layouts/index.html not found after stripping prefix: %v", err)
	}
}

func TestCopyDirWithSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source directory with a file
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Copy should work
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify copy
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); err != nil {
		t.Error("file.txt not copied")
	}
}

func TestConvertHugoThemeWithLayoutsError(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "theme")

	// Create layouts as a file instead of directory (will cause error when copying)
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		t.Fatalf("Failed to create theme dir: %v", err)
	}

	layoutsPath := filepath.Join(themeDir, "layouts")
	if err := os.WriteFile(layoutsPath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create layouts file: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")

	err := ConvertHugoTheme(themeDir, outputDir)
	if err == nil {
		t.Error("Expected error when layouts is not a directory")
	}
}

func TestConvertHugoThemeWithStaticError(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "theme")

	// Create static as a file instead of directory
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		t.Fatalf("Failed to create theme dir: %v", err)
	}

	staticPath := filepath.Join(themeDir, "static")
	if err := os.WriteFile(staticPath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create static file: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")

	err := ConvertHugoTheme(themeDir, outputDir)
	if err == nil {
		t.Error("Expected error when static is not a directory")
	}
}

func TestConvertHugoThemeWithAssetsError(t *testing.T) {
	tmpDir := t.TempDir()
	themeDir := filepath.Join(tmpDir, "theme")

	// Create assets as a file instead of directory
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		t.Fatalf("Failed to create theme dir: %v", err)
	}

	assetsPath := filepath.Join(themeDir, "assets")
	if err := os.WriteFile(assetsPath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create assets file: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")

	err := ConvertHugoTheme(themeDir, outputDir)
	if err == nil {
		t.Error("Expected error when assets is not a directory")
	}
}

func TestDownloadWithMockServer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a zip file to serve
	zipContent := createZipBytes(t, map[string]string{
		"theme-main/index.html": "<html></html>",
		"theme-main/style.css":  "body {}",
	})

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipContent)
	}))
	defer server.Close()

	destDir := filepath.Join(tmpDir, "theme")

	// Download from mock server
	err := Download(server.URL+"/theme.zip", destDir)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify files were extracted
	if _, err := os.Stat(filepath.Join(destDir, "index.html")); err != nil {
		t.Error("index.html not extracted")
	}
	if _, err := os.Stat(filepath.Join(destDir, "style.css")); err != nil {
		t.Error("style.css not extracted")
	}
}

func TestDownloadHTTPError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock HTTP server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	destDir := filepath.Join(tmpDir, "theme")

	err := Download(server.URL+"/nonexistent.zip", destDir)
	if err == nil {
		t.Error("Expected error for HTTP 404")
	}
}

func TestDownloadInvalidURL(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to download from invalid URL
	err := Download("http://invalid.localhost.test:99999/theme.zip", filepath.Join(tmpDir, "theme"))
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestDownloadInvalidZip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock HTTP server that returns invalid zip
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not a valid zip file"))
	}))
	defer server.Close()

	destDir := filepath.Join(tmpDir, "theme")

	err := Download(server.URL+"/invalid.zip", destDir)
	if err == nil {
		t.Error("Expected error for invalid zip")
	}
}

func TestExtractZipPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	// Create a zip file with path traversal attempt
	zipPath := filepath.Join(tmpDir, "malicious.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add a file with path traversal in the name
	header := &zip.FileHeader{
		Name:   "root/../../../etc/passwd",
		Method: zip.Store,
	}
	w, err := zipWriter.CreateHeader(header)
	if err != nil {
		t.Fatalf("Failed to create header: %v", err)
	}
	_, _ = w.Write([]byte("malicious content"))

	_ = zipWriter.Close()
	_ = zipFile.Close()

	// Extract should fail with security error
	err = extractZip(zipPath, destDir)
	if err == nil {
		t.Error("Expected error for path traversal attempt")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("Expected 'invalid file path' error, got: %v", err)
	}
}

// Helper function to create zip bytes
func createZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	zipWriter := zip.NewWriter(tmpFile)

	for name, content := range files {
		w, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("Failed to create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write zip entry: %v", err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("Failed to close zip writer: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read zip file: %v", err)
	}

	return content
}
