package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// Extra content sources (CONTENT-002). A normal build reads exactly one source
// tree — content/<source>/ with its pages/, posts/ and metadata.json. That is
// the wrong shape for content that already lives elsewhere in the repository:
// a docs/ folder, a monorepo package's README set, notes kept beside the code.
//
// content_sources: lists additional flat Markdown roots, each merged into the
// site as pages or posts and optionally filed under one category. Sources join
// the site BEFORE finalize, so they get the same URL, permalink, i18n, taxonomy
// and collision treatment as native content.
//
// Backward compatibility: an empty list changes nothing. The primary source
// stays required unless at least one extra source is configured, in which case
// a site may consist of extra sources alone.

// ContentSource is one extra Markdown root merged into the site.
type ContentSource struct {
	// Path is the directory to read, relative to the working directory (or
	// absolute). Markdown is loaded recursively.
	Path string
	// Type is "page" (default) or "post".
	Type string
	// Category optionally files every entry of this source under one category,
	// which is created when the loaded metadata does not already define it.
	// Frontmatter categories on an individual file win.
	Category string
}

// contentSourceTypes are the accepted Type values.
var contentSourceTypes = map[string]bool{"": true, "page": true, "post": true}

// loadExtraContentSources merges every configured extra source into the site.
// Called after the primary source (or MDDB) has loaded, so metadata-defined
// categories and authors are already available for resolution.
func (g *Generator) loadExtraContentSources() error {
	if len(g.config.ContentSources) == 0 {
		return nil
	}
	for _, src := range g.config.ContentSources {
		if err := g.loadOneContentSource(src); err != nil {
			return err
		}
	}
	// Extra posts arrive after the primary source's own sort.
	sort.Slice(g.siteData.Posts, func(i, j int) bool {
		return g.siteData.Posts[i].Date.After(g.siteData.Posts[j].Date)
	})
	// Re-resolve so category/author names from the new entries map to IDs. The
	// resolvers skip anything already resolved, so this is idempotent.
	g.siteData.ResolveFlexibleFields()
	return nil
}

// loadOneContentSource reads one extra root and appends its content.
func (g *Generator) loadOneContentSource(src ContentSource) error {
	if strings.TrimSpace(src.Path) == "" {
		return fmt.Errorf("content_sources: path is required")
	}
	if !contentSourceTypes[src.Type] {
		return fmt.Errorf("content_sources: %q has unsupported type %q (supported: page, post)", src.Path, src.Type)
	}
	info, err := os.Stat(src.Path)
	if err != nil {
		return fmt.Errorf("content_sources: %q: %w", src.Path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("content_sources: %q is not a directory", src.Path)
	}

	entries, err := g.loadMarkdownDir(src.Path)
	if err != nil {
		return fmt.Errorf("content_sources: loading %q: %w", src.Path, err)
	}
	if len(entries) == 0 {
		g.log(fmt.Sprintf("   ⚠️  Warning: content source %s contains no Markdown files", src.Path))
		return nil
	}

	categoryID := g.ensureCategory(src.Category)
	for i := range entries {
		entries[i].PageFormat = g.config.PageFormat
		if src.Type == "post" {
			entries[i].URLFormat = g.config.PostURLFormat
		}
		applySourceCategory(&entries[i], src.Category, categoryID)
	}

	if src.Type == "post" {
		g.siteData.Posts = append(g.siteData.Posts, entries...)
	} else {
		g.siteData.Pages = append(g.siteData.Pages, entries...)
	}
	g.log(fmt.Sprintf("   📂 %s → %d %ss", filepath.Clean(src.Path), len(entries), sourceType(src.Type)))
	return nil
}

// sourceType returns the display/type name, defaulting to "page".
func sourceType(t string) string {
	if t == "post" {
		return "post"
	}
	return "page"
}

// applySourceCategory files an entry under the source's category unless its own
// frontmatter already names one — a per-file category is more specific than a
// per-source default and must win.
func applySourceCategory(p *models.Page, name string, id int) {
	if name == "" || id == 0 {
		return
	}
	if len(p.Categories) > 0 || len(p.CategoriesRaw) > 0 {
		return
	}
	p.Categories = append(p.Categories, id)
}

// ensureCategory returns the ID of the named category, registering it when the
// loaded metadata does not define it — an extra source is usually a plain
// folder with no metadata.json to declare its category in. Returns 0 for an
// empty name.
func (g *Generator) ensureCategory(name string) int {
	if strings.TrimSpace(name) == "" {
		return 0
	}
	lower := strings.ToLower(name)
	maxID := 0
	for id, cat := range g.siteData.Categories {
		if strings.ToLower(cat.Name) == lower || strings.ToLower(cat.Slug) == lower {
			return id
		}
		if id > maxID {
			maxID = id
		}
	}
	if g.siteData.Categories == nil {
		g.siteData.Categories = make(map[int]models.Category)
	}
	id := maxID + 1
	g.siteData.Categories[id] = models.Category{ID: id, Name: name, Slug: slugify(name)}
	return id
}
