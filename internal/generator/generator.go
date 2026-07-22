// Package generator handles static site generation
package generator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/spagu/ssg/internal/engine"
	"github.com/spagu/ssg/internal/externalsource"
	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/images"
	"github.com/spagu/ssg/internal/mddb"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/parser"
	"github.com/spagu/ssg/internal/taxonomy"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
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
	// Vars mirrors the site-wide variables: (config) so {{$.Vars.key}} means the
	// same inside a shortcode template as in every other template (issue #37).
	// Filled per render, never from the config file.
	Vars map[string]interface{}
	// Raw is the source that produced this invocation ("{{promo}}",
	// `[promo a="b"]…[/promo]`). A shortcode whose template fails to render is
	// replaced by this text rather than by nothing, so the gap is visible in the
	// published page instead of silently shipping (issue #37).
	Raw string
}

// MddbConfig holds MDDB connection settings for generator
type MddbConfig struct {
	Enabled    bool   // Enable mddb as content source
	URL        string // Base URL (e.g., "http://localhost:11023" or "localhost:11024" for gRPC)
	Protocol   string // Connection protocol: "http" (default) or "grpc"
	APIKey     string // Optional API key
	Collection string // Collection name for content
	Lang       string // Language filter (e.g., "en_US")
	Timeout    int    // Request timeout in seconds
	BatchSize  int    // Batch size for pagination (default: 1000)
	// Watch/WatchInterval live in the CLI config only — the watch loop runs in
	// cmd/ssg, not in the generator (GO-043: dead copies removed).
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
	SourceMap         bool        // Emit v3 source maps for minified JS/CSS (BLOG-007/GO-004)
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

	// StaticDir is a project-level directory whose entire contents (all files
	// and subdirectories, recursively) are copied verbatim into the output
	// directory during generation. Default: "static". Missing directory is a
	// no-op. Fixes #8 where only some static assets reached the output.
	StaticDir string

	// DataDir holds data files (*.yaml|*.yml|*.json) loaded into .Data.* (PLAT-002).
	DataDir string

	// Permalinks maps content type ("post"/"page") to a URL pattern with tokens
	// :year :month :day :slug :category. Empty = default behaviour (SEO-001).
	Permalinks map[string]string

	// LastmodFromGit derives sitemap <lastmod> from the source file's last git
	// commit date, with graceful fallback outside git or for mddb content (SEO-004).
	LastmodFromGit bool

	// Fingerprint enables content-hash asset names + manifest + reference rewrite
	// for immutable caching; runs as the terminal asset step (ASSET-001).
	Fingerprint bool
	SCSS        bool   // compile *.scss via dart-sass (ASSET-003)
	SassBinary  string // explicit dart-sass path; empty = PATH lookup

	// Responsive image presets (ASSET-004) are consumed directly by the webp
	// package from the CLI config (GO-043: dead generator copies removed).

	// Timezone renders content dates (permalink tokens, Date/Modified template
	// context) in this IANA zone; LanguageTimezones overrides it per page
	// language. Empty = no conversion (I18N-001).
	Timezone          string
	LanguageTimezones map[string]string

	// Math enables opt-in KaTeX injection on pages containing math delimiters (AX-004).
	Math bool

	// Paginate is posts-per-page for the index; 0 disables pagination (BLOG-003).
	Paginate int

	// Languages / DefaultLanguage drive language-aware output + hreflang (PLAT-005).
	Languages       []string
	DefaultLanguage string
	LanguageConfigs []ssgi18n.LanguageConfig
	I18n            ssgi18n.Config

	// Taxonomies declares custom dynamic taxonomies / overrides the built-in
	// category, tag and series definitions (taxonomies-feature.md).
	Taxonomies map[string]taxonomy.DefinitionConfig

	// ExternalSources configures the unified external data system
	// (ssg-external-sources-implementation-plan.md, phase 1: local files).
	ExternalSources externalsource.Config

	// Hooks are lifecycle exec commands (pre_build/post_build/post_page) from
	// trusted local config only (PLAT-001).
	Hooks map[string][]string

	// Feed / highlighting / TOC / SEO / link-check / bundling / outputs / search
	// (BLOG-002, AX-001, AX-002, SEO-003, SEO-005, ASSET-002, PLAT-003, PLAT-004).
	Feed            bool
	FeedItems       int
	FeedFullContent bool
	Highlight            bool
	HighlightStyle       string
	HighlightLineNumbers bool // prefix highlighted blocks with line numbers (GO-073)
	Mermaid              bool // render ```mermaid fences as diagrams (GO-073)
	TOC                  bool
	TOCDepth        int
	SEO             bool // opt-in generator-level OG/Twitter/JSON-LD injection (v1.8.2)
	CheckLinks      string
	Bundles         map[string][]string
	Outputs         []string
	SearchIndex     bool

	// SanitizeHTML runs rendered content through bluemonday's UGCPolicy to
	// neutralise stored XSS from untrusted mddb content (FE-005 / SEC-003).
	SanitizeHTML bool

	// ContentSources are extra local Markdown roots merged into the site
	// alongside (or instead of) the primary source (CONTENT-002).
	ContentSources []ContentSource

	// LinkRewrites maps an href prefix to its replacement, for links that point
	// at repository files the site never publishes (LINK-002).
	LinkRewrites map[string]string

	// AutoExcerpt derives a missing excerpt from the content's opening
	// paragraph instead of leaving it empty (GO-057). Off by default: it
	// changes listing text, feed summaries and meta descriptions.
	AutoExcerpt bool

	// ShortcodeErrors decides what a shortcode that fails to render leaves
	// behind: "" / "drop" (historical behaviour — a warning and nothing in the
	// page), "keep" (the shortcode's raw source, so the gap is visible) or
	// "strict" (keep, then fail the build). Issue #37.
	ShortcodeErrors string

	// Headers overrides/extends the generated _headers blocks per path pattern;
	// HeadersDefaultsOff drops the built-in blocks entirely (GO-064).
	Headers            map[string]map[string]string
	HeadersDefaultsOff bool

	// Redirects are explicit `_redirects` rules; frontmatter aliases: are added
	// as 301s automatically. AliasStubsOff suppresses the meta-refresh stub
	// pages (the `_redirects` entries remain), for Cloudflare/Netlify-only
	// sites that don't need the client-side fallback (GO-063).
	Redirects     []RedirectRule
	AliasStubsOff bool

	// Worker wires a Cloudflare Pages Functions / Worker project into the
	// build output (GO-065).
	Worker WorkerConfig
}

// defaultStaticDir is the fallback name for the passthrough static directory.
const defaultStaticDir = "static"

// Shared string literals (avoids scattered duplicates).
const (
	httpsScheme      = "https://" // shared URL scheme prefix (S1192)
	indexHTMLName    = "index.html"
	pageHTMLName     = "page.html"
	postHTMLName     = "post.html"
	categoryHTMLName = "category.html"
	tagHTMLName      = "tag.html"
	seriesHTMLName   = "series.html"
	authorHTMLName   = "author.html"
	htmlGlobPattern  = "*.html"
	feedFileName     = "feed.xml"
	sitemapURLOpen   = "  <url>\n"
	sitemapURLClose  = "  </url>\n"
)

// Generator handles the static site generation process
type Generator struct {
	config       Config
	siteData     *models.SiteData
	tmpl         *template.Template
	shortcodeMap map[string]Shortcode     // Map of shortcode name to shortcode
	data         map[string]interface{}   // Data files loaded into .Data.* (PLAT-002)
	translations map[string][]Translation // slug → language variants (PLAT-005)
	catalog      *ssgi18n.Catalog
	// currentLang is the language of the page being rendered. Mutable per-render
	// state (set in pageToTemplateData / the per-language index loop): correct for
	// the single-goroutine build, but must become per-render context before any
	// future parallel rendering.
	currentLang string
	md          goldmark.Markdown  // configured Markdown renderer (AX-001/002/003)
	tagSlugs    map[string]string  // tag name → slug, for sitemap/feeds (BLOG-004)
	authorSlugs map[string]string  // author slug → slug, for sitemap (BLOG-005)
	taxonomies  *taxonomy.Registry // generic taxonomy registry (taxonomies-feature.md)
	// External sources: .ExternalData / .ExternalDataMeta namespaces plus
	// content-mode CMS imports merged into the site before finalize.
	externalData map[string]interface{}
	externalMeta map[string]externalsource.Metadata
	cmsImports   []*externalsource.CMSImportResult

	// ownedURLs caches content URLs for archive-collision checks (GO-050);
	// built lazily by archiveURLOwner after content is finalized.
	ownedURLs   map[string]string
	engine      engine.Engine // non-Go template engine when configured (GO-007)
	engineTmpls map[string]engine.Template
	sanitizer   *bluemonday.Policy // HTML sanitizer when SanitizeHTML is on (FE-005)

	shortcodeTmpls    map[string]*template.Template  // parsed shortcode templates, one parse per build (PERF-002)
	bracketRes        map[string]bracketShortcodeRes // per-shortcode bracket regexes, compiled once (PERF-006)
	shortcodeFailures []string                       // shortcodes that failed to render (issue #37)
	linkRewriteKeys   []string                       // link_rewrites prefixes, longest first (LINK-002)
	gitOnce           sync.Once                      // guards the single git-log scan (PERF-001)
	gitRoot           string                         // repo top-level dir for lastmod lookups (PERF-001)
	gitTimes          map[string]time.Time           // repo-relative path → last commit date (PERF-001)
	refCache          map[string]bool                // link-checker target memo (PERF-009)
	siteLoc           *time.Location                 // resolved Timezone; nil = no conversion (I18N-001)
	langLocs          map[string]*time.Location      // per-language zone overrides (I18N-001)

	// mdCache memoizes goldmark conversions keyed by the exact markdown source, so
	// feeds, the search index, JSON output and per-path renders do not re-convert
	// the same content (PERF-004). mdConversions counts REAL conversions and backs
	// the once-per-content acceptance test. Builds are single-goroutine.
	mdCache       map[string]string
	mdConversions int
	mdLinkWarned  map[string]bool // once-per-(link,lang) missing-translation warnings (i18n §13)

	// aliasRedirects collects frontmatter alias→URL pairs during page/post
	// generation for the _redirects file (GO-063). Single-goroutine build —
	// needs a mutex before any future parallel rendering (see currentLang).
	aliasRedirects []RedirectRule

	// images is the lazily-built processor behind the image* template helpers
	// (audit/images-processing-feature.md).
	images *images.Processor
}

// resolveLocations loads the configured IANA zones; unknown names warn and are
// skipped so a typo degrades to the no-conversion default instead of failing
// the build (I18N-001).
func resolveLocations(cfg Config) (*time.Location, map[string]*time.Location) {
	load := func(name, scope string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: unknown timezone %q for %s — dates left unconverted\n", name, scope)
			return nil
		}
		return loc
	}
	var siteLoc *time.Location
	if cfg.Timezone != "" {
		siteLoc = load(cfg.Timezone, "site")
	}
	langLocs := make(map[string]*time.Location, len(cfg.LanguageTimezones))
	for lang, name := range cfg.LanguageTimezones {
		if loc := load(name, "language "+lang); loc != nil {
			langLocs[lang] = loc
		}
	}
	return siteLoc, langLocs
}

// pageLocation returns the render zone for a page: the per-language override
// wins, then the site timezone, then nil (no conversion).
func (g *Generator) pageLocation(p models.Page) *time.Location {
	if loc, ok := g.langLocs[p.Lang]; ok {
		return loc
	}
	return g.siteLoc
}

// pageDate converts a content date into the page's configured render zone.
// Zero dates and unconfigured zones pass through untouched (I18N-001).
func (g *Generator) pageDate(p models.Page, t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	if loc := g.pageLocation(p); loc != nil {
		return t.In(loc)
	}
	return t
}

// bracketShortcodeRes holds the three bracket-syntax regexes for one shortcode
// name; compiling them once per build instead of once per page keeps shortcode
// expansion off the render hot path (PERF-006).
type bracketShortcodeRes struct {
	closing   *regexp.Regexp // [name attrs]inner[/name]
	selfAttrs *regexp.Regexp // [name attr="val"]
	simple    *regexp.Regexp // [name]
}

// compileBracketRes builds the bracket-syntax regexes for one shortcode name.
func compileBracketRes(name string) bracketShortcodeRes {
	q := regexp.QuoteMeta(name)
	return bracketShortcodeRes{
		closing:   regexp.MustCompile(`\[` + q + `((?:\s+\w+="[^"]*")*)\]([\s\S]*?)\[/` + q + `\]`),
		selfAttrs: regexp.MustCompile(`\[` + q + `(\s+\w+="[^"]*"(?:\s+\w+="[^"]*")*)\]`),
		simple:    regexp.MustCompile(`\[` + q + `\]`),
	}
}

// New creates a new Generator instance
func New(cfg Config) (*Generator, error) {
	cfg.I18n = cfg.I18n.WithDefaults()
	languages := ssgi18n.Normalize(cfg.Languages, cfg.LanguageConfigs, cfg.LanguageTimezones)
	if err := ssgi18n.Validate(languages, cfg.DefaultLanguage, cfg.I18n); err != nil {
		return nil, err
	}
	// Expanded language timezones feed the existing date-rendering path. Copy
	// the legacy map first so constructing a generator never mutates its caller.
	mergedTZ := make(map[string]string, len(cfg.LanguageTimezones)+len(languages))
	for code, zone := range cfg.LanguageTimezones {
		mergedTZ[code] = zone
	}
	for _, lang := range languages {
		if lang.Timezone != "" {
			mergedTZ[lang.Code] = lang.Timezone
		}
	}
	cfg.LanguageTimezones = mergedTZ
	var catalog *ssgi18n.Catalog
	if cfg.I18n.Enabled {
		var err error
		catalog, err = ssgi18n.LoadCatalog(cfg.I18n.TranslationsDir, languages)
		if err != nil {
			return nil, err
		}
	}
	// Build shortcode map for quick lookup
	scMap := make(map[string]Shortcode)
	bracketRes := make(map[string]bracketShortcodeRes, len(cfg.Shortcodes))
	for _, sc := range cfg.Shortcodes {
		scMap[sc.Name] = sc
		bracketRes[sc.Name] = compileBracketRes(sc.Name)
	}

	// Resolve variables (expand $ENV_VAR references) and export as SSG_* env vars
	cfg.Variables = resolveVariables(cfg.Variables)
	exportVariablesToEnv(cfg.Variables, "SSG")

	siteLoc, langLocs := resolveLocations(cfg) // I18N-001

	return &Generator{
		config: cfg,
		siteData: &models.SiteData{
			Domain:     cfg.Domain,
			Categories: make(map[int]models.Category),
			Media:      make(map[int]models.MediaItem),
			Authors:    make(map[int]models.Author),
			Tags:       make(map[int]models.Category),
		},
		shortcodeMap:   scMap,
		md:             buildMarkdown(cfg),
		sanitizer:      newSanitizer(cfg.SanitizeHTML),
		shortcodeTmpls: make(map[string]*template.Template),
		mdCache:        make(map[string]string),
		bracketRes:     bracketRes,
		refCache:       make(map[string]bool),
		siteLoc:        siteLoc,
		langLocs:       langLocs,
		catalog:        catalog,
		currentLang:    cfg.DefaultLanguage,
	}, nil
}

// newSanitizer returns a bluemonday UGC policy when sanitisation is enabled, else nil
// (FE-005 / SEC-003).
func newSanitizer(enabled bool) *bluemonday.Policy {
	if !enabled {
		return nil
	}
	return bluemonday.UGCPolicy()
}

