// Package config handles SSG configuration file parsing
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Shortcode defines a reusable content snippet
type Shortcode struct {
	Name     string            `yaml:"name" toml:"name" json:"name"`             // Shortcode name (e.g., "thunderpick")
	Type     string            `yaml:"type" toml:"type" json:"type"`             // Type for template logic (e.g., "banner")
	Template string            `yaml:"template" toml:"template" json:"template"` // Template file (required)
	Title    string            `yaml:"title" toml:"title" json:"title"`          // Title/heading
	Text     string            `yaml:"text" toml:"text" json:"text"`             // Text content
	Url      string            `yaml:"url" toml:"url" json:"url"`                // Link URL
	Logo     string            `yaml:"logo" toml:"logo" json:"logo"`             // Logo/image path
	Legal    string            `yaml:"legal" toml:"legal" json:"legal"`          // Legal text
	Ranking  float64           `yaml:"ranking" toml:"ranking" json:"ranking"`    // Ranking score (e.g., 3.5)
	Tags     []string          `yaml:"tags" toml:"tags" json:"tags"`             // Tags for categorization (e.g., ["game", "public"])
	Data     map[string]string `yaml:"data" toml:"data" json:"data"`             // Additional custom data
}

// MddbConfig holds MDDB connection settings
type MddbConfig struct {
	Enabled       bool   `yaml:"enabled" toml:"enabled" json:"enabled"`                      // Enable mddb as content source
	URL           string `yaml:"url" toml:"url" json:"url"`                                  // Base URL (e.g., "http://localhost:11023" or "localhost:11024" for gRPC)
	Protocol      string `yaml:"protocol" toml:"protocol" json:"protocol"`                   // Connection protocol: "http" (default) or "grpc"
	APIKey        string `yaml:"api_key" toml:"api_key" json:"api_key"`                      // Optional API key
	Collection    string `yaml:"collection" toml:"collection" json:"collection"`             // Collection name for content
	Lang          string `yaml:"lang" toml:"lang" json:"lang"`                               // Language filter (e.g., "en_US")
	Timeout       int    `yaml:"timeout" toml:"timeout" json:"timeout"`                      // Request timeout in seconds
	BatchSize     int    `yaml:"batch_size" toml:"batch_size" json:"batch_size"`             // Batch size for pagination (default: 1000)
	Watch         bool   `yaml:"watch" toml:"watch" json:"watch"`                            // Enable watch mode for MDDB changes
	WatchInterval int    `yaml:"watch_interval" toml:"watch_interval" json:"watch_interval"` // Watch interval in seconds (default: 30)
}

