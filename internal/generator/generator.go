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

// Shortcode defines a reusable content snippet
type Shortcode struct {
	Name     string            // Shortcode name (e.g., "thunderpick")
	Type     string            // Type: "banner", "custom", etc.
	Template string            // Template file for custom rendering
	Title    string            // Title/heading
	Text     string            // Text content
	URL      string            // Link URL
	Logo     string            // Logo/image path
	Legal    string            // Legal text
	Data     map[string]string // Additional custom data
}

// Config holds generator configuration
type Config struct {
	Source       string
	Template     string
	Domain       string
	ContentDir   string
	TemplatesDir string
	OutputDir    string
	// New options
	SitemapOff    bool        // Disable sitemap generation
	RobotsOff     bool        // Disable robots.txt generation
	PrettyHTML    bool        // Prettify HTML output (remove extra blank lines)
	PostURLFormat string      // Post URL format: "date" (/YYYY/MM/DD/slug/) or "slug" (/slug/)
	RelativeLinks bool        // Convert absolute URLs to relative links
	Shortcodes    []Shortcode // Shortcodes definitions
	MinifyHTML    bool        // Minify HTML output
	MinifyCSS     bool        // Minify CSS output
	MinifyJS      bool        // Minify JS output
	SourceMap     bool        // Include source maps
	Clean         bool        // Clean output directory before build
	Quiet         bool        // Suppress stdout output
	Engine        string      // Template engine: go, pongo2, mustache, handlebars
}

// Generator handles the static site generation process
type Generator struct {
	config       Config
	siteData     *models.SiteData
	tmpl         *template.Template
	shortcodeMap map[string]Shortcode // Map of shortcode name to shortcode
}

// New creates a new Generator instance
func New(cfg Config) (*Generator, error) {
	// Build shortcode map for quick lookup
	scMap := make(map[string]Shortcode)
	for _, sc := range cfg.Shortcodes {
		scMap[sc.Name] = sc
	}

	return &Generator{
		config: cfg,
		siteData: &models.SiteData{
			Domain:     cfg.Domain,
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
		},
		shortcodeMap: scMap,
	}, nil
}

// Generate performs the full site generation
func (g *Generator) Generate() error {
	if err := g.cleanOutputIfRequested(); err != nil {
		return err
	}

	if err := g.runStep("🔄 Loading content...", g.loadContent, "loading content"); err != nil {
		return err
	}

	if err := g.runStep("📝 Loading templates...", g.loadTemplates, "loading templates"); err != nil {
		return err
	}

	if err := g.runStep("🏗️  Generating site...", g.generateSite, "generating site"); err != nil {
		return err
	}

	if err := g.runStep("📁 Copying assets...", g.copyAssets, "copying assets"); err != nil {
		return err
	}

	if err := g.generateSitemapAndRobots(); err != nil {
		return err
	}

	if err := g.runStep("☁️  Generating Cloudflare Pages files...", g.generateCloudflareFiles, "generating Cloudflare files"); err != nil {
		return err
	}

	if err := g.convertRelativeLinksIfRequested(); err != nil {
		return err
	}

	if err := g.prettifyIfRequested(); err != nil {
		return err
	}

	if err := g.minifyIfRequested(); err != nil {
		return err
	}

	return nil
}

// log prints a message if not in quiet mode
func (g *Generator) log(msg string) {
	if !g.config.Quiet {
		fmt.Println(msg)
	}
}

// runStep executes a generation step with logging
func (g *Generator) runStep(msg string, fn func() error, errContext string) error {
	g.log(msg)
	if err := fn(); err != nil {
		return fmt.Errorf("%s: %w", errContext, err)
	}
	return nil
}

// cleanOutputIfRequested cleans the output directory if configured
func (g *Generator) cleanOutputIfRequested() error {
	if !g.config.Clean {
		return nil
	}
	g.log("🧹 Cleaning output directory...")
	if err := os.RemoveAll(g.config.OutputDir); err != nil {
		return fmt.Errorf("cleaning output: %w", err)
	}
	return nil
}

// generateSitemapAndRobots generates sitemap.xml and robots.txt if enabled
func (g *Generator) generateSitemapAndRobots() error {
	if g.config.SitemapOff && g.config.RobotsOff {
		return nil
	}

	g.log("🗺️  Generating sitemap and robots.txt...")

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
	return nil
}