// buildMarkdown assembles the goldmark renderer from config: tables + footnotes are
// always on (footnotes are a common WP-export artifact, AX-003); auto heading IDs
// back the table of contents (AX-002); Chroma syntax highlighting is added when
// enabled (AX-001). WithUnsafe preserves the SSG contract of rendering author HTML.
func buildMarkdown(cfg Config) goldmark.Markdown {
	exts := []goldmark.Extender{extension.Table, extension.Footnote}
	if cfg.Highlight {
		style := cfg.HighlightStyle
		if style == "" {
			style = "github"
		}
		opts := []highlighting.Option{highlighting.WithStyle(style)}
		if cfg.HighlightLineNumbers {
			opts = append(opts, highlighting.WithFormatOptions(chromahtml.WithLineNumbers(true)))
		}
		exts = append(exts, highlighting.NewHighlighting(opts...))
	}
	return goldmark.New(
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(
			gmparser.WithAutoHeadingID(),
			// Recompute heading ids from the VISIBLE text (issue #26): the
			// built-in generator derives them from the raw source line, so a
			// heading containing a Markdown link leaks the href into its id.
			gmparser.WithASTTransformers(gmutil.Prioritized(headingIDTransformer{}, 900)),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}

// headingIDTransformer fixes the auto ids of headings that CONTAIN a link or
// image: goldmark derives ids from the raw source line, so
// "### [Ian Zane](/authors/ian-zane/) — Generalist" became
// id="ian-zaneauthorsian-zane--generalist" (issue #26). Only such headings are
// recomputed to slugify(visible text) — plain headings keep goldmark's ids
// bit-for-bit, so anchors on pre-1.8.6 sites stay valid. De-duplication spans
// the whole document (kept goldmark ids included). The TOC (AX-002) reads the
// same attribute, so intra-page anchors stay consistent.
type headingIDTransformer struct{}

// Transform implements gmparser.ASTTransformer.
func (headingIDTransformer) Transform(doc *ast.Document, reader text.Reader, _ gmparser.Context) {
	used := map[string]bool{}
	var affected []*ast.Heading
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if headingHasLink(h) {
			affected = append(affected, h)
		} else if v, ok := h.AttributeString("id"); ok {
			if idBytes, ok := v.([]byte); ok {
				used[string(idBytes)] = true // keep goldmark's id, reserve it
			}
		}
		return ast.WalkContinue, nil
	})
	for _, h := range affected {
		id := slugify(nodeText(h, reader.Source()))
		if id == "" {
			id = "heading"
		}
		base := id
		for i := 1; used[id]; i++ {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		used[id] = true
		h.SetAttributeString("id", []byte(id))
	}
}

// headingHasLink reports whether a heading contains a link, autolink or image
// node — the cases where goldmark's raw-source id leaks the destination URL.
func headingHasLink(h ast.Node) bool {
	found := false
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindLink, ast.KindAutoLink, ast.KindImage:
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
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
	if err := g.runHooks("pre_build", nil); err != nil {
		return fmt.Errorf("pre_build hook: %w", err)
	}

	if err := g.cleanOutputIfRequested(); err != nil {
		return err
	}

	if err := g.loadPhase(); err != nil {
		return err
	}

	if err := g.runStep("🏗️  Generating site...", g.generateSite, "generating site"); err != nil {
		return err
	}

	// Fail here rather than at the end: everything after this only decorates
	// output whose content blocks are already known to be incomplete (issue #37).
	if err := g.shortcodeErrorCheck(); err != nil {
		return err
	}

	if err := g.runStep("📁 Copying assets...", g.copyAssets, "copying assets"); err != nil {
		return err
	}

	if err := g.runStep("📦 Copying static directory...", g.copyStaticDir, "copying static directory"); err != nil {
		return err
	}

	if err := g.generateSitemapAndRobots(); err != nil {
		return err
	}

	if err := g.generateFeeds(); err != nil {
		return fmt.Errorf("generating feeds: %w", err)
	}

	if err := g.generateSearchIndex(); err != nil {
		return fmt.Errorf("building search index: %w", err)
	}

	if err := g.runStep("☁️  Generating Cloudflare Pages files...", g.generateCloudflareFiles, "generating Cloudflare files"); err != nil {
		return err
	}

	if err := g.assetPhase(); err != nil {
		return err
	}

	if err := g.runHooks("post_build", nil); err != nil {
		return fmt.Errorf("post_build hook: %w", err)
	}

	return nil
}

// loadPhase runs the input steps of the build. External sources load FIRST so
// content-mode CMS imports can merge into the site before finalize; content,
// data files, taxonomies and templates follow.
func (g *Generator) loadPhase() error {
	if g.config.ExternalSources.Enabled {
		if err := g.runStep("🔌 Loading external sources...", g.loadExternalSources, "loading external sources"); err != nil {
			return err
		}
	}
	if err := g.runStep("🔄 Loading content...", g.loadContent, "loading content"); err != nil {
		return err
	}
	if err := g.runStep("🗂️  Loading data files...", g.loadData, "loading data files"); err != nil {
		return err
	}
	if err := g.runStep("🏷️  Building taxonomies...", g.buildTaxonomies, "building taxonomies"); err != nil {
		return err
	}
	return g.runStep("📝 Loading templates...", g.loadTemplates, "loading templates")
}

// assetPhase runs the global post-render passes. Per-file HTML transforms
// (SEO, math, relative links, prettify, HTML minify) are applied at render
// time in a single write (PERF-005); only genuinely global passes live here.
func (g *Generator) assetPhase() error {
	// SCSS compiles before bundling so bundles/minify/fingerprint see CSS (ASSET-003).
	if err := g.compileSCSSIfRequested(); err != nil {
		return fmt.Errorf("compiling SCSS: %w", err)
	}
	// Bundling concatenates asset groups before minification/fingerprinting (ASSET-002).
	if err := g.bundleIfRequested(); err != nil {
		return fmt.Errorf("bundling assets: %w", err)
	}
	// CSS/JS minification must run after bundling; HTML was minified at render.
	if err := g.minifyIfRequested(); err != nil {
		return err
	}
	// Fingerprinting is the terminal asset step: it must run after bundling and
	// minification so hashes reflect final byte content (ASSET-001).
	if err := g.fingerprintIfRequested(); err != nil {
		return err
	}
	// Link checking runs last, over the final output tree (SEO-005).
	return g.checkLinksIfRequested()
}

// hookTimeout bounds every lifecycle hook so a hung command cannot stall the build.
const hookTimeout = 60 * time.Second

// runHooks executes the configured commands for a lifecycle phase (PLAT-001).
// Security: commands come only from trusted local config, are split into argv and
// run WITHOUT a shell, time-limited, and never sourced from content. Build context
// is passed via the environment (SSG_OUTPUT_DIR, SSG_PHASE, plus any extraEnv). A
// non-zero exit is returned so callers can decide whether to fail or warn.
func (g *Generator) runHooks(phase string, extraEnv map[string]string) error {
	for _, cmdline := range g.config.Hooks[phase] {
		fields := strings.Fields(cmdline)
		if len(fields) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
		// #nosec G204 -- command comes from trusted local config, argv-split, no shell
		cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
		cmd.Env = append(os.Environ(),
			"SSG_OUTPUT_DIR="+g.config.OutputDir,
			"SSG_PHASE="+phase,
		)
		for k, v := range extraEnv {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			return fmt.Errorf("hook %q (%s): %w", cmdline, phase, err)
		}
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

// minifyIfRequested minifies CSS/JS assets if configured. It runs after
// bundling; HTML minification happens per file at render time (PERF-005).
func (g *Generator) minifyIfRequested() error {
	if !g.config.MinifyCSS && !g.config.MinifyJS {
		return nil
	}
	g.log("🗜️  Minifying assets...")
	if err := g.minifyAssetsOutput(); err != nil {
		return fmt.Errorf("minifying output: %w", err)
	}
	return nil
}

// minifyAssetsOutput minifies CSS and JS files in the output directory. HTML is
// deliberately excluded — it is minified in memory at render time (PERF-005).
func (g *Generator) minifyAssetsOutput() error {
	return filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if merr := g.minifyAssetByExt(path); merr != nil {
			return fmt.Errorf("minifying %s: %w", path, merr)
		}
		return nil
	})
}

// minifyAssetByExt minifies one asset file when its type's minification is on.
func (g *Generator) minifyAssetByExt(path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css":
		if g.config.MinifyCSS {
			return g.minifyAssetFile(path, minifyCSSFile, minifyCSSLinePreserving)
		}
	case ".js":
		if g.config.MinifyJS {
			return g.minifyAssetFile(path, minifyJSFile, minifyJSLinePreserving)
		}
	}
	return nil
}

// loadContent loads all content from the source directory or mddb
func (g *Generator) loadContent() error {
	// Check if mddb is enabled
	var err error
	if g.config.Mddb.Enabled {
		err = g.loadContentFromMddb()
	} else {
		err = g.loadContentFromFiles()
	}
	if err != nil {
		return err
	}

	// Extra local Markdown roots (content_sources) join the site next, so a
	// docs/ folder elsewhere in the repository is treated like native content
	// (CONTENT-002).
	if err := g.loadExtraContentSources(); err != nil {
		return err
	}

	// Content-mode CMS imports join the site before finalize so they get the
	// same URL/translation/taxonomy treatment as native content.
	g.mergeCMSContent()

	return g.finalizeLoadedContent()
}

// finalizeLoadedContent computes derived per-page fields once, for every content
// source: reading stats (BLOG-006) and configured permalink paths (SEO-001).
// Runs after metadata is loaded so category slugs are resolvable.
func (g *Generator) finalizeLoadedContent() error {
	languages := ssgi18n.Normalize(g.config.Languages, g.config.LanguageConfigs, g.config.LanguageTimezones)
	g.siteData.Languages = languages
	g.siteData.DefaultLanguage = g.config.DefaultLanguage
	finalize := func(pages []models.Page, defaultType string) {
		for i := range pages {
			if g.config.I18n.Enabled && pages[i].Lang == "" {
				pages[i].Lang = g.config.DefaultLanguage
			}
			if lang, ok := ssgi18n.Language(languages, pages[i].Lang); ok {
				pages[i].Locale = lang.Locale
			} else if g.config.I18n.Enabled {
				// Validation below turns this into a descriptive build error/warning.
				pages[i].Locale = pages[i].Lang
			}
			if g.config.I18n.Enabled && pages[i].TranslationKey == "" {
				pages[i].TranslationKey = generatedTranslationKey(pages[i])
			}
			pages[i].ComputeReadingStats()
			if g.config.Math {
				pages[i].HasMath = containsMath(pages[i].Content)
			}
			// Language prefix for non-default languages (PLAT-005).
			if g.config.I18n.Enabled {
				pages[i].LangPrefix = ssgi18n.Prefix(pages[i].Lang, g.config.DefaultLanguage, g.config.I18n)
			} else if len(g.config.Languages) > 0 && pages[i].Lang != "" && pages[i].Lang != g.config.DefaultLanguage {
				pages[i].LangPrefix = pages[i].Lang
			}
			typ := pages[i].Type
			if typ == "" {
				typ = defaultType
			}
			if pattern := g.config.Permalinks[typ]; pattern != "" {
				pages[i].PermalinkPath = g.expandPermalink(pattern, pages[i])
			}
		}
	}
	finalize(g.siteData.Pages, "page")
	finalize(g.siteData.Posts, "post")
	g.computeSeriesLinks()
	g.computeTranslations()
	if g.config.I18n.Enabled {
		if err := g.validateI18nContent(languages); err != nil {
			return err
		}
		if err := g.detectContentCollisions(); err != nil {
			return err
		}
	}
	return nil
}

func generatedTranslationKey(p models.Page) string {
	name := strings.TrimSuffix(filepath.ToSlash(p.SourceFile), filepath.Ext(p.SourceFile))
	name = regexp.MustCompile(`(?i)[._-](?:[a-z]{2}(?:-[A-Z]{2})?)$`).ReplaceAllString(name, "")
	if name == "" {
		name = p.Slug
	}
	return strings.ToLower(name)
}

func (g *Generator) validateI18nContent(languages []ssgi18n.LanguageConfig) error {
	known := map[string]bool{}
	for _, l := range languages {
		known[l.Code] = true
	}
	seen := map[string]string{}
	all := append(append([]models.Page{}, g.siteData.Pages...), g.siteData.Posts...)
	for _, p := range all {
		if !known[p.Lang] {
			msg := fmt.Sprintf("content %q uses unconfigured language %q", p.SourceFile, p.Lang)
			if g.config.I18n.InvalidLanguage == "warn" {
				fmt.Printf("   ⚠️  %s\n", msg)
				continue
			}
			return fmt.Errorf("%s", msg)
		}
		key := p.TranslationKey + "\x00" + p.Lang
		if previous, ok := seen[key]; ok {
			msg := fmt.Sprintf("duplicate translation key %q for language %q (%s and %s)", p.TranslationKey, p.Lang, previous, p.SourceFile)
			if g.config.I18n.DuplicateTranslation == "warn" {
				fmt.Printf("   ⚠️  %s\n", msg)
				continue
			}
			return fmt.Errorf("%s", msg)
		}
		seen[key] = p.SourceFile
	}
	return nil
}

func (g *Generator) detectContentCollisions() error {
	seen := map[string]string{}
	all := append(append([]models.Page{}, g.siteData.Pages...), g.siteData.Posts...)
	for _, p := range all {
		path := p.GetOutputPath()
		if previous, ok := seen[path]; ok {
			return fmt.Errorf("i18n output collision at %q between %s and %s", path, previous, p.SourceFile)
		}
		seen[path] = p.SourceFile
		for _, alias := range p.Aliases {
			rel := models.SanitizeRelPath(alias)
			if p.LangPrefix != "" {
				rel = models.SanitizeRelPath(p.LangPrefix + "/" + rel)
			}
			if previous, ok := seen[rel]; ok {
				return fmt.Errorf("i18n output collision at alias %q between %s and %s", rel, previous, p.SourceFile)
			}
			seen[rel] = p.SourceFile
		}
	}
	return nil
}

// translationsFor returns the language variants of a page (PLAT-005).
func (g *Generator) translationsFor(p models.Page) []Translation {
	if g.translations == nil {
		return nil
	}
	key := strings.ToLower(p.Slug)
	if g.config.I18n.Enabled {
		key = p.TranslationKey
	}
	return g.translations[key]
}

// hreflangTags builds <link rel="alternate" hreflang> markup for a page's
// translations, including x-default for the default language (PLAT-005). Returns
// safe HTML for direct inclusion in <head>; empty when there is nothing to link.
func (g *Generator) hreflangTags(p models.Page) template.HTML {
	trs := g.translationsFor(p)
	if len(trs) < 2 {
		return ""
	}
	domain := stdhtml.EscapeString(g.config.Domain)
	var b strings.Builder
	hasDefault := false
	for _, t := range trs {
		lang := stdhtml.EscapeString(t.Lang)
		href := httpsScheme + domain + stdhtml.EscapeString(t.URL)
		fmt.Fprintf(&b, `<link rel="alternate" hreflang="%s" href="%s">`+"\n", lang, href)
		if t.IsDefault {
			hasDefault = true
			fmt.Fprintf(&b, `<link rel="alternate" hreflang="x-default" href="%s">`+"\n", href)
		}
	}
	if !hasDefault {
		// No default-language variant in this group: x-default points at the
		// default-language site root instead (i18n §9).
		root := httpsScheme + domain + stdhtml.EscapeString(g.languageURL(g.config.DefaultLanguage))
		fmt.Fprintf(&b, `<link rel="alternate" hreflang="x-default" href="%s">`+"\n", root)
	}
	return template.HTML(b.String()) // #nosec G203 -- values are HTML-escaped above
}

// Translation is one language variant of a page for language switchers / hreflang.
type Translation struct {
	Lang      string
	Locale    string
	Title     string
	URL       string
	Canonical string
	IsCurrent bool
	IsDefault bool
}

