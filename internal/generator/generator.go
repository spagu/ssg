// Package generator handles static site generation
package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/parser"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Config holds generator configuration
type Config struct {
	Source       string
	Template     string
	Domain       string
	ContentDir   string
	TemplatesDir string
	OutputDir    string
	// New options
	SitemapOff bool // Disable sitemap generation
	RobotsOff  bool // Disable robots.txt generation
	MinifyHTML bool // Minify HTML output
	MinifyCSS  bool // Minify CSS output
	MinifyJS   bool // Minify JS output
	SourceMap  bool // Include source maps
	Clean      bool // Clean output directory before build
	Quiet      bool // Suppress stdout output
}

// Generator handles the static site generation process
type Generator struct {
	config   Config
	siteData *models.SiteData
	tmpl     *template.Template
}

// New creates a new Generator instance
func New(cfg Config) (*Generator, error) {
	return &Generator{
		config: cfg,
		siteData: &models.SiteData{
			Domain:     cfg.Domain,
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
	}, nil
}

// Generate performs the full site generation
func (g *Generator) Generate() error {
	// Clean output directory if requested
	if g.config.Clean {
		if !g.config.Quiet {
			fmt.Println("🧹 Cleaning output directory...")
		}
		if err := os.RemoveAll(g.config.OutputDir); err != nil {
			return fmt.Errorf("cleaning output: %w", err)
		}
	}

	if !g.config.Quiet {
		fmt.Println("🔄 Loading content...")
	}
	if err := g.loadContent(); err != nil {
		return fmt.Errorf("loading content: %w", err)
	}

	if !g.config.Quiet {
		fmt.Println("📝 Loading templates...")
	}
	if err := g.loadTemplates(); err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	if !g.config.Quiet {
		fmt.Println("🏗️  Generating site...")
	}
	if err := g.generateSite(); err != nil {
		return fmt.Errorf("generating site: %w", err)
	}

	if !g.config.Quiet {
		fmt.Println("📁 Copying assets...")
	}
	if err := g.copyAssets(); err != nil {
		return fmt.Errorf("copying assets: %w", err)
	}

	// Generate sitemap and robots (unless disabled)
	if !g.config.SitemapOff || !g.config.RobotsOff {
		if !g.config.Quiet {
			fmt.Println("🗺️  Generating sitemap and robots.txt...")
		}
		if !g.config.SitemapOff {
			if err := g.generateSitemap(); err != nil {
				return fmt.Errorf("generating sitemap: %w", err)
			}
		}
		if !g.config.RobotsOff {
			if err := g.generateRobots(); err != nil {
				return fmt.Errorf("generating robots.txt: %w", err)
			}
		}
	}

	if !g.config.Quiet {
		fmt.Println("☁️  Generating Cloudflare Pages files...")
	}
	if err := g.generateCloudflareFiles(); err != nil {
		return fmt.Errorf("generating Cloudflare files: %w", err)
	}

	// Minify output if requested
	if g.config.MinifyHTML || g.config.MinifyCSS || g.config.MinifyJS {
		if !g.config.Quiet {
			fmt.Println("🗜️  Minifying output...")
		}
		if err := g.minifyOutput(); err != nil {
			return fmt.Errorf("minifying output: %w", err)
		}
	}

	return nil
}

// loadContent loads all content from the source directory
func (g *Generator) loadContent() error {
	sourcePath := filepath.Join(g.config.ContentDir, g.config.Source)

	// Load metadata.json
	metadataPath := filepath.Join(sourcePath, "metadata.json")
	if err := g.loadMetadata(metadataPath); err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}

	// Load pages
	pagesPath := filepath.Join(sourcePath, "pages")
	pages, err := g.loadMarkdownDir(pagesPath)
	if err != nil {
		return fmt.Errorf("loading pages: %w", err)
	}
	g.siteData.Pages = pages

	// Load posts
	postsPath := filepath.Join(sourcePath, "posts")
	posts, err := g.loadPostsDir(postsPath)
	if err != nil {
		return fmt.Errorf("loading posts: %w", err)
	}
	g.siteData.Posts = posts

	// Sort posts by date (newest first)
	sort.Slice(g.siteData.Posts, func(i, j int) bool {
		return g.siteData.Posts[i].Date.After(g.siteData.Posts[j].Date)
	})

	fmt.Printf("   📄 Loaded %d pages\n", len(g.siteData.Pages))
	fmt.Printf("   📝 Loaded %d posts\n", len(g.siteData.Posts))
	fmt.Printf("   📁 Loaded %d categories\n", len(g.siteData.Categories))
	fmt.Printf("   🖼️  Loaded %d media items\n", len(g.siteData.Media))

	return nil
}

