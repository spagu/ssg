package mddb

import (
	"testing"
	"time"
)

func TestDocument_ToPage(t *testing.T) {
	now := time.Now()

	doc := Document{
		Key:        "hello-world",
		Collection: "blog",
		Content:    "# Hello World\n\nThis is content.",
		Metadata: map[string]any{
			"id":         float64(123),
			"title":      "Hello World",
			"slug":       "hello-world",
			"status":     "publish",
			"type":       "post",
			"link":       "/blog/hello-world/",
			"author":     float64(1),
			"excerpt":    "This is excerpt",
			"date":       "2024-01-15T10:30:00",
			"modified":   "2024-01-16T11:00:00",
			"categories": []interface{}{float64(1), float64(2)},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	page, err := doc.ToPage()

	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.ID != 123 {
		t.Errorf("page.ID = %v, want 123", page.ID)
	}

	if page.Title != "Hello World" {
		t.Errorf("page.Title = %v, want 'Hello World'", page.Title)
	}

	if page.Slug != "hello-world" {
		t.Errorf("page.Slug = %v, want 'hello-world'", page.Slug)
	}

	if page.Status != "publish" {
		t.Errorf("page.Status = %v, want 'publish'", page.Status)
	}

	if page.Type != "post" {
		t.Errorf("page.Type = %v, want 'post'", page.Type)
	}

	if page.Link != "/blog/hello-world/" {
		t.Errorf("page.Link = %v, want '/blog/hello-world/'", page.Link)
	}

	if page.Author != 1 {
		t.Errorf("page.Author = %v, want 1", page.Author)
	}

	if page.Excerpt != "This is excerpt" {
		t.Errorf("page.Excerpt = %v, want 'This is excerpt'", page.Excerpt)
	}

	if len(page.Categories) != 2 {
		t.Errorf("len(page.Categories) = %v, want 2", len(page.Categories))
	}

	if page.Date.Year() != 2024 || page.Date.Month() != 1 || page.Date.Day() != 15 {
		t.Errorf("page.Date = %v, want 2024-01-15", page.Date)
	}
}

func TestDocument_ToPage_DefaultStatus(t *testing.T) {
	doc := Document{
		Key:        "test",
		Collection: "blog",
		Metadata:   map[string]any{},
	}

	page, err := doc.ToPage()

	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.Status != "publish" {
		t.Errorf("page.Status = %v, want 'publish' (default)", page.Status)
	}
}

func TestDocument_ToPage_FallbackDates(t *testing.T) {
	now := time.Now()

	doc := Document{
		Key:        "test",
		Collection: "blog",
		Metadata:   map[string]any{},
		CreatedAt:  now,
		UpdatedAt:  now.Add(time.Hour),
	}

	page, err := doc.ToPage()

	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if !page.Date.Equal(now) {
		t.Errorf("page.Date should fall back to CreatedAt")
	}

	if !page.Modified.Equal(now.Add(time.Hour)) {
		t.Errorf("page.Modified should fall back to UpdatedAt")
	}
}

func TestToPages(t *testing.T) {
	docs := []Document{
		{
			Key:      "post-1",
			Metadata: map[string]any{"status": "publish", "title": "Post 1"},
		},
		{
			Key:      "post-2",
			Metadata: map[string]any{"status": "draft", "title": "Post 2"},
		},
		{
			Key:      "post-3",
			Metadata: map[string]any{"status": "publish", "title": "Post 3"},
		},
	}

	pages, err := ToPages(docs)

	if err != nil {
		t.Fatalf("ToPages() error = %v", err)
	}

	// Should only include published pages
	if len(pages) != 2 {
		t.Errorf("len(pages) = %v, want 2 (only published)", len(pages))
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"2024-01-15T10:30:00Z", false},
		{"2024-01-15T10:30:00", false},
		{"2024-01-15 10:30:00", false},
		{"2024-01-15", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestExtractCategory(t *testing.T) {
	doc := Document{
		Key: "technology",
		Metadata: map[string]any{
			"id":          float64(5),
			"name":        "Technology",
			"description": "Tech posts",
			"link":        "/category/technology/",
			"count":       float64(42),
			"parent":      float64(1),
		},
	}

	cat := extractCategory(doc)

	if cat.ID != 5 {
		t.Errorf("cat.ID = %v, want 5", cat.ID)
	}

	if cat.Slug != "technology" {
		t.Errorf("cat.Slug = %v, want 'technology'", cat.Slug)
	}

	if cat.Name != "Technology" {
		t.Errorf("cat.Name = %v, want 'Technology'", cat.Name)
	}

	if cat.Count != 42 {
		t.Errorf("cat.Count = %v, want 42", cat.Count)
	}
}

func TestExtractMedia(t *testing.T) {
	doc := Document{
		Key: "image-1",
		Metadata: map[string]any{
			"id":         float64(100),
			"media_type": "image",
			"mime_type":  "image/jpeg",
			"source_url": "https://example.com/image.jpg",
			"title": map[string]interface{}{
				"rendered": "My Image",
			},
		},
	}

	media := extractMedia(doc)

	if media.ID != 100 {
		t.Errorf("media.ID = %v, want 100", media.ID)
	}

	if media.MediaType != "image" {
		t.Errorf("media.MediaType = %v, want 'image'", media.MediaType)
	}

	if media.MimeType != "image/jpeg" {
		t.Errorf("media.MimeType = %v, want 'image/jpeg'", media.MimeType)
	}

	if media.Title.Rendered != "My Image" {
		t.Errorf("media.Title.Rendered = %v, want 'My Image'", media.Title.Rendered)
	}
}

func TestExtractAuthor(t *testing.T) {
	doc := Document{
		Key: "john-doe",
		Metadata: map[string]any{
			"id":   float64(1),
			"name": "John Doe",
		},
	}

	author := extractAuthor(doc)

	if author.ID != 1 {
		t.Errorf("author.ID = %v, want 1", author.ID)
	}

	if author.Slug != "john-doe" {
		t.Errorf("author.Slug = %v, want 'john-doe'", author.Slug)
	}

	if author.Name != "John Doe" {
		t.Errorf("author.Name = %v, want 'John Doe'", author.Name)
	}
}