// computeTranslations groups pages/posts that share a slug across languages so
// templates can render a language switcher and hreflang alternates (PLAT-005).
func (g *Generator) computeTranslations() {
	if len(g.config.Languages) == 0 {
		return
	}
	g.translations = make(map[string][]Translation)
	add := func(pages []models.Page) {
		for i := range pages {
			key := strings.ToLower(pages[i].Slug)
			if g.config.I18n.Enabled {
				key = pages[i].TranslationKey
			}
			g.translations[key] = append(g.translations[key], Translation{
				Lang:      pages[i].Lang,
				Locale:    pages[i].Locale,
				Title:     pages[i].Title,
				URL:       pages[i].GetURL(),
				Canonical: pages[i].GetCanonical(g.config.Domain),
				IsDefault: pages[i].Lang == g.config.DefaultLanguage || pages[i].Lang == "",
			})
		}
	}
	add(g.siteData.Pages)
	add(g.siteData.Posts)
	if g.config.I18n.Enabled {
		attach := func(pages []models.Page) {
			for i := range pages {
				for _, tr := range g.translationsFor(pages[i]) {
					pages[i].Translations = append(pages[i].Translations, models.TranslationLink{Lang: tr.Lang, Locale: tr.Locale, Title: tr.Title, URL: tr.URL, Canonical: tr.Canonical, IsCurrent: tr.Lang == pages[i].Lang})
				}
			}
		}
		attach(g.siteData.Pages)
		attach(g.siteData.Posts)
	}
}

// computeSeriesLinks fills SeriesPrev/Next for every post that belongs to a series
// (AX-005). Posts within a series are ordered by date ascending; the first has no
// previous and the last has no next.
func (g *Generator) computeSeriesLinks() {
	groups := make(map[string][]int) // series name → indices into Posts
	for i := range g.siteData.Posts {
		if s := g.siteData.Posts[i].Series; s != "" {
			groups[s] = append(groups[s], i)
		}
	}
	for _, idx := range groups {
		sort.SliceStable(idx, func(a, b int) bool {
			return g.siteData.Posts[idx[a]].Date.Before(g.siteData.Posts[idx[b]].Date)
		})
		for pos, i := range idx {
			if pos > 0 {
				prev := &g.siteData.Posts[idx[pos-1]]
				g.siteData.Posts[i].SeriesPrevURL = prev.GetURL()
				g.siteData.Posts[i].SeriesPrevTitle = prev.Title
			}
			if pos < len(idx)-1 {
				next := &g.siteData.Posts[idx[pos+1]]
				g.siteData.Posts[i].SeriesNextURL = next.GetURL()
				g.siteData.Posts[i].SeriesNextTitle = next.Title
			}
		}
	}
}

// sortPostsByDate returns a copy of posts sorted newest-first — the single sort
// used by every collection/archive renderer (BLOG-001).
func sortPostsByDate(posts []models.Page) []models.Page {
	out := append([]models.Page(nil), posts...)
	sort.SliceStable(out, func(a, b int) bool { return out[a].Date.After(out[b].Date) })
	return out
}