// loadMetadata loads the metadata.json file
func (g *Generator) loadMetadata(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	var metadata models.Metadata
	if err := json.NewDecoder(file).Decode(&metadata); err != nil {
		return err
	}

	for _, cat := range metadata.Categories {
		g.siteData.Categories[cat.ID] = cat
	}

	for _, media := range metadata.Media {
		g.siteData.Media[media.ID] = media
	}

	for _, author := range metadata.Users {
		g.siteData.Authors[author.ID] = author
	}

	return nil
}

// loadMarkdownDir loads all markdown files from a directory
func (g *Generator) loadMarkdownDir(dir string) ([]models.Page, error) {
	var pages []models.Page

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return pages, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		page, err := parser.ParseMarkdownFile(filePath)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: failed to parse %s: %v\n", entry.Name(), err)
			continue
		}
		if page.Status == "publish" {
			pages = append(pages, *page)
		}
	}

	return pages, nil
}

// loadPostsDir loads posts from category subdirectories
func (g *Generator) loadPostsDir(dir string) ([]models.Page, error) {
	var posts []models.Page

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return posts, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		categoryDir := filepath.Join(dir, entry.Name())
		categoryPosts, err := g.loadMarkdownDir(categoryDir)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: failed to load category %s: %v\n", entry.Name(), err)
			continue
		}
		posts = append(posts, categoryPosts...)
	}

	return posts, nil
}

