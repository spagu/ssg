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

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/parser"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Shortcode defines a reusable content snippet
type Shortcode struct {
	Name     string            // Shortcode name (e.g., "thunderpick")
	Type     string            // Type for template logic (e.g., "banner")
	Template string            // Template file (required)
	Title    string            // Title/heading
	Text     string            // Text content
	Url      string            // Link URL
	Logo     string            // Logo/image path
	Legal    string            // Legal text
	Ranking  float64           // Ranking score (e.g., 3.5)
	Tags     []string          // Tags for categorization (e.g., ["game", "public"])
	Data     map[string]string // Additional custom data

	// Runtime fields (set per-invocation from inline attributes/content)
	Attrs        map[string]string // Inline attributes from [name key="val"]
	InnerContent string            // Content between [name]...[/name]
}

// MddbConfig holds MDDB connection settings for generator
type MddbConfig struct {
	Enabled       bool   // Enable mddb as content source
	URL           string // Base URL (e.g., "http://localhost:11023" or "localhost:11024" for gRPC)
	Protocol      string // Connection protocol: "http" (default) or "grpc"
	APIKey        string // Optional API key
	Collection    string // Collection name for content
	Lang          string // Language filter (e.g., "en_US")
	Timeout       int    // Request timeout in seconds
	BatchSize     int    // Batch size for pagination (default: 1000)
	Watch         bool   // Enable watch mode for MDDB changes
	WatchInterval int    // Watch interval in seconds (default: 30)
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
	SitemapOff        bool        // Disable sitemap generation
	RobotsOff         bool        // Disable robots.txt generation
	PrettyHTML        bool        // Prettify HTML output (remove extra blank lines)
	PostURLFormat     string      // Post URL format: "date" (/YYYY/MM/DD/slug/) or "slug" (/slug/)
	PageFormat        string      // Page output format: "directory" (slug/index.html), "flat" (slug.html), "both"
	RelativeLinks     bool        // Convert absolute URLs to relative links
	Shortcodes        []Shortcode // Shortcodes definitions
	ShortcodeBrackets bool        // Also match [shortcode] syntax
	MinifyHTML        bool        // Minify HTML output
	MinifyCSS         bool        // Minify CSS output
	MinifyJS          bool        // Minify JS output
	SourceMap         bool        // Include source maps
	Clean             bool        // Clean output directory before build
	Quiet             bool        // Suppress stdout output
	Engine            string      // Template engine: go, pongo2, mustache, handlebars
	// MDDB content source
	Mddb MddbConfig // MDDB configuration

	// Variables defines custom variables available in all templates as {{.Vars.key}}
	// and exported as environment variables with SSG_ prefix (e.g. SSG_GTM).
	// Values starting with $ are resolved from the current environment (e.g. "$GTM_CODE").
	Variables map[string]interface{}

	// PagesPath is the subdirectory name inside source for static pages (default: "pages")
	PagesPath string
	// PostsPath is the subdirectory name inside source for blog posts (default: "posts")
	PostsPath string

	// RewriteMdLinks rewrites relative .md links in content to their final output URLs (opt-in)
	RewriteMdLinks bool

	// PreserveSlugCase keeps original casing in slugs derived from filenames.
	// Default (false): slugs lowercased. When true: original case preserved.
	PreserveSlugCase bool
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

	// Resolve variables (expand $ENV_VAR references) and export as SSG_* env vars
	cfg.Variables = resolveVariables(cfg.Variables)
	exportVariablesToEnv(cfg.Variables, "SSG")

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

// resolveVariables replaces values starting with $ with the corresponding environment variable.
// Works recursively for nested maps.
func resolveVariables(vars map[string]interface{}) map[string]interface{} {
	if vars == nil {
		return nil
	}

	resolved := make(map[string]interface{}, len(vars))
	for k, v := range vars {
		switch val := v.(type) {
		case string:
			if strings.HasPrefix(val, "$") {
				envKey := strings.TrimPrefix(val, "$")
				if envVal := os.Getenv(envKey); envVal != "" {
					resolved[k] = envVal
				} else {
					resolved[k] = val // keep original if env var not set
				}
			} else {
				resolved[k] = val
			}
		case map[string]interface{}:
			resolved[k] = resolveVariables(val)
		default:
			resolved[k] = v
		}
	}
	return resolved
}

// exportVariablesToEnv sets each variable as an environment variable with the given prefix.
// Nested maps are flattened using _ as separator (e.g. prefix_KEY_SUBKEY).
func exportVariablesToEnv(vars map[string]interface{}, prefix string) {
	for k, v := range vars {
		envKey := strings.ToUpper(prefix + "_" + k)
		// Replace non-alphanumeric chars with _
		envKey = strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return '_'
		}, envKey)

		switch val := v.(type) {
		case map[string]interface{}:
			exportVariablesToEnv(val, strings.ToUpper(prefix+"_"+k))
		case string:
			_ = os.Setenv(envKey, val)
		default:
			_ = os.Setenv(envKey, fmt.Sprintf("%v", val))
		}
	}
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

