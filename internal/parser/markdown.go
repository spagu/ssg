// Package parser handles parsing of content files (Markdown with YAML frontmatter)
package parser

import (
	"bufio"
	"os"
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
	}
}

// buildPage creates a Page from parsed content
func (p *markdownParser) buildPage() (*models.Page, error) {
	pf := &PageFrontmatter{}
	if err := yaml.Unmarshal([]byte(p.frontmatter.String()), pf); err != nil {
		return nil, err
	}

	page := pf.ToPage()
	page.Excerpt = strings.TrimSpace(p.excerpt.String())
	page.Content = strings.TrimSpace(p.content.String())

	return page, nil
}

// PageFrontmatter is a temporary struct for parsing frontmatter with string dates
type PageFrontmatter struct {
	ID         int    `yaml:"id"`
	Title      string `yaml:"title"`
	Slug       string `yaml:"slug"`
	Date       string `yaml:"date"`
	Modified   string `yaml:"modified"`
	Status     string `yaml:"status"`
	Type       string `yaml:"type"`
	Link       string `yaml:"link"`
	Author     int    `yaml:"author"`
	Categories []int  `yaml:"categories,omitempty"`
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

	return &models.Page{
		ID:         pf.ID,
		Title:      pf.Title,
		Slug:       pf.Slug,
		Date:       date,
		Modified:   modified,
		Status:     pf.Status,
		Type:       pf.Type,
		Link:       pf.Link,
		Author:     pf.Author,
		Categories: pf.Categories,
	}
}
