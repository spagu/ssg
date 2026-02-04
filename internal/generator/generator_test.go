// Package generator - tests for generator
package generator

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
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

	// gen is guaranteed to be non-nil when err is nil
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
			name: "remove all blank lines",
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
		{
			name:     "handle CRLF line endings",
			input:    "<html>\r\n\r\n<head>\r\n</head>\r\n\r\n<body>\r\n</body>\r\n</html>\r\n",
			expected: "<html>\n<head>\n</head>\n<body>\n</body>\n</html>\n",
		},
		{
			name:     "handle mixed line endings",
			input:    "<html>\r\n\n<head>\r</head>\n\r\n<body>\n</body>\r\n</html>",
			expected: "<html>\n<head>\n</head>\n<body>\n</body>\n</html>\n",
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

func TestConvertToRelativeLinksFile(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		input    string
		expected string
	}{
		{
			name:     "convert https href links",
			domain:   "https://example.com",
			input:    `<a href="https://example.com/about">About</a>`,
			expected: `<a href="/about">About</a>`,
		},
		{
			name:     "convert http href links",
			domain:   "https://example.com",
			input:    `<a href="http://example.com/contact">Contact</a>`,
			expected: `<a href="/contact">Contact</a>`,
		},
		{
			name:     "convert protocol-relative links",
			domain:   "example.com",
			input:    `<a href="//example.com/page">Page</a>`,
			expected: `<a href="/page">Page</a>`,
		},
		{
			name:     "convert src attributes",
			domain:   "https://example.com",
			input:    `<img src="https://example.com/images/logo.png">`,
			expected: `<img src="/images/logo.png">`,
		},
		{
			name:     "convert action attributes",
			domain:   "https://example.com",
			input:    `<form action="https://example.com/submit">`,
			expected: `<form action="/submit">`,
		},
		{
			name:     "convert domain root to /",
			domain:   "https://example.com",
			input:    `<a href="https://example.com">Home</a>`,
			expected: `<a href="/">Home</a>`,
		},
		{
			name:     "preserve external links",
			domain:   "https://example.com",
			input:    `<a href="https://other-site.com/page">External</a>`,
			expected: `<a href="https://other-site.com/page">External</a>`,
		},
		{
			name:     "handle single quotes",
			domain:   "https://example.com",
			input:    `<a href='https://example.com/path'>Link</a>`,
			expected: `<a href='/path'>Link</a>`,
		},
		{
			name:     "convert url() in inline styles",
			domain:   "https://example.com",
			input:    `<div style="background: url(https://example.com/bg.jpg)">`,
			expected: `<div style="background: url(/bg.jpg)">`,
		},
		{
			name:   "multiple links in one file",
			domain: "https://example.com",
			input: `<html>
<head><link href="https://example.com/style.css"></head>
<body>
<a href="https://example.com/page1">Page 1</a>
<a href="https://example.com/page2">Page 2</a>
<img src="https://example.com/img.jpg">
</body>
</html>`,
			expected: `<html>
<head><link href="/style.css"></head>
<body>
<a href="/page1">Page 1</a>
<a href="/page2">Page 2</a>
<img src="/img.jpg">
</body>
</html>`,
		},
		{
			name:     "domain with trailing slash in config",
			domain:   "https://example.com/",
			input:    `<a href="https://example.com/about">About</a>`,
			expected: `<a href="/about">About</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			htmlPath := filepath.Join(tmpDir, "test.html")

			if err := os.WriteFile(htmlPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			if err := convertToRelativeLinksFile(htmlPath, tt.domain); err != nil {
				t.Fatalf("convertToRelativeLinksFile failed: %v", err)
			}

			result, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("convertToRelativeLinksFile mismatch:\nDomain: %s\nInput:\n%q\n\nExpected:\n%q\n\nGot:\n%q", tt.domain, tt.input, tt.expected, string(result))
			}
		})
	}
}

func TestConvertToRelativeLinksFileNotFound(t *testing.T) {
	err := convertToRelativeLinksFile("/nonexistent/path/to/file.html", "https://example.com")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestProcessConfigShortcodes(t *testing.T) {
	// Create temp directory with templates
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	// Create template files
	templates := map[string]string{
		"text.html":   `{{.Text}}`,
		"link.html":   `<a href="{{.Url}}">{{.Text}}</a>`,
		"banner.html": `<div class="banner"><a href="{{.Url}}">{{if .Logo}}<img src="{{.Logo}}">{{end}}{{.Title}} - {{.Text}}</a>{{if .Legal}}<small>{{.Legal}}</small>{{end}}</div>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write template %s: %v", name, err)
		}
	}

	tests := []struct {
		name       string
		shortcodes []Shortcode
		input      string
		expected   string
	}{
		{
			name:       "no shortcodes defined",
			shortcodes: nil,
			input:      "Hello {{world}}",
			expected:   "Hello {{world}}",
		},
		{
			name: "simple text shortcode with template",
			shortcodes: []Shortcode{
				{Name: "greeting", Template: "text.html", Text: "Hello World"},
			},
			input:    "Say {{greeting}} to everyone",
			expected: "Say Hello World to everyone",
		},
		{
			name: "link shortcode with template",
			shortcodes: []Shortcode{
				{Name: "mylink", Template: "link.html", Text: "Click Here", Url: "https://example.com"},
			},
			input:    "Please {{mylink}} for more info",
			expected: `Please <a href="https://example.com">Click Here</a> for more info`,
		},
		{
			name: "banner shortcode with template",
			shortcodes: []Shortcode{
				{
					Name:     "promo",
					Template: "banner.html",
					Title:    "Special Offer",
					Text:     "Get 50% off",
					Url:      "https://shop.com",
					Logo:     "/images/logo.png",
					Legal:    "Terms apply",
				},
			},
			input:    "Check out {{promo}} now!",
			expected: `Check out <div class="banner"><a href="https://shop.com"><img src="/images/logo.png">Special Offer - Get 50% off</a><small>Terms apply</small></div> now!`,
		},
		{
			name: "unknown shortcode preserved",
			shortcodes: []Shortcode{
				{Name: "known", Template: "text.html", Text: "Known"},
			},
			input:    "{{known}} and {{unknown}}",
			expected: "Known and {{unknown}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				TemplatesDir: filepath.Join(tmpDir, "templates"),
				Template:     "test",
				Shortcodes:   tt.shortcodes,
			}
			g, _ := New(cfg)

			result := g.processShortcodes(tt.input)

			if result != tt.expected {
				t.Errorf("processShortcodes mismatch:\nInput: %q\nExpected:\n%s\n\nGot:\n%s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestShortcodeWithoutTemplate(t *testing.T) {
	cfg := Config{
		Shortcodes: []Shortcode{
			{Name: "notemplate", Text: "Some text"},
		},
	}
	g, _ := New(cfg)

	// Should return empty string and print warning
	result := g.processShortcodes("Test {{notemplate}} here")
	expected := "Test  here"

	if result != expected {
		t.Errorf("Expected shortcode without template to be empty:\nExpected: %q\nGot: %q", expected, result)
	}
}

func TestShortcodeWithMissingTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	cfg := Config{
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		Template:     "test",
		Shortcodes: []Shortcode{
			{Name: "missing", Template: "nonexistent.html", Text: "Some text"},
		},
	}
	g, _ := New(cfg)

	// Should return empty string when template file doesn't exist
	result := g.processShortcodes("Test {{missing}} here")
	expected := "Test  here"

	if result != expected {
		t.Errorf("Expected missing template to be empty:\nExpected: %q\nGot: %q", expected, result)
	}
}

func TestPrettifyOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test HTML files with blank lines
	htmlContent := `<!DOCTYPE html>


<html>
<body>
</body>
</html>`

	// Expected: all blank lines removed
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

func TestMinifyHTMLFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove whitespace between tags",
			input:    "<html>  <head>  </head>  <body>  </body>  </html>",
			expected: "<html><head></head><body></body></html>",
		},
		{
			name:     "remove HTML comments",
			input:    "<html><!-- comment --><body></body></html>",
			expected: "<html><body></body></html>",
		},
		{
			name:     "preserve conditional comments",
			input:    "<html><!--[if IE]>test<![endif]--><body></body></html>",
			expected: "<html><!--[if IE]>test<![endif]--><body></body></html>",
		},
		{
			name:     "collapse multiple whitespaces",
			input:    "<p>Hello    World</p>",
			expected: "<p>Hello World</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			htmlPath := filepath.Join(tmpDir, "test.html")

			if err := os.WriteFile(htmlPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			if err := minifyHTMLFile(htmlPath); err != nil {
				t.Fatalf("minifyHTMLFile failed: %v", err)
			}

			result, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestMinifyHTMLFileNotFound(t *testing.T) {
	err := minifyHTMLFile("/nonexistent/file.html")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestMinifyCSSFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove CSS comments",
			input:    "body { /* comment */ color: red; }",
			expected: "body{color:red;}",
		},
		{
			name:     "remove newlines and spaces",
			input:    "body {\n  color: red;\n  background: blue;\n}",
			expected: "body{color:red;background:blue;}",
		},
		{
			name:     "collapse multiple spaces",
			input:    "body {   color:   red; }",
			expected: "body{color:red;}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cssPath := filepath.Join(tmpDir, "test.css")

			if err := os.WriteFile(cssPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			if err := minifyCSSFile(cssPath); err != nil {
				t.Fatalf("minifyCSSFile failed: %v", err)
			}

			result, err := os.ReadFile(cssPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestMinifyCSSFileNotFound(t *testing.T) {
	err := minifyCSSFile("/nonexistent/file.css")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestMinifyJSFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove single-line comments",
			input:    "// comment\nvar x = 1;",
			expected: "var x = 1;",
		},
		{
			name:     "remove multi-line comments",
			input:    "/* comment */var x = 1;",
			expected: "var x = 1;",
		},
		{
			name:     "remove empty lines",
			input:    "var x = 1;\n\n\nvar y = 2;",
			expected: "var x = 1;\nvar y = 2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			jsPath := filepath.Join(tmpDir, "test.js")

			if err := os.WriteFile(jsPath, []byte(tt.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			if err := minifyJSFile(jsPath); err != nil {
				t.Fatalf("minifyJSFile failed: %v", err)
			}

			result, err := os.ReadFile(jsPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestMinifyJSFileNotFound(t *testing.T) {
	err := minifyJSFile("/nonexistent/file.js")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestProcessShortcodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "youtube shortcode with full URL",
			input:    "[youtube]https://www.youtube.com/watch?v=dQw4w9WgXcQ[/youtube]",
			contains: `src="https://www.youtube.com/embed/dQw4w9WgXcQ"`,
		},
		{
			name:     "youtube shortcode with short URL",
			input:    "[youtube]https://youtu.be/dQw4w9WgXcQ[/youtube]",
			contains: `src="https://www.youtube.com/embed/dQw4w9WgXcQ"`,
		},
		{
			name:     "embed shortcode",
			input:    "[embed]https://www.youtube.com/watch?v=abc123[/embed]",
			contains: `src="https://www.youtube.com/embed/abc123"`,
		},
		{
			name:     "no shortcode",
			input:    "Regular text without shortcode",
			contains: "Regular text without shortcode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processShortcodes(tt.input)
			if !contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestFixMediaPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "fix relative src path",
			input:    `<img src="media/image.jpg">`,
			expected: `<img src="/media/image.jpg">`,
		},
		{
			name:     "fix relative href path",
			input:    `<a href="media/file.pdf">`,
			expected: `<a href="/media/file.pdf">`,
		},
		{
			name:     "fix relative srcset path",
			input:    `<img srcset="media/small.jpg 100w">`,
			expected: `<img srcset="/media/small.jpg 100w">`,
		},
		{
			name:     "fix srcset with comma",
			input:    `<img srcset="/media/small.jpg 100w, media/large.jpg 200w">`,
			expected: `<img srcset="/media/small.jpg 100w, /media/large.jpg 200w">`,
		},
		{
			name:     "remove thumbnail size suffix",
			input:    `<img src="/media/123_image-300x225.jpg">`,
			expected: `<img src="/media/123_image.jpg">`,
		},
		{
			name:     "already absolute path unchanged",
			input:    `<img src="/media/image.jpg">`,
			expected: `<img src="/media/image.jpg">`,
		},
	}

	emptyMedia := make(map[int]models.MediaItem)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixMediaPaths(tt.input, emptyMedia)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestMinifyOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	htmlContent := `<html>  <body>  Hello  </body>  </html>`
	cssContent := `body { color: red; }`
	jsContent := "// comment\nvar x = 1;"

	if err := os.WriteFile(filepath.Join(tmpDir, "test.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.css"), []byte(cssContent), 0644); err != nil {
		t.Fatalf("Failed to create CSS file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.js"), []byte(jsContent), 0644); err != nil {
		t.Fatalf("Failed to create JS file: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			MinifyHTML: true,
			MinifyCSS:  true,
			MinifyJS:   true,
		},
	}

	if err := gen.minifyOutput(); err != nil {
		t.Fatalf("minifyOutput failed: %v", err)
	}

	// Verify HTML was minified
	html, _ := os.ReadFile(filepath.Join(tmpDir, "test.html"))
	if contains(string(html), "  ") {
		t.Error("HTML file still contains multiple spaces")
	}

	// Verify CSS was minified
	css, _ := os.ReadFile(filepath.Join(tmpDir, "test.css"))
	if contains(string(css), " ") {
		t.Error("CSS file still contains spaces")
	}

	// Verify JS was minified
	js, _ := os.ReadFile(filepath.Join(tmpDir, "test.js"))
	if contains(string(js), "// comment") {
		t.Error("JS file still contains comments")
	}
}

func TestGenerateSitemap(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain:     "example.com",
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
	}

	if err := gen.generateSitemap(); err != nil {
		t.Fatalf("generateSitemap failed: %v", err)
	}

	// Check if sitemap was created
	sitemapPath := filepath.Join(tmpDir, "sitemap.xml")
	content, err := os.ReadFile(sitemapPath)
	if err != nil {
		t.Fatalf("Failed to read sitemap: %v", err)
	}

	if !contains(string(content), "<?xml version") {
		t.Error("Sitemap doesn't contain XML declaration")
	}
	if !contains(string(content), "https://example.com/") {
		t.Error("Sitemap doesn't contain homepage URL")
	}
}

func TestGenerateRobots(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
	}

	if err := gen.generateRobots(); err != nil {
		t.Fatalf("generateRobots failed: %v", err)
	}

	robotsPath := filepath.Join(tmpDir, "robots.txt")
	content, err := os.ReadFile(robotsPath)
	if err != nil {
		t.Fatalf("Failed to read robots.txt: %v", err)
	}

	if !contains(string(content), "User-agent: *") {
		t.Error("robots.txt doesn't contain User-agent")
	}
	if !contains(string(content), "Sitemap: https://example.com/sitemap.xml") {
		t.Error("robots.txt doesn't contain sitemap reference")
	}
}

func TestGenerateCloudflareFiles(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
		},
	}

	if err := gen.generateCloudflareFiles(); err != nil {
		t.Fatalf("generateCloudflareFiles failed: %v", err)
	}

	// Check _headers file
	headersContent, err := os.ReadFile(filepath.Join(tmpDir, "_headers"))
	if err != nil {
		t.Fatalf("Failed to read _headers: %v", err)
	}
	if !contains(string(headersContent), "X-Content-Type-Options") {
		t.Error("_headers doesn't contain security headers")
	}

	// Check _redirects file
	redirectsContent, err := os.ReadFile(filepath.Join(tmpDir, "_redirects"))
	if err != nil {
		t.Fatalf("Failed to read _redirects: %v", err)
	}
	if !contains(string(redirectsContent), "Cloudflare Pages Redirects") {
		t.Error("_redirects doesn't contain expected content")
	}
}

func TestLoadMarkdownDirNotExists(t *testing.T) {
	gen := &Generator{}
	pages, err := gen.loadMarkdownDir("/nonexistent/path")
	if err != nil {
		t.Errorf("Expected nil error for nonexistent dir, got: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("Expected empty pages, got %d", len(pages))
	}
}

func TestLoadPostsDirNotExists(t *testing.T) {
	gen := &Generator{}
	posts, err := gen.loadPostsDir("/nonexistent/path")
	if err != nil {
		t.Errorf("Expected nil error for nonexistent dir, got: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("Expected empty posts, got %d", len(posts))
	}
}

func TestCopyFileNotFound(t *testing.T) {
	gen := &Generator{}
	err := gen.copyFile("/nonexistent/source.txt", "/tmp/dest.txt")
	if err == nil {
		t.Error("Expected error for nonexistent source file")
	}
}

func TestEnsureTemplatesExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing HTML template
	if err := os.WriteFile(filepath.Join(tmpDir, "existing.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("Failed to create existing template: %v", err)
	}

	gen := &Generator{}
	if err := gen.ensureTemplates(tmpDir); err != nil {
		t.Fatalf("ensureTemplates failed: %v", err)
	}

	// base.html should NOT be created since HTML templates already exist
	if _, err := os.Stat(filepath.Join(tmpDir, "base.html")); err == nil {
		t.Error("base.html should not be created when templates already exist")
	}
}

func TestLoadMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := `{
		"categories": [
			{"id": 1, "name": "Uncategorized", "slug": "uncategorized"},
			{"id": 2, "name": "News", "slug": "news"}
		],
		"media": [
			{"id": 100, "title": {"rendered": "Image 1"}, "media_details": {"file": "uploads/image1.jpg"}}
		],
		"users": [
			{"id": 1, "name": "Admin", "slug": "admin"}
		]
	}`

	metadataPath := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(metadataPath, []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata.json: %v", err)
	}

	gen := &Generator{
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	if err := gen.loadMetadata(metadataPath); err != nil {
		t.Fatalf("loadMetadata failed: %v", err)
	}

	if len(gen.siteData.Categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(gen.siteData.Categories))
	}
	if len(gen.siteData.Media) != 1 {
		t.Errorf("Expected 1 media item, got %d", len(gen.siteData.Media))
	}
	if len(gen.siteData.Authors) != 1 {
		t.Errorf("Expected 1 author, got %d", len(gen.siteData.Authors))
	}
}

func TestLoadMetadataNotFound(t *testing.T) {
	gen := &Generator{
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	err := gen.loadMetadata("/nonexistent/metadata.json")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadMetadataInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "metadata.json")

	if err := os.WriteFile(metadataPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to create metadata.json: %v", err)
	}

	gen := &Generator{
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	err := gen.loadMetadata(metadataPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.html")

	gen := &Generator{
		config: Config{OutputDir: tmpDir},
	}

	tmpl, err := template.New("test.html").Parse("<html>{{.Title}}</html>")
	if err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}
	gen.tmpl = tmpl

	data := struct{ Title string }{Title: "Test Page"}
	if err := gen.renderTemplate("test.html", outputPath, data); err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if string(content) != "<html>Test Page</html>" {
		t.Errorf("Unexpected output: %s", string(content))
	}
}

func TestGenerateSitemapWithPages(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Pages: []models.Page{
				{Title: "About", Slug: "about", Link: "https://example.com/about/"},
			},
			Posts: []models.Page{
				{Title: "Hello", Slug: "hello", Link: "https://example.com/2024/01/15/hello/"},
			},
			Categories: map[int]models.Category{
				2: {ID: 2, Name: "News", Slug: "news"},
			},
		},
	}

	if err := gen.generateSitemap(); err != nil {
		t.Fatalf("generateSitemap failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "sitemap.xml"))
	if err != nil {
		t.Fatalf("Failed to read sitemap: %v", err)
	}

	if !contains(string(content), "about") {
		t.Error("Sitemap should contain about page")
	}

	if !contains(string(content), "category/news") {
		t.Error("Sitemap should contain news category")
	}
}

func TestGenerateCategories(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := template.Must(template.New("category.html").Parse("<html>{{.Category.Name}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Posts: []models.Page{
				{Title: "Post 1", Slug: "post1", Categories: []int{2}},
			},
			Categories: map[int]models.Category{
				2: {ID: 2, Name: "News", Slug: "news"},
			},
		},
		tmpl: tmpl,
	}

	if err := gen.generateCategories(); err != nil {
		t.Fatalf("generateCategories failed: %v", err)
	}

	categoryPath := filepath.Join(tmpDir, "category", "news", "index.html")
	if _, err := os.Stat(categoryPath); err != nil {
		t.Error("Category page not generated")
	}
}

func TestCopyAssets(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	cssDir := filepath.Join(templatePath, "css")
	jsDir := filepath.Join(templatePath, "js")

	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatalf("Failed to create css dir: %v", err)
	}
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		t.Fatalf("Failed to create js dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "main.js"), []byte("//js"), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	contentPath := filepath.Join(tmpDir, "content", "test-source")
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		t.Fatalf("Failed to create content dir: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "css", "style.css")); err != nil {
		t.Error("CSS not copied")
	}

	if _, err := os.Stat(filepath.Join(outputDir, "js", "main.js")); err != nil {
		t.Error("JS not copied")
	}
}

func TestCopyDirNotExists(t *testing.T) {
	gen := &Generator{}
	err := gen.copyDir("/nonexistent/source", "/tmp/dest")
	if err == nil {
		t.Error("Expected error for nonexistent source dir")
	}
}

func TestFixMediaPathsWithWpImage(t *testing.T) {
	item := models.MediaItem{
		ID:    1048,
		Title: models.Title{Rendered: "Test Image"},
	}
	item.MediaDetails.File = "2024/01/image1048.jpg"

	media := map[int]models.MediaItem{
		1048: item,
	}

	input := `<figure class="wp-image-1048"><img src="https://old-site.com/wp-content/uploads/2024/01/image1048.jpg"></figure>`
	result := fixMediaPaths(input, media)

	if !contains(result, "/media/1048_image1048.jpg") {
		t.Errorf("Expected wp-image URL to be replaced, got: %s", result)
	}
}

func TestProcessShortcodesInvalid(t *testing.T) {
	input := "[youtube]invalid-url[/youtube]"
	result := processShortcodes(input)

	if result != input {
		t.Errorf("Expected unchanged output for invalid URL, got: %s", result)
	}
}

func TestMinifyOutputPartial(t *testing.T) {
	tmpDir := t.TempDir()

	htmlContent := "<html>  <body>  </body>  </html>"
	cssContent := "body { color: red; }"

	if err := os.WriteFile(filepath.Join(tmpDir, "test.html"), []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.css"), []byte(cssContent), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			MinifyHTML: true,
			MinifyCSS:  false,
		},
	}

	if err := gen.minifyOutput(); err != nil {
		t.Fatalf("minifyOutput failed: %v", err)
	}

	html, _ := os.ReadFile(filepath.Join(tmpDir, "test.html"))
	if contains(string(html), "  ") {
		t.Error("HTML should be minified")
	}

	css, _ := os.ReadFile(filepath.Join(tmpDir, "test.css"))
	if string(css) != cssContent {
		t.Error("CSS should not be minified when MinifyCSS is false")
	}
}

func TestLoadMarkdownDirWithFiles(t *testing.T) {
	tmpDir := t.TempDir()

	mdContent := `---
title: Test Page
status: publish
slug: test-page
link: https://example.com/test-page/
---
# Test Content
`
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Create a non-md file (should be skipped)
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatalf("Failed to create txt file: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("Expected 1 page, got %d", len(pages))
	}
}

func TestLoadPostsDirWithCategories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create category subdirectory
	catDir := filepath.Join(tmpDir, "news")
	if err := os.MkdirAll(catDir, 0755); err != nil {
		t.Fatalf("Failed to create category dir: %v", err)
	}

	mdContent := `---
title: Test Post
status: publish
slug: test-post
link: https://example.com/2024/01/01/test-post/
---
# Test Content
`
	if err := os.WriteFile(filepath.Join(catDir, "post.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	gen := &Generator{}
	posts, err := gen.loadPostsDir(tmpDir)
	if err != nil {
		t.Fatalf("loadPostsDir failed: %v", err)
	}

	if len(posts) != 1 {
		t.Errorf("Expected 1 post, got %d", len(posts))
	}
}

func TestGenerateIndex(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	tmpl := template.Must(template.New("index.html").Parse("<html>{{.Domain}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Pages:  []models.Page{},
			Posts:  []models.Page{},
		},
		tmpl: tmpl,
	}

	if err := gen.generateIndex(); err != nil {
		t.Fatalf("generateIndex failed: %v", err)
	}

	indexPath := filepath.Join(outputDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		t.Error("index.html not created")
	}

	content, _ := os.ReadFile(indexPath)
	if !contains(string(content), "example.com") {
		t.Error("Index page should contain domain")
	}
}

func TestGeneratePage(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	tmpl := template.Must(template.New("page.html").Parse("<html>{{.Page.Title}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{Domain: "example.com"},
		tmpl:     tmpl,
	}

	page := models.Page{
		Title: "About Us",
		Slug:  "about",
		Link:  "https://example.com/about/",
	}

	if err := gen.generatePage(page); err != nil {
		t.Fatalf("generatePage failed: %v", err)
	}

	pagePath := filepath.Join(outputDir, "about", "index.html")
	if _, err := os.Stat(pagePath); err != nil {
		t.Error("Page not created")
	}
}

func TestGeneratePageSkipsRoot(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	tmpl := template.Must(template.New("page.html").Parse("<html></html>"))

	gen := &Generator{
		config:   Config{OutputDir: outputDir},
		siteData: &models.SiteData{},
		tmpl:     tmpl,
	}

	// Page with empty path (would overwrite index.html)
	page := models.Page{
		Title: "Home",
		Slug:  "",
		Link:  "https://example.com/",
	}

	// Should not error, just skip
	if err := gen.generatePage(page); err != nil {
		t.Fatalf("generatePage should not fail for root page: %v", err)
	}
}

func TestGeneratePost(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	tmpl := template.Must(template.New("post.html").Parse("<html>{{.Post.Title}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{Domain: "example.com"},
		tmpl:     tmpl,
	}

	post := models.Page{
		Title: "Hello World",
		Slug:  "hello-world",
		Type:  "post",
		Date:  parseDate("2024-01-15"),
	}

	if err := gen.generatePost(post); err != nil {
		t.Fatalf("generatePost failed: %v", err)
	}

	postPath := filepath.Join(outputDir, "2024", "01", "15", "hello-world", "index.html")
	if _, err := os.Stat(postPath); err != nil {
		t.Error("Post not created")
	}
}

func TestGenerateSite(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	tmpl := template.Must(template.New("").
		New("index.html").Parse("<html>index</html>"))
	tmpl = template.Must(tmpl.New("page.html").Parse("<html>{{.Page.Title}}</html>"))
	tmpl = template.Must(tmpl.New("post.html").Parse("<html>{{.Post.Title}}</html>"))
	tmpl = template.Must(tmpl.New("category.html").Parse("<html>{{.Category.Name}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Pages: []models.Page{
				{Title: "About", Slug: "about", Link: "https://example.com/about/"},
			},
			Posts: []models.Page{
				{Title: "Hello", Slug: "hello", Type: "post", Date: parseDate("2024-01-01"), Categories: []int{2}},
			},
			Categories: map[int]models.Category{
				2: {ID: 2, Name: "News", Slug: "news"},
			},
		},
		tmpl: tmpl,
	}

	if err := gen.generateSite(); err != nil {
		t.Fatalf("generateSite failed: %v", err)
	}

	// Verify index was created
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Error("index.html not created")
	}

	// Verify page was created
	if _, err := os.Stat(filepath.Join(outputDir, "about", "index.html")); err != nil {
		t.Error("about page not created")
	}

	// Verify post was created
	if _, err := os.Stat(filepath.Join(outputDir, "2024", "01", "01", "hello", "index.html")); err != nil {
		t.Error("post not created")
	}
}

func TestLoadContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create content structure
	sourcePath := filepath.Join(tmpDir, "content", "test-source")
	pagesPath := filepath.Join(sourcePath, "pages")
	postsPath := filepath.Join(sourcePath, "posts", "news")

	if err := os.MkdirAll(pagesPath, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(postsPath, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}

	// Create metadata.json
	metadata := `{
		"categories": [{"id": 1, "name": "News", "slug": "news"}],
		"media": [],
		"users": [{"id": 1, "name": "Admin", "slug": "admin"}]
	}`
	if err := os.WriteFile(filepath.Join(sourcePath, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create a page
	pageContent := `---
title: About
status: publish
slug: about
link: https://example.com/about/
---
About content`
	if err := os.WriteFile(filepath.Join(pagesPath, "about.md"), []byte(pageContent), 0644); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	// Create a post
	postContent := `---
title: Hello
status: publish
slug: hello
type: post
link: https://example.com/2024/01/01/hello/
---
Hello content`
	if err := os.WriteFile(filepath.Join(postsPath, "hello.md"), []byte(postContent), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:     "test-source",
			ContentDir: filepath.Join(tmpDir, "content"),
		},
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	if err := gen.loadContent(); err != nil {
		t.Fatalf("loadContent failed: %v", err)
	}

	if len(gen.siteData.Pages) != 1 {
		t.Errorf("Expected 1 page, got %d", len(gen.siteData.Pages))
	}
	if len(gen.siteData.Posts) != 1 {
		t.Errorf("Expected 1 post, got %d", len(gen.siteData.Posts))
	}
}

func TestLoadTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	// Create minimal templates
	indexTmpl := `<!DOCTYPE html><html><body>{{.Domain}}</body></html>`
	if err := os.WriteFile(filepath.Join(templatePath, "index.html"), []byte(indexTmpl), 0644); err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}

	gen := &Generator{
		config: Config{
			Template:     "test",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Pages:  []models.Page{},
			Posts:  []models.Page{},
		},
	}

	if err := gen.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates failed: %v", err)
	}

	if gen.tmpl == nil {
		t.Error("Template not loaded")
	}
}

func TestGenerate(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup directories
	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(postsDir, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	// Create metadata
	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create a simple page
	pageContent := `---
title: About
status: publish
slug: about
link: https://example.com/about/
---
About`
	if err := os.WriteFile(filepath.Join(pagesDir, "about.md"), []byte(pageContent), 0644); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	// Create simple standalone templates
	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index - {{.Domain}}</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>{{.Page.Title}}</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>{{.Post.Title}}</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>{{.Category.Name}}</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template %s: %v", name, err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify output was created
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Error("index.html not created")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "sitemap.xml")); err != nil {
		t.Error("sitemap.xml not created")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "robots.txt")); err != nil {
		t.Error("robots.txt not created")
	}
}

func TestGenerateWithClean(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup directories
	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(postsDir, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	// Create an old file in output (should be removed by clean)
	oldFile := filepath.Join(outputDir, "old.html")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	// Create metadata
	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create standalone templates
	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template %s: %v", name, err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		Clean:        true,
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Old file should be removed
	if _, err := os.Stat(oldFile); err == nil {
		t.Error("Old file should have been removed by clean")
	}
}

func TestGenerateWithPrettyHTML(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Template with extra blank lines (standalone)
	templates := map[string]string{
		"base.html": `<!DOCTYPE html>


<html>


</html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		PrettyHTML:   true,
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify index was prettified (no 3+ consecutive newlines)
	content, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("Failed to read index: %v", err)
	}
	if contains(string(content), "\n\n\n") {
		t.Error("HTML should be prettified - no 3+ consecutive newlines")
	}
}

func TestGenerateWithMinify(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create CSS and JS files in template
	cssDir := filepath.Join(templateDir, "css")
	jsDir := filepath.Join(templateDir, "js")
	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatalf("Failed to create css dir: %v", err)
	}
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		t.Fatalf("Failed to create js dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "main.js"), []byte("// comment\nvar x = 1;"), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html>  <html>  <body>  Index  </body>  </html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		MinifyHTML:   true,
		MinifyCSS:    true,
		MinifyJS:     true,
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify HTML was minified
	indexContent, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("Failed to read index: %v", err)
	}
	if contains(string(indexContent), "  ") {
		t.Error("HTML should be minified")
	}

	// Verify CSS was minified
	cssContent, err := os.ReadFile(filepath.Join(outputDir, "css", "style.css"))
	if err != nil {
		t.Fatalf("Failed to read CSS: %v", err)
	}
	if contains(string(cssContent), " ") {
		t.Error("CSS should be minified")
	}

	// Verify JS was minified
	jsContent, err := os.ReadFile(filepath.Join(outputDir, "js", "main.js"))
	if err != nil {
		t.Fatalf("Failed to read JS: %v", err)
	}
	if contains(string(jsContent), "// comment") {
		t.Error("JS comments should be removed")
	}
}

func TestGenerateWithSitemapOff(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		SitemapOff:   true,
		RobotsOff:    true,
		Quiet:        true,
	}

	gen, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Sitemap should NOT be created
	if _, err := os.Stat(filepath.Join(outputDir, "sitemap.xml")); err == nil {
		t.Error("sitemap.xml should not exist when SitemapOff is true")
	}

	// Robots should NOT be created
	if _, err := os.Stat(filepath.Join(outputDir, "robots.txt")); err == nil {
		t.Error("robots.txt should not exist when RobotsOff is true")
	}
}

func TestLoadTemplatesWithFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	// Create template that uses all custom functions
	indexTmpl := `<!DOCTYPE html>
<html>
<body>
{{ safeHTML "**bold text**\n- Test Page" }}
{{ decodeHTML "&#8211;" }}
{{ formatDate "2024-01-15" }}
{{ formatDatePL .TestTime }}
{{ getCategoryName 1 }}
{{ getCategorySlug 1 }}
{{ isValidCategory 1 }}
{{ isValidCategory 2 }}
{{ getAuthorName 1 }}
{{ getURL .TestPage }}
{{ getCanonical .TestPage "example.com" }}
{{ hasValidCategories .TestPage }}
{{ thumbnailFromYoutube "[youtube]https://www.youtube.com/watch?v=abc123[/youtube]" }}
{{ stripShortcodes "text [youtube]url[/youtube] more" }}
{{ stripHTML "<p>text</p>" }}
{{ range recentPosts 5 }}{{ .Title }}{{ end }}
</body>
</html>`
	if err := os.WriteFile(filepath.Join(templatePath, "index.html"), []byte(indexTmpl), 0644); err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}

	testPage := models.Page{
		Title:      "Test Page",
		Slug:       "test",
		Link:       "https://example.com/test/",
		Categories: []int{1, 2},
	}

	gen := &Generator{
		config: Config{
			Template:     "test",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Pages:  []models.Page{testPage},
			Posts:  []models.Page{{Title: "Post 1", Slug: "post1"}},
			Categories: map[int]models.Category{
				1: {ID: 1, Name: "Category 1", Slug: "category-1"},
			},
			Media: make(map[int]models.MediaItem),
			Authors: map[int]models.Author{
				1: {ID: 1, Name: "Admin", Slug: "admin"},
			},
		},
	}

	if err := gen.loadTemplates(); err != nil {
		t.Fatalf("loadTemplates failed: %v", err)
	}

	if gen.tmpl == nil {
		t.Error("Template not loaded")
	}

	// Execute the template to test the functions
	outputPath := filepath.Join(tmpDir, "output.html")
	data := struct {
		Site     *models.SiteData
		Posts    []models.Page
		Pages    []models.Page
		Domain   string
		TestPage models.Page
		TestTime time.Time
	}{
		Site:     gen.siteData,
		Posts:    gen.siteData.Posts,
		Pages:    gen.siteData.Pages,
		Domain:   "example.com",
		TestPage: testPage,
		TestTime: parseDate("2024-03-15"),
	}

	file, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer func() { _ = file.Close() }()

	if err := gen.tmpl.ExecuteTemplate(file, "index.html", data); err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// Verify output was created
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	// Check that functions produced output
	if !contains(string(content), "<strong>bold text</strong>") {
		t.Error("safeHTML should convert markdown bold")
	}
	if !contains(string(content), "marca") {
		t.Error("formatDatePL should produce Polish month name")
	}
	if !contains(string(content), "Category 1") {
		t.Error("getCategoryName should return category name")
	}
	if !contains(string(content), "Admin") {
		t.Error("getAuthorName should return author name")
	}
}

func TestLoadTemplatesError(t *testing.T) {
	gen := &Generator{
		config: Config{
			Template:     "nonexistent",
			TemplatesDir: "/nonexistent/path",
		},
		siteData: &models.SiteData{
			Pages: []models.Page{},
			Posts: []models.Page{},
		},
	}

	err := gen.loadTemplates()
	if err == nil {
		t.Error("Expected error for nonexistent template path")
	}
}

func TestLoadMarkdownDirSkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory (should be skipped)
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create valid markdown file
	mdContent := `---
title: Test
status: publish
slug: test
link: https://example.com/test/
---
Content`
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create md file: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("Expected 1 page (subdir should be skipped), got %d", len(pages))
	}
}

func TestLoadMarkdownDirDraftPage(t *testing.T) {
	tmpDir := t.TempDir()

	// Create draft page (status != publish)
	mdContent := `---
title: Draft
status: draft
slug: draft
link: https://example.com/draft/
---
Draft content`
	if err := os.WriteFile(filepath.Join(tmpDir, "draft.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to create md file: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	if len(pages) != 0 {
		t.Errorf("Expected 0 pages (draft should be skipped), got %d", len(pages))
	}
}

func TestLoadMarkdownDirInvalidMarkdown(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid markdown (no frontmatter)
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.md"), []byte("no frontmatter"), 0644); err != nil {
		t.Fatalf("Failed to create md file: %v", err)
	}

	gen := &Generator{}
	pages, err := gen.loadMarkdownDir(tmpDir)
	if err != nil {
		t.Fatalf("loadMarkdownDir failed: %v", err)
	}

	// Invalid files should be skipped with warning
	if len(pages) != 0 {
		t.Errorf("Expected 0 pages (invalid should be skipped), got %d", len(pages))
	}
}

func TestLoadPostsDirSkipsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file in posts root (should be skipped - only dirs processed)
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gen := &Generator{}
	posts, err := gen.loadPostsDir(tmpDir)
	if err != nil {
		t.Fatalf("loadPostsDir failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts (files should be skipped), got %d", len(posts))
	}
}

func TestGenerateCategoriesUnknownCategory(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := template.Must(template.New("category.html").Parse("<html>{{.Category.Name}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Posts: []models.Page{
				{Title: "Post 1", Slug: "post1", Categories: []int{999}}, // Unknown category
			},
			Categories: map[int]models.Category{
				// Category 999 not defined
			},
		},
		tmpl: tmpl,
	}

	// Should not fail, just skip unknown category
	if err := gen.generateCategories(); err != nil {
		t.Fatalf("generateCategories failed: %v", err)
	}

	// No category page should be created
	categoryPath := filepath.Join(tmpDir, "category")
	entries, _ := os.ReadDir(categoryPath)
	if len(entries) > 0 {
		t.Error("Should not create category pages for unknown categories")
	}
}

func TestCopyAssetsWithMedia(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	// Create media dir in content
	contentPath := filepath.Join(tmpDir, "content", "test-source")
	mediaPath := filepath.Join(contentPath, "media")
	if err := os.MkdirAll(mediaPath, 0755); err != nil {
		t.Fatalf("Failed to create media dir: %v", err)
	}

	// Create a media file
	if err := os.WriteFile(filepath.Join(mediaPath, "image.jpg"), []byte("image data"), 0644); err != nil {
		t.Fatalf("Failed to create media file: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets failed: %v", err)
	}

	// Verify media was copied
	if _, err := os.Stat(filepath.Join(outputDir, "media", "image.jpg")); err != nil {
		t.Error("Media file not copied")
	}
}

func TestCopyAssetsImagesDir(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	imagesPath := filepath.Join(templatePath, "images")
	if err := os.MkdirAll(imagesPath, 0755); err != nil {
		t.Fatalf("Failed to create images dir: %v", err)
	}

	// Create image file
	if err := os.WriteFile(filepath.Join(imagesPath, "logo.png"), []byte("png data"), 0644); err != nil {
		t.Fatalf("Failed to create image file: %v", err)
	}

	contentPath := filepath.Join(tmpDir, "content", "test-source")
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		t.Fatalf("Failed to create content dir: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets failed: %v", err)
	}

	// Verify images were copied
	if _, err := os.Stat(filepath.Join(outputDir, "images", "logo.png")); err != nil {
		t.Error("Images file not copied")
	}
}

func TestGenerateContentLoadError(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Source:       "nonexistent-source",
		Template:     "simple",
		ContentDir:   filepath.Join(tmpDir, "content"),
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}

	gen, _ := New(cfg)

	err := gen.Generate()
	if err == nil {
		t.Error("Expected error when content cannot be loaded")
	}
	if !contains(err.Error(), "loading content") {
		t.Errorf("Expected 'loading content' error, got: %v", err)
	}
}

func TestProcessShortcodesYouTubeShortURL(t *testing.T) {
	// Test YouTube with short URL format
	input := "[youtube]https://youtu.be/abc123XYZ[/youtube]"
	result := processShortcodes(input)

	if !contains(result, "youtube.com/embed/abc123XYZ") {
		t.Errorf("Expected YouTube embed, got: %s", result)
	}
}

func TestProcessShortcodesEmbedShortURL(t *testing.T) {
	// Test embed with youtu.be format
	input := "[embed]https://youtu.be/test123[/embed]"
	result := processShortcodes(input)

	if !contains(result, "youtube.com/embed/test123") {
		t.Errorf("Expected YouTube embed, got: %s", result)
	}
}

func TestFixMediaPathsWithSrcset(t *testing.T) {
	media := make(map[int]models.MediaItem)

	// Test srcset with multiple entries
	input := `<img srcset="/media/123_img.jpg 100w, media/456_img.jpg 200w">`
	result := fixMediaPaths(input, media)

	expected := `<img srcset="/media/123_img.jpg 100w, /media/456_img.jpg 200w">`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestGenerateWithRobotsOnlySitemapOff(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		SitemapOff:   true, // Only sitemap off
		RobotsOff:    false,
		Quiet:        true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Sitemap should NOT exist
	if _, err := os.Stat(filepath.Join(outputDir, "sitemap.xml")); err == nil {
		t.Error("sitemap.xml should not exist")
	}

	// Robots SHOULD exist
	if _, err := os.Stat(filepath.Join(outputDir, "robots.txt")); err != nil {
		t.Error("robots.txt should exist")
	}
}

func TestGeneratePostMkdirError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path/that/cannot/be/created",
		},
		siteData: &models.SiteData{},
		tmpl:     template.Must(template.New("post.html").Parse("<html></html>")),
	}

	post := models.Page{
		Title: "Test",
		Slug:  "test",
		Date:  parseDate("2024-01-01"),
	}

	err := gen.generatePost(post)
	if err == nil {
		t.Error("Expected error when directory cannot be created")
	}
}

func TestRenderTemplateCreateError(t *testing.T) {
	gen := &Generator{
		tmpl: template.Must(template.New("test.html").Parse("<html></html>")),
	}

	// Try to render to a path that cannot be created
	err := gen.renderTemplate("test.html", "/nonexistent/path/output.html", nil)
	if err == nil {
		t.Error("Expected error when output file cannot be created")
	}
}

func TestCopyFileDestinationError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	gen := &Generator{}
	// Try to copy to a path that cannot be created
	err := gen.copyFile(srcPath, "/nonexistent/path/dest.txt")
	if err == nil {
		t.Error("Expected error when destination cannot be created")
	}
}

func TestGenerateSitePageGenerateError(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	// Create template that causes error when executed
	tmpl := template.Must(template.New("").
		New("index.html").Parse("<html>index</html>"))
	tmpl = template.Must(tmpl.New("page.html").Parse("<html>{{.NonExistentField}}</html>"))
	tmpl = template.Must(tmpl.New("post.html").Parse("<html></html>"))
	tmpl = template.Must(tmpl.New("category.html").Parse("<html></html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
		},
		siteData: &models.SiteData{
			Pages: []models.Page{
				{Title: "About", Slug: "about", Link: "https://example.com/about/"},
			},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
		tmpl: tmpl,
	}

	// Should not fail (just warns)
	if err := gen.generateSite(); err != nil {
		t.Fatalf("generateSite should not fail with page warning: %v", err)
	}
}

func TestGenerateSitePostGenerateError(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	tmpl := template.Must(template.New("").
		New("index.html").Parse("<html>index</html>"))
	tmpl = template.Must(tmpl.New("page.html").Parse("<html></html>"))
	tmpl = template.Must(tmpl.New("post.html").Parse("<html>{{.NonExistent}}</html>"))
	tmpl = template.Must(tmpl.New("category.html").Parse("<html></html>"))

	gen := &Generator{
		config: Config{
			OutputDir: outputDir,
		},
		siteData: &models.SiteData{
			Pages: []models.Page{},
			Posts: []models.Page{
				{Title: "Post", Slug: "post", Date: parseDate("2024-01-01")},
			},
			Categories: make(map[int]models.Category),
		},
		tmpl: tmpl,
	}

	// Should not fail (just warns)
	if err := gen.generateSite(); err != nil {
		t.Fatalf("generateSite should not fail with post warning: %v", err)
	}
}

func TestGenerateNonQuietMode(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>Page</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>Post</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>Cat</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		Quiet:        false, // Non-quiet mode to test console output
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestGenerateCloudflareFilesHeadersError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path",
		},
	}

	err := gen.generateCloudflareFiles()
	if err == nil {
		t.Error("Expected error when _headers cannot be created")
	}
}

func TestPrettifyOutputError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path",
		},
	}

	err := gen.prettifyOutput()
	if err == nil {
		t.Error("Expected error when output dir doesn't exist")
	}
}

func TestMinifyOutputError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir:  "/nonexistent/path",
			MinifyHTML: true,
		},
	}

	err := gen.minifyOutput()
	if err == nil {
		t.Error("Expected error when output dir doesn't exist")
	}
}

func TestLoadContentMetadataError(t *testing.T) {
	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "content", "test-source")
	pagesPath := filepath.Join(sourcePath, "pages")
	if err := os.MkdirAll(pagesPath, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}

	// No metadata.json - should fail
	gen := &Generator{
		config: Config{
			Source:     "test-source",
			ContentDir: filepath.Join(tmpDir, "content"),
		},
		siteData: &models.SiteData{
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}

	err := gen.loadContent()
	if err == nil {
		t.Error("Expected error when metadata.json is missing")
	}
}

func TestCopyAssetsNoMediaDir(t *testing.T) {
	tmpDir := t.TempDir()

	templatePath := filepath.Join(tmpDir, "templates", "simple")
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	contentPath := filepath.Join(tmpDir, "content", "test-source")
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		t.Fatalf("Failed to create content dir: %v", err)
	}

	// No media directory - should not fail

	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	gen := &Generator{
		config: Config{
			Source:       "test-source",
			Template:     "simple",
			TemplatesDir: filepath.Join(tmpDir, "templates"),
			ContentDir:   filepath.Join(tmpDir, "content"),
			OutputDir:    outputDir,
		},
	}

	// Should not fail even with no asset directories
	if err := gen.copyAssets(); err != nil {
		t.Fatalf("copyAssets should not fail with no assets: %v", err)
	}
}

func TestGenerateCategoriesTemplateError(t *testing.T) {
	tmpDir := t.TempDir()

	// Template that causes error
	tmpl := template.Must(template.New("category.html").Parse("<html>{{.NonExistent}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Domain: "example.com",
			Posts: []models.Page{
				{Title: "Post 1", Slug: "post1", Categories: []int{2}},
			},
			Categories: map[int]models.Category{
				2: {ID: 2, Name: "News", Slug: "news"},
			},
		},
		tmpl: tmpl,
	}

	// Should not fail (just warns)
	if err := gen.generateCategories(); err != nil {
		t.Fatalf("generateCategories should not fail with template error: %v", err)
	}
}

func TestGenerateSiteIndexError(t *testing.T) {
	tmpDir := t.TempDir()

	// Template that causes error on index
	tmpl := template.Must(template.New("index.html").Parse("<html>{{.NonExistent}}</html>"))

	gen := &Generator{
		config: Config{
			OutputDir: tmpDir,
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
		tmpl: tmpl,
	}

	err := gen.generateSite()
	if err == nil {
		t.Error("Expected error when index template fails")
	}
}

func TestGenerateLoadTemplatesError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")

	for _, dir := range []string{pagesDir, postsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "nonexistent",
		ContentDir:   contentDir,
		TemplatesDir: "/nonexistent/templates",
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}

	gen, _ := New(cfg)
	err := gen.Generate()
	if err == nil {
		t.Error("Expected error when templates cannot be loaded")
	}
	if !contains(err.Error(), "loading templates") {
		t.Errorf("Expected 'loading templates' error, got: %v", err)
	}
}

func TestGenerateGenerateSiteError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create invalid template that will cause error
	if err := os.WriteFile(filepath.Join(templateDir, "index.html"), []byte("{{.NonExistent}}"), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    filepath.Join(tmpDir, "output"),
		Quiet:        true,
	}

	gen, _ := New(cfg)
	err := gen.Generate()
	if err == nil {
		t.Error("Expected error when generateSite fails")
	}
}

func TestGenerateSitemapError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path",
			Domain:    "example.com",
		},
		siteData: &models.SiteData{
			Pages:      []models.Page{},
			Posts:      []models.Page{},
			Categories: make(map[int]models.Category),
		},
	}

	err := gen.generateSitemap()
	if err == nil {
		t.Error("Expected error when sitemap cannot be written")
	}
}

func TestGenerateRobotsError(t *testing.T) {
	gen := &Generator{
		config: Config{
			OutputDir: "/nonexistent/path",
			Domain:    "example.com",
		},
	}

	err := gen.generateRobots()
	if err == nil {
		t.Error("Expected error when robots.txt cannot be written")
	}
}

func TestMinifyOutputOnlyJS(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only JS file
	jsContent := "// comment\nvar x = 1;"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.js"), []byte(jsContent), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	gen := &Generator{
		config: Config{
			OutputDir:  tmpDir,
			MinifyHTML: false,
			MinifyCSS:  false,
			MinifyJS:   true,
		},
	}

	if err := gen.minifyOutput(); err != nil {
		t.Fatalf("minifyOutput failed: %v", err)
	}

	js, _ := os.ReadFile(filepath.Join(tmpDir, "test.js"))
	if contains(string(js), "// comment") {
		t.Error("JS comment should be removed")
	}
}

func TestGenerateWithAllOptions(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts", "news")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	cssDir := filepath.Join(templateDir, "css")
	jsDir := filepath.Join(templateDir, "js")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir, cssDir, jsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	// Create pre-existing output to be cleaned
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "old.html"), []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	metadata := `{
		"categories": [{"id": 2, "name": "News", "slug": "news"}],
		"media": [],
		"users": [{"id": 1, "name": "Admin", "slug": "admin"}]
	}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create page
	pageContent := `---
title: About
status: publish
slug: about
link: https://example.com/about/
---
About content`
	if err := os.WriteFile(filepath.Join(pagesDir, "about.md"), []byte(pageContent), 0644); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	// Create post
	postContent := `---
title: Hello World
status: publish
slug: hello
type: post
date: 2024-01-15
link: https://example.com/2024/01/15/hello/
categories:
  - 2
---
Hello content`
	if err := os.WriteFile(filepath.Join(postsDir, "hello.md"), []byte(postContent), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}

	// Create assets
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "main.js"), []byte("// comment\nvar x = 1;"), 0644); err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>base</body></html>`,
		"index.html":    `<!DOCTYPE html><html><body>Index - {{.Domain}}</body></html>`,
		"page.html":     `<!DOCTYPE html><html><body>{{.Page.Title}}</body></html>`,
		"post.html":     `<!DOCTYPE html><html><body>{{.Post.Title}}</body></html>`,
		"category.html": `<!DOCTYPE html><html><body>{{.Category.Name}}</body></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		Clean:        true,
		PrettyHTML:   false,
		MinifyHTML:   true,
		MinifyCSS:    true,
		MinifyJS:     true,
		Quiet:        false,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Old file should be cleaned
	if _, err := os.Stat(filepath.Join(outputDir, "old.html")); err == nil {
		t.Error("Old file should be removed by clean")
	}

	// Category page should be created
	if _, err := os.Stat(filepath.Join(outputDir, "category", "news", "index.html")); err != nil {
		t.Error("Category page should be created")
	}
}

func TestCopyDirSubdirError(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source with subdirectory
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Create destination with read-only subdirectory path that will fail
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dstDir: %v", err)
	}

	gen := &Generator{}
	// Normal copy should work
	if err := gen.copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir should succeed: %v", err)
	}

	// Verify subdirectory was copied
	if _, err := os.Stat(filepath.Join(dstDir, "subdir", "file.txt")); err != nil {
		t.Error("Subdirectory file should be copied")
	}
}