// loadContent loads all content from the source directory or mddb
func (g *Generator) loadContent() error {
	// Check if mddb is enabled
	if g.config.Mddb.Enabled {
		return g.loadContentFromMddb()
	}

	return g.loadContentFromFiles()
}

// normalizeSlug returns the slug to use for a page.
// If slug is set in frontmatter it is used as-is.
// Otherwise it is derived from the source filename (without .md extension).
// Casing: lowercased by default; preserved when PreserveSlugCase is true.
func (g *Generator) normalizeSlug(slug, filename string) string {
	if slug != "" {
		if g.config.PreserveSlugCase {
			return slug
		}
		return strings.ToLower(slug)
	}
	// Derive from filename
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if g.config.PreserveSlugCase {
		return base
	}
	return strings.ToLower(base)
}

// pagesPath returns the configured pages subdirectory name, defaulting to "pages"
func (g *Generator) pagesPath() string {
	if g.config.PagesPath != "" {
		return g.config.PagesPath
	}
	return "pages"
}

// postsPath returns the configured posts subdirectory name, defaulting to "posts"
func (g *Generator) postsPath() string {
	if g.config.PostsPath != "" {
		return g.config.PostsPath
	}
	return "posts"
}

// loadContentFromFiles loads content from the local filesystem
func (g *Generator) loadContentFromFiles() error {
	sourcePath := filepath.Join(g.config.ContentDir, g.config.Source)

	// Load metadata.json
	metadataPath := filepath.Join(sourcePath, "metadata.json")
	if err := g.loadMetadata(metadataPath); err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}

	// Load pages
	pagesPath := filepath.Join(sourcePath, g.pagesPath())
	pages, err := g.loadMarkdownDir(pagesPath)
	if err != nil {
		return fmt.Errorf("loading pages: %w", err)
	}
	// Set page format for pages
	for i := range pages {
		pages[i].PageFormat = g.config.PageFormat
	}
	g.siteData.Pages = pages

	// Load posts
	postsPath := filepath.Join(sourcePath, g.postsPath())
	posts, err := g.loadPostsDir(postsPath)
	if err != nil {
		return fmt.Errorf("loading posts: %w", err)
	}

	// Set URL format and page format for posts based on config
	for i := range posts {
		posts[i].URLFormat = g.config.PostURLFormat
		posts[i].PageFormat = g.config.PageFormat
	}

	g.siteData.Posts = posts

	// Sort posts by date (newest first)
	sort.Slice(g.siteData.Posts, func(i, j int) bool {
		return g.siteData.Posts[i].Date.After(g.siteData.Posts[j].Date)
	})

	// Resolve flexible author/category fields (string → ID lookup)
	g.siteData.ResolveFlexibleFields()

	g.logContentStats()

	return nil
}