// renderArchive is the shared collection renderer (BLOG-001): it writes one archive
// listing to /{kind}/{slug}/index.html using primaryTmpl (falling back to
// category.html), with a context compatible with the category/series templates.
// ascending controls order (series read forward; tag/author/category newest-first).
func (g *Generator) renderArchive(kind, name, slug string, posts []models.Page, primaryTmpl string, ascending bool) error {
	slug = models.SanitizeRelPath(slug)
	if slug == "" {
		return nil
	}
	// Explicit content wins: a page/post/alias that already owns /kind/slug/
	// (e.g. a hand-written /author/ian-zane/ profile) suppresses the
	// auto-generated archive instead of being silently overwritten (GO-050).
	if owner, taken := g.archiveURLOwner(kind, slug); taken {
		fmt.Printf("   ⚠️  Skipping auto %s archive /%s/%s/: %s already owns that URL\n", kind, kind, slug, owner)
		return nil
	}
	ordered := sortPostsByDate(posts)
	if ascending {
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}
	data := struct {
		Site         *models.SiteData
		Category     models.Category
		Kind         string
		Name         string
		Series       string // back-compat for series.html
		Posts        []models.Page
		Domain       string
		Vars         map[string]interface{}
		Data         map[string]interface{}
		ExternalData map[string]interface{}
	}{
		Site:         g.siteData,
		Category:     models.Category{Name: name, Slug: slug},
		Kind:         kind,
		Name:         name,
		Series:       name,
		Posts:        ordered,
		Domain:       g.config.Domain,
		Vars:         g.config.Variables,
		Data:         g.data,
		ExternalData: g.externalData,
	}
	outputPath := filepath.Join(g.config.OutputDir, kind, slug, indexHTMLName)
	if err := g.ensureWithinOutput(outputPath); err != nil {
		fmt.Printf("   ⚠️  Skipping %s %q with unsafe slug: %v\n", kind, name, err)
		return nil
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	// A define-shell template (file present, body empty — GO-051) would render
	// a blank archive; treat it as absent so the category.html fallback applies.
	if !g.hasTemplate(primaryTmpl) {
		primaryTmpl = categoryHTMLName
	}
	if err := g.renderTemplate(primaryTmpl, outputPath, data); err != nil {
		if err := g.renderTemplate(categoryHTMLName, outputPath, data); err != nil {
			fmt.Printf("   ⚠️  Failed to generate %s %s: %v\n", kind, name, err)
		}
	}
	return nil
}

// generateSeries renders a landing page per series at /series/{slug}/ (AX-005),
// consuming the shared collection renderer.
func (g *Generator) generateSeries() error {
	groups := make(map[string][]models.Page)
	for _, post := range g.siteData.Posts {
		if post.Series != "" {
			groups[post.Series] = append(groups[post.Series], post)
		}
	}
	for _, name := range sortedKeys(groups) {
		if err := g.renderArchive("series", name, slugify(name), groups[name], seriesHTMLName, true); err != nil {
			return err
		}
	}
	return nil
}

// generateTags renders a listing per tag at /tag/{slug}/ using tag.html (fallback
// category.html), and returns the tag→slug map for the sitemap (BLOG-004).
func (g *Generator) generateTags() (map[string]string, error) {
	groups := make(map[string][]models.Page)
	for _, post := range g.siteData.Posts {
		for _, tag := range post.Tags {
			groups[tag] = append(groups[tag], post)
		}
	}
	slugs := make(map[string]string, len(groups))
	for _, name := range sortedKeys(groups) {
		// Canonical export slugs apply ONLY to tags resolved from numeric ids
		// (issue #27); hand-written names keep their historical derived slugs,
		// so pre-1.8.6 tag URLs never change.
		slug := g.siteData.TagSlugs[strings.ToLower(name)]
		if slug == "" {
			slug = slugify(name)
		}
		if owner, taken := g.archiveURLOwner("tag", slug); taken {
			// Suppressed archives stay out of the sitemap/feed slug map (GO-050).
			fmt.Printf("   ⚠️  Skipping auto tag archive /tag/%s/: %s already owns that URL\n", slug, owner)
			continue
		}
		slugs[name] = slug
		if err := g.renderArchive("tag", name, slug, groups[name], tagHTMLName, false); err != nil {
			return nil, err
		}
	}
	return slugs, nil
}

// generateAuthors renders a listing per author at /author/{slug}/ using author.html
// (fallback category.html), and returns the author→slug map for the sitemap (BLOG-005).
func (g *Generator) generateAuthors() (map[string]string, error) {
	groups := make(map[int][]models.Page)
	for _, post := range g.siteData.Posts {
		if post.Author != 0 {
			groups[post.Author] = append(groups[post.Author], post)
		}
	}
	slugs := make(map[string]string)
	ids := make([]int, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		name, slug := g.authorNameSlug(id)
		if slug == "" {
			continue
		}
		if owner, taken := g.archiveURLOwner("author", slug); taken {
			// Suppressed archives stay out of the sitemap slug map (GO-050).
			fmt.Printf("   ⚠️  Skipping auto author archive /author/%s/: %s already owns that URL\n", slug, owner)
			continue
		}
		slugs[slug] = slug
		if err := g.renderArchive("author", name, slug, groups[id], authorHTMLName, false); err != nil {
			return nil, err
		}
	}
	return slugs, nil
}

// authorNameSlug resolves an author ID to a display name and URL slug (BLOG-005).
func (g *Generator) authorNameSlug(id int) (name, slug string) {
	if a, ok := g.siteData.Authors[id]; ok {
		name = a.Name
		if a.Slug != "" {
			return name, slugify(a.Slug)
		}
		return name, slugify(a.Name)
	}
	return fmt.Sprintf("author-%d", id), fmt.Sprintf("author-%d", id)
}

// sortedKeys returns the map keys sorted for deterministic output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// slugify converts an arbitrary label into a URL-safe slug (lowercase, spaces and
// punctuation → hyphens), used for series/tag names (AX-005).
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// mathDelimiterRe detects display math ($$…$$) or fenced ```math blocks (AX-004).
var mathDelimiterRe = regexp.MustCompile("(?s)\\$\\$.+?\\$\\$|```math")

// containsMath reports whether content carries math that KaTeX should render.
func containsMath(content string) bool {
	return mathDelimiterRe.MatchString(content)
}

// expandPermalink expands a permalink pattern into a sanitized relative path
// using the tokens :year :month :day :slug :category (SEO-001). Empty date
// segments collapse cleanly; the result is always confined to the output root.
func (g *Generator) expandPermalink(pattern string, p models.Page) string {
	// Date tokens honour the configured timezone so URLs match the site's
	// local calendar, not the build host's or UTC (I18N-001).
	date := g.pageDate(p, p.Date)
	repl := strings.NewReplacer(
		":year", fmt.Sprintf("%04d", date.Year()),
		":month", fmt.Sprintf("%02d", int(date.Month())),
		":day", fmt.Sprintf("%02d", date.Day()),
		":slug", p.Slug,
		":category", g.permalinkCategorySlug(p),
	)
	return models.SanitizeRelPath(repl.Replace(pattern))
}

// permalinkCategorySlug resolves the :category token: the frontmatter category
// string wins, else the first resolved category's slug, else "uncategorized".
func (g *Generator) permalinkCategorySlug(p models.Page) string {
	if p.Category != "" {
		return p.Category
	}
	for _, id := range p.Categories {
		if cat, ok := g.siteData.Categories[id]; ok && cat.Slug != "" {
			return cat.Slug
		}
	}
	return "uncategorized"
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
	// A site may consist of content_sources alone: with no primary source there
	// is no source tree to read, and metadata.json is not required (CONTENT-002).
	if g.config.Source == "" && len(g.config.ContentSources) > 0 {
		return nil
	}
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
	// A fresh client is created on every Generate(); close it so watch-mode
	// rebuilds do not leak gRPC connections/goroutines (GO-005).
	defer func() { _ = client.Close() }()

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
	// Batch size 0 → the client's configured default (PERF-010).
	catDocs, err := client.GetAll("categories", g.config.Mddb.Lang, 0)
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
	mediaDocs, err := client.GetAll("media", g.config.Mddb.Lang, 0)
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
	userDocs, err := client.GetAll("users", g.config.Mddb.Lang, 0)
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

// extractCategoryFromDoc extracts Category from an mddb Document.
// Delegates to the shared mddb extractor to avoid duplicated logic (DRY, GO-010).
func extractCategoryFromDoc(doc mddb.Document) models.Category {
	return mddb.ExtractCategory(doc)
}

// extractMediaFromDoc extracts MediaItem (including media_details) from an mddb
// Document. Delegates to the shared mddb extractor so the generator always gets
// media_details populated (GO-006) without duplicating the logic (GO-010).
func extractMediaFromDoc(doc mddb.Document) models.MediaItem {
	return mddb.ExtractMedia(doc)
}

// extractAuthorFromDoc extracts Author from an mddb Document.
// Delegates to the shared mddb extractor to avoid duplicated logic (DRY, GO-010).
func extractAuthorFromDoc(doc mddb.Document) models.Author {
	return mddb.ExtractAuthor(doc)
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

	for _, tag := range metadata.Tags {
		g.siteData.Tags[tag.ID] = tag // numeric-tag-id resolution (issue #27)
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
			// auto_excerpt fills the excerpt from the opening paragraph for
			// content that has no "## Excerpt" section (GO-057, opt-in).
			if g.config.AutoExcerpt && page.Excerpt == "" {
				page.Excerpt = parser.DeriveExcerpt(page.Content)
			}
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

	pageLinks := g.buildPageLinks()
	funcs := g.buildTemplateFuncs(pageLinks)

	// Non-Go engine (pongo2/mustache/handlebars): load the theme's own templates
	// through the selected engine instead of html/template. No Go defaults are
	// scaffolded — alt-engine themes must ship templates in that engine's syntax
	// (GO-007).
	if g.config.Engine != "" && !strings.EqualFold(g.config.Engine, engine.EngineGo) {
		return g.loadEngineTemplates(templatePath, funcs)
	}

	if err := g.ensureTemplates(templatePath); err != nil {
		return err
	}

	tmpl, err := template.New("").Funcs(funcs).ParseGlob(filepath.Join(templatePath, htmlGlobPattern))
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}
	warnShellTemplates(tmpl, g.config.Quiet)

	// Also load templates from the layouts/ and partials/ subdirectories when
	// they exist. partials/ is part of the documented theme structure and holds
	// the {{define}} blocks a theme shares between roles (header, footer, head);
	// it was listed in docs/TEMPLATES.md but never parsed, so those defines were
	// silently unavailable (DOC-014).
	for _, sub := range []string{"layouts", "partials"} {
		subPath := filepath.Join(templatePath, sub, htmlGlobPattern)
		files, _ := filepath.Glob(subPath)
		if len(files) == 0 {
			continue
		}
		if tmpl, err = tmpl.ParseGlob(subPath); err != nil {
			return fmt.Errorf("parsing %s templates: %w", sub, err)
		}
	}

	g.tmpl = tmpl
	return nil
}

// loadEngineTemplates parses every theme template (root + layouts/) through the
// configured non-Go engine, keyed by base filename (GO-007).
func (g *Generator) loadEngineTemplates(templatePath string, funcs template.FuncMap) error {
	eng, err := engine.New(g.config.Engine)
	if err != nil {
		return err
	}
	g.engine = eng
	g.engineTmpls = make(map[string]engine.Template)

	patterns := []string{
		filepath.Join(templatePath, htmlGlobPattern),
		filepath.Join(templatePath, "layouts", htmlGlobPattern),
	}
	loaded := 0
	for _, pat := range patterns {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			t, perr := eng.ParseFile(f, funcs)
			if perr != nil {
				return fmt.Errorf("parsing %s template %s: %w", eng.Name(), filepath.Base(f), perr)
			}
			g.engineTmpls[filepath.Base(f)] = t
			loaded++
		}
	}
	if loaded == 0 {
		return fmt.Errorf("no %s templates found in %s (alt-engine themes must ship their own templates)", eng.Name(), templatePath)
	}
	return nil
}

// renderWithEngine renders a named template via the configured non-Go engine (GO-007).
func (g *Generator) renderWithEngine(templateName, outputPath string, data interface{}, page *models.Page, isPost bool) error {
	t, ok := g.engineTmpls[templateName]
	if !ok {
		// Mirror html/template's message so existing fallback logic keeps working.
		return fmt.Errorf("no such template %q", templateName)
	}
	// Render into memory so the per-file transforms produce one write (PERF-005).
	var buf bytes.Buffer
	if err := t.Execute(&buf, g.prepAltData(data)); err != nil {
		return err
	}
	out := buf.String()
	if strings.HasSuffix(strings.ToLower(outputPath), ".html") {
		out = g.transformHTMLPage(out, page, isPost)
	}
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(outputPath, []byte(out), 0644)
}

// prepAltData adapts the Go template context for non-Go engines: template.HTML
// values become plain strings and the raw-markdown Content field is pre-rendered
// to HTML (alt engines have no safeHTML function). Non-map data passes through
// unchanged so archive structs still work via reflection (GO-007).
func (g *Generator) prepAltData(data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if hv, isHTML := v.(template.HTML); isHTML {
			if k == "Content" {
				// Sanitize like the Go-engine path does, so --sanitize-html
				// holds for pongo2/mustache/handlebars too (SEC-014).
				out[k] = g.sanitizeHTML(g.convertMarkdownToHTML(string(hv)))
			} else {
				out[k] = string(hv)
			}
			continue
		}
		// With the sanitizer on, Content is passed as a plain string (SEC-014);
		// alt engines still need it pre-rendered to HTML.
		if k == "Content" {
			if sv, isStr := v.(string); isStr {
				out[k] = g.sanitizeHTML(g.convertMarkdownToHTML(sv))
				continue
			}
		}
		out[k] = v
	}
	return out
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
// buildMdLinkMap indexes every recognizable .md link key to its per-language
// URLs. Language-aware (i18n §13): translated variants of the same filename or
// slug no longer overwrite each other — the renderer picks the right language
// at rewrite time. First writer wins per (key, language), so the map is
// deterministic across builds (pages, then posts, in load order).
func (g *Generator) buildMdLinkMap() map[string]map[string]string {
	mdLinks := make(map[string]map[string]string)
	add := func(key, lang, url string) {
		if key == "" {
			return
		}
		byLang := mdLinks[key]
		if byLang == nil {
			byLang = make(map[string]string)
			mdLinks[key] = byLang
		}
		if _, exists := byLang[lang]; !exists {
			byLang[lang] = url
		}
	}
	allPages := append(g.siteData.Pages, g.siteData.Posts...)
	for _, p := range allPages {
		url := p.GetURL()
		lang := p.Lang // "" in single-language builds

		addAll := func(key string) {
			add(key, lang, url)
			// Enrich the key with the whole translation group, so a link written
			// against any variant's filename/slug resolves in every language (§13).
			for _, tr := range p.Translations {
				add(key, tr.Lang, tr.URL)
			}
		}

		// 1. Actual source filename — highest priority (e.g. "AUTHENTICATION.md")
		if p.SourceFile != "" {
			addAll(p.SourceFile)
			addAll(strings.ToLower(p.SourceFile))
		}

		// 2. Slug-derived variants — fallback when SourceFile not available (e.g. mddb source)
		addAll(p.Slug + ".md")
		addAll(strings.ToUpper(p.Slug) + ".md")
		addAll(p.Slug)
	}
	return mdLinks
}

// resolveMdLink picks the URL for one link key given the rendering language
// (i18n §13): the requested-language translation wins; otherwise the
// content-fallback chain (fallback_languages → default language) applies only
// when content_fallback is enabled. Returns ok=false when unresolvable.
func (g *Generator) resolveMdLink(byLang map[string]string, lang string) (string, bool) {
	if len(byLang) == 0 {
		return "", false
	}
	if url, ok := byLang[lang]; ok {
		return url, true
	}
	if g.config.I18n.ContentFallback {
		chain := append([]string{}, g.config.I18n.FallbackLanguages[lang]...)
		chain = append(chain, g.config.DefaultLanguage)
		for _, candidate := range chain {
			if url, ok := byLang[candidate]; ok {
				return url, true
			}
		}
	}
	return "", false
}

// mdLinkSuffixRe captures a language suffix in a link filename (about.en.md).
var mdLinkSuffixRe = regexp.MustCompile(`\.([a-z]{2}(?:-[A-Z]{2})?)\.md$`)

// explicitMdLinkLang detects an explicit language choice embedded in the link
// itself (e.g. "about.en.md" from a Polish page): such links keep the author's
// language instead of the rendering language (§13 "preserve explicit links").
func (g *Generator) explicitMdLinkLang(base, fallback string) string {
	m := mdLinkSuffixRe.FindStringSubmatch(base)
	if m == nil {
		return fallback
	}
	if _, ok := ssgi18n.Language(g.siteData.Languages, m[1]); ok {
		return m[1]
	}
	return fallback
}

// rewriteMdLinks replaces relative .md hrefs in HTML with final output URLs.
// Handles: href="file.md", href="./file.md", href="../dir/file.md", each with
// an optional "#anchor" or "?query" suffix which is carried over to the
// rewritten URL — a deep link between two documents used not to match at all
// and silently shipped as a dead .md href (GO-056). Rendering language comes
// from g.currentLang (set before each page render); unresolved multilingual
// targets warn once per (link, language) and are left as-is.
var mdLinkRe = regexp.MustCompile(`href="([^"#?]*\.md)([#?][^"]*)?"`)

func (g *Generator) rewriteMdLinks(html string, mdLinkMap map[string]map[string]string) string {
	return mdLinkRe.ReplaceAllStringFunc(html, func(match string) string {
		parts := mdLinkRe.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		// parts[1] is the .md path, parts[2] the "#anchor"/"?query" tail (may be "").
		base := filepath.Base(parts[1])
		suffix := parts[2]
		lang := g.explicitMdLinkLang(base, g.currentLang)
		if url, ok := g.resolveMdLink(mdLinkMap[base], lang); ok {
			return `href="` + url + suffix + `"`
		}
		// Try without .md extension
		noExt := strings.TrimSuffix(base, ".md")
		if url, ok := g.resolveMdLink(mdLinkMap[noExt], lang); ok {
			return `href="` + url + suffix + `"`
		}
		g.warnMdLink(base, lang, mdLinkMap)
		return match // no match — leave as-is
	})
}

// warnMdLink reports a .md link whose target translation is missing (i18n §13),
// once per (link, language) to avoid flooding the build log.
func (g *Generator) warnMdLink(base, lang string, mdLinkMap map[string]map[string]string) {
	if !g.config.I18n.Enabled || len(mdLinkMap[base]) == 0 {
		return // plain unknown link: keep the historical silent pass-through
	}
	key := base + "\x00" + lang
	if g.mdLinkWarned == nil {
		g.mdLinkWarned = map[string]bool{}
	}
	if g.mdLinkWarned[key] {
		return
	}
	g.mdLinkWarned[key] = true
	fmt.Printf("   ⚠️  link %q has no %q translation (enable i18n.content_fallback or add the translation)\n", base, lang)
}

// buildTemplateFuncs creates the template function map
func (g *Generator) buildTemplateFuncs(pageLinks map[string]string) template.FuncMap {
	mdLinkMap := g.buildMdLinkMap()
	funcs := template.FuncMap{
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
		"add":                  tmplAdd, // arithmetic for themes (TPL-003)
		"sub":                  tmplSub,
		"mul":                  tmplMul,
		"div":                  tmplDiv,
		"t":                    g.translationValue,
		"hasTranslation":       func(lang string, page any) bool { return g.translationURL(lang, page) != "" },
		"translationURL":       g.translationURL,
		"languageURL":          g.languageURL,
		"localizeDate":         g.localizeDate,

		// Collection helpers (v1.8.3): the collection is the FINAL argument so
		// helpers chain in pipelines — see docs/TEMPLATE_HELPERS.md.
		"where":   tmplWhere,
		"filter":  tmplFilter,
		"sort":    tmplSortBy,
		"first":   tmplFirst,
		"last":    tmplLast,
		"limit":   tmplLimit,
		"offset":  tmplOffset,
		"groupBy": tmplGroupBy,
		"uniq":    tmplUniq,
		"uniqBy":  tmplUniqBy,
		"reverse": tmplReverse,
		"slice":   tmplSliceOf, // NOTE: overrides Go's builtin slice(str,i,j)
		"pluck":   tmplPluck,
		"indexBy": tmplIndexBy,

		// Conditional helpers (v1.8.3).
		"in":         tmplIn,
		"notIn":      tmplNotIn,
		"contains":   tmplContains,
		"startsWith": strings.HasPrefix,
		"endsWith":   strings.HasSuffix,
		"hasPrefix":  strings.HasPrefix, // Hugo-compatible aliases (v1.8.5)
		"hasSuffix":  strings.HasSuffix,
		"matches":    tmplMatches,
		"isNil":      tmplIsNil,
		"isEmpty":    tmplIsEmpty,
		"ternary":    tmplTernary,

		// Content helpers (v1.8.3): wrappers over the generic ones.
		"latest":     tmplLatest,
		"published":  tmplPublished,
		"byTag":      tmplByTag,
		"byCategory": g.tmplByCategory,
		"byAuthor":   g.tmplByAuthor,
		"related":    tmplRelated,
	}
	// Image-processing helpers (imageInfo/Resize/Crop/Process/Filter/SrcSet).
	for name, fn := range g.imageFuncs() {
		funcs[name] = fn
	}
	// Taxonomy helpers (taxonomies/taxonomy/taxonomyTerms/pageTerms/termURL/
	// hasTerm/pagesByTerm) — taxonomies-feature.md.
	for name, fn := range g.taxonomyFuncs() {
		funcs[name] = fn
	}
	// External-source helpers (getExternal/getExternalMeta).
	for name, fn := range g.externalFuncs() {
		funcs[name] = fn
	}
	return funcs
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
func (g *Generator) tmplSafeHTML(pageLinks map[string]string, mdLinkMap map[string]map[string]string) func(string) template.HTML {
	return func(s string) template.HTML {
		// Shortcode output comes from author-controlled templates, not untrusted
		// content: swap it for placeholder tokens so it survives the sanitizer
		// while raw HTML in the content itself does not (SEC-014/GO-037).
		var protected []string
		protect := func(html string) string {
			if g.sanitizer == nil || html == "" {
				return html
			}
			protected = append(protected, html)
			return fmt.Sprintf("ssg-protected-%d-token", len(protected)-1)
		}
		s = g.processShortcodesWith(s, func(sc Shortcode) string { return protect(g.renderShortcode(sc)) })
		if g.sanitizer != nil {
			s = processWPShortcodesWith(s, protect) // [youtube]/[embed] iframes (GO-037)
		}
		s = cleanMarkdownArtifacts(s)
		s = autolinkListItems(s, pageLinks)
		s = g.replaceTOCMarker(s)
		s = g.convertMarkdownToHTML(s)
		s = fixMediaPaths(s, g.siteData.Media)
		if g.config.RewriteMdLinks {
			s = g.rewriteMdLinks(s, mdLinkMap)
		}
		s = g.applyLinkRewrites(s)
		if g.sanitizer != nil {
			s = g.sanitizer.Sanitize(s) // FE-005 / SEC-003: strip XSS from untrusted content
			for i, html := range protected {
				s = strings.Replace(s, fmt.Sprintf("ssg-protected-%d-token", i), html, 1)
			}
		}
		return template.HTML(s) // #nosec G203 -- rendered markdown (optionally sanitized, FE-005)
	}
}

// sanitizeHTML applies the configured HTML sanitizer when enabled (SEC-014).
func (g *Generator) sanitizeHTML(s string) string {
	if g.sanitizer == nil {
		return s
	}
	return g.sanitizer.Sanitize(s)
}

// Static content-cleanup patterns, compiled once (PERF-006).
var (
	mdStarLineRe = regexp.MustCompile(`(?m)^\s*\*\*\s*$`)
	mdBoldRe     = regexp.MustCompile(`\*\*(.*?)\*\*`)
)

// cleanMarkdownArtifacts removes markdown artifacts and fixes bolding
func cleanMarkdownArtifacts(s string) string {
	s = mdStarLineRe.ReplaceAllString(s, "")
	s = mdBoldRe.ReplaceAllString(s, "<strong>$1</strong>")
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

// convertMarkdownToHTML converts markdown content to HTML using the generator's
// configured renderer (footnotes/highlighting/heading-IDs per config).
func (g *Generator) convertMarkdownToHTML(s string) string {
	// Fenced ```math blocks become $$ display math before conversion, so the
	// KaTeX injection gate and browser auto-render both see them (GO-055).
	if g.config.Math {
		s = mathFencesToDisplay(s)
	}
	// Fenced ```mermaid blocks become <pre class="mermaid"> so goldmark passes
	// the diagram source through verbatim and the runtime renders it (GO-073).
	if g.config.Mermaid {
		s = mermaidFencesToHTML(s)
	}
	// Memoized per exact source: feeds, search index, JSON output and both
	// page-format paths reuse one conversion instead of 6–8 (PERF-004).
	if g.mdCache != nil {
		if html, ok := g.mdCache[s]; ok {
			return html
		}
	}
	md := g.md
	if md == nil {
		md = buildMarkdown(g.config)
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(s), &buf); err != nil {
		fmt.Printf("   ⚠️  Warning: markdown conversion failed: %v\n", err)
		return s
	}
	out := buf.String()
	if g.mdCache != nil {
		g.mdCache[s] = out
		g.mdConversions++
	}
	return out
}

// tocHTML builds a table of contents from the headings in markdown source, using
// the same auto-generated anchor IDs goldmark emits, up to toc_depth (AX-002).
// Returns a flat <ul> with per-level classes (toc-h1..toc-h6) for CSS indentation.
func (g *Generator) tocHTML(mdSource string) template.HTML {
	maxDepth := g.config.TOCDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	src := []byte(mdSource)
	doc := g.md.Parser().Parse(text.NewReader(src))

	var b strings.Builder
	count := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level > maxDepth {
			return ast.WalkContinue, nil
		}
		id := ""
		if v, ok := h.AttributeString("id"); ok {
			if idBytes, ok := v.([]byte); ok {
				id = string(idBytes)
			}
		}
		fmt.Fprintf(&b, `<li class="toc-h%d"><a href="#%s">%s</a></li>`,
			h.Level, stdhtml.EscapeString(id), stdhtml.EscapeString(nodeText(h, src))) // #nosec G104
		count++
		return ast.WalkContinue, nil
	})
	if count == 0 {
		return ""
	}
	return template.HTML(`<ul class="toc">` + b.String() + `</ul>`) // #nosec G203 -- id/text escaped above
}

// nodeText concatenates the plain text of an AST node's inline children, using the
// non-deprecated Text.Segment API (AX-002).
func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		} else {
			b.WriteString(nodeText(c, src))
		}
	}
	return b.String()
}

// tocContext returns the .TOC value: populated only when toc is enabled (AX-002).
func (g *Generator) tocContext(mdSource string) template.HTML {
	if !g.config.TOC {
		return ""
	}
	return g.tocHTML(mdSource)
}

// tocMarkerRe matches the [toc] placeholder that expands to a table of contents.
var tocMarkerRe = regexp.MustCompile(`(?i)\[toc\]`)

// replaceTOCMarker replaces a [toc] marker in content with the generated TOC.
// The marker is explicit author intent, so it works regardless of the toc config.
func (g *Generator) replaceTOCMarker(s string) string {
	if !tocMarkerRe.MatchString(s) {
		return s
	}
	toc := string(g.tocHTML(tocMarkerRe.ReplaceAllString(s, "")))
	// Surround the injected HTML block with blank lines so goldmark treats it as a
	// standalone block and keeps parsing the markdown that follows (otherwise the
	// rest of the document is swallowed into one raw-HTML block).
	return tocMarkerRe.ReplaceAllString(s, "\n\n"+toc+"\n\n")
}

// shortcodeNameRe matches {{shortcode_name}} markers (PERF-006).
var shortcodeNameRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// processShortcodesWith replaces {{shortcode_name}} with HTML from a pluggable
// renderer, so the sanitizing pipeline can wrap shortcode output in protected
// tokens (SEC-014). When ShortcodeBrackets is enabled, also replaces
// [shortcode_name] for defined shortcodes only.
func (g *Generator) processShortcodesWith(content string, render func(Shortcode) string) string {
	// Match {{shortcode_name}} pattern
	content = shortcodeNameRe.ReplaceAllStringFunc(content, func(match string) string {
		name := match[2 : len(match)-2]
		sc, ok := g.shortcodeMap[name]
		if !ok {
			return "" // Remove undefined shortcodes
		}
		sc.Raw = match
		return render(sc)
	})

	// Match bracket shortcodes (only defined shortcodes, opt-in)
	if g.config.ShortcodeBrackets && len(g.shortcodeMap) > 0 {
		content = g.processBracketShortcodesWith(content, render)
	}

	return content
}

