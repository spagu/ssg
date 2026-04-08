// Package parser - tests for markdown parser
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMarkdownFile(t *testing.T) {
	// Create temp test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	content := `---
id: 123
title: "Test Page"
slug: "test-page"
date: 2024-01-15T10:00:00Z
modified: 2024-01-15T12:00:00Z
status: "publish"
type: "page"
link: "https://example.com/test-page/"
author: 1
---

# Test Page

## Excerpt

This is the excerpt text.

## Content

This is the main content of the page.

With multiple paragraphs.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	// Test frontmatter parsing
	if page.ID != 123 {
		t.Errorf("Expected ID 123, got %d", page.ID)
	}

	if page.Title != "Test Page" {
		t.Errorf("Expected title 'Test Page', got '%s'", page.Title)
	}

	if page.Slug != "test-page" {
		t.Errorf("Expected slug 'test-page', got '%s'", page.Slug)
	}

	if page.Status != "publish" {
		t.Errorf("Expected status 'publish', got '%s'", page.Status)
	}

	if page.Type != "page" {
		t.Errorf("Expected type 'page', got '%s'", page.Type)
	}

	// Test excerpt parsing
	if page.Excerpt != "This is the excerpt text." {
		t.Errorf("Expected excerpt 'This is the excerpt text.', got '%s'", page.Excerpt)
	}

	// Test content parsing
	expectedContent := "This is the main content of the page.\n\nWith multiple paragraphs."
	if page.Content != expectedContent {
		t.Errorf("Expected content '%s', got '%s'", expectedContent, page.Content)
	}
}

func TestParseMarkdownFileWithCategories(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-post.md")

	content := `---
id: 456
title: "Test Post"
slug: "test-post"
date: 2024-01-15T10:00:00Z
modified: 2024-01-15T12:00:00Z
status: "publish"
type: "post"
link: "https://example.com/test-post/"
author: 2
categories:
  - 1
  - 5
  - 10
---

# Test Post

## Excerpt

Post excerpt here.

## Content

Post content here.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	if len(page.Categories) != 3 {
		t.Errorf("Expected 3 categories, got %d", len(page.Categories))
	}

	expectedCats := []int{1, 5, 10}
	for i, cat := range expectedCats {
		if page.Categories[i] != cat {
			t.Errorf("Expected category %d at index %d, got %d", cat, i, page.Categories[i])
		}
	}
}

func TestParseMarkdownFileNotFound(t *testing.T) {
	_, err := ParseMarkdownFile("/nonexistent/file.md")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestParseFlexibleDateFormats(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		expected string
	}{
		{"RFC3339", "2024-01-15T10:00:00Z", "2024-01-15"},
		{"datetime", "2024-01-15T10:00:00", "2024-01-15"},
		{"datetime with space", "2024-01-15 10:00:00", "2024-01-15"},
		{"date only", "2024-01-15", "2024-01-15"},
		{"date DD-MM-YYYY", "15-01-2024", "2024-01-15"},
		{"date slash", "2024/01/15", "2024-01-15"},
		{"empty", "", "0001-01-01"},
		{"invalid", "not-a-date", "0001-01-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFlexibleDate(tt.dateStr)
			got := result.Format("2006-01-02")
			if got != tt.expected {
				t.Errorf("parseFlexibleDate(%q) = %s, want %s", tt.dateStr, got, tt.expected)
			}
		})
	}
}

func TestParseMarkdownFileNoExcerpt(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "no-excerpt.md")

	content := `---
title: "No Excerpt"
slug: "no-excerpt"
status: "publish"
---

# Content Only

This file has no excerpt section.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	if page.Excerpt != "" {
		t.Errorf("Expected empty excerpt, got '%s'", page.Excerpt)
	}
}

func TestParseMarkdownFileNoContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "no-content.md")

	content := `---
title: "No Content"
slug: "no-content"
status: "publish"
---

# Title

## Excerpt

Just an excerpt, no content section.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	if page.Content != "" {
		t.Errorf("Expected empty content, got '%s'", page.Content)
	}
}

func TestParseMarkdownFileInvalidFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.md")

	content := `---
title: "Missing closing fence
---
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := ParseMarkdownFile(testFile)
	if err == nil {
		t.Error("Expected error for invalid frontmatter")
	}
}

func TestParseMarkdownFileNoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "no-frontmatter.md")

	content := `# Just markdown

No frontmatter here.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Parser may or may not return error for missing frontmatter
	// Just ensure it doesn't panic
	_, _ = ParseMarkdownFile(testFile)
}

func TestParseMarkdownFileMalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "malformed.md")

	content := `---
title: [invalid yaml
  - broken
---

Content
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := ParseMarkdownFile(testFile)
	if err == nil {
		t.Error("Expected error for malformed YAML")
	}
}

func TestParseMarkdownFileAuthorAsString(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "string-author.md")

	content := `---
title: "String Author Post"
slug: "string-author"
status: "publish"
type: "post"
author: "Jan Kowalski"
---

Content here.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	// Author should be 0 (unresolved) with raw value set
	if page.Author != 0 {
		t.Errorf("Expected Author=0 for string author, got %d", page.Author)
	}
	if page.AuthorRaw != "Jan Kowalski" {
		t.Errorf("Expected AuthorRaw='Jan Kowalski', got %v", page.AuthorRaw)
	}
}

func TestParseMarkdownFileAuthorAsInt(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "int-author.md")

	content := `---
