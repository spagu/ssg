// Package models - tests for content models
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexIntUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected FlexInt
		wantErr  bool
	}{
		{
			name:     "integer value",
			input:    `123`,
			expected: FlexInt(123),
			wantErr:  false,
		},
		{
			name:     "string value",
			input:    `"456"`,
			expected: FlexInt(456),
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    `""`,
			expected: FlexInt(0),
			wantErr:  false,
		},
		{
			name:     "zero integer",
			input:    `0`,
			expected: FlexInt(0),
			wantErr:  false,
		},
		{
			name:     "negative integer",
			input:    `-10`,
			expected: FlexInt(-10),
			wantErr:  false,
		},
		{
			name:     "invalid string",
			input:    `"abc"`,
			expected: FlexInt(0),
			wantErr:  true,
		},
		{
			name:     "invalid type",
			input:    `true`,
			expected: FlexInt(0),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fi FlexInt
			err := json.Unmarshal([]byte(tt.input), &fi)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if fi != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, fi)
			}
		})
	}
}

func TestPageGetURL(t *testing.T) {
	tests := []struct {
		name     string
		page     Page
		expected string
	}{
		{
			name: "post with date (default URLFormat)",
			page: Page{
				Type: "post",
				Slug: "test-post",
				Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			},
			expected: "/2025/01/15/test-post/",
		},
		{
			name: "post with URLFormat=date",
			page: Page{
				Type:      "post",
				Slug:      "test-post",
				Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				URLFormat: "date",
			},
			expected: "/2025/01/15/test-post/",
		},
		{
			name: "post with URLFormat=slug",
			page: Page{
				Type:      "post",
				Slug:      "my-awesome-post",
				Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				URLFormat: "slug",
			},
			expected: "/my-awesome-post/",
		},
		{
			name: "post with URLFormat=slug and link field",
			page: Page{
				Type:      "post",
				Slug:      "ignored",
				Link:      "https://example.com/custom-url",
				Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				URLFormat: "slug",
			},
			expected: "/custom-url/",
		},
		{
			name: "page with slug only",
			page: Page{
				Type: "page",
				Slug: "about",
			},
			expected: "/about/",
		},
		{
			name: "page with link",
			page: Page{
				Type: "page",
				Slug: "ignored",
				Link: "https://example.com/custom/path",
			},
			expected: "/custom/path/",
		},
		{
			name: "page with link without trailing slash",
			page: Page{
				Type: "page",
				Slug: "ignored",
				Link: "https://example.com/my-page",
			},
			expected: "/my-page/",
		},
		{
			name: "page with link without leading slash",
			page: Page{
				Type: "page",
				Slug: "ignored",
				Link: "https://example.com/some/deep/path/",
			},
			expected: "/some/deep/path/",
		},
		{
			name: "page with invalid link falls back to slug",
			page: Page{
				Type: "page",
				Slug: "fallback-slug",
				Link: "://invalid",
			},
			expected: "/fallback-slug/",
		},
		{
			name: "page with relative path link",
			page: Page{
				Type: "page",
				Slug: "ignored",
				Link: "relative/path",
			},
			expected: "/relative/path/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.page.GetURL()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPageGetCanonical(t *testing.T) {
	page := Page{
		Type: "page",
		Slug: "about",
	}

	result := page.GetCanonical("example.com")
	expected := "https://example.com/about/"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestPageGetOutputPath(t *testing.T) {
	tests := []struct {
		name     string
		page     Page
		expected string
	}{
		{
			name: "post with date (default URLFormat)",
			page: Page{
				Type: "post",
				Slug: "my-post",
				Date: time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
			},
			expected: "2025/03/20/my-post",
		},
		{
			name: "post with URLFormat=date",
			page: Page{
				Type:      "post",
				Slug:      "my-post",
				Date:      time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
				URLFormat: "date",
			},
			expected: "2025/03/20/my-post",
		},
		{
			name: "post with URLFormat=slug",
			page: Page{
				Type:      "post",
				Slug:      "my-post",
				Date:      time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
				URLFormat: "slug",
			},
			expected: "my-post",
		},
		{
			name: "post with URLFormat=slug and link field",
			page: Page{
				Type:      "post",
				Slug:      "ignored",
				Link:      "https://example.com/custom/post-url",
				Date:      time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
				URLFormat: "slug",
			},
			expected: "custom/post-url",
		},
		{
			name: "page with slug",
			page: Page{
				Type: "page",
				Slug: "contact",
			},
			expected: "contact",
		},
		{
			name: "page with link",
			page: Page{
				Type: "page",
				Slug: "ignored",
				Link: "https://example.com/services/web/",
			},
			expected: "services/web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.page.GetOutputPath()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPageHasValidCategories(t *testing.T) {
	tests := []struct {
		name       string
		categories []int
		expected   bool
	}{
		{
			name:       "no categories",
			categories: []int{},
			expected:   false,
		},
		{
			name:       "only uncategorized (ID 1)",
			categories: []int{1},
			expected:   false,
		},
		{
			name:       "valid category",
			categories: []int{5},
			expected:   true,
		},
		{
			name:       "mixed categories",
			categories: []int{1, 5, 10},
			expected:   true,
		},
		{
			name:       "multiple uncategorized",
			categories: []int{1, 1, 1},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := Page{Categories: tt.categories}
			result := page.HasValidCategories()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMetadataUnmarshal(t *testing.T) {
	jsonData := `{
		"categories": [
			{"id": 1, "name": "Test", "slug": "test", "count": 5}
		],
		"exported_at": "2025-01-15T10:00:00Z",
		"media": [
			{
				"id": 100,
				"slug": "image",
				"media_type": "image",
				"mime_type": "image/jpeg",
				"source_url": "https://example.com/image.jpg",
				"media_details": {
					"width": 800,
					"height": "600",
					"file": "uploads/image.jpg"
				}
			}
		],
		"users": [
			{"id": 1, "name": "Admin", "slug": "admin"}
		]
	}`

	var metadata Metadata
	if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(metadata.Categories) != 1 {
		t.Errorf("Expected 1 category, got %d", len(metadata.Categories))
	}
	if metadata.Categories[0].Name != "Test" {
		t.Errorf("Expected category name 'Test', got %q", metadata.Categories[0].Name)
	}

	if len(metadata.Media) != 1 {
		t.Errorf("Expected 1 media item, got %d", len(metadata.Media))
	}
	if int(metadata.Media[0].MediaDetails.Width) != 800 {
		t.Errorf("Expected width 800, got %d", metadata.Media[0].MediaDetails.Width)
	}
	if int(metadata.Media[0].MediaDetails.Height) != 600 {
		t.Errorf("Expected height 600 (from string), got %d", metadata.Media[0].MediaDetails.Height)
	}

	if len(metadata.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(metadata.Users))
	}
}
