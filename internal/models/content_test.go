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
			name: "post with URLFormat=date and link field (link takes priority)",
			page: Page{
				Type:      "post",
				Slug:      "ignored",
				Link:      "https://example.com/my-custom-link",
				Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				URLFormat: "date",
			},
			expected: "/my-custom-link/",
		},
		{
			name: "post with default URLFormat and link field (link takes priority)",
			page: Page{
				Type: "post",
				Slug: "ignored",
				Link: "https://example.com/link-priority",
				Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			},
			expected: "/link-priority/",
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
			name: "post with URLFormat=date and link field (link takes priority)",
			page: Page{
				Type:      "post",
				Slug:      "ignored",
				Link:      "https://example.com/link/takes/priority",
				Date:      time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
				URLFormat: "date",
			},
			expected: "link/takes/priority",
		},
		{
			name: "post with default URLFormat and link field (link takes priority)",
			page: Page{
				Type: "post",
				Slug: "ignored",
				Link: "https://example.com/always-link",
				Date: time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
			},
			expected: "always-link",
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

func TestResolveFlexibleFields_AuthorByName(t *testing.T) {
	sd := &SiteData{
		Authors: map[int]Author{
			3: {ID: 3, Name: "Jan Kowalski", Slug: "jan-kowalski"},
		},
		Categories: make(map[int]Category),
		Posts: []Page{
			{AuthorRaw: "Jan Kowalski"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Posts[0].Author != 3 {
		t.Errorf("Expected Author=3 resolved by name, got %d", sd.Posts[0].Author)
	}
}

func TestResolveFlexibleFields_AuthorBySlug(t *testing.T) {
	sd := &SiteData{
		Authors: map[int]Author{
			5: {ID: 5, Name: "Anna Nowak", Slug: "anna-nowak"},
		},
		Categories: make(map[int]Category),
		Pages: []Page{
			{AuthorRaw: "anna-nowak"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Pages[0].Author != 5 {
		t.Errorf("Expected Author=5 resolved by slug, got %d", sd.Pages[0].Author)
	}
}

func TestResolveFlexibleFields_AuthorCaseInsensitive(t *testing.T) {
	sd := &SiteData{
		Authors: map[int]Author{
			1: {ID: 1, Name: "Admin User", Slug: "admin-user"},
		},
		Categories: make(map[int]Category),
		Posts: []Page{
			{AuthorRaw: "admin user"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Posts[0].Author != 1 {
		t.Errorf("Expected Author=1 (case-insensitive), got %d", sd.Posts[0].Author)
	}
}

func TestResolveFlexibleFields_AuthorNumericString(t *testing.T) {
	sd := &SiteData{
		Authors:    make(map[int]Author),
		Categories: make(map[int]Category),
		Posts: []Page{
			{AuthorRaw: "42"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Posts[0].Author != 42 {
		t.Errorf("Expected Author=42 (numeric string fallback), got %d", sd.Posts[0].Author)
	}
}

func TestResolveFlexibleFields_AuthorAlreadyResolved(t *testing.T) {
	sd := &SiteData{
		Authors:    map[int]Author{3: {ID: 3, Name: "Test", Slug: "test"}},
		Categories: make(map[int]Category),
		Posts: []Page{
			{Author: 3, AuthorRaw: "something-else"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Posts[0].Author != 3 {
		t.Errorf("Expected Author=3 (already resolved), got %d", sd.Posts[0].Author)
	}
}

func TestResolveFlexibleFields_CategoriesByName(t *testing.T) {
	sd := &SiteData{
		Authors: make(map[int]Author),
		Categories: map[int]Category{
			59: {ID: 59, Name: "Humor", Slug: "humor"},
			10: {ID: 10, Name: "Technology", Slug: "technology"},
		},
		Posts: []Page{
			{CategoriesRaw: []interface{}{"Humor", "Technology"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 2 {
		t.Fatalf("Expected 2 categories, got %d", len(sd.Posts[0].Categories))
	}
	if sd.Posts[0].Categories[0] != 59 {
		t.Errorf("Expected category 59 (Humor), got %d", sd.Posts[0].Categories[0])
	}
	if sd.Posts[0].Categories[1] != 10 {
		t.Errorf("Expected category 10 (Technology), got %d", sd.Posts[0].Categories[1])
	}
}

func TestResolveFlexibleFields_CategoriesBySlug(t *testing.T) {
	sd := &SiteData{
		Authors: make(map[int]Author),
		Categories: map[int]Category{
			5: {ID: 5, Name: "Web Dev", Slug: "web-dev"},
		},
		Posts: []Page{
			{CategoriesRaw: []interface{}{"web-dev"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 1 || sd.Posts[0].Categories[0] != 5 {
		t.Errorf("Expected [5], got %v", sd.Posts[0].Categories)
	}
}

func TestResolveFlexibleFields_CategoriesCaseInsensitive(t *testing.T) {
	sd := &SiteData{
		Authors: make(map[int]Author),
		Categories: map[int]Category{
			2: {ID: 2, Name: "Humor", Slug: "humor"},
		},
		Posts: []Page{
			{CategoriesRaw: []interface{}{"humor"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 1 || sd.Posts[0].Categories[0] != 2 {
		t.Errorf("Expected [2], got %v", sd.Posts[0].Categories)
	}
}

func TestResolveFlexibleFields_CategoriesAlreadyResolved(t *testing.T) {
	sd := &SiteData{
		Authors:    make(map[int]Author),
		Categories: make(map[int]Category),
		Posts: []Page{
			{Categories: []int{1, 5}, CategoriesRaw: []interface{}{"Ignored"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 2 || sd.Posts[0].Categories[0] != 1 {
		t.Errorf("Expected [1 5] (already resolved), got %v", sd.Posts[0].Categories)
	}
}

func TestResolveFlexibleFields_CategoriesNumericStrings(t *testing.T) {
	sd := &SiteData{
		Authors:    make(map[int]Author),
		Categories: make(map[int]Category),
		Posts: []Page{
			{CategoriesRaw: []interface{}{"10", "20"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 2 || sd.Posts[0].Categories[0] != 10 || sd.Posts[0].Categories[1] != 20 {
		t.Errorf("Expected [10 20], got %v", sd.Posts[0].Categories)
	}
}

func TestResolveFlexibleFields_MixedIntAndStringCategories(t *testing.T) {
	sd := &SiteData{
		Authors: make(map[int]Author),
		Categories: map[int]Category{
			59: {ID: 59, Name: "Humor", Slug: "humor"},
		},
		Posts: []Page{
			{CategoriesRaw: []interface{}{float64(1), "Humor"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 2 {
		t.Fatalf("Expected 2 categories, got %d", len(sd.Posts[0].Categories))
	}
	if sd.Posts[0].Categories[0] != 1 {
		t.Errorf("Expected category 1, got %d", sd.Posts[0].Categories[0])
	}
	if sd.Posts[0].Categories[1] != 59 {
		t.Errorf("Expected category 59 (Humor), got %d", sd.Posts[0].Categories[1])
	}
}

func TestResolveFlexibleFields_PagesAndPosts(t *testing.T) {
	sd := &SiteData{
		Authors: map[int]Author{
			1: {ID: 1, Name: "Admin", Slug: "admin"},
		},
		Categories: map[int]Category{
			2: {ID: 2, Name: "News", Slug: "news"},
		},
		Pages: []Page{
			{AuthorRaw: "Admin"},
		},
		Posts: []Page{
			{AuthorRaw: "admin", CategoriesRaw: []interface{}{"News"}},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Pages[0].Author != 1 {
		t.Errorf("Page Author: expected 1, got %d", sd.Pages[0].Author)
	}
	if sd.Posts[0].Author != 1 {
		t.Errorf("Post Author: expected 1, got %d", sd.Posts[0].Author)
	}
	if len(sd.Posts[0].Categories) != 1 || sd.Posts[0].Categories[0] != 2 {
		t.Errorf("Post Categories: expected [2], got %v", sd.Posts[0].Categories)
	}
}

func TestResolveFlexibleFields_UnknownAuthorString(t *testing.T) {
	sd := &SiteData{
		Authors:    map[int]Author{1: {ID: 1, Name: "Admin", Slug: "admin"}},
		Categories: make(map[int]Category),
		Posts: []Page{
			{AuthorRaw: "unknown-person"},
		},
	}

	sd.ResolveFlexibleFields()

	if sd.Posts[0].Author != 0 {
		t.Errorf("Expected Author=0 for unknown string, got %d", sd.Posts[0].Author)
	}
}

func TestResolveFlexibleFields_UnknownCategoryString(t *testing.T) {
	sd := &SiteData{
		Authors:    make(map[int]Author),
		Categories: map[int]Category{1: {ID: 1, Name: "News", Slug: "news"}},
		Posts: []Page{
			{CategoriesRaw: []interface{}{"nonexistent"}},
		},
	}

	sd.ResolveFlexibleFields()

	if len(sd.Posts[0].Categories) != 0 {
		t.Errorf("Expected empty categories for unknown string, got %v", sd.Posts[0].Categories)
	}
}

func TestGetURLWithPageFormat(t *testing.T) {
	tests := []struct {
		name    string
		page    Page
		wantURL string
	}{
		{
			name:    "directory format (default)",
			page:    Page{Slug: "about", Type: "page"},
			wantURL: "/about/",
		},
		{
			name:    "flat format page",
			page:    Page{Slug: "about", Type: "page", PageFormat: "flat"},
			wantURL: "/about.html",
		},
		{
			name:    "both format uses directory URL",
			page:    Page{Slug: "about", Type: "page", PageFormat: "both"},
			wantURL: "/about/",
		},
		{
			name:    "flat format post with date",
			page:    Page{Slug: "hello", Type: "post", Date: time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC), PageFormat: "flat"},
			wantURL: "/2026/03/30/hello.html",
		},
		{
			name:    "flat format post with slug URL",
			page:    Page{Slug: "hello", Type: "post", URLFormat: "slug", PageFormat: "flat"},
			wantURL: "/hello.html",
		},
		{
			name:    "directory format post with date",
			page:    Page{Slug: "hello", Type: "post", Date: time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)},
			wantURL: "/2026/03/30/hello/",
		},
		{
			name:    "link always takes priority over page format",
			page:    Page{Slug: "about", Type: "page", Link: "https://example.com/custom/path", PageFormat: "flat"},
			wantURL: "/custom/path/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.page.GetURL()
			if got != tt.wantURL {
				t.Errorf("GetURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