// Config represents all SSG configuration options
type Config struct {
	// Positional arguments (can be set in config)
	Source   string `yaml:"source" toml:"source" json:"source"`
	Template string `yaml:"template" toml:"template" json:"template"`
	Domain   string `yaml:"domain" toml:"domain" json:"domain"`

	// Paths
	ContentDir   string `yaml:"content_dir" toml:"content_dir" json:"content_dir"`
	TemplatesDir string `yaml:"templates_dir" toml:"templates_dir" json:"templates_dir"`
	OutputDir    string `yaml:"output_dir" toml:"output_dir" json:"output_dir"`

	// MDDB Content Source
	Mddb MddbConfig `yaml:"mddb" toml:"mddb" json:"mddb"`

	// Template Engine
	Engine      string `yaml:"engine" toml:"engine" json:"engine"`                   // go, pongo2, mustache, handlebars
	OnlineTheme string `yaml:"online_theme" toml:"online_theme" json:"online_theme"` // URL to download theme

	// Server & Development
	HTTP  bool   `yaml:"http" toml:"http" json:"http"`
	Host  string `yaml:"host" toml:"host" json:"host"` // Dev-server bind address (default: 127.0.0.1; use 0.0.0.0 to expose)
	Port  int    `yaml:"port" toml:"port" json:"port"`
	Watch bool   `yaml:"watch" toml:"watch" json:"watch"`
	Clean bool   `yaml:"clean" toml:"clean" json:"clean"`

	// TLS for the built-in server (v1.8.1). Manual: point TLSCert/TLSKey at a
	// certificate+key. Auto: TLSAuto obtains a Let's Encrypt cert for TLSDomain
	// (needs a public domain and ports 80/443). Manual takes priority.
	TLSCert   string `yaml:"tls_cert" toml:"tls_cert" json:"tls_cert"`
	TLSKey    string `yaml:"tls_key" toml:"tls_key" json:"tls_key"`
	TLSAuto   bool   `yaml:"tls_auto" toml:"tls_auto" json:"tls_auto"`
	TLSDomain string `yaml:"tls_domain" toml:"tls_domain" json:"tls_domain"`

	// Server hardening for public serving (v1.8.1). Cache-Control and security
	// headers are applied automatically; these tune the rest.
	Gzip     bool   `yaml:"gzip" toml:"gzip" json:"gzip"`                // gzip-compress responses on the fly
	MaxConns int    `yaml:"max_conns" toml:"max_conns" json:"max_conns"` // cap concurrent connections (0 = unlimited)
	MemLimit string `yaml:"mem_limit" toml:"mem_limit" json:"mem_limit"` // soft runtime memory limit, e.g. "512MiB"
	// HTTP3 also serves HTTP/3 (QUIC) alongside HTTPS/2 and advertises it via
	// Alt-Svc. Requires TLS (QUIC is always encrypted). HTTP/2 is already automatic
	// over TLS; this adds QUIC on the same UDP port (v1.8.1).
	HTTP3 bool `yaml:"http3" toml:"http3" json:"http3"`

	// Output Control
	SitemapOff    bool   `yaml:"sitemap_off" toml:"sitemap_off" json:"sitemap_off"`
	RobotsOff     bool   `yaml:"robots_off" toml:"robots_off" json:"robots_off"`
	PrettyHTML    bool   `yaml:"pretty_html" toml:"pretty_html" json:"pretty_html"`
	PostURLFormat string `yaml:"post_url_format" toml:"post_url_format" json:"post_url_format"` // "date" (default) or "slug"
	PageFormat    string `yaml:"page_format" toml:"page_format" json:"page_format"`             // "directory" (default), "flat", or "both"
	RelativeLinks bool   `yaml:"relative_links" toml:"relative_links" json:"relative_links"`    // Convert absolute URLs to relative
	MinifyAll     bool   `yaml:"minify_all" toml:"minify_all" json:"minify_all"`
	MinifyHTML    bool   `yaml:"minify_html" toml:"minify_html" json:"minify_html"`
	MinifyCSS     bool   `yaml:"minify_css" toml:"minify_css" json:"minify_css"`
	MinifyJS      bool   `yaml:"minify_js" toml:"minify_js" json:"minify_js"`
	// SourceMap emits v3 source maps (*.js.map / *.css.map) alongside minified
	// JS/CSS, embedding the original source; minification is line-preserving so
	// the mapping stays exact (GO-004 / BLOG-007). Requires the matching minify_*.
	SourceMap bool `yaml:"sourcemap" toml:"sourcemap" json:"sourcemap"`

	// Permalinks maps a content type ("post"/"page") to a URL pattern with tokens
	// :year :month :day :slug :category (e.g. "/:year/:month/:slug/"). Empty =
	// default date/slug behaviour, so this is not a breaking change (SEO-001).
	Permalinks map[string]string `yaml:"permalinks" toml:"permalinks" json:"permalinks"`

	// Timezone is the IANA zone (e.g. "Europe/Warsaw") used to render content
	// dates: :year/:month/:day permalink tokens and the Date/Modified template
	// context. Empty = no conversion (dates stay as parsed, i.e. UTC) — the
	// pre-feature behaviour, so this is not a breaking change (I18N-001).
	Timezone string `yaml:"timezone" toml:"timezone" json:"timezone"`
	// LanguageTimezones overrides Timezone per content language (PLAT-005 langs),
	// e.g. {en_US: "America/New_York", pl_PL: "Europe/Warsaw"} (I18N-001).
	LanguageTimezones map[string]string `yaml:"language_timezones" toml:"language_timezones" json:"language_timezones"`

	// LastmodFromGit derives sitemap <lastmod> from each source file's last git
	// commit date instead of frontmatter (SEO-004). Falls back gracefully outside
	// a git repository or for content without a source file (e.g. mddb).
	LastmodFromGit bool `yaml:"lastmod_from_git" toml:"lastmod_from_git" json:"lastmod_from_git"`

	// Fingerprint renames CSS/JS to name.<hash8>.ext, writes a manifest and
	// rewrites references in HTML and CSS for immutable caching (ASSET-001).
	Fingerprint bool `yaml:"fingerprint" toml:"fingerprint" json:"fingerprint"`

	// Shortcodes
	Shortcodes        []Shortcode `yaml:"shortcodes" toml:"shortcodes" json:"shortcodes"`
	ShortcodeBrackets bool        `yaml:"shortcode_brackets" toml:"shortcode_brackets" json:"shortcode_brackets"` // Also match [shortcode] syntax (default: false)

	// Variables defines custom variables available in all templates as {{.Vars.key}}
	// and exported as environment variables with SSG_ prefix (e.g. SSG_GTM).
	// Values starting with $ are resolved from the current environment (e.g. "$GTM_CODE").
	Variables map[string]interface{} `yaml:"variables" toml:"variables" json:"variables"`

	// PagesPath is the subdirectory name inside source for static pages (default: "pages")
	PagesPath string `yaml:"pages_path" toml:"pages_path" json:"pages_path"`
	// PostsPath is the subdirectory name inside source for blog posts (default: "posts")
	PostsPath string `yaml:"posts_path" toml:"posts_path" json:"posts_path"`

	// StaticDir is a project-level directory copied verbatim (all files and
	// subdirectories) into the output during generation (default: "static").
	StaticDir string `yaml:"static_dir" toml:"static_dir" json:"static_dir"`

	// RewriteMdLinks rewrites relative .md links in content to their final output URLs (opt-in)
	RewriteMdLinks bool `yaml:"rewrite_md_links" toml:"rewrite_md_links" json:"rewrite_md_links"`

	// PreserveSlugCase keeps original casing in slugs/URLs derived from filenames.
	// Default (false): slugs are lowercased (e.g. "API.md" → slug "api" → /api/).
	// When true: original case is preserved (e.g. "API.md" → slug "API" → /API/).
	PreserveSlugCase bool `yaml:"preserve_slug_case" toml:"preserve_slug_case" json:"preserve_slug_case"`

	// Image Processing
	WebP            bool `yaml:"webp" toml:"webp" json:"webp"`
	WebPQuality     int  `yaml:"webp_quality" toml:"webp_quality" json:"webp_quality"`
	ReconvertImages bool `yaml:"reconvert_images" toml:"reconvert_images" json:"reconvert_images"` // Force reconvert even if WebP exists

	// ImageSizes lists responsive width presets (px). For each image the WebP
	// pipeline emits a variant per width (no upscaling) and rewrites <img> with
	// srcset/sizes (ASSET-004). Empty = single-size behaviour (unchanged).
	ImageSizes []int `yaml:"image_sizes" toml:"image_sizes" json:"image_sizes"`
	// ImageSizesAttr is the value of the generated sizes attribute (default "100vw").
	ImageSizesAttr string `yaml:"image_sizes_attr" toml:"image_sizes_attr" json:"image_sizes_attr"`

	// Math enables opt-in math rendering: math delimiters ($$…$$) are detected
	// and KaTeX assets are injected only on pages that use them (AX-004).
	Math bool `yaml:"math" toml:"math" json:"math"`

	// Paginate sets posts-per-page for the index listing; 0 disables pagination
	// (default), preserving the single-page index (BLOG-003).
	Paginate int `yaml:"paginate" toml:"paginate" json:"paginate"`

	// Languages / DefaultLanguage enable language-aware output trees with hreflang
	// alternates (PLAT-005). Empty = single-language build (unchanged).
	Languages       []string `yaml:"languages" toml:"languages" json:"languages"`
	DefaultLanguage string   `yaml:"default_language" toml:"default_language" json:"default_language"`

	// Hooks are exec commands run at build lifecycle phases: pre_build, post_build,
	// post_page. Trusted local config only; never sourced from content (PLAT-001).
	Hooks map[string][]string `yaml:"hooks" toml:"hooks" json:"hooks"`

	// Feed generates Atom feeds (feed.xml at root + per category/tag) (BLOG-002).
	Feed            bool `yaml:"feed" toml:"feed" json:"feed"`
	FeedItems       int  `yaml:"feed_items" toml:"feed_items" json:"feed_items"`                      // item cap (default 20)
	FeedFullContent bool `yaml:"feed_full_content" toml:"feed_full_content" json:"feed_full_content"` // true=full content, false=summary

	// Syntax highlighting via Chroma (AX-001).
	Highlight      bool   `yaml:"highlight" toml:"highlight" json:"highlight"`
	HighlightStyle string `yaml:"highlight_style" toml:"highlight_style" json:"highlight_style"` // Chroma style (default "github")

	// Table of contents (AX-002): .TOC context + [toc] shortcode.
	TOC      bool `yaml:"toc" toml:"toc" json:"toc"`
	TOCDepth int  `yaml:"toc_depth" toml:"toc_depth" json:"toc_depth"` // max heading level (default 3)

	// SEO opts in to the generator-level SEO partial (OpenGraph/Twitter/JSON-LD)
	// injected into pages that lack their own OpenGraph tags (SEO-003). Off by default
	// so ssg never rewrites your rendered <head> unless you ask (v1.8.2). The legacy
	// `seo_off` key / `--seo-off` flag are still accepted as no-ops for compatibility.
	SEO    bool `yaml:"seo" toml:"seo" json:"seo"`
	SEOOff bool `yaml:"seo_off" toml:"seo_off" json:"seo_off"` // deprecated: injection is now opt-in

	// CheckLinks validates internal links after build: "" (off), "warn", or "strict"
	// (non-zero exit on a dead internal link) (SEO-005).
	CheckLinks string `yaml:"check_links" toml:"check_links" json:"check_links"`

	// Bundles concatenate groups of CSS/JS into one artifact before minify/fingerprint
	// (ASSET-002): {"app.css": ["reset.css","theme.css"]}.
	Bundles map[string][]string `yaml:"bundles" toml:"bundles" json:"bundles"`

	// Outputs lists per-page output formats; "html" always emitted, add "json" for a
	// headless JSON representation next to index.html (PLAT-003).
	Outputs []string `yaml:"outputs" toml:"outputs" json:"outputs"`

	// SearchIndex writes search-index.json (title/url/tags/excerpt/text) for a
	// client-side search widget (PLAT-004).
	SearchIndex bool `yaml:"search_index" toml:"search_index" json:"search_index"`

	// DataDir is the directory of data files (*.yaml|*.yml|*.json) loaded into
	// the .Data.* template namespace (default "data", PLAT-002).
	DataDir string `yaml:"data_dir" toml:"data_dir" json:"data_dir"`

	// Deployment
	Zip   bool `yaml:"zip" toml:"zip" json:"zip"`
	TarGz bool `yaml:"targz" toml:"targz" json:"targz"` // create <domain>.tar.gz (v1.8.1)
	TarXz bool `yaml:"tarxz" toml:"tarxz" json:"tarxz"` // create <domain>.tar.xz (v1.8.1)

	// Native deployment (v1.8.1). Deploy names the provider; empty = no deploy.
	// Supported: cloudflare, github-pages, netlify, vercel, ftp, sftp. All secrets
	// (API tokens, passwords, SSH keys) come from the environment — never the config
	// file. DeployProject = Pages/site/project name; DeployBranch = target branch
	// (cloudflare/github-pages); DeployTarget = ftp/sftp URL or git remote.
	Deploy        string `yaml:"deploy" toml:"deploy" json:"deploy"`
	DeployProject string `yaml:"deploy_project" toml:"deploy_project" json:"deploy_project"`
	DeployBranch  string `yaml:"deploy_branch" toml:"deploy_branch" json:"deploy_branch"`
	DeployTarget  string `yaml:"deploy_target" toml:"deploy_target" json:"deploy_target"`

	// SanitizeHTML runs rendered content through an HTML sanitizer (bluemonday
	// UGCPolicy) before it reaches the template, neutralising stored XSS from an
	// untrusted mddb source (FE-005 / SEC-003). Default off (trusted local content).
	SanitizeHTML bool `yaml:"sanitize_html" toml:"sanitize_html" json:"sanitize_html"`

	// Other
	Quiet bool `yaml:"quiet" toml:"quiet" json:"quiet"`
}

