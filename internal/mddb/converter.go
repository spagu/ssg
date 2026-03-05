// Package mddb provides conversion utilities for mddb documents
package mddb

import (
	"fmt"
	"time"

	"github.com/spagu/ssg/internal/models"
)

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

	if id, ok := d.Metadata["id"].(float64); ok {
		page.ID = int(id)
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

	if author, ok := d.Metadata["author"].(float64); ok {
		page.Author = int(author)
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

	// Parse categories
	if cats, ok := d.Metadata["categories"].([]interface{}); ok {
		for _, cat := range cats {
			if catID, ok := cat.(float64); ok {
				page.Categories = append(page.Categories, int(catID))
			}
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

// ToMetadata extracts models.Metadata from mddb documents
func ToMetadata(docs []Document) (*models.Metadata, error) {
	metadata := &models.Metadata{}

	for _, doc := range docs {
		switch doc.Collection {
		case "categories":
			cat := extractCategory(doc)
			metadata.Categories = append(metadata.Categories, cat)
		case "media":
			media := extractMedia(doc)
			metadata.Media = append(metadata.Media, media)
		case "users":
			author := extractAuthor(doc)
			metadata.Users = append(metadata.Users, author)
		}
	}

	return metadata, nil
}

func extractCategory(doc Document) models.Category {
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

func extractMedia(doc Document) models.MediaItem {
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

func extractAuthor(doc Document) models.Author {
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
