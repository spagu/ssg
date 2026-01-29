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

func TestPrettifyHTMLFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "collapse multiple blank lines",
			input: `<!DOCTYPE html>
<html>


<head>
<title>Test</title>
</head>



<body>
<p>Hello</p>
</body>
</html>`,
			expected: `<!DOCTYPE html>
<html>

<head>
<title>Test</title>
</head>

<body>
<p>Hello</p>
</body>
</html>
`,
		},
		{
			name: "remove whitespace-only lines",
			input: `<html>

<head>
</head>
</html>`,
			expected: `<html>

<head>
</head>
</html>
`,
		},
		{
			name: "remove trailing whitespace",
			input: `<html>
<head>
<title>Test</title>
</head>
</html>`,
			expected: `<html>
<head>
<title>Test</title>
</head>
</html>
`,
		},
		{
			name: "remove leading blank lines",
			input: `

<!DOCTYPE html>
<html>
</html>`,
			expected: `<!DOCTYPE html>
<html>
</html>
`,
		},
		{
			name:     "ensure single trailing newline",
			input:    `<html></html>`,
			expected: `<html></html>` + "\n",
		},
		{
			name: "complex case - all transformations",
			input: `

<!DOCTYPE html>
<html>



<head>

<title>Test</title>
</head>


<body>
<p>Content</p>
</body>


</html>


`,
			expected: `<!DOCTYPE html>
<html>

<head>

<title>Test</title>
</head>

<body>
<p>Content</p>
</body>

</html>
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			htmlPath := filepath.Join(tmpDir, "test.html")

			if err := os.WriteFile(htmlPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			if err := prettifyHTMLFile(htmlPath); err != nil {
				t.Fatalf("prettifyHTMLFile failed: %v", err)
			}

			result, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("prettifyHTMLFile mismatch:\nInput:\n%q\n\nExpected:\n%q\n\nGot:\n%q", tt.input, tt.expected, string(result))
			}
		})
	}
}

func TestPrettifyHTMLFileNotFound(t *testing.T) {
	err := prettifyHTMLFile("/nonexistent/path/to/file.html")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestPrettifyOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test HTML files
	htmlContent := `<!DOCTYPE html>


<html>
<body>
</body>
</html>`

	expectedContent := `<!DOCTYPE html>

<html>
<body>
</body>
</html>
`

	// Create directory structure
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create HTML files
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "subdir", "page.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create page.html: %v", err)
	}

	// Create non-HTML file (should be ignored)
	if err := os.WriteFile(filepath.Join(tmpDir, "style.css"), []byte("body { }"), 0644); err != nil {
		t.Fatalf("Failed to create style.css: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			PrettyHTML: true,
		},
	}

	if err := gen.prettifyOutput(); err != nil {
		t.Fatalf("prettifyOutput failed: %v", err)
	}

	// Verify HTML files were prettified
	for _, path := range []string{
		filepath.Join(tmpDir, "index.html"),
		filepath.Join(tmpDir, "subdir", "page.html"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", path, err)
		}
		if string(content) != expectedContent {
			t.Errorf("File %s not properly prettified:\nExpected:\n%q\nGot:\n%q", path, expectedContent, string(content))
		}
	}

	// Verify CSS file was not modified
	cssContent, _ := os.ReadFile(filepath.Join(tmpDir, "style.css"))
	if string(cssContent) != "body { }" {
		t.Error("CSS file was incorrectly modified")
	}
}