// loadTemplates loads HTML templates
func (g *Generator) loadTemplates() error {
	templatePath := filepath.Join(g.config.TemplatesDir, g.config.Template)

	// Create default templates if template directory is empty
	if err := g.ensureTemplates(templatePath); err != nil {
		return err
	}

	// Build title -> URL map for autolinking
	pageLinks := make(map[string]string)
	for _, p := range g.siteData.Pages {
		pageLinks[strings.TrimSpace(p.Title)] = p.GetURL()
		pageLinks[stdhtml.UnescapeString(strings.TrimSpace(p.Title))] = p.GetURL()
	}
	for _, p := range g.siteData.Posts {
		pageLinks[strings.TrimSpace(p.Title)] = p.GetURL()
		pageLinks[stdhtml.UnescapeString(strings.TrimSpace(p.Title))] = p.GetURL()
	}

	funcs := template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			// Cleanup markdown artifacts (e.g. orphan **)
			starRegex := regexp.MustCompile(`(?m)^\s*\*\*\s*$`)
			s = starRegex.ReplaceAllString(s, "")

			// Fix bolding inside HTML tags (WP artifact)
			// e.g., <p>**text**</p> is not parsed by goldmark because it's inside HTML block
			// Use \*\*(.*?)\*\* to match within valid lines, avoiding multi-line spans across HTML tags
			boldRegex := regexp.MustCompile(`\*\*(.*?)\*\*`)
			s = boldRegex.ReplaceAllString(s, "<strong>$1</strong>")

			// Autolink list items matching page titles
			lines := strings.Split(s, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				// Match list item: "- Item Name" or "* Item Name"
				var content string
				if strings.HasPrefix(trimmed, "- ") {
					content = strings.TrimSpace(trimmed[2:])
				} else if strings.HasPrefix(trimmed, "* ") {
					content = strings.TrimSpace(trimmed[2:])
				}

				if content != "" {
					// Check strict match
					if url, ok := pageLinks[content]; ok {
						lines[i] = strings.Replace(line, content, fmt.Sprintf("[%s](%s)", content, url), 1)
					} else {
						// Check unescaped match
						unescaped := stdhtml.UnescapeString(content)
						if url, ok := pageLinks[unescaped]; ok {
							lines[i] = strings.Replace(line, content, fmt.Sprintf("[%s](%s)", content, url), 1)
						}
					}
				}
			}
			s = strings.Join(lines, "\n")

			// Convert Markdown to HTML
			var buf bytes.Buffer
			md := goldmark.New(
				goldmark.WithExtensions(
					extension.Table,
				),
				goldmark.WithRendererOptions(
					html.WithUnsafe(),
				),
			)
			if err := md.Convert([]byte(s), &buf); err != nil {
				fmt.Printf("   ⚠️  Warning: markdown conversion failed: %v\n", err)
			} else {
				s = buf.String()
			}

			// Fix relative media paths to absolute and rewrite WordPress URLs
			s = fixMediaPaths(s, g.siteData.Media)
			return template.HTML(s)
		},
		"decodeHTML": func(s string) string {
			// Decode HTML entities like &#8211; -> –
			return stdhtml.UnescapeString(s)
		},
		"formatDate": func(t interface{}) string {
			switch v := t.(type) {
			case string:
				return v
			default:
				return fmt.Sprintf("%v", v)
			}
		},
		"formatDatePL": func(t time.Time) string {
			// Polish month names
			months := []string{
				"", "stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca",
				"lipca", "sierpnia", "września", "października", "listopada", "grudnia",
			}
			return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
		},
		"getCategoryName": func(id int) string {
			if cat, ok := g.siteData.Categories[id]; ok {
				return cat.Name
			}
			return ""
		},
		"getCategorySlug": func(id int) string {
			if cat, ok := g.siteData.Categories[id]; ok {
				return cat.Slug
			}
			return ""
		},
		"isValidCategory": func(id int) bool {
			// Returns false for "Bez kategorii" (ID 1)
			return id != 1
		},
		"getAuthorName": func(id int) string {
			if author, ok := g.siteData.Authors[id]; ok {
				return author.Name
			}
			return ""
		},
		"getURL": func(p models.Page) string {
			return p.GetURL()
		},
		"getCanonical": func(p models.Page, domain string) string {
			return p.GetCanonical(domain)
		},
		"hasValidCategories": func(p models.Page) bool {
			return p.HasValidCategories()
		},
		"thumbnailFromYoutube": func(s string) string {
			// Extract YouTube video ID and return thumbnail URL
			youtubeRegex := regexp.MustCompile(`\[youtube\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/youtube\]`)
			matches := youtubeRegex.FindStringSubmatch(s)
			if len(matches) >= 2 {
				return fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", matches[1])
			}
			return ""
		},
		"stripShortcodes": func(s string) string {
			// Remove [youtube]...[/youtube] and [embed]...[/embed] from excerpt
			youtubeRegex := regexp.MustCompile(`\[youtube\][^\[]*\[/youtube\]`)
			s = youtubeRegex.ReplaceAllString(s, "")
			embedRegex := regexp.MustCompile(`\[embed\][^\[]*\[/embed\]`)
			s = embedRegex.ReplaceAllString(s, "")
			return strings.TrimSpace(s)
		},
		"stripHTML": func(s string) string {
			// Remove HTML tags
			regex := regexp.MustCompile(`<[^>]*>`)
			return strings.TrimSpace(regex.ReplaceAllString(s, ""))
		},
		"recentPosts": func(n int) []models.Page {
			if n > len(g.siteData.Posts) {
				n = len(g.siteData.Posts)
			}
			return g.siteData.Posts[:n]
		},
	}

	tmpl, err := template.New("").Funcs(funcs).ParseGlob(filepath.Join(templatePath, "*.html"))
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}

	g.tmpl = tmpl
	return nil
}