// convertRelativeLinksIfRequested converts absolute URLs to relative if configured
func (g *Generator) convertRelativeLinksIfRequested() error {
	if !g.config.RelativeLinks || g.config.Domain == "" {
		return nil
	}
	g.log("🔗 Converting to relative links...")
	if err := g.convertToRelativeLinks(); err != nil {
		return fmt.Errorf("converting to relative links: %w", err)
	}
	return nil
}

// prettifyIfRequested prettifies HTML if configured
func (g *Generator) prettifyIfRequested() error {
	if !g.config.PrettyHTML || g.config.MinifyHTML {
		return nil
	}
	g.log("✨ Prettifying HTML output...")
	if err := g.prettifyOutput(); err != nil {
		return fmt.Errorf("prettifying output: %w", err)
	}
	return nil
}

// minifyIfRequested minifies output if configured
func (g *Generator) minifyIfRequested() error {
	if !g.config.MinifyHTML && !g.config.MinifyCSS && !g.config.MinifyJS {
		return nil
	}
	g.log("🗜️  Minifying output...")
	if err := g.minifyOutput(); err != nil {
		return fmt.Errorf("minifying output: %w", err)
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

	// Set URL format for posts based on config
	for i := range posts {
		posts[i].URLFormat = g.config.PostURLFormat
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
	file, err := os.Open(path) // #nosec G304 -- CLI tool reads user's content files
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

	if err := g.ensureTemplates(templatePath); err != nil {
		return err
	}

	pageLinks := g.buildPageLinks()
	funcs := g.buildTemplateFuncs(pageLinks)

	tmpl, err := template.New("").Funcs(funcs).ParseGlob(filepath.Join(templatePath, "*.html"))
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}

	g.tmpl = tmpl
	return nil
}

// buildPageLinks creates a map of page titles to URLs for autolinking
func (g *Generator) buildPageLinks() map[string]string {
	pageLinks := make(map[string]string)
	for _, p := range g.siteData.Pages {
		pageLinks[strings.TrimSpace(p.Title)] = p.GetURL()
		pageLinks[stdhtml.UnescapeString(strings.TrimSpace(p.Title))] = p.GetURL()
	}
	for _, p := range g.siteData.Posts {
		pageLinks[strings.TrimSpace(p.Title)] = p.GetURL()
		pageLinks[stdhtml.UnescapeString(strings.TrimSpace(p.Title))] = p.GetURL()
	}
	return pageLinks
}

// buildTemplateFuncs creates the template function map
func (g *Generator) buildTemplateFuncs(pageLinks map[string]string) template.FuncMap {
	return template.FuncMap{
		"safeHTML":            g.tmplSafeHTML(pageLinks),
		"decodeHTML":          tmplDecodeHTML,
		"formatDate":          tmplFormatDate,
		"formatDatePL":        tmplFormatDatePL,
		"getCategoryName":     g.tmplGetCategoryName,
		"getCategorySlug":     g.tmplGetCategorySlug,
		"isValidCategory":     tmplIsValidCategory,
		"getAuthorName":       g.tmplGetAuthorName,
		"getURL":              tmplGetURL,
		"getCanonical":        tmplGetCanonical,
		"hasValidCategories":  tmplHasValidCategories,
		"thumbnailFromYoutube": tmplThumbnailFromYoutube,
		"stripShortcodes":     tmplStripShortcodes,
		"stripHTML":           tmplStripHTML,
		"recentPosts":         g.tmplRecentPosts,
	}
}

// tmplSafeHTML returns the safeHTML template function
func (g *Generator) tmplSafeHTML(pageLinks map[string]string) func(string) template.HTML {
	return func(s string) template.HTML {
		s = g.processShortcodes(s) // Process shortcodes first
		s = cleanMarkdownArtifacts(s)
		s = autolinkListItems(s, pageLinks)
		s = convertMarkdownToHTML(s)
		s = fixMediaPaths(s, g.siteData.Media)
		return template.HTML(s) // #nosec G203 -- SSG intentionally renders markdown as HTML
	}
}

// cleanMarkdownArtifacts removes markdown artifacts and fixes bolding
func cleanMarkdownArtifacts(s string) string {
	starRegex := regexp.MustCompile(`(?m)^\s*\*\*\s*$`)
	s = starRegex.ReplaceAllString(s, "")
	boldRegex := regexp.MustCompile(`\*\*(.*?)\*\*`)
	s = boldRegex.ReplaceAllString(s, "<strong>$1</strong>")
	return s
}

// autolinkListItems converts list items matching page titles to links
func autolinkListItems(s string, pageLinks map[string]string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		content := extractListItemContent(line)
		if content != "" {
			lines[i] = linkifyListItem(line, content, pageLinks)
		}
	}
	return strings.Join(lines, "\n")
}

