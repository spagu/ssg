// Package models defines data structures for content parsing
package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FlexInt is an int that can be unmarshaled from either int or string JSON
type FlexInt int

// UnmarshalJSON implements custom unmarshaling for FlexInt
func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	// Try int first
	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		*fi = FlexInt(intVal)
		return nil
	}

	// Try string
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		if strVal == "" {
			*fi = 0
			return nil
		}
		parsed, err := strconv.Atoi(strVal)
		if err != nil {
			return fmt.Errorf("cannot parse %q as int: %w", strVal, err)
		}
		*fi = FlexInt(parsed)
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into FlexInt", string(data))
}

// Page represents a page or post content with frontmatter metadata
type Page struct {
	ID         int       `yaml:"id"`
	Title      string    `yaml:"title"`
	Slug       string    `yaml:"slug"`
	Date       time.Time `yaml:"date"`
	Modified   time.Time `yaml:"modified"`
	Status     string    `yaml:"status"`
	Type       string    `yaml:"type"`
	Link       string    `yaml:"link"`
	Author     int       `yaml:"author"`
	Categories []int     `yaml:"categories,omitempty"`
	Excerpt    string    `yaml:"-"`
	Content    string    `yaml:"-"`
	URLFormat  string    `yaml:"-"` // URL format: "date" or "slug" (set by generator)
}

// GetURL returns the URL path for this page/post
// Posts use date-based URLs by default: /YYYY/MM/DD/slug/
// With URLFormat="slug", posts use: /slug/
// Pages always use simple URLs: /slug/ or path from Link field
func (p Page) GetURL() string {
	if p.Type == "post" {
		// If URLFormat is "slug", use slug-only URL
		if p.URLFormat == "slug" {
			// Check if Link field has a custom path
			if p.Link != "" {
				if u, err := url.Parse(p.Link); err == nil {
					path := u.Path
					if !strings.HasPrefix(path, "/") {
						path = "/" + path
					}
					if !strings.HasSuffix(path, "/") {
						path = path + "/"
					}
					return path
				}
			}
			return fmt.Sprintf("/%s/", p.Slug)
		}
		// Default: date-based URL
		return fmt.Sprintf("/%d/%02d/%02d/%s/",
			p.Date.Year(), p.Date.Month(), p.Date.Day(), p.Slug)
	}
	// Pages: If Link meta is present, try to extract path from it to preserve hierarchy
	if p.Link != "" {
		if u, err := url.Parse(p.Link); err == nil {
			path := u.Path
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if !strings.HasSuffix(path, "/") {
				path = path + "/"
			}
			return path
		}
	}
	return fmt.Sprintf("/%s/", p.Slug)
}

// GetCanonical returns the full canonical URL for this page/post
func (p Page) GetCanonical(domain string) string {
	return fmt.Sprintf("https://%s%s", domain, p.GetURL())
}

// GetOutputPath returns the filesystem path for this page/post
func (p Page) GetOutputPath() string {
	if p.Type == "post" {
		// If URLFormat is "slug", use slug-only path
		if p.URLFormat == "slug" {
			// Check if Link field has a custom path
			if p.Link != "" {
				if u, err := url.Parse(p.Link); err == nil {
					path := u.Path
					return strings.Trim(path, "/")
				}
			}
			return p.Slug
		}
		// Default: date-based path
		return fmt.Sprintf("%d/%02d/%02d/%s",
			p.Date.Year(), p.Date.Month(), p.Date.Day(), p.Slug)
	}
	// Pages: Use path from Link if available (via GetURL logic)
	path := p.GetURL()
	return strings.Trim(path, "/")
}

// HasValidCategories returns true if post has categories other than "Bez kategorii" (ID 1)
func (p Page) HasValidCategories() bool {
	for _, catID := range p.Categories {
		if catID != 1 { // 1 is usually "Bez kategorii"
			return true
		}
	}
	return false
}

// Category represents a content category
type Category struct {
	ID          int    `json:"id"`
	Count       int    `json:"count"`
	Description string `json:"description"`
	Link        string `json:"link"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Parent      int    `json:"parent"`
}

// Author represents a site author
type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// MediaItem represents a media file
type MediaItem struct {
	ID           int    `json:"id"`
	Slug         string `json:"slug"`
	Title        Title  `json:"title"`
	MediaType    string `json:"media_type"`
	MimeType     string `json:"mime_type"`
	SourceURL    string `json:"source_url"`
	MediaDetails struct {
		Width  FlexInt `json:"width"`
		Height FlexInt `json:"height"`
		File   string  `json:"file"`
	} `json:"media_details"`
}

// Title represents rendered title
type Title struct {
	Rendered string `json:"rendered"`
}

// Metadata represents the full metadata.json structure
type Metadata struct {
	Categories []Category  `json:"categories"`
	ExportedAt string      `json:"exported_at"`
	Media      []MediaItem `json:"media"`
	Users      []Author    `json:"users"`
}

// SiteData holds all parsed content for template rendering
type SiteData struct {
	Domain     string
	Pages      []Page
	Posts      []Page
	Categories map[int]Category
	Media      map[int]MediaItem
	Authors    map[int]Author
}