// ensureTemplates creates default templates if they don't exist
func (g *Generator) ensureTemplates(templatePath string) error {
	if err := os.MkdirAll(templatePath, 0755); err != nil {
		return err
	}

	// Check if HTML templates exist
	entries, _ := os.ReadDir(templatePath)
	hasHTMLTemplates := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
			hasHTMLTemplates = true
			break
		}
	}
	if hasHTMLTemplates {
		return nil
	}

	// Create default templates
	templates := map[string]string{
		"base.html":     baseTemplate,
		"index.html":    indexTemplate,
		"page.html":     pageTemplate,
		"post.html":     postTemplate,
		"category.html": categoryTemplate,
	}

	for name, content := range templates {
		path := filepath.Join(templatePath, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("creating template %s: %w", name, err)
		}
	}

	fmt.Printf("   📝 Created default templates in %s\n", templatePath)
	return nil
}

// generateSite generates all HTML files
func (g *Generator) generateSite() error {
	outputPath := g.config.OutputDir
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return err
	}

	// Generate index page
	if err := g.generateIndex(); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}

	// Generate pages
	for _, page := range g.siteData.Pages {
		if err := g.generatePage(page); err != nil {
			fmt.Printf("   ⚠️  Warning: failed to generate page %s: %v\n", page.Slug, err)
		}
	}

	// Generate posts
	for _, post := range g.siteData.Posts {
		if err := g.generatePost(post); err != nil {
			fmt.Printf("   ⚠️  Warning: failed to generate post %s: %v\n", post.Slug, err)
		}
	}

	// Generate category pages
	if err := g.generateCategories(); err != nil {
		return fmt.Errorf("generating categories: %w", err)
	}

	return nil
}

// generateIndex generates the main index.html
func (g *Generator) generateIndex() error {
	data := struct {
		Site   *models.SiteData
		Posts  []models.Page
		Pages  []models.Page
		Domain string
	}{
		Site:   g.siteData,
		Posts:  g.siteData.Posts,
		Pages:  g.siteData.Pages,
		Domain: g.config.Domain,
	}

	return g.renderTemplate("index.html", filepath.Join(g.config.OutputDir, "index.html"), data)
}

// generatePage generates a single page
func (g *Generator) generatePage(page models.Page) error {
	data := struct {
		Site   *models.SiteData
		Page   models.Page
		Domain string
	}{
		Site:   g.siteData,
		Page:   page,
		Domain: g.config.Domain,
	}

	outputPath := filepath.Join(g.config.OutputDir, page.GetOutputPath(), "index.html")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	return g.renderTemplate("page.html", outputPath, data)
}

// generatePost generates a single post
func (g *Generator) generatePost(post models.Page) error {
	data := struct {
		Site   *models.SiteData
		Post   models.Page
		Domain string
	}{
		Site:   g.siteData,
		Post:   post,
		Domain: g.config.Domain,
	}

	// Create date-based URL structure: /YYYY/MM/DD/slug/
	outputPath := filepath.Join(g.config.OutputDir, post.GetOutputPath(), "index.html")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	return g.renderTemplate("post.html", outputPath, data)
}

// generateCategories generates category listing pages
func (g *Generator) generateCategories() error {
	categoryPosts := make(map[int][]models.Page)

	for _, post := range g.siteData.Posts {
		for _, catID := range post.Categories {
			categoryPosts[catID] = append(categoryPosts[catID], post)
		}
	}

	for catID, posts := range categoryPosts {
		cat, ok := g.siteData.Categories[catID]
		if !ok {
			continue
		}

		data := struct {
			Site     *models.SiteData
			Category models.Category
			Posts    []models.Page
			Domain   string
		}{
			Site:     g.siteData,
			Category: cat,
			Posts:    posts,
			Domain:   g.config.Domain,
		}

		outputPath := filepath.Join(g.config.OutputDir, "category", cat.Slug, "index.html")
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return err
		}

		if err := g.renderTemplate("category.html", outputPath, data); err != nil {
			fmt.Printf("   ⚠️  Warning: failed to generate category %s: %v\n", cat.Slug, err)
		}
	}

	return nil
}

// renderTemplate renders a template to a file
func (g *Generator) renderTemplate(templateName, outputPath string, data interface{}) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return g.tmpl.ExecuteTemplate(file, templateName, data)
}