// DefaultConfig returns configuration with default values
func DefaultConfig() *Config {
	return &Config{
		ContentDir:      "content",
		TemplatesDir:    "templates",
		OutputDir:       "output",
		StaticDir:       "static",
		DataDir:         "data",
		Host:            "127.0.0.1",
		Port:            8888,
		WebPQuality:     60,
		ImageSizesAttr:  "100vw",
		FeedItems:       20,
		FeedFullContent: false,
		HighlightStyle:  "github",
		TOCDepth:        3,
		Mddb: MddbConfig{
			Timeout:       30,
			BatchSize:     1000,
			WatchInterval: 30,
		},
	}
}

// Load loads configuration from a file (YAML, TOML, or JSON)
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- CLI tool reads user's config file
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing YAML config: %w", err)
		}
	case ".toml":
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing TOML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing JSON config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format: %s (use .yaml, .toml, or .json)", ext)
	}

	// Apply minify_all
	if cfg.MinifyAll {
		cfg.MinifyHTML = true
		cfg.MinifyCSS = true
		cfg.MinifyJS = true
	}

	return cfg, nil
}

// FindConfigFile looks for default config files in current directory
func FindConfigFile() string {
	candidates := []string{
		".ssg.yaml",
		".ssg.yml",
		".ssg.toml",
		".ssg.json",
		"ssg.yaml",
		"ssg.yml",
		"ssg.toml",
		"ssg.json",
	}

	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}

	return ""
}
