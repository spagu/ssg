// Package models defines data structures for content parsing
package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
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

	// Raw fields for flexible parsing (string or int values before resolution)
	AuthorRaw     interface{}   `yaml:"-" json:"-"` // Unresolved author (int or string)
	CategoriesRaw []interface{} `yaml:"-" json:"-"` // Unresolved categories (int or string values)
	Excerpt       string        `yaml:"-"`
	Content       string        `yaml:"-"`
	URLFormat     string        `yaml:"-"` // URL format: "date" or "slug" (set by generator)
	PageFormat    string        `yaml:"-"` // Page output format: "directory", "flat", or "both" (set by generator)
	SourceDir     string        `yaml:"-"` // Source directory path (for co-located asset copying)
	SourceFile    string        `yaml:"-"` // Source filename (e.g. "AUTHENTICATION.md") for .md link rewriting

	// SEO and metadata fields
	Description   string   `yaml:"description"`
	Keywords      string   `yaml:"keywords"`
	Lang          string   `yaml:"lang"`
	Canonical     string   `yaml:"canonical"`
	Robots        string   `yaml:"robots"`
	Sitemap       string   `yaml:"sitemap"`
	FeaturedImage string   `yaml:"featured_image"`
	Tags          []string `yaml:"tags,omitempty"`
	Category      string   `yaml:"category"`

	// Aliases are old paths that should redirect here. Each generates a
	// meta-refresh + canonical redirect stub excluded from the sitemap (SEO-002).
	Aliases []string `yaml:"aliases,omitempty"`

	// Series groups posts into an ordered set with a landing page and prev/next
	// navigation (AX-005). The neighbour fields are computed by the generator.
	Series          string `yaml:"series,omitempty"`
	SeriesPrevURL   string `yaml:"-"`
	SeriesPrevTitle string `yaml:"-"`
	SeriesNextURL   string `yaml:"-"`
	SeriesNextTitle string `yaml:"-"`

	// Computed reading metadata, exposed to templates as .WordCount / .ReadingTime
	// (BLOG-006). Set by ComputeReadingStats; not read from frontmatter.
	WordCount   int `yaml:"-"`
	ReadingTime int `yaml:"-"` // Estimated minutes at wordsPerMinute

	// HasMath marks pages containing math delimiters so KaTeX assets are injected
	// only where needed (AX-004). Set by the generator when math is enabled.
	HasMath bool `yaml:"-"`

	// PermalinkPath is a pre-expanded, sanitized relative output path (SEO-001).
	// When set by the generator from a configured permalink pattern it overrides
	// the default date/slug path for both URL and output-path computation. The
	// frontmatter Link field still takes priority over this.
	PermalinkPath string `yaml:"-"`

	// LangPrefix is a language segment (e.g. "en") prepended to the URL/output path
	// for non-default languages in multilingual builds (PLAT-005). Not applied when
	// an explicit Link is set.
	LangPrefix string `yaml:"-"`

	// Template selection
	Layout   string `yaml:"layout"`   // Custom layout template (e.g., "blog-hub", "landing")
	Template string `yaml:"template"` // Custom template name

	// Extra holds any additional frontmatter fields not explicitly defined
	// This allows templates to access custom fields like defaultVideo, playlist, etc.
	Extra map[string]interface{} `yaml:"-"`
}

// GetURL returns the URL path for this page/post
// Link field from frontmatter ALWAYS takes priority
// Posts without Link: use URLFormat ("date" or "slug")
// Pages without Link: use slug
// PageFormat "flat" returns .html suffix, "directory"/"both" returns trailing slash
func (p Page) GetURL() string {
	// Link field ALWAYS takes priority (for both posts and pages)
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

	basePath := p.getBasePath()
	if p.PageFormat == "flat" {
		return basePath + ".html"
	}
	return basePath + "/"
}

// getBasePath returns the base URL path without trailing slash or extension,
// applying any language prefix (PLAT-005).
func (p Page) getBasePath() string {
	base := p.getBasePathRaw()
	if p.LangPrefix != "" {
		return "/" + p.LangPrefix + base
	}
	return base
}

// getBasePathRaw computes the language-agnostic base path.
func (p Page) getBasePathRaw() string {
	// Configured permalink pattern (SEO-001) overrides the default date/slug path.
	if p.PermalinkPath != "" {
		return "/" + strings.Trim(SanitizeRelPath(p.PermalinkPath), "/")
	}
	if p.Type == "post" {
		if p.URLFormat == "slug" {
			return fmt.Sprintf("/%s", p.Slug)
		}
		return fmt.Sprintf("/%d/%02d/%02d/%s",
			p.Date.Year(), p.Date.Month(), p.Date.Day(), p.Slug)
	}
	return fmt.Sprintf("/%s", p.Slug)
}

// GetCanonical returns the full canonical URL for this page/post
func (p Page) GetCanonical(domain string) string {
	return fmt.Sprintf("https://%s%s", domain, p.GetURL())
}

// GetOutputPath returns the filesystem path for this page/post
// Link field from frontmatter ALWAYS takes priority.
//
// The returned sub-path is always sanitized (see SanitizeRelPath) so that
// untrusted slug/link values — e.g. supplied by a remote mddb server — can
// never escape the output directory via path traversal (SEC-001).
func (p Page) GetOutputPath() string {
	// Link field ALWAYS takes priority (for both posts and pages); no language prefix.
	if p.Link != "" {
		if u, err := url.Parse(p.Link); err == nil {
			return SanitizeRelPath(u.Path)
		}
	}

	sub := p.getOutputSubPath()
	if p.LangPrefix != "" {
		return SanitizeRelPath(p.LangPrefix + "/" + sub)
	}
	return SanitizeRelPath(sub)
}