// copyAssets copies static assets (CSS, JS, images) to output
func (g *Generator) copyAssets() error {
	templatePath := filepath.Join(g.config.TemplatesDir, g.config.Template)

	// Copy CSS
	if err := g.copyDir(filepath.Join(templatePath, "css"), filepath.Join(g.config.OutputDir, "css")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	// Copy JS
	if err := g.copyDir(filepath.Join(templatePath, "js"), filepath.Join(g.config.OutputDir, "js")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	// Copy Images
	if err := g.copyDir(filepath.Join(templatePath, "images"), filepath.Join(g.config.OutputDir, "images")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	// Copy media from content source
	sourcePath := filepath.Join(g.config.ContentDir, g.config.Source)
	mediaSourcePath := filepath.Join(sourcePath, "media")
	mediaOutputPath := filepath.Join(g.config.OutputDir, "media")

	if err := g.copyDir(mediaSourcePath, mediaOutputPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("   ⚠️  Warning: couldn't copy media: %v\n", err)
		}
	} else {
		fmt.Printf("   🖼️  Copied media files\n")
	}

	return nil
}

// copyDir copies a directory recursively
func (g *Generator) copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := g.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := g.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func (g *Generator) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// fixMediaPaths converts relative media paths to absolute paths
// and fixes WordPress thumbnail URLs to point to local files
func fixMediaPaths(content string, media map[int]models.MediaItem) string {
	// First, fix WordPress absolute URLs using wp-image-ID class
	// Pattern: wp-image-1048 ... src="http://...krowy.net/..." -> src="/media/1048_filename.jpg"
	wpImageRegex := regexp.MustCompile(`wp-image-(\d+)`)
	matches := wpImageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		idStr := match[1]
		var id int
		_, _ = fmt.Sscanf(idStr, "%d", &id)
		if mediaItem, ok := media[id]; ok {
			// Get the filename from the media item
			filename := filepath.Base(mediaItem.MediaDetails.File)
			localPath := fmt.Sprintf("/media/%d_%s", id, filename)

			// Replace old WordPress URLs for this image
			// Match patterns like src="http://...krowy.net/.../filename..."
			oldURLRegex := regexp.MustCompile(`(src=["'])https?://[^"']*` + regexp.QuoteMeta(filename[:len(filename)-4]) + `[^"']*\.(jpg|jpeg|png|gif|webp)(["'])`)
			content = oldURLRegex.ReplaceAllString(content, `${1}`+localPath+`${3}`)
		}
	}

	// Fix src="media/..." to src="/media/..."
	srcRegex := regexp.MustCompile(`(src=["'])media/`)
	content = srcRegex.ReplaceAllString(content, `${1}/media/`)

	// Fix href="media/..." to href="/media/..."
	hrefRegex := regexp.MustCompile(`(href=["'])media/`)
	content = hrefRegex.ReplaceAllString(content, `${1}/media/`)

	// Fix srcset="media/..." to srcset="/media/..."
	srcsetRegex := regexp.MustCompile(`(srcset=["'])media/`)
	content = srcsetRegex.ReplaceAllString(content, `${1}/media/`)

	// Fix URLs in srcset attribute (multiple entries separated by comma)
	srcsetItemRegex := regexp.MustCompile(`, media/`)
	content = srcsetItemRegex.ReplaceAllString(content, `, /media/`)

	// Remove WordPress thumbnail size suffixes from media paths
	// e.g., /media/1048_IMG_0316_p-300x225.jpg -> /media/1048_IMG_0316_p.jpg
	thumbnailRegex := regexp.MustCompile(`(/media/\d+_[^"'\s]+)-\d+x\d+(\.(?:jpg|jpeg|png|gif|webp))`)
	content = thumbnailRegex.ReplaceAllString(content, `${1}${2}`)

	// Also handle srcset entries with size descriptors
	// e.g., /media/1048_file-300x225.jpg 300w -> /media/1048_file.jpg 300w
	srcsetThumbnailRegex := regexp.MustCompile(`(/media/\d+_[^"'\s,]+)-\d+x\d+(\.(?:jpg|jpeg|png|gif|webp))\s+(\d+w)`)
	content = srcsetThumbnailRegex.ReplaceAllString(content, `${1}${2} ${3}`)

	// Process WordPress shortcodes
	content = processShortcodes(content)

	return content
}

// processShortcodes converts WordPress shortcodes to HTML
func processShortcodes(content string) string {
	// YouTube shortcode: [youtube]URL[/youtube] -> iframe embed
	youtubeRegex := regexp.MustCompile(`\[youtube\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/youtube\]`)
	content = youtubeRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract video ID
		submatches := youtubeRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		videoID := submatches[1]
		return fmt.Sprintf(`<div class="video-container"><iframe width="560" height="315" src="https://www.youtube.com/embed/%s" title="YouTube video" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>`, videoID)
	})

	// Also handle embed shortcode: [embed]URL[/embed]
	embedRegex := regexp.MustCompile(`\[embed\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/embed\]`)
	content = embedRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := embedRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		videoID := submatches[1]
		return fmt.Sprintf(`<div class="video-container"><iframe width="560" height="315" src="https://www.youtube.com/embed/%s" title="YouTube video" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>`, videoID)
	})

	return content
}