// processBracketShortcodesWith handles WordPress-style bracket shortcodes:
//   - [name] — simple self-closing
//   - [name attr="val" attr2="val2"] — with attributes
//   - [name]inner content[/name] — with inner content
//   - [name attr="val"]inner content[/name] — with both
//
// Regexes are precompiled per shortcode in New() (PERF-006).
func (g *Generator) processBracketShortcodesWith(content string, render func(Shortcode) string) string {
	// Process each defined shortcode by name (avoids backreference limitation in Go regexp)
	for name, baseSc := range g.shortcodeMap {
		res, ok := g.bracketRes[name]
		if !ok {
			// Generators built as struct literals (tests) miss New()'s precompile.
			res = compileBracketRes(name)
			if g.bracketRes == nil {
				g.bracketRes = make(map[string]bracketShortcodeRes)
			}
			g.bracketRes[name] = res
		}
		// First: closing-tag with optional attrs [name ...]...[/name]
		content = res.closing.ReplaceAllStringFunc(content, func(match string) string {
			parts := res.closing.FindStringSubmatch(match)
			if len(parts) < 3 {
				return match
			}
			sc := g.shortcodeWithOverrides(baseSc, parts[1], parts[2])
			sc.Raw = match
			return render(sc)
		})

		// Second: self-closing with attrs [name attr="val"]
		content = res.selfAttrs.ReplaceAllStringFunc(content, func(match string) string {
			parts := res.selfAttrs.FindStringSubmatch(match)
			if len(parts) < 2 {
				return match
			}
			sc := g.shortcodeWithOverrides(baseSc, parts[1], "")
			sc.Raw = match
			return render(sc)
		})

		// Third: simple [name]
		content = res.simple.ReplaceAllStringFunc(content, func(match string) string {
			sc := baseSc
			sc.Raw = match
			return render(sc)
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

// shortcodeAttrRe matches key="value" attribute pairs (PERF-006).
var shortcodeAttrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// parseShortcodeAttrs extracts key="value" pairs from an attribute string
func parseShortcodeAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range shortcodeAttrRe.FindAllStringSubmatch(s, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// renderShortcode renders a single shortcode to HTML using its template file.
// Parsed templates are cached for the build, so a shortcode used on every page
// costs one disk read and one parse instead of thousands (PERF-002).
func (g *Generator) renderShortcode(sc Shortcode) string {
	if sc.Template == "" {
		return g.shortcodeFailed(sc, fmt.Sprintf("shortcode %q has no template defined", sc.Name))
	}

	templatePath := filepath.Join(g.config.TemplatesDir, g.config.Template, sc.Template)

	tmpl, cached := g.shortcodeTmpls[templatePath]
	if !cached {
		tmpl = g.parseShortcodeTemplate(templatePath)
		if g.shortcodeTmpls == nil { // struct-literal generators (tests) skip New()
			g.shortcodeTmpls = make(map[string]*template.Template)
		}
		g.shortcodeTmpls[templatePath] = tmpl // nil is cached too: warn once, not per page
	}
	if tmpl == nil {
		return g.shortcodeFailed(sc, fmt.Sprintf("shortcode %q: template %s could not be loaded", sc.Name, templatePath))
	}

	// Site variables are supplied here rather than stored on the configured
	// shortcode, so one map is shared by every invocation (issue #37).
	sc.Vars = g.config.Variables

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sc); err != nil {
		return g.shortcodeFailed(sc, fmt.Sprintf("shortcode %q: %v", sc.Name, err))
	}

	return buf.String()
}

// shortcodeFailed records a shortcode that could not be rendered and returns
// what takes its place in the page, per shortcode_errors:
//
//   - "" / "drop" (default, unchanged since before issue #37): nothing, so a
//     site that already tolerates a failing shortcode keeps its exact output.
//   - "keep" / "strict": the shortcode's raw source, which keeps the failure
//     visible — a page that quietly lost its payment widget looks fine, one
//     showing `[stripe_form]` does not — and survives HTML minification, which
//     an HTML comment would not.
func (g *Generator) shortcodeFailed(sc Shortcode, msg string) string {
	fmt.Printf("   ⚠️  Warning: %s\n", msg)
	g.shortcodeFailures = append(g.shortcodeFailures, msg)
	switch g.config.ShortcodeErrors {
	case "keep", "strict":
		return sc.Raw
	default:
		return ""
	}
}

// shortcodeErrorCheck fails the build when shortcode_errors is "strict" and any
// shortcode failed to render. Rendering is sequential, so the slice needs no
// lock; messages are already deduplicated per template by the parse cache.
func (g *Generator) shortcodeErrorCheck() error {
	if g.config.ShortcodeErrors != "strict" || len(g.shortcodeFailures) == 0 {
		return nil
	}
	return fmt.Errorf("shortcode_errors: strict — %d shortcode(s) failed to render:\n   - %s",
		len(g.shortcodeFailures), strings.Join(g.shortcodeFailures, "\n   - "))
}

// parseShortcodeTemplate loads one shortcode template from disk; nil on failure.
func (g *Generator) parseShortcodeTemplate(templatePath string) *template.Template {
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		fmt.Printf("   ⚠️  Warning: shortcode template not found: %s\n", templatePath)
		return nil
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(g.shortcodeFuncMap()).ParseFiles(templatePath)
	if err != nil {
		fmt.Printf("   ⚠️  Warning: shortcode template parse error: %v\n", err)
		return nil
	}
	return tmpl
}

// shortcodeFuncMap returns template functions available in shortcode templates
func (g *Generator) shortcodeFuncMap() template.FuncMap {
	funcs := template.FuncMap{
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
		"add":             tmplAdd,
		"sub":             tmplSub,
		"mul":             tmplMul,
		"div":             tmplDiv,

		// Safe, deterministic conditional helpers (v1.8.3). Collection helpers
		// that depend on site-wide data stay normal-template-only by design.
		"slice":      tmplSliceOf,
		"in":         tmplIn,
		"notIn":      tmplNotIn,
		"contains":   tmplContains,
		"startsWith": strings.HasPrefix,
		"endsWith":   strings.HasSuffix,
		"hasPrefix":  strings.HasPrefix, // Hugo-compatible aliases (v1.8.5)
		"hasSuffix":  strings.HasSuffix,
		"matches":    tmplMatches,
		"isNil":      tmplIsNil,
		"isEmpty":    tmplIsEmpty,
		"ternary":    tmplTernary,
	}
	// Image-processing helpers — shortcodes are a primary use case for them.
	for name, fn := range g.imageFuncs() {
		funcs[name] = fn
	}
	// External-source helpers (read-only), so `getExternal` really does work
	// in every context as EXTERNAL_SOURCES.md promises (DOC-016).
	for name, fn := range g.externalFuncs() {
		funcs[name] = fn
	}
	return funcs
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

// Template helper patterns, compiled once (PERF-006).
var (
	stripYoutubeRe = regexp.MustCompile(`\[youtube\][^\[]*\[/youtube\]`)
	stripEmbedRe   = regexp.MustCompile(`\[embed\][^\[]*\[/embed\]`)
	stripHTMLRe    = regexp.MustCompile(`<[^>]*>`)
)

func tmplThumbnailFromYoutube(s string) string {
	matches := wpVideoShortcodeRes[0].FindStringSubmatch(s)
	if len(matches) >= 2 {
		return fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", matches[1])
	}
	return ""
}

func tmplStripShortcodes(s string) string {
	s = stripYoutubeRe.ReplaceAllString(s, "")
	s = stripEmbedRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func tmplStripHTML(s string) string {
	return strings.TrimSpace(stripHTMLRe.ReplaceAllString(s, ""))
}

func (g *Generator) tmplRecentPosts(n int) []models.Page {
	// GO-008: clamp both ends so a negative n (e.g. {{recentPosts -1}}) cannot
	// panic with a slice-bounds-out-of-range.
	if n < 0 {
		n = 0
	}
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

	// Bundled starter theme first (DOC-013): extract the full embedded tree
	// (HTML + assets) so the CLI honours the README promise for simple/krowy.
	if ok, err := scaffoldEmbeddedTheme(g.config.Template, templatePath); err != nil {
		return err
	} else if ok {
		fmt.Printf("   📦 Extracted embedded theme %q into %s\n", g.config.Template, templatePath)
		return nil
	}

	// Create default templates
	templates := map[string]string{
		"base.html":      baseTemplate,
		indexHTMLName:    indexTemplate,
		pageHTMLName:     pageTemplate,
		postHTMLName:     postTemplate,
		categoryHTMLName: categoryTemplate,
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

	// Generate series landing pages (AX-005)
	if err := g.generateSeries(); err != nil {
		return fmt.Errorf("generating series: %w", err)
	}

	// Generate tag archives (BLOG-004)
	tagSlugs, err := g.generateTags()
	if err != nil {
		return fmt.Errorf("generating tags: %w", err)
	}
	g.tagSlugs = tagSlugs

	// Generate author archives (BLOG-005)
	authorSlugs, err := g.generateAuthors()
	if err != nil {
		return fmt.Errorf("generating authors: %w", err)
	}
	g.authorSlugs = authorSlugs

	// Generate custom taxonomy archives (taxonomies-feature.md)
	if err := g.generateTaxonomies(); err != nil {
		return fmt.Errorf("generating taxonomies: %w", err)
	}

	return nil
}

// Pager carries pagination state to the index template (BLOG-003). Zero value
// (Total 1) represents an un-paginated single index page.
type Pager struct {
	Current int    // 1-based current page
	Total   int    // total number of pages
	PerPage int    // posts per page
	PrevURL string // "" on the first page
	NextURL string // "" on the last page
}

// generateIndex generates the main index.html, paginated into /page/N/ when
// paginate > 0 (BLOG-003). With paginate == 0 the behaviour is unchanged: a single
// index page listing every post.
func (g *Generator) generateIndex() error {
	if g.config.I18n.Enabled {
		for _, lang := range g.siteData.Languages {
			g.currentLang = lang.Code
			g.siteData.Language = lang
			g.siteData.LanguagePages = languagePages(g.siteData.Pages, lang.Code)
			g.siteData.LanguagePosts = languagePages(g.siteData.Posts, lang.Code)
			prefix := ssgi18n.Prefix(lang.Code, g.config.DefaultLanguage, g.config.I18n)
			if err := g.generateLanguageIndex(g.siteData.LanguagePosts, prefix); err != nil {
				return err
			}
		}
		return nil
	}
	return g.generateLanguageIndex(g.siteData.Posts, "")
}

func (g *Generator) generateLanguageIndex(posts []models.Page, prefix string) error {
	per := g.config.Paginate
	root := filepath.Join(g.config.OutputDir, prefix)
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}

	if per <= 0 || len(posts) <= per {
		return g.renderIndexPage(posts, Pager{Current: 1, Total: 1, PerPage: per},
			filepath.Join(root, indexHTMLName))
	}

	total := (len(posts) + per - 1) / per
	for page := 1; page <= total; page++ {
		start := (page - 1) * per
		end := start + per
		if end > len(posts) {
			end = len(posts)
		}
		pager := Pager{Current: page, Total: total, PerPage: per}
		if page > 1 {
			pager.PrevURL = pageURLWithPrefix(prefix, page-1)
		}
		if page < total {
			pager.NextURL = pageURLWithPrefix(prefix, page+1)
		}

		outPath := filepath.Join(root, indexHTMLName)
		if page > 1 {
			outPath = filepath.Join(root, "page", fmt.Sprintf("%d", page), indexHTMLName)
			// #nosec G301 -- Web content directories need to be world-traversable
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return err
			}
		}
		if err := g.renderIndexPage(posts[start:end], pager, outPath); err != nil {
			return err
		}
	}
	return nil
}

// pageURLWithPrefix returns the URL for paginated index page n under prefix
// (page 1 is the section root; an empty prefix means the site root).
func pageURLWithPrefix(prefix string, n int) string {
	root := "/"
	if prefix != "" {
		root += strings.Trim(prefix, "/") + "/"
	}
	if n <= 1 {
		return root
	}
	return fmt.Sprintf("%spage/%d/", root, n)
}

// renderIndexPage renders one index page with the given posts and pager.
func (g *Generator) renderIndexPage(posts []models.Page, pager Pager, outPath string) error {
	pages := g.siteData.Pages
	if g.config.I18n.Enabled {
		pages = g.siteData.LanguagePages
	}
	data := struct {
		Site             *models.SiteData
		Posts            []models.Page
		Pages            []models.Page
		Domain           string
		Vars             map[string]interface{}
		Data             map[string]interface{}
		ExternalData     map[string]interface{}
		ExternalDataMeta map[string]externalsource.Metadata
		Pager            Pager
	}{
		Site:             g.siteData,
		Posts:            posts,
		Pages:            pages,
		Domain:           g.config.Domain,
		Vars:             g.config.Variables,
		Data:             g.data,
		ExternalData:     g.externalData,
		ExternalDataMeta: g.externalMeta,
		Pager:            pager,
	}
	return g.renderTemplate(indexHTMLName, outPath, data)
}

// getOutputPaths returns one or more output file paths based on PageFormat config.
// "directory" (default): slug/index.html
// "flat": slug.html
// "both": slug/index.html AND slug.html
// Special case: "404" always generates 404.html in root for Cloudflare Pages/Netlify compatibility
// ensureWithinOutput verifies that the resolved output path stays within the
// configured OutputDir. Defense-in-depth against path traversal from untrusted
// slug/link values (e.g. from a remote mddb server) — complements the
// sanitization in models.Page.GetOutputPath (SEC-001).
func (g *Generator) ensureWithinOutput(p string) error {
	root := filepath.Clean(g.config.OutputDir)
	clean := filepath.Clean(p)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return fmt.Errorf("output path %q escapes output directory %q", p, root)
	}
	return nil
}

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
			filepath.Join(g.config.OutputDir, subPath, indexHTMLName),
			filepath.Join(g.config.OutputDir, subPath+".html"),
		}
	default: // "directory" or empty
		return []string{filepath.Join(g.config.OutputDir, subPath, indexHTMLName)}
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
		// Reject any path that escapes the output directory (SEC-001).
		if err := g.ensureWithinOutput(outputPath); err != nil {
			return err
		}
		outputDir := filepath.Dir(outputPath)
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		// Copy co-located assets only to the directory-style path (avoid duplicates)
		if page.SourceDir != "" && strings.HasSuffix(outputPath, indexHTMLName) {
			if err := g.copyColocatedAssets(page.SourceDir, outputDir, page.Content); err != nil {
				fmt.Printf("   ⚠️  Warning: couldn't copy co-located assets for page %s: %v\n", page.Slug, err)
			}
		}

		// Use custom layout/template if specified, otherwise default to page.html
		templateName := pageHTMLName
		if page.Layout != "" {
			templateName = g.layoutTemplateName(page.Layout)
		} else if page.Template != "" {
			templateName = page.Template + ".html"
		}

		// Render + per-file transforms (SEO/math/relative/prettify/minify) in a
		// single write (PERF-005).
		if err := g.renderPageTemplate(templateName, outputPath, data, &page, false); err != nil {
			// Fallback to page.html if custom template not found
			if strings.Contains(err.Error(), "no such template") || strings.Contains(err.Error(), "is undefined") {
				if err := g.renderPageTemplate(pageHTMLName, outputPath, data, &page, false); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		g.writeJSONOutput(page, outputPath)
	}

	g.writeAliasStubs(page)
	g.runPostPageHook(page)
	return nil
}

// runPostPageHook runs post_page hooks for a rendered page (PLAT-001). Failures are
// non-fatal — a page hook should not abort the whole build — and are logged.
func (g *Generator) runPostPageHook(page models.Page) {
	if len(g.config.Hooks["post_page"]) == 0 {
		return
	}
	if err := g.runHooks("post_page", map[string]string{"SSG_PAGE_PATH": page.GetOutputPath()}); err != nil {
		fmt.Printf("   ⚠️  post_page hook: %v\n", err)
	}
}

// generatePost generates a single post
func (g *Generator) generatePost(post models.Page) error {
	// Same guard as generatePage: a post whose link has no path resolves to an
	// empty output path and would silently overwrite the homepage (GO-023).
	outputSubPath := post.GetOutputPath()
	if outputSubPath == "" || outputSubPath == "." {
		fmt.Printf("   ⚠️  Skipping post '%s' (slug: %s) - would overwrite main index.html\n", post.Title, post.Slug)
		fmt.Printf("      Hint: Change the 'link' field in frontmatter or use a different slug\n")
		return nil
	}

	// Convert post to flat map with Extra fields at top level
	data := g.pageToTemplateData(post, true)

	outputPaths := g.getOutputPaths(outputSubPath)
	for _, outputPath := range outputPaths {
		// Reject any path that escapes the output directory (SEC-001).
		if err := g.ensureWithinOutput(outputPath); err != nil {
			return err
		}
		outputDir := filepath.Dir(outputPath)
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		// Copy co-located assets only to the directory-style path (avoid duplicates)
		if post.SourceDir != "" && strings.HasSuffix(outputPath, indexHTMLName) {
			if err := g.copyColocatedAssets(post.SourceDir, outputDir, post.Content); err != nil {
				fmt.Printf("   ⚠️  Warning: couldn't copy co-located assets for post %s: %v\n", post.Slug, err)
			}
		}

		// Render + per-file transforms in a single write (PERF-005).
		if err := g.renderPageTemplate(postHTMLName, outputPath, data, &post, true); err != nil {
			return err
		}
		g.writeJSONOutput(post, outputPath)
	}

	g.writeAliasStubs(post)
	g.runPostPageHook(post)
	return nil
}

// writeAliasStubs emits meta-refresh + canonical redirect stubs for each alias of
// a page (SEO-002). Alias paths are sanitized and confined to the output root
// (SEC-001); aliases are excluded from the sitemap because they are not real
// pages. An alias colliding with an already-generated page is skipped.
func (g *Generator) writeAliasStubs(page models.Page) {
	if len(page.Aliases) == 0 {
		return
	}
	target := page.GetURL()
	for _, alias := range page.Aliases {
		rel := models.SanitizeRelPath(alias)
		if g.config.I18n.Enabled && page.LangPrefix != "" {
			rel = models.SanitizeRelPath(page.LangPrefix + "/" + rel)
		}
		if rel == "" || rel == "." {
			continue
		}
		// Record the alias as a 301 for the _redirects file (GO-063).
		g.aliasRedirects = append(g.aliasRedirects, RedirectRule{From: "/" + rel, To: target, Status: 301})
		// Meta-refresh stubs are the client-side fallback for non-CF hosts;
		// alias_stubs: false drops them and keeps only the _redirects entry.
		if !g.config.AliasStubsOff {
			g.writeAliasStub(alias, rel, target)
		}
	}
}

// writeAliasStub writes one meta-refresh stub page for an alias, confined to
// the output root and skipped when it would collide with a real page (SEO-002).
func (g *Generator) writeAliasStub(alias, rel, target string) {
	outPath := filepath.Join(g.config.OutputDir, rel, indexHTMLName)
	if strings.HasSuffix(strings.ToLower(rel), ".html") {
		outPath = filepath.Join(g.config.OutputDir, rel)
	}
	if err := g.ensureWithinOutput(outPath); err != nil {
		fmt.Printf("   ⚠️  Skipping unsafe alias %q: %v\n", alias, err)
		return
	}
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("   ⚠️  Alias %q collides with an existing page; skipping\n", alias)
		return
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Printf("   ⚠️  Alias %q: %v\n", alias, err)
		return
	}
	// Alias stubs get the same per-file transforms as rendered pages (PERF-005),
	// matching the former tree-walk behaviour (minify/prettify/relative links).
	stub := g.transformHTMLPage(aliasStubHTML(target), nil, false)
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(outPath, []byte(stub), 0644); err != nil {
		fmt.Printf("   ⚠️  Alias %q: %v\n", alias, err)
	}
}

// buildOpenGraph renders OpenGraph + Twitter Card + JSON-LD markup for a page (SEO-003).
func (g *Generator) buildOpenGraph(page models.Page, isPost bool) string {
	title := page.Title
	desc := page.Description
	url := page.GetCanonical(g.config.Domain)
	ogType := "website"
	ldType := "WebSite"
	if isPost {
		ogType = "article"
		ldType = "Article"
	}
	// HTML-escape attribute values. Go's %q backslash-escapes inner quotes,
	// which HTML parsers read as end-of-attribute — an attribute-injection
	// vector via untrusted titles/descriptions (SEC-015).
	var b strings.Builder
	fmt.Fprintf(&b, `<meta property="og:title" content="%s">`+"\n", stdhtml.EscapeString(title))
	if desc != "" {
		fmt.Fprintf(&b, `<meta property="og:description" content="%s">`+"\n", stdhtml.EscapeString(desc))
	}
	fmt.Fprintf(&b, `<meta property="og:type" content="%s">`+"\n", stdhtml.EscapeString(ogType))
	fmt.Fprintf(&b, `<meta property="og:url" content="%s">`+"\n", stdhtml.EscapeString(url))
	if page.Locale != "" {
		fmt.Fprintf(&b, `<meta property="og:locale" content="%s">`+"\n", stdhtml.EscapeString(strings.ReplaceAll(page.Locale, "-", "_")))
		for _, tr := range page.Translations {
			if !tr.IsCurrent && tr.Locale != "" {
				fmt.Fprintf(&b, `<meta property="og:locale:alternate" content="%s">`+"\n", stdhtml.EscapeString(strings.ReplaceAll(tr.Locale, "-", "_")))
			}
		}
	}
	if page.FeaturedImage != "" {
		fmt.Fprintf(&b, `<meta property="og:image" content="%s">`+"\n", stdhtml.EscapeString(page.FeaturedImage))
	}
	fmt.Fprintf(&b, `<meta name="twitter:card" content="summary_large_image">`+"\n")
	fmt.Fprintf(&b, `<meta name="twitter:title" content="%s">`+"\n", stdhtml.EscapeString(title))
	if desc != "" {
		fmt.Fprintf(&b, `<meta name="twitter:description" content="%s">`+"\n", stdhtml.EscapeString(desc))
	}
	ld := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    ldType,
		"name":     title,
		"headline": title,
		"url":      url,
	}
	if page.Locale != "" {
		ld["inLanguage"] = page.Locale
	}
	if desc != "" {
		ld["description"] = desc
	}
	if isPost && !page.Date.IsZero() {
		ld["datePublished"] = page.Date.UTC().Format(time.RFC3339)
	}
	if j, err := json.Marshal(ld); err == nil {
		b.WriteString(`<script type="application/ld+json">` + string(j) + "</script>\n")
	}
	return b.String()
}

// aliasStubHTML returns a minimal redirect page (meta-refresh + canonical +
// noindex) pointing at target (SEO-002).
func aliasStubHTML(target string) string {
	esc := template.HTMLEscapeString(target)
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n" +
		"<meta http-equiv=\"refresh\" content=\"0; url=" + esc + "\">\n" +
		"<link rel=\"canonical\" href=\"" + esc + "\">\n" +
		"<meta name=\"robots\" content=\"noindex\">\n<title>Redirecting…</title>\n</head>\n" +
		"<body>Redirecting to <a href=\"" + esc + "\">" + esc + "</a>.</body>\n</html>\n"
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
			Site         *models.SiteData
			Category     models.Category
			Kind         string
			Name         string
			Posts        []models.Page
			Domain       string
			Vars         map[string]interface{}
			Data         map[string]interface{}
			ExternalData map[string]interface{}
		}{
			Site:         g.siteData,
			Category:     cat,
			Kind:         "category",
			Name:         cat.Name,
			Posts:        sortPostsByDate(posts),
			Domain:       g.config.Domain,
			Vars:         g.config.Variables,
			Data:         g.data,
			ExternalData: g.externalData,
		}

		// Sanitize the category slug so a malicious value cannot escape the
		// output directory, then verify the final path (SEC-001).
		catSlug := models.SanitizeRelPath(cat.Slug)
		// Explicit content wins over the auto category archive (GO-050).
		if owner, taken := g.archiveURLOwner("category", catSlug); taken {
			fmt.Printf("   ⚠️  Skipping auto category archive /category/%s/: %s already owns that URL\n", catSlug, owner)
			continue
		}
		outputPath := filepath.Join(g.config.OutputDir, "category", catSlug, indexHTMLName)
		if err := g.ensureWithinOutput(outputPath); err != nil {
			fmt.Printf("   ⚠️  Warning: skipping category %q with unsafe slug: %v\n", cat.Slug, err)
			continue
		}
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return err
		}

		if err := g.renderTemplate(categoryHTMLName, outputPath, data); err != nil {
			fmt.Printf("   ⚠️  Warning: failed to generate category %s: %v\n", cat.Slug, err)
		}
	}

	return nil
}