title: "Int Author Post"
slug: "int-author"
status: "publish"
type: "post"
author: 5
---

Content here.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	if page.Author != 5 {
		t.Errorf("Expected Author=5, got %d", page.Author)
	}
	if page.AuthorRaw != nil {
		t.Errorf("Expected AuthorRaw=nil for int author, got %v", page.AuthorRaw)
	}
}

func TestParseMarkdownFileAuthorAsNumericString(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "numeric-string-author.md")

	content := `---
title: "Numeric String Author"
slug: "numeric-string-author"
status: "publish"
author: "42"
---

Content.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	if page.Author != 42 {
		t.Errorf("Expected Author=42, got %d", page.Author)
	}
	if page.AuthorRaw != nil {
		t.Errorf("Expected AuthorRaw=nil for numeric string, got %v", page.AuthorRaw)
	}
}

func TestParseMarkdownFileCategoriesAsStrings(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "string-cats.md")

	content := `---
title: "String Categories"
slug: "string-cats"
status: "publish"
type: "post"
categories:
  - "Humor"
  - "Technology"
---

Content.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	// Categories should be empty (unresolved), CategoriesRaw should have values
	if len(page.Categories) != 0 {
		t.Errorf("Expected empty Categories for string values, got %v", page.Categories)
	}
	if len(page.CategoriesRaw) != 2 {
		t.Errorf("Expected 2 CategoriesRaw, got %d", len(page.CategoriesRaw))
	}
}

func TestParseMarkdownFileCategoriesAsInts(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "int-cats.md")

	content := `---
title: "Int Categories"
slug: "int-cats"
status: "publish"
type: "post"
categories:
  - 1
  - 5
  - 10
---

Content.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	page, err := ParseMarkdownFile(testFile)
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	expectedCats := []int{1, 5, 10}
	if len(page.Categories) != 3 {
		t.Fatalf("Expected 3 categories, got %d", len(page.Categories))
	}
	for i, cat := range expectedCats {
		if page.Categories[i] != cat {
			t.Errorf("Expected category %d at index %d, got %d", cat, i, page.Categories[i])
		}
	}
	if page.CategoriesRaw != nil {
		t.Errorf("Expected nil CategoriesRaw for int categories, got %v", page.CategoriesRaw)
	}
}

func TestResolveFlexibleAuthor(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		wantID     int
		wantRawNil bool
	}{
		{"nil", nil, 0, true},
		{"int", 5, 5, true},
		{"float64", float64(3), 3, true},
		{"numeric string", "42", 42, true},
		{"name string", "Jan Kowalski", 0, false},
		{"slug string", "jan-kowalski", 0, false},
		{"unknown type (bool)", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, raw := resolveFlexibleAuthor(tt.input)
			if id != tt.wantID {
				t.Errorf("resolveFlexibleAuthor(%v) id = %d, want %d", tt.input, id, tt.wantID)
			}
			if tt.wantRawNil && raw != nil {
				t.Errorf("resolveFlexibleAuthor(%v) raw = %v, want nil", tt.input, raw)
			}
			if !tt.wantRawNil && raw == nil {
				t.Errorf("resolveFlexibleAuthor(%v) raw = nil, want non-nil", tt.input)
			}
		})
	}
}

func TestResolveFlexibleCategories(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		ids, raw := resolveFlexibleCategories(nil)
		if ids != nil || raw != nil {
			t.Errorf("Expected nil, nil; got %v, %v", ids, raw)
		}
	})

	t.Run("all ints", func(t *testing.T) {
		ids, raw := resolveFlexibleCategories([]interface{}{1, 5, 10})
		if len(ids) != 3 || ids[0] != 1 || ids[1] != 5 || ids[2] != 10 {
			t.Errorf("Expected [1 5 10], got %v", ids)
		}
		if raw != nil {
			t.Errorf("Expected nil raw, got %v", raw)
		}
	})

	t.Run("all float64", func(t *testing.T) {
		ids, raw := resolveFlexibleCategories([]interface{}{float64(2), float64(7)})
		if len(ids) != 2 || ids[0] != 2 || ids[1] != 7 {
			t.Errorf("Expected [2 7], got %v", ids)
		}
		if raw != nil {
			t.Errorf("Expected nil raw, got %v", raw)
		}
	})

	t.Run("all strings", func(t *testing.T) {
		input := []interface{}{"Humor", "Technology"}
		ids, raw := resolveFlexibleCategories(input)
		if ids != nil {
			t.Errorf("Expected nil ids for strings, got %v", ids)
		}
		if len(raw) != 2 {
			t.Errorf("Expected 2 raw values, got %d", len(raw))
		}
	})

	t.Run("numeric strings", func(t *testing.T) {
		ids, raw := resolveFlexibleCategories([]interface{}{"1", "5"})
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 5 {
			t.Errorf("Expected [1 5], got %v", ids)
		}
		if raw != nil {
			t.Errorf("Expected nil raw for numeric strings, got %v", raw)
		}
	})

	t.Run("mixed int and string", func(t *testing.T) {
		input := []interface{}{float64(1), "Humor"}
		ids, raw := resolveFlexibleCategories(input)
		if ids != nil {
			t.Errorf("Expected nil ids for mixed, got %v", ids)
		}
		if len(raw) != 2 {
			t.Errorf("Expected 2 raw values for mixed, got %d", len(raw))
		}
	})
}
