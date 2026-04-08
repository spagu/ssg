// Package parser handles parsing of content files (Markdown with YAML frontmatter)
package parser

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
	"gopkg.in/yaml.v3"
)

// markdownParser handles state during markdown file parsing
type markdownParser struct {
	frontmatter      strings.Builder
	content          strings.Builder
	excerpt          strings.Builder
	inFrontmatter    bool
	inContent        bool
	inExcerpt        bool
	frontmatterEnded bool
}

// ParseMarkdownFile parses a markdown file with YAML frontmatter
func ParseMarkdownFile(filepath string) (*models.Page, error) {
	file, err := os.Open(filepath) // #nosec G304 -- CLI tool reads user's content files
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	p := &markdownParser{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		p.processLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return p.buildPage()
}

// processLine handles a single line during parsing
func (p *markdownParser) processLine(line string) {
	if p.handleFrontmatterDelimiter(line) {
		return
	}
	if p.inFrontmatter {
		p.frontmatter.WriteString(line + "\n")
		return
	}
	if p.frontmatterEnded {
		p.processContentLine(line)
	}
}

// handleFrontmatterDelimiter handles --- delimiters
func (p *markdownParser) handleFrontmatterDelimiter(line string) bool {
	if line != "---" || p.frontmatterEnded {
		return false
	}
	if !p.inFrontmatter {
		p.inFrontmatter = true
	} else {
		p.inFrontmatter = false
		p.frontmatterEnded = true
	}
	return true
}

// processContentLine handles lines after frontmatter
// If no ## Excerpt or ## Content markers are found, all content goes to content
func (p *markdownParser) processContentLine(line string) {
	if strings.HasPrefix(line, "## Excerpt") {
		p.inExcerpt = true
		p.inContent = false
		return
	}
	if strings.HasPrefix(line, "## Content") {
		p.inExcerpt = false
		p.inContent = true
		return
	}
	if strings.HasPrefix(line, "# ") {
		return // Skip title line
	}
	if p.inExcerpt && line != "" {
		p.excerpt.WriteString(line + "\n")
	} else if p.inContent {
		p.content.WriteString(line + "\n")
	} else if !p.inExcerpt && !p.inContent {
		// No markers found - treat all content as content (fallback mode)
		p.content.WriteString(line + "\n")
	}
}

// buildPage creates a Page from parsed content
func (p *markdownParser) buildPage() (*models.Page, error) {
	pf := &PageFrontmatter{}
	if err := yaml.Unmarshal([]byte(p.frontmatter.String()), pf); err != nil {
		return nil, err
	}

	// Parse all frontmatter into a map for Extra fields
	var allFields map[string]interface{}
	if err := yaml.Unmarshal([]byte(p.frontmatter.String()), &allFields); err != nil {
		return nil, err
	}

	page := pf.ToPage()
	page.Excerpt = strings.TrimSpace(p.excerpt.String())
	page.Content = strings.TrimSpace(p.content.String())

	// Copy extra fields (those not in the struct)
	page.Extra = extractExtraFields(allFields)

	return page, nil
}

// knownFields lists all fields that are handled by PageFrontmatter struct
var knownFields = map[string]bool{
	"id": true, "title": true, "slug": true, "date": true, "modified": true,
	"status": true, "type": true, "link": true, "author": true, "categories": true,
	"description": true, "keywords": true, "lang": true, "canonical": true,
	"robots": true, "featured_image": true, "tags": true, "category": true,
	"layout": true, "template": true,
}

// extractExtraFields returns fields not in knownFields
func extractExtraFields(allFields map[string]interface{}) map[string]interface{} {
	extra := make(map[string]interface{})
	for k, v := range allFields {
		if !knownFields[k] {
			extra[k] = v
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// PageFrontmatter is a temporary struct for parsing frontmatter with string dates
type PageFrontmatter struct {
	ID       int    `yaml:"id"`
	Title    string `yaml:"title"`
	Slug     string `yaml:"slug"`
	Date     string `yaml:"date"`
	Modified string `yaml:"modified"`
	Status   string `yaml:"status"`
	Type     string `yaml:"type"`
	Link     string `yaml:"link"`

	// Flexible fields: accept int or string for author and categories
	Author     interface{}   `yaml:"author"`
	Categories []interface{} `yaml:"categories,omitempty"`

	// SEO and metadata fields
	Description   string   `yaml:"description"`
	Keywords      string   `yaml:"keywords"`
	Lang          string   `yaml:"lang"`
	Canonical     string   `yaml:"canonical"`
	Robots        string   `yaml:"robots"`
	FeaturedImage string   `yaml:"featured_image"`
	Tags          []string `yaml:"tags,omitempty"`
	Category      string   `yaml:"category"`

	// Template selection
	Layout   string `yaml:"layout"`
	Template string `yaml:"template"`
}

// resolveFlexibleAuthor converts a flexible author value (int or string) from frontmatter.
// Returns (resolvedID, rawValue). If the value is an int, resolvedID is set immediately.
// If string, resolvedID=0 and rawValue holds the string for later resolution.
func resolveFlexibleAuthor(v interface{}) (int, interface{}) {
	if v == nil {
		return 0, nil
	}
	switch val := v.(type) {
	case int:
		return val, nil
	case float64:
		return int(val), nil
	case string:
		// Try numeric string first
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed, nil
		}
		// String name/slug — defer resolution
		return 0, val
	}
	return 0, nil
}

// resolveFlexibleCategories converts flexible category values (int or string) from frontmatter.
// Returns (resolvedIDs, rawValues). Integer values are resolved immediately.
// String values are stored in rawValues for later resolution.
func resolveFlexibleCategories(vals []interface{}) ([]int, []interface{}) {
	if len(vals) == 0 {
		return nil, nil
	}
	var resolved []int
	hasStrings := false
	for _, v := range vals {
		switch val := v.(type) {
		case int:
			resolved = append(resolved, val)
		case float64:
			resolved = append(resolved, int(val))
		case string:
			if parsed, err := strconv.Atoi(val); err == nil {
				resolved = append(resolved, parsed)
			} else {
				hasStrings = true
			}
		}
	}
	if hasStrings {
		// Has string values — store all raw for full resolution later
		return nil, vals
	}
	return resolved, nil
}

// parseFlexibleDate parses dates in multiple formats
// Supports: RFC3339 (2025-01-01T12:00:00Z), date-only (2025-01-01), datetime (2025-01-01T12:00:00)
func parseFlexibleDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}

	// List of formats to try (most specific first)
	formats := []string{
		time.RFC3339,          // 2025-01-01T12:00:00Z
		"2006-01-02T15:04:05", // 2025-01-01T12:00:00
		"2006-01-02 15:04:05", // 2025-01-01 12:00:00
		"2006-01-02",          // 2025-01-01
		"02-01-2006",          // 01-01-2025
		"2006/01/02",          // 2025/01/01
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, dateStr); err == nil {
			return parsed
		}
	}

	return time.Time{}
}

// ParseFrontmatterWithDates handles date parsing from frontmatter
func (pf *PageFrontmatter) ToPage() *models.Page {
	date := parseFlexibleDate(pf.Date)
	modified := parseFlexibleDate(pf.Modified)

	authorID, authorRaw := resolveFlexibleAuthor(pf.Author)
	catIDs, catRaw := resolveFlexibleCategories(pf.Categories)

	return &models.Page{
		ID:            pf.ID,
		Title:         pf.Title,
		Slug:          pf.Slug,
		Date:          date,
		Modified:      modified,
		Status:        pf.Status,
		Type:          pf.Type,
		Link:          pf.Link,
		Author:        authorID,
		AuthorRaw:     authorRaw,
		Categories:    catIDs,
		CategoriesRaw: catRaw,
		// SEO and metadata fields
		Description:   pf.Description,
		Keywords:      pf.Keywords,
		Lang:          pf.Lang,
		Canonical:     pf.Canonical,
		Robots:        pf.Robots,
		FeaturedImage: pf.FeaturedImage,
		Tags:          pf.Tags,
		Category:      pf.Category,
		// Template selection
		Layout:   pf.Layout,
		Template: pf.Template,
	}
}