// generateSitemap creates sitemap.xml
func (g *Generator) generateSitemap() error {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sb.WriteString("\n")

	// Homepage
	sb.WriteString("  <url>\n")
	sb.WriteString(fmt.Sprintf("    <loc>https://%s/</loc>\n", g.config.Domain))
	sb.WriteString("    <changefreq>daily</changefreq>\n")
	sb.WriteString("    <priority>1.0</priority>\n")
	sb.WriteString("  </url>\n")

	// Pages
	for _, page := range g.siteData.Pages {
		sb.WriteString("  <url>\n")
		sb.WriteString(fmt.Sprintf("    <loc>%s</loc>\n", page.GetCanonical(g.config.Domain)))
		sb.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", page.Modified.Format("2006-01-02")))
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.8</priority>\n")
		sb.WriteString("  </url>\n")
	}

	// Posts
	for _, post := range g.siteData.Posts {
		sb.WriteString("  <url>\n")
		sb.WriteString(fmt.Sprintf("    <loc>%s</loc>\n", post.GetCanonical(g.config.Domain)))
		sb.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", post.Modified.Format("2006-01-02")))
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.6</priority>\n")
		sb.WriteString("  </url>\n")
	}

	// Categories
	for _, cat := range g.siteData.Categories {
		if cat.ID != 1 { // Skip "Bez kategorii"
			sb.WriteString("  <url>\n")
			sb.WriteString(fmt.Sprintf("    <loc>https://%s/category/%s/</loc>\n", g.config.Domain, cat.Slug))
			sb.WriteString("    <changefreq>weekly</changefreq>\n")
			sb.WriteString("    <priority>0.5</priority>\n")
			sb.WriteString("  </url>\n")
		}
	}

	sb.WriteString("</urlset>\n")

	return os.WriteFile(filepath.Join(g.config.OutputDir, "sitemap.xml"), []byte(sb.String()), 0644)
}

// generateRobots creates robots.txt
func (g *Generator) generateRobots() error {
	content := fmt.Sprintf(`User-agent: *
Allow: /

Sitemap: https://%s/sitemap.xml
`, g.config.Domain)

	return os.WriteFile(filepath.Join(g.config.OutputDir, "robots.txt"), []byte(content), 0644)
}