// getOutputSubPath computes the language-agnostic output sub-path (SEO-001).
func (p Page) getOutputSubPath() string {
	// Configured permalink pattern (SEO-001) overrides the default date/slug path.
	if p.PermalinkPath != "" {
		return p.PermalinkPath
	}
	if p.Type == "post" {
		// URLFormat="slug" uses slug-only path
		if p.URLFormat == "slug" {
			return p.Slug
		}
		// Default: date-based path
		return fmt.Sprintf("%d/%02d/%02d/%s",
			p.Date.Year(), p.Date.Month(), p.Date.Day(), p.Slug)
	}
	// Pages: use slug
	return p.Slug
}

// SanitizeRelPath returns a cleaned, relative sub-path that cannot escape its
// root. Untrusted input (slug/link from mddb) may contain "..", absolute paths
// or Windows separators; anchoring at "/" before path.Clean collapses any such
// traversal, and the leading separator is then stripped to yield a safe
// relative path. Guards against path traversal / arbitrary write (SEC-001).
func SanitizeRelPath(p string) string {
	// Normalize Windows separators so "\.." cannot bypass the cleaning.
	p = strings.ReplaceAll(p, "\\", "/")
	// Anchor at root: path.Clean("/"+"../../etc") == "/etc", never above root.
	cleaned := path.Clean("/" + p)
	// Strip the leading slash to produce a relative path ("" stays "").
	return strings.TrimPrefix(cleaned, "/")
}

// wordsPerMinute is the assumed silent-reading speed used for ReadingTime (BLOG-006).
const wordsPerMinute = 200

// markupStripRe removes markup so word counts reflect prose, not tags/shortcodes:
// HTML tags (<...>), brace shortcodes ({{...}}) and bracket shortcodes ([name ...]).
var markupStripRe = regexp.MustCompile(`<[^>]*>|{{[^}]*}}|\[/?[a-zA-Z][^\]]*\]`)

// ComputeReadingStats computes WordCount and ReadingTime from Content (BLOG-006).
// Markup is stripped first so the count reflects readable prose. ReadingTime is
// ceil(words / wordsPerMinute), at least 1 minute for any non-empty text and 0
// for empty content. Safe to call multiple times (idempotent).
func (p *Page) ComputeReadingStats() {
	text := markupStripRe.ReplaceAllString(p.Content, " ")
	words := len(strings.Fields(text))
	p.WordCount = words
	if words == 0 {
		p.ReadingTime = 0
		return
	}
	minutes := (words + wordsPerMinute - 1) / wordsPerMinute
	if minutes < 1 {
		minutes = 1
	}
	p.ReadingTime = minutes
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

// Metadata represents the full metadata.json structure. Unconsumed export
// fields (e.g. exported_at) are simply not declared — the JSON decoder skips
// them (GO-042).
type Metadata struct {
	Categories []Category  `json:"categories"`
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

// ResolveFlexibleFields resolves raw author/category strings to integer IDs
// using reverse-lookup maps built from loaded metadata.
// Call this after all metadata (authors, categories) has been loaded.
func (sd *SiteData) ResolveFlexibleFields() {
	authorByName := make(map[string]int)
	authorBySlug := make(map[string]int)
	for _, a := range sd.Authors {
		authorByName[strings.ToLower(a.Name)] = a.ID
		authorBySlug[strings.ToLower(a.Slug)] = a.ID
	}

	catByName := make(map[string]int)
	catBySlug := make(map[string]int)
	for _, c := range sd.Categories {
		catByName[strings.ToLower(c.Name)] = c.ID
		catBySlug[strings.ToLower(c.Slug)] = c.ID
	}

	resolvePages := func(pages []Page) {
		for i := range pages {
			resolveAuthor(&pages[i], authorByName, authorBySlug)
			resolveCategories(&pages[i], catByName, catBySlug)
		}
	}

	resolvePages(sd.Pages)
	resolvePages(sd.Posts)
}

// resolveAuthor resolves AuthorRaw to Author ID
func resolveAuthor(p *Page, byName, bySlug map[string]int) {
	if p.AuthorRaw == nil || p.Author != 0 {
		return
	}
	switch v := p.AuthorRaw.(type) {
	case int:
		p.Author = v
	case float64:
		p.Author = int(v)
	case string:
		lower := strings.ToLower(v)
		if id, ok := byName[lower]; ok {
			p.Author = id
		} else if id, ok := bySlug[lower]; ok {
			p.Author = id
		}
		// If still 0 — try parsing as numeric string
		if p.Author == 0 {
			if parsed, err := strconv.Atoi(v); err == nil {
				p.Author = parsed
			}
		}
	}
}

// resolveCategories resolves CategoriesRaw to Categories IDs
func resolveCategories(p *Page, byName, bySlug map[string]int) {
	if len(p.CategoriesRaw) == 0 || len(p.Categories) > 0 {
		return
	}
	for _, raw := range p.CategoriesRaw {
		switch v := raw.(type) {
		case int:
			p.Categories = append(p.Categories, v)
		case float64:
			p.Categories = append(p.Categories, int(v))
		case string:
			lower := strings.ToLower(v)
			if id, ok := byName[lower]; ok {
				p.Categories = append(p.Categories, id)
			} else if id, ok := bySlug[lower]; ok {
				p.Categories = append(p.Categories, id)
			} else if parsed, err := strconv.Atoi(v); err == nil {
				p.Categories = append(p.Categories, parsed)
			}
		}
	}
}
