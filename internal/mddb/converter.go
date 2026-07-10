// Package mddb provides conversion utilities for mddb documents
package mddb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// asSlice normalizes a metadata value that may be either a scalar or a slice.
// toDocument/protoMetaToMetadata flatten single-element meta arrays to their
// scalar value, so a post with exactly one tag/category/alias arrives as a
// scalar — asserting only .([]interface{}) silently drops the field (GO-014).
func asSlice(v any) []any {
	switch s := v.(type) {
	case nil:
		return nil
	case []any:
		return s
	default:
		return []any{v}
	}
}

// asInt normalizes a numeric metadata value. HTTP/JSON delivers numbers as
// float64, while the gRPC transport delivers every meta value as a string —
// asserting .(float64) there silently yields 0 IDs (GO-030).
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

// stringSliceMeta returns the string elements of a metadata value that may
// arrive as a scalar or as a slice (GO-014). Non-string elements are skipped.
func stringSliceMeta(v any) []string {
	var out []string
	for _, item := range asSlice(v) {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// applyCategories resolves flexible categories metadata onto the page:
// numeric IDs (float64 from JSON, numeric strings over gRPC, GO-030) fill
// Categories, while any non-numeric name switches the whole list to
// CategoriesRaw for later resolution. A single category arrives flattened to
// a scalar (GO-014).
func applyCategories(page *models.Page, v any) {
	cats := asSlice(v)
	if len(cats) == 0 {
		return
	}

	hasNames := false
	for _, cat := range cats {
		if id, ok := asInt(cat); ok {
			page.Categories = append(page.Categories, id)
		} else if _, isString := cat.(string); isString {
			hasNames = true
		}
	}
	if hasNames {
		// Has non-numeric name values — store raw for later resolution
		page.Categories = nil
		page.CategoriesRaw = cats
	}
}

// ToPage converts an mddb Document to a models.Page
func (d *Document) ToPage() (*models.Page, error) {
	page := &models.Page{
		Content: d.Content,
		Slug:    d.Key,
	}

	// Extract metadata fields
	if title, ok := d.Metadata["title"].(string); ok {
		page.Title = title
	}

	// IDs arrive as float64 over HTTP/JSON but as strings over gRPC (GO-030)
	if id, ok := asInt(d.Metadata["id"]); ok {
		page.ID = id
	}

	if slug, ok := d.Metadata["slug"].(string); ok {
		page.Slug = slug
	}

	if status, ok := d.Metadata["status"].(string); ok {
		page.Status = status
	} else {
		page.Status = "publish" // Default to published
	}

	if docType, ok := d.Metadata["type"].(string); ok {
		page.Type = docType
	}

	if link, ok := d.Metadata["link"].(string); ok {
		page.Link = link
	}

	// Flexible author: accept an ID (float64 from JSON, numeric string over
	// gRPC, GO-030) or a name string stored raw for later resolution
	switch authorVal := d.Metadata["author"].(type) {
	case float64:
		page.Author = int(authorVal)
	case string:
		if id, ok := asInt(authorVal); ok {
			page.Author = id
		} else {
			page.AuthorRaw = authorVal
		}
	}

	if excerpt, ok := d.Metadata["excerpt"].(string); ok {
		page.Excerpt = excerpt
	}

	// Parse date
	if dateStr, ok := d.Metadata["date"].(string); ok {
		if t, err := parseDate(dateStr); err == nil {
			page.Date = t
		}
	}
	if page.Date.IsZero() {
		page.Date = d.CreatedAt
	}

	// Parse modified date
	if modStr, ok := d.Metadata["modified"].(string); ok {
		if t, err := parseDate(modStr); err == nil {
			page.Modified = t
		}
	}
	if page.Modified.IsZero() {
		page.Modified = d.UpdatedAt
	}

	applyCategories(page, d.Metadata["categories"])

	// SEO and additional standard fields
	if desc, ok := d.Metadata["description"].(string); ok {
		page.Description = desc
	}
	if keywords, ok := d.Metadata["keywords"].(string); ok {
		page.Keywords = keywords
	}
	if lang, ok := d.Metadata["lang"].(string); ok {
		page.Lang = lang
	}
	if canonical, ok := d.Metadata["canonical"].(string); ok {
		page.Canonical = canonical
	}
	if robots, ok := d.Metadata["robots"].(string); ok {
		page.Robots = robots
	}
	if sitemap, ok := d.Metadata["sitemap"].(string); ok {
		page.Sitemap = sitemap
	}
	if featuredImage, ok := d.Metadata["featured_image"].(string); ok {
		page.FeaturedImage = featuredImage
	}
	if category, ok := d.Metadata["category"].(string); ok {
		page.Category = category
	}
	if layout, ok := d.Metadata["layout"].(string); ok {
		page.Layout = layout
	}
	if template, ok := d.Metadata["template"].(string); ok {
		page.Template = template
	}

	// Parse tags — a single tag arrives flattened to a scalar (GO-014)
	page.Tags = stringSliceMeta(d.Metadata["tags"])

	// Parse aliases (old paths that redirect here, SEO-002); a single alias
	// arrives flattened to a scalar (GO-014)
	page.Aliases = stringSliceMeta(d.Metadata["aliases"])

	// Series grouping (AX-005)
	if series, ok := d.Metadata["series"].(string); ok {
		page.Series = series
	}

	// Copy ALL remaining metadata fields to Extra for dynamic template access
	// This allows templates to use any custom field like {{.Extra.dupa}}
	knownFields := map[string]bool{
		"id": true, "title": true, "slug": true, "status": true, "type": true,
		"link": true, "author": true, "excerpt": true, "date": true, "modified": true,
		"categories": true, "description": true, "keywords": true, "lang": true,
		"canonical": true, "robots": true, "sitemap": true, "featured_image": true, "tags": true,
		"category": true, "layout": true, "template": true, "aliases": true, "series": true,
	}

	page.Extra = make(map[string]interface{})
	for key, value := range d.Metadata {
		if !knownFields[key] {
			page.Extra[key] = value
		}
	}

	return page, nil
}

// ToPages converts multiple mddb Documents to models.Page slice
func ToPages(docs []Document) ([]models.Page, error) {
	var pages []models.Page
	for _, doc := range docs {
		page, err := doc.ToPage()
		if err != nil {
			return nil, fmt.Errorf("converting document %s: %w", doc.Key, err)
		}
		if page.Status == "publish" {
			pages = append(pages, *page)
		}
	}
	return pages, nil
}

// parseDate attempts to parse a date string in common formats
func parseDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// ExtractCategory builds a models.Category from an mddb category Document.
// Shared by the generator's mddb metadata loader (DRY, GO-006/GO-010).
func ExtractCategory(doc Document) models.Category {
	cat := models.Category{
		Slug: doc.Key,
	}

	if id, ok := doc.Metadata["id"].(float64); ok {
		cat.ID = int(id)
	}
	if name, ok := doc.Metadata["name"].(string); ok {
		cat.Name = name
	}
	if desc, ok := doc.Metadata["description"].(string); ok {
		cat.Description = desc
	}
	if link, ok := doc.Metadata["link"].(string); ok {
		cat.Link = link
	}
	if count, ok := doc.Metadata["count"].(float64); ok {
		cat.Count = int(count)
	}
	if parent, ok := doc.Metadata["parent"].(float64); ok {
		cat.Parent = int(parent)
	}

	return cat
}

// ExtractMedia builds a models.MediaItem (including media_details) from an mddb
// media Document. Shared by the metadata converter and the generator's mddb
// loader (DRY, GO-006/GO-010).
func ExtractMedia(doc Document) models.MediaItem {
	media := models.MediaItem{
		Slug: doc.Key,
	}

	if id, ok := doc.Metadata["id"].(float64); ok {
		media.ID = int(id)
	}
	if mediaType, ok := doc.Metadata["media_type"].(string); ok {
		media.MediaType = mediaType
	}
	if mimeType, ok := doc.Metadata["mime_type"].(string); ok {
		media.MimeType = mimeType
	}
	if sourceURL, ok := doc.Metadata["source_url"].(string); ok {
		media.SourceURL = sourceURL
	}
	if title, ok := doc.Metadata["title"].(map[string]interface{}); ok {
		if rendered, ok := title["rendered"].(string); ok {
			media.Title.Rendered = rendered
		}
	}
	if details, ok := doc.Metadata["media_details"].(map[string]interface{}); ok {
		if width, ok := details["width"].(float64); ok {
			media.MediaDetails.Width = models.FlexInt(int(width))
		}
		if height, ok := details["height"].(float64); ok {
			media.MediaDetails.Height = models.FlexInt(int(height))
		}
		if file, ok := details["file"].(string); ok {
			media.MediaDetails.File = file
		}
	}

	return media
}

// ExtractAuthor builds a models.Author from an mddb user Document.
// Shared by the metadata converter and the generator's mddb loader (DRY, GO-010).
func ExtractAuthor(doc Document) models.Author {
	author := models.Author{
		Slug: doc.Key,
	}

	if id, ok := doc.Metadata["id"].(float64); ok {
		author.ID = int(id)
	}
	if name, ok := doc.Metadata["name"].(string); ok {
		author.Name = name
	}

	return author
}