// extractListItemContent extracts content from a list item line
func extractListItemContent(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") {
		return strings.TrimSpace(trimmed[2:])
	}
	if strings.HasPrefix(trimmed, "* ") {
		return strings.TrimSpace(trimmed[2:])
	}
	return ""
}

// linkifyListItem converts list item content to a link if matching page exists
func linkifyListItem(line, content string, pageLinks map[string]string) string {
	if url, ok := pageLinks[content]; ok {
		return strings.Replace(line, content, fmt.Sprintf("[%s](%s)", content, url), 1)
	}
	unescaped := stdhtml.UnescapeString(content)
	if url, ok := pageLinks[unescaped]; ok {
		return strings.Replace(line, content, fmt.Sprintf("[%s](%s)", content, url), 1)
	}
	return line
}

// convertMarkdownToHTML converts markdown content to HTML
func convertMarkdownToHTML(s string) string {
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	if err := md.Convert([]byte(s), &buf); err != nil {
		fmt.Printf("   ⚠️  Warning: markdown conversion failed: %v\n", err)
		return s
	}
	return buf.String()
}

// processShortcodes replaces {{shortcode_name}} with rendered HTML
func (g *Generator) processShortcodes(content string) string {
	if len(g.shortcodeMap) == 0 {
		return content
	}

	// Match {{shortcode_name}} pattern
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		// Extract shortcode name from {{name}}
		name := match[2 : len(match)-2]

		sc, ok := g.shortcodeMap[name]
		if !ok {
			return match // Leave unmatched shortcodes as-is
		}

		return g.renderShortcode(sc)
	})
}

// renderShortcode renders a single shortcode to HTML
func (g *Generator) renderShortcode(sc Shortcode) string {
	// Try custom template first
	if sc.Template != "" {
		rendered, err := g.renderShortcodeTemplate(sc)
		if err == nil {
			return rendered
		}
		fmt.Printf("   ⚠️  Warning: shortcode template %s failed: %v, using built-in\n", sc.Template, err)
	}

	// Use built-in templates based on type
	switch sc.Type {
	case "banner":
		return g.renderBannerShortcode(sc)
	case "link":
		return g.renderLinkShortcode(sc)
	case "image":
		return g.renderImageShortcode(sc)
	default:
		// Default: simple text or link
		if sc.URL != "" {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`,
				stdhtml.EscapeString(sc.URL), stdhtml.EscapeString(sc.Text))
		}
		return stdhtml.EscapeString(sc.Text)
	}
}

// renderShortcodeTemplate renders shortcode using a custom template file
func (g *Generator) renderShortcodeTemplate(sc Shortcode) (string, error) {
	templatePath := filepath.Join(g.config.TemplatesDir, g.config.Template, sc.Template)

	// Check if template exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return "", fmt.Errorf("template not found: %s", templatePath)
	}

	// Parse and execute template
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sc); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// renderBannerShortcode renders a banner-type shortcode
func (g *Generator) renderBannerShortcode(sc Shortcode) string {
	var logoHTML string
	if sc.Logo != "" {
		logoHTML = fmt.Sprintf(`<img src="%s" alt="%s" class="shortcode-banner-logo">`,
			stdhtml.EscapeString(sc.Logo), stdhtml.EscapeString(sc.Name))
	}

	var titleHTML string
	if sc.Title != "" {
		titleHTML = fmt.Sprintf(`<span class="shortcode-banner-title">%s</span>`,
			stdhtml.EscapeString(sc.Title))
	}

	var legalHTML string
	if sc.Legal != "" {
		legalHTML = fmt.Sprintf(`<span class="shortcode-banner-legal">%s</span>`,
			stdhtml.EscapeString(sc.Legal))
	}

	return fmt.Sprintf(`<div class="shortcode-banner">
<a href="%s" target="_blank" rel="noopener sponsored" class="shortcode-banner-link">
%s
%s
<span class="shortcode-banner-text">%s</span>
</a>
%s
</div>`, stdhtml.EscapeString(sc.URL), logoHTML, titleHTML, stdhtml.EscapeString(sc.Text), legalHTML)
}

// renderLinkShortcode renders a link-type shortcode
func (g *Generator) renderLinkShortcode(sc Shortcode) string {
	text := sc.Text
	if text == "" {
		text = sc.Name
	}
	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`,
		stdhtml.EscapeString(sc.URL), stdhtml.EscapeString(text))
}

// renderImageShortcode renders an image-type shortcode
func (g *Generator) renderImageShortcode(sc Shortcode) string {
	alt := sc.Text
	if alt == "" {
		alt = sc.Name
	}
	if sc.URL != "" {
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener"><img src="%s" alt="%s" class="shortcode-image"></a>`,
			stdhtml.EscapeString(sc.URL), stdhtml.EscapeString(sc.Logo), stdhtml.EscapeString(alt))
	}
	return fmt.Sprintf(`<img src="%s" alt="%s" class="shortcode-image">`,
		stdhtml.EscapeString(sc.Logo), stdhtml.EscapeString(alt))
}