// loadContentFromMddb loads content from mddb server
func (g *Generator) loadContentFromMddb() error {
	client, err := mddb.NewMddbClient(mddb.ClientConfig{
		URL:       g.config.Mddb.URL,
		Protocol:  g.config.Mddb.Protocol,
		APIKey:    g.config.Mddb.APIKey,
		Timeout:   g.config.Mddb.Timeout,
		BatchSize: g.config.Mddb.BatchSize,
	})
	if err != nil {
		return fmt.Errorf("creating mddb client: %w", err)
	}

	// Check server health
	if err := client.Health(); err != nil {
		return fmt.Errorf("mddb server not available: %w", err)
	}

	g.log("   🔗 Connected to mddb server")

	// Load pages from mddb
	pageDocs, err := client.GetByType(g.config.Mddb.Collection, "page", g.config.Mddb.Lang)
	if err != nil {
		return fmt.Errorf("loading pages from mddb: %w", err)
	}

	pages, err := mddb.ToPages(pageDocs)
	if err != nil {
		return fmt.Errorf("converting pages: %w", err)
	}
	for i := range pages {
		pages[i].PageFormat = g.config.PageFormat
	}
	g.siteData.Pages = pages

	// Load posts from mddb
	postDocs, err := client.GetByType(g.config.Mddb.Collection, "post", g.config.Mddb.Lang)
	if err != nil {
		return fmt.Errorf("loading posts from mddb: %w", err)
	}

	posts, err := mddb.ToPages(postDocs)
	if err != nil {
		return fmt.Errorf("converting posts: %w", err)
	}

	// Set URL format and page format for posts based on config
	for i := range posts {
		posts[i].URLFormat = g.config.PostURLFormat
		posts[i].PageFormat = g.config.PageFormat
	}

	g.siteData.Posts = posts

	// Sort posts by date (newest first)
	sort.Slice(g.siteData.Posts, func(i, j int) bool {
		return g.siteData.Posts[i].Date.After(g.siteData.Posts[j].Date)
	})

	// Load metadata (categories, media, users) from mddb
	if err := g.loadMetadataFromMddb(client); err != nil {
		return fmt.Errorf("loading metadata from mddb: %w", err)
	}

	// Resolve flexible author/category fields (string → ID lookup)
	g.siteData.ResolveFlexibleFields()

	g.logContentStats()

	return nil
}

// loadMetadataFromMddb loads categories, media, and users from mddb
func (g *Generator) loadMetadataFromMddb(client mddb.MddbClient) error {
	// Load categories
	catDocs, err := client.GetAll("categories", g.config.Mddb.Lang, 100)
	if err != nil {
		// Categories collection might not exist - not critical
		g.log("   ⚠️  Warning: could not load categories from mddb")
	} else {
		for _, doc := range catDocs {
			cat := extractCategoryFromDoc(doc)
			g.siteData.Categories[cat.ID] = cat
		}
	}

	// Load media
	mediaDocs, err := client.GetAll("media", g.config.Mddb.Lang, 100)
	if err != nil {
		// Media collection might not exist - not critical
		g.log("   ⚠️  Warning: could not load media from mddb")
	} else {
		for _, doc := range mediaDocs {
			media := extractMediaFromDoc(doc)
			g.siteData.Media[media.ID] = media
		}
	}

	// Load users/authors
	userDocs, err := client.GetAll("users", g.config.Mddb.Lang, 100)
	if err != nil {
		// Users collection might not exist - not critical
		g.log("   ⚠️  Warning: could not load users from mddb")
	} else {
		for _, doc := range userDocs {
			author := extractAuthorFromDoc(doc)
			g.siteData.Authors[author.ID] = author
		}
	}

	return nil
}