// executedByFileName lists the template names ssg executes directly by file
// name; a define-shell under one of these names is almost certainly a theme
// bug worth flagging (GO-051).
var executedByFileName = []string{
	indexHTMLName, postHTMLName, pageHTMLName, categoryHTMLName,
	tagHTMLName, seriesHTMLName, authorHTMLName,
	"taxonomy.html", "taxonomy-term.html", "archive.html",
}

// layoutTemplateName resolves frontmatter `layout: <name>` to a parsed template
// name. ParseGlob registers a template under its BASE filename, so a theme's
// layouts/blog.html is parsed as "blog.html" — looking only for
// "layouts/blog.html" matched nothing, and the page fell back to page.html
// without a word, which made the documented layout feature impossible to use
// (GO-058). Both spellings are accepted, path form first, so a theme that
// already writes {{define "layouts/blog.html"}} keeps working.
func (g *Generator) layoutTemplateName(layout string) string {
	pathForm := "layouts/" + layout + ".html"
	if g.tmpl == nil {
		return pathForm
	}
	for _, name := range []string{pathForm, layout + ".html"} {
		if g.tmpl.Lookup(name) != nil {
			return name
		}
	}
	return pathForm // unresolved: the caller still falls back to page.html
}

// warnShellTemplates flags theme files that ssg executes by file name but
// whose body is empty because the file only holds {{define}} blocks for OTHER
// names — e.g. author.html copied from category.html with the define still
// reading "category.html". Such shells silently fall back (GO-051); the
// warning tells the author how to activate the file.
func warnShellTemplates(tmpl *template.Template, quiet bool) {
	if quiet || tmpl == nil {
		return
	}
	for _, name := range executedByFileName {
		t := tmpl.Lookup(name)
		if t == nil || t.Tree == nil || t.Tree.Root == nil {
			continue
		}
		if strings.TrimSpace(t.Tree.Root.String()) == "" {
			fmt.Printf("   ⚠️  Template file %s only contains {{define}} blocks for other names — "+
				"rename a define to %q to activate it (falling back for now)\n", name, name)
		}
	}
}

// renderTemplate renders a template to a file, dispatching to the configured
// non-Go engine when one is active (GO-007).
func (g *Generator) renderTemplate(templateName, outputPath string, data interface{}) error {
	return g.renderPageTemplate(templateName, outputPath, data, nil, false)
}

// contentContextValue returns the template-context value for raw page content.
// Without the sanitizer it stays a template.HTML for backward compatibility.
// With --sanitize-html it is a plain string, so a theme printing {{.Content}}
// directly gets auto-escaped output instead of raw untrusted HTML; the safeHTML
// pipeline (which sanitizes) is the only road to rendered markup (SEC-014).
func (g *Generator) contentContextValue(content string) interface{} {
	if g.sanitizer != nil {
		return content
	}
	return template.HTML(content) // #nosec G203 -- SSG intentionally renders user's markdown as HTML
}