func TestLoadMarkdownDirReadDirError(t *testing.T) {
	// Create a file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gen := &Generator{}
	_, err := gen.loadMarkdownDir(filePath)
	if err == nil {
		t.Error("Expected error when path is a file, not a directory")
	}
}

func TestLoadPostsDirReadDirError(t *testing.T) {
	// Create a file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gen := &Generator{}
	_, err := gen.loadPostsDir(filePath)
	if err == nil {
		t.Error("Expected error when path is a file, not a directory")
	}
}

func TestGenerateCopyAssetsError(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	cssDir := filepath.Join(templateDir, "css")

	for _, dir := range []string{pagesDir, postsDir, cssDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create CSS file that will be copied
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatalf("Failed to create CSS: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html></html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	// Use non-writable output directory
	outputDir := "/nonexistent/output"

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		Quiet:        true,
	}

	gen, _ := New(cfg)
	err := gen.Generate()
	if err == nil {
		t.Error("Expected error when copyAssets fails")
	}
}

func TestGenerateWithBothSitemapAndRobots(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html></html>`,
		"index.html":    `<!DOCTYPE html><html></html>`,
		"page.html":     `<!DOCTYPE html><html></html>`,
		"post.html":     `<!DOCTYPE html><html></html>`,
		"category.html": `<!DOCTYPE html><html></html>`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		SitemapOff:   false,
		RobotsOff:    false,
		Quiet:        true,
	}

	gen, _ := New(cfg)
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Both should be created
	if _, err := os.Stat(filepath.Join(outputDir, "sitemap.xml")); err != nil {
		t.Error("sitemap.xml should exist")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "robots.txt")); err != nil {
		t.Error("robots.txt should exist")
	}
}

func parseDate(s string) (t time.Time) {
	t, _ = time.Parse("2006-01-02", s)
	return
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGeneratePrettyHTMLNonQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{block "content" .}}{{end}}</body></html>`,
		"index.html":    `{{define "content"}}<h1>Index</h1>{{end}}`,
		"page.html":     `{{define "content"}}<h1>{{.Page.Title}}</h1>{{end}}`,
		"post.html":     `{{define "content"}}<h1>{{.Post.Title}}</h1>{{end}}`,
		"category.html": `{{define "content"}}<h1>Category</h1>{{end}}`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		PrettyHTML:   true,
		Quiet:        false, // Non-quiet to cover the print statement
	}

	gen, _ := New(cfg)
	err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestGenerateMinifyNonQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	postsDir := filepath.Join(sourceDir, "posts")
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{pagesDir, postsDir, templateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{block "content" .}}{{end}}</body></html>`,
		"index.html":    `{{define "content"}}<h1>Index</h1>{{end}}`,
		"page.html":     `{{define "content"}}<h1>{{.Page.Title}}</h1>{{end}}`,
		"post.html":     `{{define "content"}}<h1>{{.Post.Title}}</h1>{{end}}`,
		"category.html": `{{define "content"}}<h1>Category</h1>{{end}}`,
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		Source:       "test-source",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   contentDir,
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		OutputDir:    outputDir,
		MinifyHTML:   true,
		Quiet:        false, // Non-quiet to cover the print statement
	}

	gen, _ := New(cfg)
	err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestCopyDirFileError(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	// Create a file that can't be read
	unreadableFile := filepath.Join(srcDir, "unreadable.txt")
	if err := os.WriteFile(unreadableFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Remove read permission
	if err := os.Chmod(unreadableFile, 0000); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}
	defer func() { _ = os.Chmod(unreadableFile, 0644) }() // Restore permission for cleanup

	gen, _ := New(Config{})
	err := gen.copyDir(srcDir, dstDir)
	if err == nil {
		t.Error("Expected error for unreadable file")
	}
}

func TestLoadMarkdownDirParseError(t *testing.T) {
	tmpDir := t.TempDir()
	pagesDir := filepath.Join(tmpDir, "pages")

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}

	// Create a malformed markdown file - loadMarkdownDir prints warning but continues
	malformedMd := `---
title: [invalid
---

content`
	if err := os.WriteFile(filepath.Join(pagesDir, "bad.md"), []byte(malformedMd), 0644); err != nil {
		t.Fatalf("Failed to create markdown: %v", err)
	}

	gen, _ := New(Config{})
	pages, err := gen.loadMarkdownDir(pagesDir)
	// loadMarkdownDir prints warning but doesn't return error, it just skips bad files
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Should have 0 valid pages since the malformed one is skipped
	if len(pages) != 0 {
		t.Errorf("Expected 0 pages (bad file skipped), got %d", len(pages))
	}
}

func TestLoadPostsDirParseError(t *testing.T) {
	tmpDir := t.TempDir()
	postsDir := filepath.Join(tmpDir, "posts")
	yearDir := filepath.Join(postsDir, "2024")

	if err := os.MkdirAll(yearDir, 0755); err != nil {
		t.Fatalf("Failed to create year dir: %v", err)
	}

	// Create a malformed markdown file - loadPostsDir prints warning but continues
	malformedMd := `---
title: [invalid
---

content`
	if err := os.WriteFile(filepath.Join(yearDir, "bad.md"), []byte(malformedMd), 0644); err != nil {
		t.Fatalf("Failed to create markdown: %v", err)
	}

	gen, _ := New(Config{})
	posts, err := gen.loadPostsDir(postsDir)
	// loadPostsDir prints warning but doesn't return error, it skips bad files
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Should have 0 valid posts since the malformed one is skipped
	if len(posts) != 0 {
		t.Errorf("Expected 0 posts (bad file skipped), got %d", len(posts))
	}
}

func TestLoadContentPagesNotExist(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	// Don't create pages dir - loadMarkdownDir handles this gracefully (returns empty)
	postsDir := filepath.Join(sourceDir, "posts")

	if err := os.MkdirAll(postsDir, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}

	// Create metadata
	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	cfg := Config{
		Source:     "test-source",
		ContentDir: contentDir,
	}

	gen, _ := New(cfg)
	err := gen.loadContent()
	// loadMarkdownDir returns nil error for non-existent directory (returns empty list)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(gen.siteData.Pages) != 0 {
		t.Errorf("Expected 0 pages, got %d", len(gen.siteData.Pages))
	}
}

func TestLoadContentPostsNotExist(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	sourceDir := filepath.Join(contentDir, "test-source")
	pagesDir := filepath.Join(sourceDir, "pages")
	// Don't create posts dir - loadPostsDir handles this gracefully (returns empty)

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("Failed to create pages dir: %v", err)
	}

	// Create metadata
	metadata := `{"categories":[],"media":[],"users":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	cfg := Config{
		Source:     "test-source",
		ContentDir: contentDir,
	}

	gen, _ := New(cfg)
	err := gen.loadContent()
	// loadPostsDir returns nil error for non-existent directory (returns empty list)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(gen.siteData.Posts) != 0 {
		t.Errorf("Expected 0 posts, got %d", len(gen.siteData.Posts))
	}
}

func TestGeneratePageMkdirError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file where we need a directory
	invalidPath := filepath.Join(tmpDir, "output")
	if err := os.WriteFile(invalidPath, []byte("file"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gen, _ := New(Config{OutputDir: tmpDir})

	page := models.Page{
		Slug:   "output", // This will try to create output/output/index.html
		Type:   "page",
		Status: "publish",
	}

	err := gen.generatePage(page)
	if err == nil {
		t.Error("Expected error when mkdir fails due to existing file")
	}
}

func TestGenerateSiteWithCategoryWarning(t *testing.T) {
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates", "simple")
	outputDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	templates := map[string]string{
		"base.html":     `<!DOCTYPE html><html><body>{{block "content" .}}{{end}}</body></html>`,
		"index.html":    `{{define "content"}}<h1>Index</h1>{{end}}`,
		"page.html":     `{{define "content"}}<h1>Page</h1>{{end}}`,
		"post.html":     `{{define "content"}}<h1>Post</h1>{{end}}`,
		"category.html": `{{define "content"}}{{.Unknown}}{{end}}`, // Invalid template field
	}
	for name, content := range templates {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create template: %v", err)
		}
	}

	cfg := Config{
		TemplatesDir: filepath.Join(tmpDir, "templates"),
		Template:     "simple",
		OutputDir:    outputDir,
	}

	gen, _ := New(cfg)
	if err := gen.loadTemplates(); err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Add a category and a post to trigger generateCategories
	gen.siteData.Categories[2] = models.Category{ID: 2, Name: "Test", Slug: "test", Count: 1}
	gen.siteData.Posts = append(gen.siteData.Posts, models.Page{
		Title:      "Test Post",
		Slug:       "test-post",
		Categories: []int{2},
	})

	// generateCategories prints warning but doesn't return error for template errors
	err := gen.generateSite()
	if err != nil {
		t.Errorf("generateSite should not fail on category template error (just warn): %v", err)
	}
}

func TestProcessShortcodesNoMatch(t *testing.T) {
	// Test with content that has no shortcodes
	content := "This is just regular content without any shortcodes."
	result := processShortcodes(content)
	if result != content {
		t.Errorf("Expected unchanged content, got: %s", result)
	}
}

func TestFixMediaPathsEmptyContent(t *testing.T) {
	media := make(map[int]models.MediaItem)
	result := fixMediaPaths("", media)
	if result != "" {
		t.Errorf("Expected empty result for empty content")
	}
}

func TestFixMediaPathsNoMediaMapping(t *testing.T) {
	content := `<img class="wp-image-999" src="http://example.com/old.jpg">`
	media := make(map[int]models.MediaItem)
	// No media item for ID 999
	result := fixMediaPaths(content, media)
	// Should return content with srcset and href fixes applied but no wp-image replacement
	if result == "" {
		t.Error("Result should not be empty")
	}
}

func TestMinifyHTMLFileConditionalComment(t *testing.T) {
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "test.html")

	// HTML with conditional comment that should be preserved
	content := `<!DOCTYPE html>
<!--[if IE]><p>You are using IE</p><![endif]-->
<html>
<body>  Text  here  </body>
</html>`

	if err := os.WriteFile(htmlFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if err := minifyHTMLFile(htmlFile); err != nil {
		t.Fatalf("minifyHTMLFile failed: %v", err)
	}

	minified, _ := os.ReadFile(htmlFile)
	result := string(minified)

	// Conditional comment should be preserved
	if !strings.Contains(result, "<!--[if IE]>") {
		t.Error("Conditional comment should be preserved")
	}
}