// extractCategoryFromDoc extracts Category from mddb Document
func extractCategoryFromDoc(doc mddb.Document) models.Category {
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

// extractMediaFromDoc extracts MediaItem from mddb Document
func extractMediaFromDoc(doc mddb.Document) models.MediaItem {
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

	return media
}

// extractAuthorFromDoc extracts Author from mddb Document
func extractAuthorFromDoc(doc mddb.Document) models.Author {
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

// logContentStats prints content loading statistics
func (g *Generator) logContentStats() {
	fmt.Printf("   📄 Loaded %d pages\n", len(g.siteData.Pages))
	fmt.Printf("   📝 Loaded %d posts\n", len(g.siteData.Posts))
	fmt.Printf("   📁 Loaded %d categories\n", len(g.siteData.Categories))
	fmt.Printf("   🖼️  Loaded %d media items\n", len(g.siteData.Media))
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

// loadMarkdownDir loads all markdown files from a directory (recursively)
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
		entryPath := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			// Recursively load subdirectories
			subPages, err := g.loadMarkdownDir(entryPath)
			if err != nil {
				fmt.Printf("   ⚠️  Warning: failed to load subdirectory %s: %v\n", entry.Name(), err)
				continue
			}
			pages = append(pages, subPages...)
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		page, err := parser.ParseMarkdownFile(entryPath)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: failed to parse %s: %v\n", entry.Name(), err)
			continue
		}
		if page.Status == "publish" {
			page.SourceDir = dir
			page.SourceFile = entry.Name() // original filename e.g. "AUTHENTICATION.md"
			page.Slug = g.normalizeSlug(page.Slug, entry.Name())

			// Use file modification time as fallback for missing dates
			if page.Date.IsZero() || page.Modified.IsZero() {
				if info, err := entry.Info(); err == nil {
					if page.Date.IsZero() {
						page.Date = info.ModTime()
					}
					if page.Modified.IsZero() {
						page.Modified = info.ModTime()
					}
				}
			}

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

	// Also load templates from layouts subdirectory if it exists
	layoutsPath := filepath.Join(templatePath, "layouts", "*.html")
	if files, _ := filepath.Glob(layoutsPath); len(files) > 0 {
		tmpl, err = tmpl.ParseGlob(layoutsPath)
		if err != nil {
			return fmt.Errorf("parsing layout templates: %w", err)
		}
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

// buildMdLinkMap creates a map of .md filename variants to final output URLs.
// Priority order: exact SourceFile match > lowercase SourceFile > slug variants.
// This ensures that the actual filename on disk (e.g. "AUTHENTICATION.md") is always
// preferred over slug-derived names, so slug and filename can differ independently.
func (g *Generator) buildMdLinkMap() map[string]string {
	mdLinks := make(map[string]string)
	allPages := append(g.siteData.Pages, g.siteData.Posts...)
	for _, p := range allPages {
		url := p.GetURL()

		// 1. Actual source filename — highest priority (e.g. "AUTHENTICATION.md")
		if p.SourceFile != "" {
			mdLinks[p.SourceFile] = url
			mdLinks[strings.ToLower(p.SourceFile)] = url
		}

		// 2. Slug-derived variants — fallback when SourceFile not available (e.g. mddb source)
		slug := p.Slug
		mdLinks[slug+".md"] = url
		mdLinks[strings.ToUpper(slug)+".md"] = url
		mdLinks[slug] = url
	}
	return mdLinks
}

// rewriteMdLinks replaces relative .md hrefs in HTML with final output URLs.
// Handles: href="file.md", href="./file.md", href="../dir/file.md"
var mdLinkRe = regexp.MustCompile(`href="([^"]*\.md)"`)

func rewriteMdLinks(html string, mdLinkMap map[string]string) string {
	return mdLinkRe.ReplaceAllStringFunc(html, func(match string) string {
		// Extract path from href="..."
		inner := match[6 : len(match)-1] // strip href=" and "
		// Get base filename (last path segment)
		base := filepath.Base(inner)
		if url, ok := mdLinkMap[base]; ok {
			return `href="` + url + `"`
		}
		// Try without .md extension
		noExt := strings.TrimSuffix(base, ".md")
		if url, ok := mdLinkMap[noExt]; ok {
			return `href="` + url + `"`
		}
		return match // no match — leave as-is
	})
}

// buildTemplateFuncs creates the template function map
func (g *Generator) buildTemplateFuncs(pageLinks map[string]string) template.FuncMap {
	mdLinkMap := g.buildMdLinkMap()
	return template.FuncMap{
		"safeHTML":             g.tmplSafeHTML(pageLinks, mdLinkMap),
		"decodeHTML":           tmplDecodeHTML,
		"formatDate":           tmplFormatDate,
		"formatDatePL":         tmplFormatDatePL,
		"getCategoryName":      g.tmplGetCategoryName,
		"getCategorySlug":      g.tmplGetCategorySlug,
		"isValidCategory":      tmplIsValidCategory,
		"getAuthorName":        g.tmplGetAuthorName,
		"getURL":               tmplGetURL,
		"getCanonical":         tmplGetCanonical,
		"hasValidCategories":   tmplHasValidCategories,
		"thumbnailFromYoutube": tmplThumbnailFromYoutube,
		"stripShortcodes":      tmplStripShortcodes,
		"stripHTML":            tmplStripHTML,
		"recentPosts":          g.tmplRecentPosts,
		"default":              tmplDefault,
		"dict":                 tmplDict,
	}
}

// tmplDefault returns the default value if the given value is empty
func tmplDefault(defaultVal, val interface{}) interface{} {
	if val == nil || val == "" || val == 0 {
		return defaultVal
	}
	return val
}

// tmplDict creates a map from key-value pairs for template use
func tmplDict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}

// tmplSafeHTML returns the safeHTML template function
func (g *Generator) tmplSafeHTML(pageLinks map[string]string, mdLinkMap map[string]string) func(string) template.HTML {
	return func(s string) template.HTML {
		s = g.processShortcodes(s) // Process shortcodes first
		s = cleanMarkdownArtifacts(s)
		s = autolinkListItems(s, pageLinks)
		s = convertMarkdownToHTML(s)
		s = fixMediaPaths(s, g.siteData.Media)
		if g.config.RewriteMdLinks {
			s = rewriteMdLinks(s, mdLinkMap)
		}
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

// processShortcodes replaces {{shortcode_name}} with rendered HTML.
// When ShortcodeBrackets is enabled, also replaces [shortcode_name] for defined shortcodes only.
func (g *Generator) processShortcodes(content string) string {
	// Match {{shortcode_name}} pattern
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	content = re.ReplaceAllStringFunc(content, func(match string) string {
		name := match[2 : len(match)-2]
		sc, ok := g.shortcodeMap[name]
		if !ok {
			return "" // Remove undefined shortcodes
		}
		return g.renderShortcode(sc)
	})

	// Match bracket shortcodes (only defined shortcodes, opt-in)
	if g.config.ShortcodeBrackets && len(g.shortcodeMap) > 0 {
		content = g.processBracketShortcodes(content)
	}

	return content
}

// processBracketShortcodes handles WordPress-style bracket shortcodes:
//   - [name] — simple self-closing
//   - [name attr="val" attr2="val2"] — with attributes
//   - [name]inner content[/name] — with inner content
//   - [name attr="val"]inner content[/name] — with both
func (g *Generator) processBracketShortcodes(content string) string {
	// Process each defined shortcode by name (avoids backreference limitation in Go regexp)
	for name, baseSc := range g.shortcodeMap {
		// First: closing-tag with optional attrs [name ...]...[/name]
		reClosing := regexp.MustCompile(`\[` + regexp.QuoteMeta(name) + `((?:\s+\w+="[^"]*")*)\]([\s\S]*?)\[/` + regexp.QuoteMeta(name) + `\]`)
		content = reClosing.ReplaceAllStringFunc(content, func(match string) string {
			parts := reClosing.FindStringSubmatch(match)
			if len(parts) < 3 {
				return match
			}
			sc := g.shortcodeWithOverrides(baseSc, parts[1], parts[2])
			return g.renderShortcode(sc)
		})

		// Second: self-closing with attrs [name attr="val"]
		reSelfAttrs := regexp.MustCompile(`\[` + regexp.QuoteMeta(name) + `(\s+\w+="[^"]*"(?:\s+\w+="[^"]*")*)\]`)
		content = reSelfAttrs.ReplaceAllStringFunc(content, func(match string) string {
			parts := reSelfAttrs.FindStringSubmatch(match)
			if len(parts) < 2 {
				return match
			}
			sc := g.shortcodeWithOverrides(baseSc, parts[1], "")
			return g.renderShortcode(sc)
		})

		// Third: simple [name]
		reSimple := regexp.MustCompile(`\[` + regexp.QuoteMeta(name) + `\]`)
		content = reSimple.ReplaceAllStringFunc(content, func(_ string) string {
			return g.renderShortcode(baseSc)
		})
	}

	return content
}

// shortcodeWithOverrides creates a copy of a shortcode with inline attributes and inner content applied
func (g *Generator) shortcodeWithOverrides(base Shortcode, attrStr, innerContent string) Shortcode {
	sc := base
	sc.InnerContent = strings.TrimSpace(innerContent)
	sc.Attrs = parseShortcodeAttrs(attrStr)
	return sc
}

// parseShortcodeAttrs extracts key="value" pairs from an attribute string
func parseShortcodeAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	re := regexp.MustCompile(`(\w+)="([^"]*)"`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// renderShortcode renders a single shortcode to HTML using its template file
func (g *Generator) renderShortcode(sc Shortcode) string {
	if sc.Template == "" {
		fmt.Printf("   ⚠️  Warning: shortcode '%s' has no template defined, skipping\n", sc.Name)
		return ""
	}

	templatePath := filepath.Join(g.config.TemplatesDir, g.config.Template, sc.Template)

	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		fmt.Printf("   ⚠️  Warning: shortcode template not found: %s\n", templatePath)
		return ""
	}

	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(g.shortcodeFuncMap()).ParseFiles(templatePath)
	if err != nil {
		fmt.Printf("   ⚠️  Warning: shortcode template parse error: %v\n", err)
		return ""
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sc); err != nil {
		fmt.Printf("   ⚠️  Warning: shortcode template execute error: %v\n", err)
		return ""
	}

	return buf.String()
}

// shortcodeFuncMap returns template functions available in shortcode templates
func (g *Generator) shortcodeFuncMap() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s) // #nosec G203 -- shortcode content is author-controlled
		},
		"decodeHTML":      tmplDecodeHTML,
		"formatDate":      tmplFormatDate,
		"formatDatePL":    tmplFormatDatePL,
		"getCategoryName": g.tmplGetCategoryName,
		"getCategorySlug": g.tmplGetCategorySlug,
		"isValidCategory": tmplIsValidCategory,
		"getAuthorName":   g.tmplGetAuthorName,
		"stripShortcodes": tmplStripShortcodes,
		"stripHTML":       tmplStripHTML,
		"default":         tmplDefault,
		"dict":            tmplDict,
	}
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
		Vars   map[string]interface{}
	}{
		Site:   g.siteData,
		Posts:  g.siteData.Posts,
		Pages:  g.siteData.Pages,
		Domain: g.config.Domain,
		Vars:   g.config.Variables,
	}

	return g.renderTemplate("index.html", filepath.Join(g.config.OutputDir, "index.html"), data)
}