// pageToTemplateData converts a Page to a map for templates, flattening Extra fields to top level
// This allows templates to use {{.dupa}} instead of {{.Page.Extra.dupa}}
func (g *Generator) pageToTemplateData(page models.Page, isPost bool) map[string]interface{} {
	if g.config.I18n.Enabled {
		g.currentLang = page.Lang
		if lang, ok := ssgi18n.Language(g.siteData.Languages, page.Lang); ok {
			g.siteData.Language = lang
		}
		g.siteData.LanguagePages = languagePages(g.siteData.Pages, page.Lang)
		g.siteData.LanguagePosts = languagePages(g.siteData.Posts, page.Lang)
	}
	data := map[string]interface{}{
		"Site":             g.siteData,
		"Domain":           g.config.Domain,
		"Vars":             g.config.Variables,
		"Data":             g.data,
		"ExternalData":     g.externalData,
		"ExternalDataMeta": g.externalMeta,
		// i18n (PLAT-005)
		"Languages":       g.config.Languages,
		"DefaultLanguage": g.config.DefaultLanguage,
		"Translations":    g.translationsFor(page),
		"Hreflang":        g.hreflangTags(page),
		// Standard Page fields
		"ID":             page.ID,
		"Title":          page.Title,
		"Slug":           page.Slug,
		"Date":           g.pageDate(page, page.Date),     // rendered in the configured zone (I18N-001)
		"Modified":       g.pageDate(page, page.Modified), // rendered in the configured zone (I18N-001)
		"Status":         page.Status,
		"Type":           page.Type,
		"Link":           page.Link,
		"Author":         page.Author,
		"Categories":     page.Categories,
		"Excerpt":        page.Excerpt,
		"Content":        g.contentContextValue(page.Content),
		"URLFormat":      page.URLFormat,
		"PageFormat":     page.PageFormat,
		"SourceDir":      page.SourceDir,
		"Description":    page.Description,
		"Keywords":       page.Keywords,
		"Lang":           page.Lang,
		"Locale":         page.Locale,
		"TranslationKey": page.TranslationKey,
		"Canonical":      page.Canonical,
		"Robots":         page.Robots,
		"Sitemap":        page.Sitemap,
		"FeaturedImage":  page.FeaturedImage,
		"Tags":           page.Tags,
		"Category":       page.Category,
		"Layout":         page.Layout,
		"Template":       page.Template,
		// Computed metadata (BLOG-006 / AX-004 / AX-002)
		"WordCount":   page.WordCount,
		"ReadingTime": page.ReadingTime,
		"HasMath":     page.HasMath,
		"TOC":         g.tocContext(page.Content),
		// Series navigation (AX-005)
		"Series":          page.Series,
		"SeriesPrevURL":   page.SeriesPrevURL,
		"SeriesPrevTitle": page.SeriesPrevTitle,
		"SeriesNextURL":   page.SeriesNextURL,
		"SeriesNextTitle": page.SeriesNextTitle,
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

// copyStaticDir copies the project's static directory verbatim into the output
// directory. Every file and subdirectory (e.g. downloads/, assets/, scripts/,
// styles/, manifest.json) is copied recursively, fixing #8 where only a fixed
// subset of static assets reached the output. A missing directory is a no-op so
// the step is safe for sites that do not use one.
func (g *Generator) copyStaticDir() error {
	staticDir := g.config.StaticDir
	if staticDir == "" {
		staticDir = defaultStaticDir
	}

	info, err := os.Stat(staticDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no static/ directory — nothing to copy
		}
		return err
	}
	if !info.IsDir() {
		return nil // a file named "static" is not a passthrough directory
	}

	if err := g.copyDir(staticDir, g.config.OutputDir); err != nil {
		return err
	}

	if !g.config.Quiet {
		fmt.Printf("   📦 Copied static/ directory (%s) to output\n", staticDir)
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
func (g *Generator) copyColocatedAssets(sourceDir, outputDir, content string) error {
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
		// PERF-007: a post's SourceDir is its whole category directory, so copying
		// every asset would duplicate them into every sibling post's output dir
		// (O(posts × assets) I/O and disk bloat). Copy only assets this page
		// actually references by filename.
		if !strings.Contains(content, entry.Name()) {
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

// Media-path rewrite patterns, compiled once instead of per rendered page;
// wpSrcURLre replaces the per-image regex + full-document rescan that made this
// O(images × content) per post (PERF-006).
var (
	wpImageIDRe       = regexp.MustCompile(`wp-image-(\d+)`)
	wpSrcURLRe        = regexp.MustCompile(`(src=["'])(https?://[^"']*\.(?:jpg|jpeg|png|gif|webp))(["'])`)
	mediaSrcRe        = regexp.MustCompile(`((?:src|href|srcset)=["'])media/`)
	mediaSrcsetItemRe = regexp.MustCompile(`, media/`)
	mediaThumbRe      = regexp.MustCompile(`(/media/\d+_[^"'\s]+)-\d+x\d+(\.(?:jpg|jpeg|png|gif|webp))`)
	mediaSrcsetSizeRe = regexp.MustCompile(`(/media/\d+_[^"'\s,]+)-\d+x\d+(\.(?:jpg|jpeg|png|gif|webp))\s+(\d+w)`)
)

// buildWPMediaReplacements maps media filenames (sans extension) referenced via
// wp-image-ID classes in content to their local /media/ paths.
func buildWPMediaReplacements(content string, media map[int]models.MediaItem) map[string]string {
	replacements := map[string]string{}
	for _, match := range wpImageIDRe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		var id int
		_, _ = fmt.Sscanf(match[1], "%d", &id)
		mediaItem, ok := media[id]
		if !ok {
			continue
		}
		// Get the filename from the media item
		filename := filepath.Base(mediaItem.MediaDetails.File)
		// Guard against empty/short media filenames: filepath.Base("") == "."
		// so filename[:len-4] would panic (slice bounds out of range).
		// Strip the extension safely instead (GO-001).
		nameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))
		if nameNoExt == "" || nameNoExt == "." {
			continue
		}
		replacements[nameNoExt] = fmt.Sprintf("/media/%d_%s", id, filename)
	}
	return replacements
}

// fixMediaPaths converts relative media paths to absolute paths
// and fixes WordPress thumbnail URLs to point to local files
func fixMediaPaths(content string, media map[int]models.MediaItem) string {
	// First, fix WordPress absolute URLs using wp-image-ID class
	// Pattern: wp-image-1048 ... src="http://...krowy.net/..." -> src="/media/1048_filename.jpg"
	replacements := buildWPMediaReplacements(content, media)
	if len(replacements) > 0 {
		// One pass over all src URLs; each candidate URL is matched against the
		// known media filenames (PERF-006: no per-image full-document rescans).
		content = wpSrcURLRe.ReplaceAllStringFunc(content, func(m string) string {
			parts := wpSrcURLRe.FindStringSubmatch(m)
			if len(parts) < 4 {
				return m
			}
			for nameNoExt, localPath := range replacements {
				if strings.Contains(parts[2], nameNoExt) {
					return parts[1] + localPath + parts[3]
				}
			}
			return m
		})
	}

	// Fix src/href/srcset="media/..." to ".../media/..."
	content = mediaSrcRe.ReplaceAllString(content, `${1}/media/`)

	// Fix URLs in srcset attribute (multiple entries separated by comma)
	content = mediaSrcsetItemRe.ReplaceAllString(content, `, /media/`)

	// Remove WordPress thumbnail size suffixes from media paths
	// e.g., /media/1048_IMG_0316_p-300x225.jpg -> /media/1048_IMG_0316_p.jpg
	content = mediaThumbRe.ReplaceAllString(content, `${1}${2}`)

	// Also handle srcset entries with size descriptors
	// e.g., /media/1048_file-300x225.jpg 300w -> /media/1048_file.jpg 300w
	content = mediaSrcsetSizeRe.ReplaceAllString(content, `${1}${2} ${3}`)

	// Process WordPress shortcodes
	content = processShortcodes(content)

	return content
}

// WordPress video shortcode patterns, compiled once (PERF-006).
var wpVideoShortcodeRes = []*regexp.Regexp{
	// [youtube]URL[/youtube]
	regexp.MustCompile(`\[youtube\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/youtube\]`),
	// [embed]URL[/embed]
	regexp.MustCompile(`\[embed\]\s*(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]+)\s*\[/embed\]`),
}

// youtubeEmbedHTML renders the iframe embed for a YouTube video ID.
func youtubeEmbedHTML(videoID string) string {
	return fmt.Sprintf(`<div class="video-container"><iframe width="560" height="315" src="https://www.youtube.com/embed/%s" title="YouTube video" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>`, videoID)
}

// processShortcodes converts WordPress shortcodes to HTML
func processShortcodes(content string) string {
	return processWPShortcodesWith(content, func(html string) string { return html })
}

// processWPShortcodesWith converts WordPress video shortcodes to HTML, passing
// each embed through emit so the sanitizing pipeline can protect it (GO-037).
func processWPShortcodesWith(content string, emit func(string) string) string {
	for _, re := range wpVideoShortcodeRes {
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			submatches := re.FindStringSubmatch(match)
			if len(submatches) < 2 {
				return match
			}
			return emit(youtubeEmbedHTML(submatches[1]))
		})
	}
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
// lastModFor computes the sitemap <lastmod> for a page: the git commit date of
// its source file when lastmod_from_git is enabled (SEO-004), otherwise the
// frontmatter modified date, falling back to the publish date.
func (g *Generator) lastModFor(p models.Page) time.Time {
	if g.config.LastmodFromGit {
		if t, ok := g.gitLastMod(p); ok {
			return t
		}
	}
	if !p.Modified.IsZero() {
		return p.Modified
	}
	return p.Date
}

// gitLastMod returns the last commit date of a page's source file. It fails
// gracefully (ok=false) outside a git repository, for untracked files, or for
// content with no source file (e.g. mddb). Instead of spawning one `git log`
// per page (an N+1 that costs minutes on large sites), a single history scan
// builds a path→date map on first use (PERF-001).
func (g *Generator) gitLastMod(p models.Page) (time.Time, bool) {
	if p.SourceFile == "" {
		return time.Time{}, false
	}
	g.gitOnce.Do(func() { g.gitRoot, g.gitTimes = loadGitLastModTimes() })
	if len(g.gitTimes) == 0 {
		return time.Time{}, false
	}
	abs, err := filepath.Abs(filepath.Join(p.SourceDir, p.SourceFile))
	if err != nil {
		return time.Time{}, false
	}
	rel, err := filepath.Rel(g.gitRoot, abs)
	if err != nil {
		return time.Time{}, false
	}
	t, ok := g.gitTimes[filepath.ToSlash(rel)]
	return t, ok
}

// loadGitLastModTimes runs one `git log --name-only` pass over the repository
// and returns its top-level directory plus a map of repo-relative file path →
// most recent commit date (PERF-001). Both git invocations use fixed arguments
// and never go through a shell; failures degrade to an empty map.
func loadGitLastModTimes() (string, map[string]time.Time) {
	// #nosec G204 -- fixed args, never a shell
	rootOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output() // NOSONAR S4036: git is intentionally resolved from PATH (portable across systems), reviewed
	if err != nil {
		return "", nil
	}
	root := strings.TrimSpace(string(rootOut))

	// -c core.quotepath=off keeps non-ASCII paths literal in the output.
	// #nosec G204 -- fixed args, never a shell
	cmd := exec.Command("git", "-c", "core.quotepath=off", "log", "--format=%cI", "--name-only") // NOSONAR S4036: reviewed, see above
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	times := make(map[string]time.Time)
	var current time.Time
	haveDate := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, line); err == nil {
			current = t
			haveDate = true
			continue
		}
		// git log walks newest-first: the first date seen for a path is its last modification.
		if haveDate {
			if _, seen := times[line]; !seen {
				times[line] = current
			}
		}
	}
	return root, times
}

func (g *Generator) generateSitemap() error {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`)
	if g.config.I18n.Enabled {
		sb.WriteString(` xmlns:xhtml="http://www.w3.org/1999/xhtml"`)
	}
	sb.WriteString(`>`)
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
		if g.config.I18n.Enabled {
			for _, lang := range g.siteData.Languages {
				sb.WriteString(sitemapURLOpen)
				fmt.Fprintf(&sb, "    <loc>https://%s%s</loc>\n", g.config.Domain, g.languageURL(lang.Code))
				sb.WriteString("    <changefreq>daily</changefreq>\n    <priority>1.0</priority>\n")
				sb.WriteString(sitemapURLClose)
			}
		} else {
			sb.WriteString(sitemapURLOpen)
			fmt.Fprintf(&sb, "    <loc>https://%s/</loc>\n", g.config.Domain)
			sb.WriteString("    <changefreq>daily</changefreq>\n")
			sb.WriteString("    <priority>1.0</priority>\n")
			sb.WriteString(sitemapURLClose)
		}
	}

	// Pages
	for _, page := range g.siteData.Pages {
		if excludeFromSitemap(page) {
			continue
		}
		sb.WriteString(sitemapURLOpen)
		fmt.Fprintf(&sb, "    <loc>%s</loc>\n", page.GetCanonical(g.config.Domain))
		g.writeSitemapAlternates(&sb, page)
		if lastmod := g.lastModFor(page); !lastmod.IsZero() {
			fmt.Fprintf(&sb, "    <lastmod>%s</lastmod>\n", lastmod.Format("2006-01-02"))
		}
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.8</priority>\n")
		sb.WriteString(sitemapURLClose)
	}

	// Posts
	for _, post := range g.siteData.Posts {
		if excludeFromSitemap(post) {
			continue
		}
		sb.WriteString(sitemapURLOpen)
		fmt.Fprintf(&sb, "    <loc>%s</loc>\n", post.GetCanonical(g.config.Domain))
		g.writeSitemapAlternates(&sb, post)
		if lastmod := g.lastModFor(post); !lastmod.IsZero() {
			fmt.Fprintf(&sb, "    <lastmod>%s</lastmod>\n", lastmod.Format("2006-01-02"))
		}
		sb.WriteString("    <changefreq>monthly</changefreq>\n")
		sb.WriteString("    <priority>0.6</priority>\n")
		sb.WriteString(sitemapURLClose)
	}

	// Categories (archives suppressed by an explicit page stay out too, GO-050)
	g.writeSitemapCategories(&sb)

	// Tag archives (BLOG-004)
	for _, slug := range sortedValues(g.tagSlugs) {
		g.writeSitemapArchive(&sb, "tag", slug)
	}

	// Author archives (BLOG-005)
	for _, slug := range sortedValues(g.authorSlugs) {
		g.writeSitemapArchive(&sb, "author", slug)
	}

	// Custom taxonomy indexes + term archives (taxonomies-feature.md)
	g.writeTaxonomySitemap(&sb)

	sb.WriteString("</urlset>\n")

	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(filepath.Join(g.config.OutputDir, "sitemap.xml"), []byte(sb.String()), 0644)
}

func (g *Generator) writeSitemapAlternates(sb *strings.Builder, page models.Page) {
	if !g.config.I18n.Enabled {
		return
	}
	for _, tr := range page.Translations {
		fmt.Fprintf(sb, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", stdhtml.EscapeString(tr.Lang), stdhtml.EscapeString(tr.Canonical))
		if tr.Lang == g.config.DefaultLanguage {
			fmt.Fprintf(sb, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n", stdhtml.EscapeString(tr.Canonical))
		}
	}
}

// writeSitemapCategories appends the category archive entries, skipping
// "Bez kategorii" and archives suppressed by an explicit page (GO-050).
func (g *Generator) writeSitemapCategories(sb *strings.Builder) {
	for _, cat := range g.siteData.Categories {
		if _, taken := g.archiveURLOwner("category", models.SanitizeRelPath(cat.Slug)); taken {
			continue
		}
		if cat.ID != 1 { // Skip "Bez kategorii"
			g.writeSitemapArchive(sb, "category", cat.Slug)
		}
	}
}

// writeSitemapArchive appends a sitemap entry for an archive page (category/tag/author).
func (g *Generator) writeSitemapArchive(sb *strings.Builder, kind, slug string) {
	sb.WriteString(sitemapURLOpen)
	fmt.Fprintf(sb, "    <loc>https://%s/%s/%s/</loc>\n", g.config.Domain, kind, slug)
	sb.WriteString("    <changefreq>weekly</changefreq>\n")
	sb.WriteString("    <priority>0.5</priority>\n")
	sb.WriteString(sitemapURLClose)
}

// sortedValues returns the deduplicated, sorted values of a string map.
func sortedValues(m map[string]string) []string {
	seen := make(map[string]bool, len(m))
	var out []string
	for _, v := range m {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// generateFeeds writes an Atom feed at the site root plus one per category and tag
// when feeds are enabled (BLOG-002). Item count is capped by feed_items (default 20);
// feed_full_content switches between rendered content and the excerpt/summary.
func (g *Generator) generateFeeds() error {
	if !g.config.Feed {
		return nil
	}
	g.log("📰 Generating Atom feeds...")
	limit := g.config.FeedItems
	if limit <= 0 {
		limit = 20
	}
	base := httpsScheme + g.config.Domain
	if g.config.I18n.Enabled {
		for _, lang := range g.siteData.Languages {
			posts := languagePages(g.siteData.Posts, lang.Code)
			prefix := ssgi18n.Prefix(lang.Code, g.config.DefaultLanguage, g.config.I18n)
			rel := feedFileName
			rootURL := "/"
			if prefix != "" {
				rel = filepath.Join(prefix, feedFileName)
				rootURL = "/" + prefix + "/"
			}
			if err := g.writeFeed(rel, g.config.Domain+" ("+lang.Name+")", base+rootURL, posts, limit); err != nil {
				return err
			}
		}
		return g.generateTaxonomyFeeds(limit)
	}

	if err := g.writeFeed(feedFileName, g.config.Domain, base+"/", g.siteData.Posts, limit); err != nil {
		return err
	}

	catPosts := make(map[int][]models.Page)
	for _, p := range g.siteData.Posts {
		for _, id := range p.Categories {
			catPosts[id] = append(catPosts[id], p)
		}
	}
	for id, posts := range catPosts {
		cat, ok := g.siteData.Categories[id]
		if !ok || cat.ID == 1 {
			continue
		}
		slug := models.SanitizeRelPath(cat.Slug)
		if slug == "" {
			continue
		}
		rel := filepath.Join("category", slug, feedFileName)
		if err := g.writeFeed(rel, cat.Name, base+"/category/"+slug+"/", posts, limit); err != nil {
			return err
		}
	}

	tagPosts := make(map[string][]models.Page)
	for _, p := range g.siteData.Posts {
		for _, tag := range p.Tags {
			tagPosts[tag] = append(tagPosts[tag], p)
		}
	}
	for name, slug := range g.tagSlugs {
		rel := filepath.Join("tag", slug, feedFileName)
		if err := g.writeFeed(rel, name, base+"/tag/"+slug+"/", tagPosts[name], limit); err != nil {
			return err
		}
	}
	return g.generateTaxonomyFeeds(limit)
}

// writeFeed renders an Atom 1.0 feed for up to limit newest posts and writes it to
// relPath under the output directory (BLOG-002). All text is XML-escaped.
func (g *Generator) writeFeed(relPath, title, altURL string, posts []models.Page, limit int) error {
	ordered := sortPostsByDate(posts)
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	updated := time.Time{}
	for _, p := range ordered {
		if m := g.lastModFor(p); m.After(updated) {
			updated = m
		}
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"`)
	if len(ordered) > 0 && ordered[0].Locale != "" {
		fmt.Fprintf(&sb, ` xml:lang="%s"`, stdhtml.EscapeString(ordered[0].Locale))
	}
	sb.WriteString(">\n")
	fmt.Fprintf(&sb, "  <title>%s</title>\n", stdhtml.EscapeString(title))
	fmt.Fprintf(&sb, "  <link href=%q rel=\"alternate\"/>\n", altURL)
	fmt.Fprintf(&sb, "  <link href=%q rel=\"self\"/>\n", httpsScheme+g.config.Domain+"/"+filepath.ToSlash(relPath))
	fmt.Fprintf(&sb, "  <id>%s</id>\n", altURL)
	if !updated.IsZero() {
		fmt.Fprintf(&sb, "  <updated>%s</updated>\n", updated.UTC().Format(time.RFC3339))
	}
	for _, p := range ordered {
		g.writeFeedEntry(&sb, p)
	}
	sb.WriteString("</feed>\n")

	outPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(relPath))
	if err := g.ensureWithinOutput(outPath); err != nil {
		return err
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(outPath, []byte(sb.String()), 0644)
}