func tmplDecodeHTML(s string) string {
	return stdhtml.UnescapeString(s)
}

func tmplFormatDate(t interface{}) string {
	if v, ok := t.(string); ok {
		return v
	}
	return fmt.Sprintf("%v", t)
}

func tmplFormatDatePL(t time.Time) string {
	months := []string{
		"", "stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca",
		"lipca", "sierpnia", "września", "października", "listopada", "grudnia",
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}

func (g *Generator) tmplGetCategoryName(id int) string {
	if cat, ok := g.siteData.Categories[id]; ok {
		return cat.Name
	}
	return ""
}

func (g *Generator) tmplGetCategorySlug(id int) string {
	if cat, ok := g.siteData.Categories[id]; ok {
		return cat.Slug
	}
	return ""
}

func tmplIsValidCategory(id int) bool {
	return id != 1
}

func (g *Generator) tmplGetAuthorName(id int) string {
	if author, ok := g.siteData.Authors[id]; ok {
		return author.Name
	}
	return ""
}

func tmplGetURL(p models.Page) string {
	return p.GetURL()
}

func tmplGetCanonical(p models.Page, domain string) string {
	return p.GetCanonical(domain)
}

func tmplHasValidCategories(p models.Page) bool {
	return p.HasValidCategories()
}

func tmplThumbnailFromYoutube(s string) string {
	youtubeRegex := regexp.MustCompile(`\[youtube\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/youtube\]`)
	matches := youtubeRegex.FindStringSubmatch(s)
	if len(matches) >= 2 {
		return fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", matches[1])
	}
	return ""
}

func tmplStripShortcodes(s string) string {
	youtubeRegex := regexp.MustCompile(`\[youtube\][^\[]*\[/youtube\]`)
	s = youtubeRegex.ReplaceAllString(s, "")
	embedRegex := regexp.MustCompile(`\[embed\][^\[]*\[/embed\]`)
	s = embedRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func tmplStripHTML(s string) string {
	regex := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(regex.ReplaceAllString(s, ""))
}

func (g *Generator) tmplRecentPosts(n int) []models.Page {
	if n > len(g.siteData.Posts) {
		n = len(g.siteData.Posts)
	}
	return g.siteData.Posts[:n]
}

// ensureTemplates creates default templates if they don't exist
func (g *Generator) ensureTemplates(templatePath string) error {
	// #nosec G301 -- Web content directories need to be world-traversable
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
		// #nosec G306 -- Web content files need to be world-readable
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
	// #nosec G301 -- Web content directories need to be world-traversable
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
	// Skip pages that would overwrite the main index.html
	// This happens when a page has link="https://domain/" pointing to root
	outputSubPath := page.GetOutputPath()
	if outputSubPath == "" || outputSubPath == "." {
		fmt.Printf("   ⚠️  Skipping page '%s' (slug: %s) - would overwrite main index.html\n", page.Title, page.Slug)
		fmt.Printf("      Hint: Change the 'link' field in frontmatter or use a different slug\n")
		return nil
	}

	data := struct {
		Site   *models.SiteData
		Page   models.Page
		Domain string
	}{
		Site:   g.siteData,
		Page:   page,
		Domain: g.config.Domain,
	}

	outputPath := filepath.Join(g.config.OutputDir, outputSubPath, "index.html")
	// #nosec G301 -- Web content directories need to be world-traversable
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
	// #nosec G301 -- Web content directories need to be world-traversable
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
		// #nosec G301 -- Web content directories need to be world-traversable
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
	file, err := os.Create(outputPath) // #nosec G304 -- CLI tool creates user's output files
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
	// #nosec G301 -- Web content directories need to be world-traversable
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
	srcFile, err := os.Open(src) // #nosec G304 -- CLI tool reads user's content files
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst) // #nosec G304 -- CLI tool creates user's output files
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

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(filepath.Join(g.config.OutputDir, "sitemap.xml"), []byte(sb.String()), 0644)
}

// generateRobots creates robots.txt
func (g *Generator) generateRobots() error {
	content := fmt.Sprintf(`User-agent: *
Allow: /

Sitemap: https://%s/sitemap.xml
`, g.config.Domain)

	// #nosec G306 -- Web content files need to be world-readable
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
	// #nosec G306 -- Web content files need to be world-readable
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
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(redirectsPath, []byte(redirects), 0644); err != nil {
		return fmt.Errorf("writing _redirects: %w", err)
	}

	return nil
}

// prettifyOutput prettifies HTML files in the output directory
func (g *Generator) prettifyOutput() error {
	return filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" {
			if err := prettifyHTMLFile(path); err != nil {
				return fmt.Errorf("prettifying %s: %w", path, err)
			}
		}

		return nil
	})
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
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
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

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(path, []byte(s), 0644)
}