// getOutputPaths returns one or more output file paths based on PageFormat config.
// "directory" (default): slug/index.html
// "flat": slug.html
// "both": slug/index.html AND slug.html
// Special case: "404" always generates 404.html in root for Cloudflare Pages/Netlify compatibility
func (g *Generator) getOutputPaths(subPath string) []string {
	// Special handling for 404 page - always generate as flat file for static hosting compatibility
	if subPath == "404" {
		return []string{filepath.Join(g.config.OutputDir, "404.html")}
	}

	switch g.config.PageFormat {
	case "flat":
		return []string{filepath.Join(g.config.OutputDir, subPath+".html")}
	case "both":
		return []string{
			filepath.Join(g.config.OutputDir, subPath, "index.html"),
			filepath.Join(g.config.OutputDir, subPath+".html"),
		}
	default: // "directory" or empty
		return []string{filepath.Join(g.config.OutputDir, subPath, "index.html")}
	}
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

	// Convert page to flat map with Extra fields at top level
	data := g.pageToTemplateData(page, false)

	outputPaths := g.getOutputPaths(outputSubPath)
	for _, outputPath := range outputPaths {
		outputDir := filepath.Dir(outputPath)
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		// Copy co-located assets only to the directory-style path (avoid duplicates)
		if page.SourceDir != "" && strings.HasSuffix(outputPath, "index.html") {
			if err := g.copyColocatedAssets(page.SourceDir, outputDir); err != nil {
				fmt.Printf("   ⚠️  Warning: couldn't copy co-located assets for page %s: %v\n", page.Slug, err)
			}
		}

		// Use custom layout/template if specified, otherwise default to page.html
		templateName := "page.html"
		if page.Layout != "" {
			templateName = "layouts/" + page.Layout + ".html"
		} else if page.Template != "" {
			templateName = page.Template + ".html"
		}

		if err := g.renderTemplate(templateName, outputPath, data); err != nil {
			// Fallback to page.html if custom template not found
			if strings.Contains(err.Error(), "no such template") || strings.Contains(err.Error(), "is undefined") {
				if err := g.renderTemplate("page.html", outputPath, data); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	return nil
}

// generatePost generates a single post
func (g *Generator) generatePost(post models.Page) error {
	// Convert post to flat map with Extra fields at top level
	data := g.pageToTemplateData(post, true)

	outputPaths := g.getOutputPaths(post.GetOutputPath())
	for _, outputPath := range outputPaths {
		outputDir := filepath.Dir(outputPath)
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		// Copy co-located assets only to the directory-style path (avoid duplicates)
		if post.SourceDir != "" && strings.HasSuffix(outputPath, "index.html") {
			if err := g.copyColocatedAssets(post.SourceDir, outputDir); err != nil {
				fmt.Printf("   ⚠️  Warning: couldn't copy co-located assets for post %s: %v\n", post.Slug, err)
			}
		}

		if err := g.renderTemplate("post.html", outputPath, data); err != nil {
			return err
		}
	}

	return nil
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
			Vars     map[string]interface{}
		}{
			Site:     g.siteData,
			Category: cat,
			Posts:    posts,
			Domain:   g.config.Domain,
			Vars:     g.config.Variables,
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

// pageToTemplateData converts a Page to a map for templates, flattening Extra fields to top level
// This allows templates to use {{.dupa}} instead of {{.Page.Extra.dupa}}
func (g *Generator) pageToTemplateData(page models.Page, isPost bool) map[string]interface{} {
	data := map[string]interface{}{
		"Site":   g.siteData,
		"Domain": g.config.Domain,
		"Vars":   g.config.Variables,
		// Standard Page fields
		"ID":            page.ID,
		"Title":         page.Title,
		"Slug":          page.Slug,
		"Date":          page.Date,
		"Modified":      page.Modified,
		"Status":        page.Status,
		"Type":          page.Type,
		"Link":          page.Link,
		"Author":        page.Author,
		"Categories":    page.Categories,
		"Excerpt":       page.Excerpt,
		"Content":       template.HTML(page.Content), // #nosec G203 -- SSG intentionally renders user's markdown as HTML
		"URLFormat":     page.URLFormat,
		"PageFormat":    page.PageFormat,
		"SourceDir":     page.SourceDir,
		"Description":   page.Description,
		"Keywords":      page.Keywords,
		"Lang":          page.Lang,
		"Canonical":     page.Canonical,
		"Robots":        page.Robots,
		"Sitemap":       page.Sitemap,
		"FeaturedImage": page.FeaturedImage,
		"Tags":          page.Tags,
		"Category":      page.Category,
		"Layout":        page.Layout,
		"Template":      page.Template,
		// URL helpers
		"URL":          page.GetURL(),
		"CanonicalURL": page.GetCanonical(g.config.Domain),
		"OutputPath":   page.GetOutputPath(),
	}

	// Keep backward compatibility - include Page/Post struct
	if isPost {
		data["Post"] = page
	} else {
		data["Page"] = page
	}

	// Flatten Extra fields to top level for direct access like {{.dupa}}
	for key, value := range page.Extra {
		// Don't overwrite standard fields
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}

	return data
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

// isContentAsset returns true if the file is a non-markdown content asset (image, etc.)
func isContentAsset(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	assetExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
		".webp": true, ".ico": true, ".bmp": true, ".tiff": true, ".avif": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mp3": true, ".wav": true,
		".pdf": true, ".zip": true, ".tar": true, ".gz": true,
	}
	return assetExts[ext]
}

// copyColocatedAssets copies non-markdown files from a content source directory
// to the corresponding output directory of the generated page/post
func (g *Generator) copyColocatedAssets(sourceDir, outputDir string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil // Source dir might not exist, that's fine
	}

	copied := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if !isContentAsset(entry.Name()) {
			continue
		}

		srcPath := filepath.Join(sourceDir, entry.Name())
		dstPath := filepath.Join(outputDir, entry.Name())

		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		if err := g.copyFile(srcPath, dstPath); err != nil {
			fmt.Printf("   ⚠️  Warning: couldn't copy co-located asset %s: %v\n", entry.Name(), err)
			continue
		}
		copied++
	}

	if copied > 0 && !g.config.Quiet {
		fmt.Printf("   📎 Copied %d co-located asset(s) from %s\n", copied, filepath.Base(sourceDir))
	}

	return nil
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

// excludeFromSitemap returns true if a page should be excluded from sitemap.xml.
// Excluded: pages with robots containing "noindex", layout "redirect", or sitemap "no".
func excludeFromSitemap(page models.Page) bool {
	if strings.Contains(strings.ToLower(page.Robots), "noindex") {
		return true
	}
	if page.Layout == "redirect" {
		return true
	}
	if strings.EqualFold(page.Sitemap, "no") {
		return true
	}
	return false
}

// generateSitemap creates sitemap.xml
func (g *Generator) generateSitemap() error {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sb.WriteString("\n")

	// Homepage — skip if any index page has noindex
	skipHomepage := false
	for _, page := range g.siteData.Pages {
		if (page.Slug == "" || page.Slug == "index") && excludeFromSitemap(page) {
			skipHomepage = true
			break
		}
	}
	if !skipHomepage {
		sb.WriteString("  <url>\n")
		fmt.Fprintf(&sb, "    <loc>https://%s/</loc>\n", g.config.Domain)
		sb.WriteString("    <changefreq>daily</changefreq>\n")
		sb.WriteString("    <priority>1.0</priority>\n")
		sb.WriteString("  </url>\n")
	}

	// Pages
	for _, page := range g.siteData.Pages {
		if excludeFromSitemap(page) {
			continue
		}
		sb.WriteString("  <url>\n")
		fmt.Fprintf(&sb, "    <loc>%s</loc>\n", page.GetCanonical(g.config.Domain))
		lastmod := page.Modified
		if lastmod.IsZero() {
			lastmod = page.Date
		}
		if !lastmod.IsZero() {
			fmt.Fprintf(&sb, "    <lastmod>%s</lastmod>\n", lastmod.Format("2006-01-02"))
		}
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.8</priority>\n")
		sb.WriteString("  </url>\n")
	}

	// Posts
	for _, post := range g.siteData.Posts {
		if excludeFromSitemap(post) {
			continue
		}
		sb.WriteString("  <url>\n")
		fmt.Fprintf(&sb, "    <loc>%s</loc>\n", post.GetCanonical(g.config.Domain))
		lastmod := post.Modified
		if lastmod.IsZero() {
			lastmod = post.Date
		}
		if !lastmod.IsZero() {
			fmt.Fprintf(&sb, "    <lastmod>%s</lastmod>\n", lastmod.Format("2006-01-02"))
		}
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.6</priority>\n")
		sb.WriteString("  </url>\n")
	}

	// Categories
	for _, cat := range g.siteData.Categories {
		if cat.ID != 1 { // Skip "Bez kategorii"
			sb.WriteString("  <url>\n")
			fmt.Fprintf(&sb, "    <loc>https://%s/category/%s/</loc>\n", g.config.Domain, cat.Slug)
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
// Supports <!-- htmlmin:ignore --> ... <!-- /htmlmin:ignore --> to skip minification
func minifyHTMLFile(path string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}

	s := string(content)

	// Extract and preserve htmlmin:ignore blocks
	reIgnore := regexp.MustCompile(`(?s)<!--\s*htmlmin:ignore\s*-->(.*?)<!--\s*/htmlmin:ignore\s*-->`)
	preservedBlocks := make(map[string]string)
	blockIndex := 0
	s = reIgnore.ReplaceAllStringFunc(s, func(match string) string {
		// Extract content between ignore tags
		inner := reIgnore.FindStringSubmatch(match)
		if len(inner) > 1 {
			placeholder := fmt.Sprintf("__HTMLMIN_PRESERVE_%d__", blockIndex)
			preservedBlocks[placeholder] = inner[1]
			blockIndex++
			return placeholder
		}
		return match
	})

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

	// Restore preserved blocks
	for placeholder, content := range preservedBlocks {
		s = strings.ReplaceAll(s, placeholder, content)
	}

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
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

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
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

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
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

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
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

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
	return os.WriteFile(path, []byte(s), 0644)
}
