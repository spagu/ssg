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

func TestToMetadata(t *testing.T) {
	docs := []Document{
		{
			Collection: "categories",
			Key:        "tech",
			Metadata: map[string]any{
				"id":          float64(1),
				"name":        "Technology",
				"description": "Tech posts",
				"link":        "/category/tech/",
				"count":       float64(10),
				"parent":      float64(0),
			},
		},
		{
			Collection: "media",
			Key:        "img-1",
			Metadata: map[string]any{
				"id":         float64(100),
				"media_type": "image",
				"mime_type":  "image/png",
				"source_url": "https://example.com/img.png",
			},
		},
		{
			Collection: "users",
			Key:        "admin",
			Metadata: map[string]any{
				"id":   float64(1),
				"name": "Admin",
			},
		},
		{
			Collection: "unknown",
			Key:        "other",
			Metadata:   map[string]any{},
		},
	}

	metadata, err := ToMetadata(docs)
	if err != nil {
		t.Fatalf("ToMetadata() error = %v", err)
	}

	if len(metadata.Categories) != 1 {
		t.Errorf("len(Categories) = %v, want 1", len(metadata.Categories))
	}
	if metadata.Categories[0].Name != "Technology" {
		t.Errorf("Categories[0].Name = %v, want Technology", metadata.Categories[0].Name)
	}

	if len(metadata.Media) != 1 {
		t.Errorf("len(Media) = %v, want 1", len(metadata.Media))
	}
	if metadata.Media[0].MediaType != "image" {
		t.Errorf("Media[0].MediaType = %v, want image", metadata.Media[0].MediaType)
	}

	if len(metadata.Users) != 1 {
		t.Errorf("len(Users) = %v, want 1", len(metadata.Users))
	}
	if metadata.Users[0].Name != "Admin" {
		t.Errorf("Users[0].Name = %v, want Admin", metadata.Users[0].Name)
	}
}

func TestToMetadata_EmptyDocs(t *testing.T) {
	metadata, err := ToMetadata(nil)
	if err != nil {
		t.Fatalf("ToMetadata() error = %v", err)
	}
	if len(metadata.Categories) != 0 {
		t.Error("expected no categories")
	}
	if len(metadata.Media) != 0 {
		t.Error("expected no media")
	}
	if len(metadata.Users) != 0 {
		t.Error("expected no users")
	}
}

func TestExtractMedia_WithDetails(t *testing.T) {
	doc := Document{
		Key: "photo-1",
		Metadata: map[string]any{
			"id":         float64(200),
			"media_type": "image",
			"mime_type":  "image/jpeg",
			"source_url": "https://example.com/photo.jpg",
			"title": map[string]interface{}{
				"rendered": "Photo Title",
			},
			"media_details": map[string]interface{}{
				"width":  float64(1920),
				"height": float64(1080),
				"file":   "2024/01/photo.jpg",
			},
		},
	}

	media := extractMedia(doc)

	if media.ID != 200 {
		t.Errorf("media.ID = %v, want 200", media.ID)
	}
	if int(media.MediaDetails.Width) != 1920 {
		t.Errorf("media.MediaDetails.Width = %v, want 1920", media.MediaDetails.Width)
	}
	if int(media.MediaDetails.Height) != 1080 {
		t.Errorf("media.MediaDetails.Height = %v, want 1080", media.MediaDetails.Height)
	}
	if media.MediaDetails.File != "2024/01/photo.jpg" {
		t.Errorf("media.MediaDetails.File = %v, want '2024/01/photo.jpg'", media.MediaDetails.File)
	}
	if media.Title.Rendered != "Photo Title" {
		t.Errorf("media.Title.Rendered = %v, want 'Photo Title'", media.Title.Rendered)
	}
	if media.SourceURL != "https://example.com/photo.jpg" {
		t.Errorf("media.SourceURL = %v, want 'https://example.com/photo.jpg'", media.SourceURL)
	}
}

func TestExtractMedia_EmptyMetadata(t *testing.T) {
	doc := Document{
		Key:      "empty",
		Metadata: map[string]any{},
	}

	media := extractMedia(doc)

	if media.Slug != "empty" {
		t.Errorf("media.Slug = %v, want empty", media.Slug)
	}
	if media.ID != 0 {
		t.Errorf("media.ID = %v, want 0", media.ID)
	}
}

func TestExtractCategory_EmptyMetadata(t *testing.T) {
	doc := Document{
		Key:      "uncategorized",
		Metadata: map[string]any{},
	}

	cat := extractCategory(doc)

	if cat.Slug != "uncategorized" {
		t.Errorf("cat.Slug = %v, want uncategorized", cat.Slug)
	}
	if cat.ID != 0 {
		t.Errorf("cat.ID = %v, want 0", cat.ID)
	}
}