// generateCloudflareFiles creates _headers and _redirects files for Cloudflare Pages
func (g *Generator) generateCloudflareFiles() error {
	// Create _headers file with caching and security headers
	headers := `# Cloudflare Pages Headers
# Generated by SSG

# Security headers for all pages
/*
  X-Content-Type-Options: nosniff
  X-Frame-Options: DENY
  X-XSS-Protection: 1; mode=block
  Referrer-Policy: strict-origin-when-cross-origin
  Permissions-Policy: geolocation=(), microphone=(), camera=()

# Cache static assets for 1 year
/css/*
  Cache-Control: public, max-age=31536000, immutable

/js/*
  Cache-Control: public, max-age=31536000, immutable

/images/*
  Cache-Control: public, max-age=31536000, immutable

/media/*
  Cache-Control: public, max-age=31536000, immutable

# Cache HTML pages for 1 hour
/*.html
  Cache-Control: public, max-age=3600

/
  Cache-Control: public, max-age=3600
`
	headersPath := filepath.Join(g.config.OutputDir, "_headers")
	if err := os.WriteFile(headersPath, []byte(headers), 0644); err != nil {
		return fmt.Errorf("writing _headers: %w", err)
	}

	// Create _redirects file (empty for now, can be extended)
	redirects := `# Cloudflare Pages Redirects
# Generated by SSG
# Format: /source /destination [status]

# Trailing slash normalization handled by Cloudflare automatically
`
	redirectsPath := filepath.Join(g.config.OutputDir, "_redirects")
	if err := os.WriteFile(redirectsPath, []byte(redirects), 0644); err != nil {
		return fmt.Errorf("writing _redirects: %w", err)
	}

	return nil
}

// minifyOutput minifies HTML, CSS, and JS files in the output directory
func (g *Generator) minifyOutput() error {
	return filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))

		switch ext {
		case ".html":
			if g.config.MinifyHTML {
				if err := minifyHTMLFile(path); err != nil {
					return fmt.Errorf("minifying %s: %w", path, err)
				}
			}
		case ".css":
			if g.config.MinifyCSS {
				if err := minifyCSSFile(path); err != nil {
					return fmt.Errorf("minifying %s: %w", path, err)
				}
			}
		case ".js":
			if g.config.MinifyJS {
				if err := minifyJSFile(path); err != nil {
					return fmt.Errorf("minifying %s: %w", path, err)
				}
			}
		}

		return nil
	})
}

// minifyHTMLFile removes unnecessary whitespace from HTML
func minifyHTMLFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	s := string(content)
	// Remove HTML comments (except conditionals)
	reComment := regexp.MustCompile(`<!--[\s\S]*?-->`)
	s = reComment.ReplaceAllStringFunc(s, func(match string) string {
		if strings.HasPrefix(match, "<!--[if") {
			return match
		}
		return ""
	})
	// Remove whitespace between tags
	reSpace := regexp.MustCompile(`>\s+<`)
	s = reSpace.ReplaceAllString(s, "><")
	// Collapse multiple whitespaces
	reMultiSpace := regexp.MustCompile(`\s{2,}`)
	s = reMultiSpace.ReplaceAllString(s, " ")
	// Trim lines
	s = strings.TrimSpace(s)

	return os.WriteFile(path, []byte(s), 0644)
}

// minifyCSSFile removes unnecessary whitespace and comments from CSS
func minifyCSSFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	s := string(content)
	// Remove CSS comments
	reComment := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	s = reComment.ReplaceAllString(s, "")
	// Remove newlines
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	// Remove spaces around : ; { } ,
	reSpaces := regexp.MustCompile(`\s*([:{};,])\s*`)
	s = reSpaces.ReplaceAllString(s, "$1")
	// Collapse multiple spaces
	reMultiSpace := regexp.MustCompile(`\s{2,}`)
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	return os.WriteFile(path, []byte(s), 0644)
}

// minifyJSFile removes unnecessary whitespace and comments from JS
func minifyJSFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	s := string(content)
	// Remove single-line comments (but not in strings - simplified)
	reSingleComment := regexp.MustCompile(`(?m)^\s*//.*$`)
	s = reSingleComment.ReplaceAllString(s, "")
	// Remove multi-line comments
	reMultiComment := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	s = reMultiComment.ReplaceAllString(s, "")
	// Remove empty lines
	reEmptyLines := regexp.MustCompile(`\n\s*\n`)
	s = reEmptyLines.ReplaceAllString(s, "\n")
	// Trim
	s = strings.TrimSpace(s)

	return os.WriteFile(path, []byte(s), 0644)
}
