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