// writeFeedEntry appends one Atom <entry> for a post (BLOG-002).
func (g *Generator) writeFeedEntry(sb *strings.Builder, p models.Page) {
	canonical := p.GetCanonical(g.config.Domain)
	sb.WriteString("  <entry>\n")
	fmt.Fprintf(sb, "    <title>%s</title>\n", stdhtml.EscapeString(p.Title))
	fmt.Fprintf(sb, "    <link href=%q/>\n", canonical)
	fmt.Fprintf(sb, "    <id>%s</id>\n", canonical)
	if !p.Date.IsZero() {
		fmt.Fprintf(sb, "    <published>%s</published>\n", p.Date.UTC().Format(time.RFC3339))
	}
	if m := g.lastModFor(p); !m.IsZero() {
		fmt.Fprintf(sb, "    <updated>%s</updated>\n", m.UTC().Format(time.RFC3339))
	}
	if g.config.FeedFullContent {
		// Feed readers render this HTML — sanitize like page output (SEC-014).
		htmlBody := g.sanitizeHTML(g.convertMarkdownToHTML(p.Content))
		fmt.Fprintf(sb, "    <content type=\"html\">%s</content>\n", stdhtml.EscapeString(htmlBody))
	} else {
		summary := p.Excerpt
		if summary == "" {
			summary = tmplStripHTML(g.convertMarkdownToHTML(p.Content))
			// Truncate by runes, not bytes — a byte slice can cut a multibyte
			// character in half and emit invalid UTF-8 into the feed (GO-021).
			if utf8.RuneCountInString(summary) > 300 {
				summary = string([]rune(summary)[:300])
			}
		}
		fmt.Fprintf(sb, "    <summary>%s</summary>\n", stdhtml.EscapeString(summary))
	}
	sb.WriteString("  </entry>\n")
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

// generateCloudflareFiles creates _headers and _redirects files for Cloudflare
// Pages: configurable headers (GO-064) and the redirects engine (GO-063).
func (g *Generator) generateCloudflareFiles() error {
	if err := g.generateHeadersFile(); err != nil {
		return err
	}
	if err := g.generateRedirectsFile(); err != nil {
		return err
	}
	return g.generateWorkerFiles()
}

// HTML/CSS/JS minification patterns, compiled once (PERF-006).
var (
	minIgnoreBlockRe = regexp.MustCompile(`(?s)<!--\s*htmlmin:ignore\s*-->(.*?)<!--\s*/htmlmin:ignore\s*-->`)
	// Whitespace-sensitive elements minification must never touch (GO-022):
	// <pre>/<textarea> render whitespace, <script>/<style> may break semantically.
	minPreserveTagRe  = regexp.MustCompile(`(?is)<pre\b[^>]*>.*?</pre>|<textarea\b[^>]*>.*?</textarea>|<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>`)
	minHTMLCommentRe  = regexp.MustCompile(`<!--[\s\S]*?-->`)
	minTagGapRe       = regexp.MustCompile(`>\s+<`)
	minMultiSpaceRe   = regexp.MustCompile(`\s{2,}`)
	minCSSCommentRe   = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	minCSSSpacesRe    = regexp.MustCompile(`\s*([:{};,])\s*`)
	minJSLineCmtRe    = regexp.MustCompile(`(?m)^\s*//.*$`)
	minJSEmptyLinesRe = regexp.MustCompile(`\n\s*\n`)
	minLineCommentRe  = regexp.MustCompile(`^\s*//.*$`)
	minIntraSpaceRe   = regexp.MustCompile(`[ \t]{2,}`)
)

// minifyCSSFile removes unnecessary whitespace and comments from CSS
func minifyCSSFile(path string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}

	s := string(content)
	// Remove CSS comments
	s = minCSSCommentRe.ReplaceAllString(s, "")
	// Remove newlines
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	// Remove spaces around : ; { } ,
	s = minCSSSpacesRe.ReplaceAllString(s, "$1")
	// Collapse multiple spaces
	s = minMultiSpaceRe.ReplaceAllString(s, " ")
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
	s = minJSLineCmtRe.ReplaceAllString(s, "")
	// Remove multi-line comments
	s = minCSSCommentRe.ReplaceAllString(s, "")
	// Remove empty lines
	s = minJSEmptyLinesRe.ReplaceAllString(s, "\n")
	// Trim
	s = strings.TrimSpace(s)

	// #nosec G306,G703 -- Web content files need to be world-readable, CLI tool writes user's output
	return os.WriteFile(path, []byte(s), 0644)
}

// minifyAssetFile minifies a CSS/JS file. Without source maps it uses the given
// full minifier. With source maps (BLOG-007/GO-004) it uses the line-preserving
// minifier and writes an accurate v3 map next to the file. Empty inputs are left
// untouched so no dangling map is produced.
func (g *Generator) minifyAssetFile(path string, full func(string) error, linePreserving func(string) string) error {
	if !g.config.SourceMap {
		return full(path)
	}
	original, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(original)) == "" {
		return nil
	}
	minified := linePreserving(string(original))
	return writeWithSourceMap(path, string(original), minified)
}

// blockCommentToNewlines replaces /* ... */ comments with the same number of
// newlines they spanned, so total line count (and thus a line-level source map)
// is preserved across removal.
func blockCommentToNewlines(s string) string {
	return minCSSCommentRe.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("\n", strings.Count(m, "\n"))
	})
}

// minifyCSSLinePreserving strips comments and collapses intra-line whitespace but
// keeps one output line per input line, so the emitted source map is exact.
func minifyCSSLinePreserving(s string) string {
	s = blockCommentToNewlines(s)
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(minIntraSpaceRe.ReplaceAllString(ln, " "), " \t")
	}
	return strings.Join(lines, "\n")
}

// minifyJSLinePreserving strips comments and collapses intra-line whitespace while
// keeping the line count stable, so the emitted source map is exact.
func minifyJSLinePreserving(s string) string {
	s = blockCommentToNewlines(s)
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if minLineCommentRe.MatchString(ln) {
			lines[i] = ""
			continue
		}
		lines[i] = strings.TrimRight(minIntraSpaceRe.ReplaceAllString(ln, " "), " \t")
	}
	return strings.Join(lines, "\n")
}

// sourceMapV3 is the Source Map Revision 3 document embedded next to minified assets.
type sourceMapV3 struct {
	Version        int      `json:"version"`
	File           string   `json:"file"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
}

// writeWithSourceMap overwrites path with the (line-preserving) minified content
// plus a sourceMappingURL comment, and writes the companion <base>.map embedding
// the original source with an exact identity line mapping.
func writeWithSourceMap(path, original, minified string) error {
	base := filepath.Base(path)
	mapName := base + ".map"
	lineCount := strings.Count(minified, "\n") + 1

	sm := sourceMapV3{
		Version:        3,
		File:           base,
		Sources:        []string{base + "?source"},
		SourcesContent: []string{original},
		Names:          []string{},
		Mappings:       identityLineMappings(lineCount),
	}
	mapJSON, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	comment := "js"
	if strings.EqualFold(filepath.Ext(path), ".css") {
		comment = "css"
	}
	var out string
	if comment == "css" {
		out = minified + "\n/*# sourceMappingURL=" + mapName + " */\n"
	} else {
		out = minified + "\n//# sourceMappingURL=" + mapName + "\n"
	}

	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		return err
	}
	// #nosec G306 -- source maps are served alongside assets
	return os.WriteFile(filepath.Join(filepath.Dir(path), mapName), mapJSON, 0644)
}

// identityLineMappings builds VLQ mappings where generated line i maps to source
// line i at column 0 (valid because minification is line-preserving). Line 0 is
// [0,0,0,0]="AAAA"; each later line advances the source line by one: "AACA".
func identityLineMappings(lineCount int) string {
	if lineCount <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("AAAA")
	for i := 1; i < lineCount; i++ {
		b.WriteString(";AACA")
	}
	return b.String()
}

// fingerprintIfRequested runs the terminal cache-busting pass when enabled (ASSET-001).
func (g *Generator) fingerprintIfRequested() error {
	if !g.config.Fingerprint {
		return nil
	}
	g.log("🔏 Fingerprinting assets...")
	return g.fingerprintAssets()
}

// fingerprintAssets renames CSS/JS to name.<hash8>.ext, rewrites references inside
// HTML and CSS (url()/@import), and writes assets-manifest.json (ASSET-001). CSS is
// hashed after any CSS it @imports so dependency references stay valid. Hashes are
// content-derived, so two identical builds yield byte-identical names (determinism).
func (g *Generator) fingerprintAssets() error {
	jsFiles, cssFiles, err := g.collectFingerprintAssets()
	if err != nil {
		return err
	}

	manifest := make(map[string]string) // original rel path → hashed rel path
	byBasename := make(map[string]string)

	// JS first (independent), then CSS ordered so @import leaves are hashed first.
	sort.SliceStable(cssFiles, func(i, j int) bool {
		return atImportCount(cssFiles[i]) < atImportCount(cssFiles[j])
	})
	for _, path := range append(jsFiles, cssFiles...) {
		if err := g.fingerprintOne(path, manifest, byBasename); err != nil {
			return err
		}
	}

	// Rewrite references inside every generated HTML file.
	if err := g.rewriteHTMLAssetRefs(byBasename); err != nil {
		return err
	}

	mj, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	// #nosec G306 -- manifest is public build metadata served with the site
	return os.WriteFile(filepath.Join(g.config.OutputDir, "assets-manifest.json"), mj, 0644)
}

// collectFingerprintAssets returns the JS and CSS files under the output dir,
// sorted for deterministic processing.
func (g *Generator) collectFingerprintAssets() (js, css []string, err error) {
	err = filepath.Walk(g.config.OutputDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".js":
			js = append(js, path)
		case ".css":
			css = append(css, path)
		}
		return nil
	})
	sort.Strings(js)
	sort.Strings(css)
	return js, css, err
}

// fingerprintOne hashes a single asset (after rewriting references to assets that
// were already hashed), renames it and records the mapping.
func (g *Generator) fingerprintOne(path string, manifest, byBasename map[string]string) error {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return err
	}
	s := rewriteAssetRefs(string(content), byBasename)

	sum := sha256.Sum256([]byte(s))
	hash := hex.EncodeToString(sum[:])[:8]

	base := filepath.Base(path)
	ext := filepath.Ext(base)
	hashedBase := strings.TrimSuffix(base, ext) + "." + hash + ext
	hashedPath := filepath.Join(filepath.Dir(path), hashedBase)

	// #nosec G306,G703 -- CLI writes its own output; hashedPath derived from a local Walk, not attacker-controlled
	if err := os.WriteFile(hashedPath, []byte(s), 0644); err != nil {
		return err
	}
	if hashedPath != path {
		_ = os.Remove(path)
	}

	rel, _ := filepath.Rel(g.config.OutputDir, path)
	hashedRel, _ := filepath.Rel(g.config.OutputDir, hashedPath)
	manifest[filepath.ToSlash(rel)] = filepath.ToSlash(hashedRel)
	byBasename[base] = hashedBase
	return nil
}

// rewriteHTMLAssetRefs updates asset references in every generated HTML file.
// The rewriter (and its regexes) is built once for the whole walk instead of
// once per file per asset (PERF-003).
func (g *Generator) rewriteHTMLAssetRefs(byBasename map[string]string) error {
	rw := newAssetRefRewriter(byBasename)
	return filepath.Walk(g.config.OutputDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		content, e := os.ReadFile(path) // #nosec G304,G122 -- CLI reads its own output; path from local Walk
		if e != nil {
			return e
		}
		out := rw.rewrite(string(content))
		if out == string(content) {
			return nil
		}
		// #nosec G306,G703,G122 -- CLI writes its own output; path from local Walk
		return os.WriteFile(path, []byte(out), 0644)
	})
}

// atImportCount counts @import statements in a CSS file (best-effort, for ordering).
func atImportCount(path string) int {
	content, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "@import")
}

// assetRefRewriter holds precompiled basename regexes so the fingerprint walk
// compiles each pattern once instead of per file per asset (PERF-003).
type assetRefRewriter struct {
	res  []*regexp.Regexp
	repl []string
}

// newAssetRefRewriter compiles one regex per known asset basename, longest-first
// for deterministic, non-overlapping replacement (ASSET-001).
func newAssetRefRewriter(byBasename map[string]string) *assetRefRewriter {
	bases := make([]string, 0, len(byBasename))
	for b := range byBasename {
		bases = append(bases, b)
	}
	sort.Slice(bases, func(i, j int) bool { return len(bases[i]) > len(bases[j]) })
	rw := &assetRefRewriter{
		res:  make([]*regexp.Regexp, 0, len(bases)),
		repl: make([]string, 0, len(bases)),
	}
	for _, base := range bases {
		rw.res = append(rw.res, regexp.MustCompile(`([/"'(=])`+regexp.QuoteMeta(base)+`([)"'?#\s])`))
		rw.repl = append(rw.repl, `${1}`+byBasename[base]+`${2}`)
	}
	return rw
}

// rewrite replaces each known asset basename with its hashed basename when it
// appears as a URL/path segment (bounded by a delimiter), covering href/src
// attributes, CSS url() and @import.
func (rw *assetRefRewriter) rewrite(s string) string {
	for i, re := range rw.res {
		s = re.ReplaceAllString(s, rw.repl[i])
	}
	return s
}

// rewriteAssetRefs is the one-shot form used while hashing assets, where the
// basename map still grows between calls.
func rewriteAssetRefs(s string, byBasename map[string]string) string {
	if len(byBasename) == 0 {
		return s
	}
	return newAssetRefRewriter(byBasename).rewrite(s)
}

// mermaidVersion pins the mermaid release injected for diagram pages (GO-073).
const mermaidVersion = "11.16.0"

// injectMermaidAssets adds the mermaid.js ES module and an initializer before
// </body>, so <pre class="mermaid"> blocks render as diagrams. Loaded only on
// pages that contain a diagram (mermaidHTMLString gate).
func injectMermaidAssets(html string) string {
	body := `<script type="module">import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@` + mermaidVersion +
		`/dist/mermaid.esm.min.mjs";mermaid.initialize({startOnLoad:true});</script>`
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		return html[:i] + body + "\n" + html[i:]
	}
	return html + body
}

// katexVersion pins the KaTeX release injected for math pages (AX-004).
const katexVersion = "0.16.11"

// injectKatexAssets adds the KaTeX stylesheet to <head> and the KaTeX + auto-render
// scripts plus an init call before </body>. Display math uses $$…$$ and inline math
// uses \(…\) to avoid clashing with currency ($). Loaded with crossorigin; for
// production, self-host or add SRI (documented in README).
func injectKatexAssets(html string) string {
	base := "https://cdn.jsdelivr.net/npm/katex@" + katexVersion + "/dist/"
	head := `<link rel="stylesheet" href="` + base + `katex.min.css" crossorigin="anonymous">`
	body := `<script defer src="` + base + `katex.min.js" crossorigin="anonymous"></script>` +
		`<script defer src="` + base + `contrib/auto-render.min.js" crossorigin="anonymous"></script>` +
		`<script>document.addEventListener("DOMContentLoaded",function(){renderMathInElement(document.body,` +
		`{delimiters:[{left:"$$",right:"$$",display:true},{left:"\\(",right:"\\)",display:false}]});});</script>`

	if i := strings.LastIndex(html, "</head>"); i >= 0 {
		html = html[:i] + head + "\n" + html[i:]
	} else {
		html = head + "\n" + html
	}
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		html = html[:i] + body + "\n" + html[i:]
	} else {
		html += body
	}
	return html
}
