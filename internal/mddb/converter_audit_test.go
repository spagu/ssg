// Package mddb - regression tests for the audited converter defects:
// GO-014 (single-element meta arrays flattened to scalars and dropped) and
// GO-030 (gRPC delivers all metadata values as strings, breaking .(float64)
// ID assertions).
package mddb

import (
	"testing"
)

// --- normalization helpers ---

func TestAsSlice(t *testing.T) {
	if got := asSlice(nil); got != nil {
		t.Errorf("asSlice(nil) = %v, want nil", got)
	}

	multi := asSlice([]any{"a", "b"})
	if len(multi) != 2 || multi[0] != "a" || multi[1] != "b" {
		t.Errorf("asSlice([a b]) = %v, want [a b]", multi)
	}

	// A flattened single-element array arrives as a scalar (GO-014)
	scalar := asSlice("golang")
	if len(scalar) != 1 || scalar[0] != "golang" {
		t.Errorf("asSlice(golang) = %v, want [golang]", scalar)
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		name   string
		in     any
		want   int
		wantOk bool
	}{
		{"float64", float64(5), 5, true},
		{"float64 truncated", float64(4.9), 4, true},
		{"int", int(3), 3, true},
		{"int64", int64(7), 7, true},
		{"numeric string (gRPC)", "42", 42, true},
		{"padded numeric string", " 8 ", 8, true},
		{"negative string", "-2", -2, true},
		{"non-numeric string", "golang", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asInt(tt.in)
			if got != tt.want || ok != tt.wantOk {
				t.Errorf("asInt(%v) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

// --- GO-014: single-element meta arrays must not be dropped ---

func TestDocument_ToPage_SingleElementMetaFlattened(t *testing.T) {
	// End to end: toDocument flattens 1-element meta arrays to scalars; the
	// converter must still populate tags/categories/aliases (GO-014).
	mddbDoc := mddbDocument{
		Key: "single-post",
		Meta: map[string][]any{
			"title":      {"Single"},
			"tags":       {"golang"},
			"categories": {float64(5)},
			"aliases":    {"/old-post/"},
		},
	}

	doc := mddbDoc.toDocument("blog")
	page, err := doc.ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if len(page.Tags) != 1 || page.Tags[0] != "golang" {
		t.Errorf("page.Tags = %v, want [golang] (GO-014)", page.Tags)
	}
	if len(page.Categories) != 1 || page.Categories[0] != 5 {
		t.Errorf("page.Categories = %v, want [5] (GO-014)", page.Categories)
	}
	if len(page.Aliases) != 1 || page.Aliases[0] != "/old-post/" {
		t.Errorf("page.Aliases = %v, want [/old-post/] (GO-014)", page.Aliases)
	}
}

func TestDocument_ToPage_ScalarStringCategory(t *testing.T) {
	page, err := (&Document{
		Key:      "test",
		Metadata: map[string]any{"categories": "News"},
	}).ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if len(page.Categories) != 0 {
		t.Errorf("page.Categories = %v, want empty (name category)", page.Categories)
	}
	if len(page.CategoriesRaw) != 1 || page.CategoriesRaw[0] != "News" {
		t.Errorf("page.CategoriesRaw = %v, want [News] (GO-014)", page.CategoriesRaw)
	}
}

func TestDocument_ToPage_MultiElementMetaUnchanged(t *testing.T) {
	page, err := (&Document{
		Key: "multi-post",
		Metadata: map[string]any{
			"tags":       []any{"go", "ssg"},
			"categories": []any{float64(1), float64(2)},
			"aliases":    []any{"/a/", "/b/"},
		},
	}).ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if len(page.Tags) != 2 {
		t.Errorf("page.Tags = %v, want 2 tags", page.Tags)
	}
	if len(page.Categories) != 2 {
		t.Errorf("page.Categories = %v, want 2 categories", page.Categories)
	}
	if len(page.Aliases) != 2 {
		t.Errorf("page.Aliases = %v, want 2 aliases", page.Aliases)
	}
}

// --- GO-030: gRPC string metadata must resolve to the same IDs as JSON ---

func TestDocument_ToPage_GRPCStringMetadata(t *testing.T) {
	// Over gRPC every meta value is a string (protoMetaToMetadata), and a
	// single-element list arrives as a plain string.
	page, err := (&Document{
		Key: "grpc-post",
		Metadata: map[string]any{
			"id":         "123",
			"author":     "7",
			"categories": []any{"5", "9"},
			"tags":       "golang",
		},
	}).ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.ID != 123 {
		t.Errorf("page.ID = %v, want 123 (GO-030)", page.ID)
	}
	if page.Author != 7 {
		t.Errorf("page.Author = %v, want 7 (GO-030)", page.Author)
	}
	if page.AuthorRaw != nil {
		t.Errorf("page.AuthorRaw = %v, want nil for numeric author", page.AuthorRaw)
	}
	if len(page.Categories) != 2 || page.Categories[0] != 5 || page.Categories[1] != 9 {
		t.Errorf("page.Categories = %v, want [5 9] (GO-030)", page.Categories)
	}
	if len(page.Tags) != 1 || page.Tags[0] != "golang" {
		t.Errorf("page.Tags = %v, want [golang]", page.Tags)
	}
}

func TestExtractCategory_GRPCStringMetadata(t *testing.T) {
	cat := ExtractCategory(Document{
		Key: "technology",
		Metadata: map[string]any{
			"id":     "5",
			"name":   "Technology",
			"count":  "42",
			"parent": "1",
		},
	})

	if cat.ID != 5 {
		t.Errorf("cat.ID = %v, want 5 (GO-030)", cat.ID)
	}
	if cat.Count != 42 {
		t.Errorf("cat.Count = %v, want 42 (GO-030)", cat.Count)
	}
	if cat.Parent != 1 {
		t.Errorf("cat.Parent = %v, want 1 (GO-030)", cat.Parent)
	}
	if cat.Name != "Technology" {
		t.Errorf("cat.Name = %v, want Technology", cat.Name)
	}
}

func TestExtractMedia_GRPCStringMetadata(t *testing.T) {
	media := ExtractMedia(Document{
		Key: "image-1",
		Metadata: map[string]any{
			"id":         "100",
			"media_type": "image",
			"media_details": map[string]interface{}{
				"width":  "1920",
				"height": "1080",
			},
		},
	})

	if media.ID != 100 {
		t.Errorf("media.ID = %v, want 100 (GO-030)", media.ID)
	}
	if int(media.MediaDetails.Width) != 1920 {
		t.Errorf("media.MediaDetails.Width = %v, want 1920 (GO-030)", media.MediaDetails.Width)
	}
	if int(media.MediaDetails.Height) != 1080 {
		t.Errorf("media.MediaDetails.Height = %v, want 1080 (GO-030)", media.MediaDetails.Height)
	}
}

func TestExtractAuthor_GRPCStringMetadata(t *testing.T) {
	author := ExtractAuthor(Document{
		Key:      "admin",
		Metadata: map[string]any{"id": "9", "name": "Admin"},
	})

	if author.ID != 9 {
		t.Errorf("author.ID = %v, want 9 (GO-030)", author.ID)
	}
	if author.Name != "Admin" {
		t.Errorf("author.Name = %v, want Admin", author.Name)
	}
}

func TestDocument_ToPage_Float64IDsStillWork(t *testing.T) {
	// HTTP/JSON regression guard: float64 IDs must keep working (GO-030).
	page, err := (&Document{
		Key: "json-post",
		Metadata: map[string]any{
			"id":         float64(11),
			"author":     float64(3),
			"categories": []any{float64(2)},
		},
	}).ToPage()
	if err != nil {
		t.Fatalf("ToPage() error = %v", err)
	}

	if page.ID != 11 || page.Author != 3 {
		t.Errorf("ID/Author = %v/%v, want 11/3", page.ID, page.Author)
	}
	if len(page.Categories) != 1 || page.Categories[0] != 2 {
		t.Errorf("page.Categories = %v, want [2]", page.Categories)
	}
}
