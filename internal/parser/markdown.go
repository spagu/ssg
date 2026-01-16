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

// ParseMarkdownFile parses a markdown file with YAML frontmatter
func ParseMarkdownFile(filepath string) (*models.Page, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var frontmatter strings.Builder
	var content strings.Builder
	var excerpt strings.Builder

	inFrontmatter := false
	inContent := false
	inExcerpt := false
	frontmatterEnded := false

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" && !frontmatterEnded {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			inFrontmatter = false
			frontmatterEnded = true
			continue
		}

		if inFrontmatter {
			frontmatter.WriteString(line + "\n")
			continue
		}

		if frontmatterEnded {
			if strings.HasPrefix(line, "## Excerpt") {
				inExcerpt = true
				inContent = false
				continue
			}
			if strings.HasPrefix(line, "## Content") {
				inExcerpt = false
				inContent = true
				continue
			}
			if strings.HasPrefix(line, "# ") {
				// Skip title line
				continue
			}

			if inExcerpt {
				if line != "" {
					excerpt.WriteString(line + "\n")
				}
			} else if inContent {
				content.WriteString(line + "\n")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// First unmarshal to PageFrontmatter (uses string dates)
	pf := &PageFrontmatter{}
	if err := yaml.Unmarshal([]byte(frontmatter.String()), pf); err != nil {
		return nil, err
	}

	// Convert to models.Page (parses dates flexibly)
	page := pf.ToPage()
	page.Excerpt = strings.TrimSpace(excerpt.String())
	page.Content = strings.TrimSpace(content.String())

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