func TestExtractAuthor_EmptyMetadata(t *testing.T) {
	doc := Document{
		Key:      "anon",
		Metadata: map[string]any{},
	}

	author := extractAuthor(doc)

	if author.Slug != "anon" {
		t.Errorf("author.Slug = %v, want anon", author.Slug)
	}
	if author.ID != 0 {
		t.Errorf("author.ID = %v, want 0", author.ID)
	}
}

func TestToPages_EmptySlice(t *testing.T) {
	pages, err := ToPages(nil)
	if err != nil {
		t.Fatalf("ToPages() error = %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected empty pages, got %d", len(pages))
	}
}

func TestToPages_AllDrafts(t *testing.T) {
	docs := []Document{
		{Key: "d1", Metadata: map[string]any{"status": "draft"}},
		{Key: "d2", Metadata: map[string]any{"status": "private"}},
	}

	pages, err := ToPages(docs)
	if err != nil {
		t.Fatalf("ToPages() error = %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

func TestDocument_ToPage_AllDateFormats(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		wantYear int
	}{
		{"RFC3339", "2024-06-15T10:30:00Z", 2024},
		{"datetime no tz", "2024-06-15T10:30:00", 2024},
		{"datetime space", "2024-06-15 10:30:00", 2024},
		{"date only", "2024-06-15", 2024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Document{
				Key:      "test",
				Metadata: map[string]any{"date": tt.dateStr, "modified": tt.dateStr},
			}

			page, err := doc.ToPage()
			if err != nil {
				t.Fatalf("ToPage() error = %v", err)
			}
			if page.Date.Year() != tt.wantYear {
				t.Errorf("Date.Year = %v, want %v", page.Date.Year(), tt.wantYear)
			}
			if page.Modified.Year() != tt.wantYear {
				t.Errorf("Modified.Year = %v, want %v", page.Modified.Year(), tt.wantYear)
			}
		})
	}
}

func TestDocument_ToPage_InvalidDate(t *testing.T) {
	now := time.Now()
	doc := Document{
		Key:       "test",
		Metadata:  map[string]any{"date": "not-a-date", "modified": "also-bad"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}
	if !page.Date.Equal(now) {
		t.Error("Date should fall back to CreatedAt for invalid date string")
	}
	if !page.Modified.Equal(now) {
		t.Error("Modified should fall back to UpdatedAt for invalid modified string")
	}
}

func TestDocument_ToPage_NoCategories(t *testing.T) {
	doc := Document{
		Key:      "test",
		Metadata: map[string]any{},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}
	if len(page.Categories) != 0 {
		t.Errorf("expected no categories, got %v", page.Categories)
	}
}

func TestDocument_ToPage_SlugFallback(t *testing.T) {
	doc := Document{
		Key:      "my-key",
		Metadata: map[string]any{},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}
	if page.Slug != "my-key" {
		t.Errorf("Slug = %v, want my-key (fallback to Key)", page.Slug)
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

func TestDocument_ToPage_FlexibleAuthorString(t *testing.T) {
	doc := Document{
		Key: "test",
		Metadata: map[string]any{
			"author": "Jan Kowalski",
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.Author != 0 {
		t.Errorf("page.Author = %v, want 0 (unresolved string)", page.Author)
	}
	if page.AuthorRaw != "Jan Kowalski" {
		t.Errorf("page.AuthorRaw = %v, want 'Jan Kowalski'", page.AuthorRaw)
	}
}

func TestDocument_ToPage_FlexibleAuthorFloat(t *testing.T) {
	doc := Document{
		Key: "test",
		Metadata: map[string]any{
			"author": float64(7),
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.Author != 7 {
		t.Errorf("page.Author = %v, want 7", page.Author)
	}
	if page.AuthorRaw != nil {
		t.Errorf("page.AuthorRaw = %v, want nil", page.AuthorRaw)
	}
}

func TestDocument_ToPage_FlexibleCategoriesStrings(t *testing.T) {
	doc := Document{
		Key: "test",
		Metadata: map[string]any{
			"categories": []interface{}{"Humor", "Technology"},
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if len(page.Categories) != 0 {
		t.Errorf("page.Categories = %v, want empty (string values)", page.Categories)
	}
	if len(page.CategoriesRaw) != 2 {
		t.Errorf("page.CategoriesRaw length = %v, want 2", len(page.CategoriesRaw))
	}
}

func TestDocument_ToPage_FlexibleCategoriesMixed(t *testing.T) {
	doc := Document{
		Key: "test",
		Metadata: map[string]any{
			"categories": []interface{}{float64(1), "Humor"},
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	// Mixed values — should store raw for later resolution
	if len(page.Categories) != 0 {
		t.Errorf("page.Categories = %v, want empty (mixed values)", page.Categories)
	}
	if len(page.CategoriesRaw) != 2 {
		t.Errorf("page.CategoriesRaw length = %v, want 2", len(page.CategoriesRaw))
	}
}

func TestDocument_ToPage_FlexibleCategoriesInts(t *testing.T) {
	doc := Document{
		Key: "test",
		Metadata: map[string]any{
			"categories": []interface{}{float64(1), float64(5)},
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if len(page.Categories) != 2 || page.Categories[0] != 1 || page.Categories[1] != 5 {
		t.Errorf("page.Categories = %v, want [1 5]", page.Categories)
	}
	if len(page.CategoriesRaw) != 0 {
		t.Errorf("page.CategoriesRaw = %v, want empty", page.CategoriesRaw)
	}
}

func TestDocument_ToPage_ExtraFields(t *testing.T) {
	doc := Document{
		Key: "custom-page",
		Metadata: map[string]any{
			"title":        "Custom Page",
			"status":       "publish",
			"dupa":         "custom value",
			"defaultVideo": "https://youtube.com/xyz",
			"playlist":     []interface{}{"a", "b", "c"},
			"rating":       float64(4.5),
			"featured":     true,
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	// Standard fields should NOT be in Extra
	if _, ok := page.Extra["title"]; ok {
		t.Error("title should not be in Extra")
	}
	if _, ok := page.Extra["status"]; ok {
		t.Error("status should not be in Extra")
	}

	// Custom fields should be in Extra
	if page.Extra["dupa"] != "custom value" {
		t.Errorf("Extra[dupa] = %v, want 'custom value'", page.Extra["dupa"])
	}

	if page.Extra["defaultVideo"] != "https://youtube.com/xyz" {
		t.Errorf("Extra[defaultVideo] = %v, want 'https://youtube.com/xyz'", page.Extra["defaultVideo"])
	}

	if page.Extra["rating"] != float64(4.5) {
		t.Errorf("Extra[rating] = %v, want 4.5", page.Extra["rating"])
	}

	if page.Extra["featured"] != true {
		t.Errorf("Extra[featured] = %v, want true", page.Extra["featured"])
	}

	// Check array
	playlist, ok := page.Extra["playlist"].([]interface{})
	if !ok {
		t.Fatalf("Extra[playlist] is not []interface{}")
	}
	if len(playlist) != 3 {
		t.Errorf("Extra[playlist] length = %v, want 3", len(playlist))
	}
}

func TestDocument_ToPage_SEOFields(t *testing.T) {
	doc := Document{
		Key: "seo-page",
		Metadata: map[string]any{
			"title":          "SEO Page",
			"status":         "publish",
			"description":    "Page description for SEO",
			"keywords":       "ssg, static, generator",
			"lang":           "en_US",
			"canonical":      "https://example.com/seo-page/",
			"robots":         "index, follow",
			"featured_image": "https://example.com/image.jpg",
			"layout":         "landing",
			"template":       "special",
			"category":       "Technology",
			"tags":           []interface{}{"go", "ssg", "static"},
		},
	}

	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.Description != "Page description for SEO" {
		t.Errorf("Description = %v, want 'Page description for SEO'", page.Description)
	}

	if page.Keywords != "ssg, static, generator" {
		t.Errorf("Keywords = %v, want 'ssg, static, generator'", page.Keywords)
	}

	if page.Lang != "en_US" {
		t.Errorf("Lang = %v, want 'en_US'", page.Lang)
	}

	if page.Canonical != "https://example.com/seo-page/" {
		t.Errorf("Canonical = %v", page.Canonical)
	}

	if page.Robots != "index, follow" {
		t.Errorf("Robots = %v", page.Robots)
	}

	if page.FeaturedImage != "https://example.com/image.jpg" {
		t.Errorf("FeaturedImage = %v", page.FeaturedImage)
	}

	if page.Layout != "landing" {
		t.Errorf("Layout = %v, want 'landing'", page.Layout)
	}

	if page.Template != "special" {
		t.Errorf("Template = %v, want 'special'", page.Template)
	}

	if page.Category != "Technology" {
		t.Errorf("Category = %v, want 'Technology'", page.Category)
	}

	if len(page.Tags) != 3 || page.Tags[0] != "go" {
		t.Errorf("Tags = %v, want [go ssg static]", page.Tags)
	}
}