// minifyCSSFile removes unnecessary whitespace and comments from CSS
func minifyCSSFile(path string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
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

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(path, []byte(s), 0644)
}

// minifyJSFile removes unnecessary whitespace and comments from JS
func minifyJSFile(path string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
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

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(path, []byte(s), 0644)
}

// prettifyHTMLFile cleans up HTML by removing all blank lines for cleaner output
func prettifyHTMLFile(path string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}

	s := string(content)

	// Normalize line endings (handle CRLF)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Split into lines and filter out empty/whitespace-only lines
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t") // Remove trailing whitespace
		if strings.TrimSpace(trimmed) != "" {     // Keep only non-empty lines
			result = append(result, trimmed)
		}
	}

	// Join with newlines and ensure file ends with single newline
	s = strings.Join(result, "\n") + "\n"

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(path, []byte(s), 0644)
}

// convertToRelativeLinks converts absolute URLs to relative links in all HTML files
func (g *Generator) convertToRelativeLinks() error {
	return filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(strings.ToLower(path), ".html") {
			return convertToRelativeLinksFile(path, g.config.Domain)
		}
		return nil
	})
}

// convertToRelativeLinksFile converts absolute URLs to relative links in a single HTML file
func convertToRelativeLinksFile(path string, domain string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}

	s := string(content)

	// Build patterns for the domain (with and without trailing slash, http and https)
	// Remove protocol if present to get base domain
	baseDomain := domain
	baseDomain = strings.TrimPrefix(baseDomain, "https://")
	baseDomain = strings.TrimPrefix(baseDomain, "http://")
	baseDomain = strings.TrimSuffix(baseDomain, "/")

	// Replace patterns: href="https://domain/path" -> href="/path"
	// and src="https://domain/path" -> src="/path"
	patterns := []string{
		"https://" + baseDomain,
		"http://" + baseDomain,
		"//" + baseDomain,
	}

	for _, pattern := range patterns {
		// Replace href="pattern/..." with href="/..."
		// Replace href="pattern" with href="/"
		s = strings.ReplaceAll(s, `href="`+pattern+`"`, `href="/"`)
		s = strings.ReplaceAll(s, `href='`+pattern+`'`, `href='/'`)
		s = strings.ReplaceAll(s, `href="`+pattern+`/`, `href="/`)
		s = strings.ReplaceAll(s, `href='`+pattern+`/`, `href='/`)

		// Replace src="pattern/..." with src="/..."
		s = strings.ReplaceAll(s, `src="`+pattern+`"`, `src="/"`)
		s = strings.ReplaceAll(s, `src='`+pattern+`'`, `src='/'`)
		s = strings.ReplaceAll(s, `src="`+pattern+`/`, `src="/`)
		s = strings.ReplaceAll(s, `src='`+pattern+`/`, `src='/`)

		// Replace action="pattern/..." with action="/..."
		s = strings.ReplaceAll(s, `action="`+pattern+`"`, `action="/"`)
		s = strings.ReplaceAll(s, `action='`+pattern+`'`, `action='/'`)
		s = strings.ReplaceAll(s, `action="`+pattern+`/`, `action="/`)
		s = strings.ReplaceAll(s, `action='`+pattern+`/`, `action='/`)

		// Replace url(pattern/...) in inline styles
		s = strings.ReplaceAll(s, `url(`+pattern+`/`, `url(/`)
		s = strings.ReplaceAll(s, `url("`+pattern+`/`, `url("/`)
		s = strings.ReplaceAll(s, `url('`+pattern+`/`, `url('/`)
	}

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(path, []byte(s), 0644)
}
