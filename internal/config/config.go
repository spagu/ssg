// Package config handles SSG configuration file parsing
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spagu/ssg/internal/externalsource"
	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/taxonomy"
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

// ContentSource is one extra Markdown root merged into the site (CONTENT-002).
// `path` is required; `type` is "page" (default) or "post"; `category` files
// every entry of the source under one category, created when the loaded
// metadata does not already define it. Per-file frontmatter always wins.
type ContentSource struct {
	Path     string `yaml:"path" toml:"path" json:"path"`
	Type     string `yaml:"type" toml:"type" json:"type"`
	Category string `yaml:"category" toml:"category" json:"category"`
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
	HTTP        bool   `yaml:"http" toml:"http" json:"http"`
	Host        string `yaml:"host" toml:"host" json:"host"` // Dev-server bind address (default: 127.0.0.1; use 0.0.0.0 to expose)
	Port        int    `yaml:"port" toml:"port" json:"port"`
	Watch       bool   `yaml:"watch" toml:"watch" json:"watch"`
	WatchRunner string `yaml:"watch_runner" toml:"watch_runner" json:"watch_runner"`
	// WatchRunnerConfig points the watch runner at a config file living outside
	// the project root (e.g. a wrangler.toml kept in deploy/ instead of .ssg/).
	// Passed as `--config <path>` to wrangler and custom runners, and as the
	// positional config argument to workerd (GO-054).
	WatchRunnerConfig string `yaml:"watch_runner_config" toml:"watch_runner_config" json:"watch_runner_config"`
	// WatchRunnerDir is the working directory the runner is started in — the
	// monorepo case, where the Worker lives in a subdirectory (booking/apps/api)
	// while content and templates live at the repo root (GO-054).
	WatchRunnerDir string `yaml:"watch_runner_dir" toml:"watch_runner_dir" json:"watch_runner_dir"`
	Clean          bool   `yaml:"clean" toml:"clean" json:"clean"`

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

	// SCSS compiles *.scss in the output to *.css via the dart-sass CLI before
	// bundling/minify (ASSET-003). Optional system tool like cwebp: when absent
	// the step is skipped with a warning. SassBinary overrides PATH lookup.
	SCSS       bool   `yaml:"scss" toml:"scss" json:"scss"`
	SassBinary string `yaml:"sass_binary" toml:"sass_binary" json:"sass_binary"`

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
	// WebPKeepOriginal emits .webp files next to the originals instead of
	// replacing them, so themes with hardcoded .png/.jpg references keep
	// working (GO-052). Default false = historical replace-in-place.
	WebPKeepOriginal bool `yaml:"webp_keep_original" toml:"webp_keep_original" json:"webp_keep_original"`

	// ImageSizes lists responsive width presets (px). For each image the WebP
	// pipeline emits a variant per width (no upscaling) and rewrites <img> with
	// srcset/sizes (ASSET-004). Empty = single-size behaviour (unchanged).
	ImageSizes []int `yaml:"image_sizes" toml:"image_sizes" json:"image_sizes"`
	// ImageSizesAttr is the value of the generated sizes attribute (default "100vw").
	ImageSizesAttr string `yaml:"image_sizes_attr" toml:"image_sizes_attr" json:"image_sizes_attr"`
	// ImagesGC prunes image-cache entries not referenced by the current build
	// after generation; ImagesGCDry only reports what would be removed (GO-057).
	ImagesGC    bool `yaml:"images_gc" toml:"images_gc" json:"images_gc"`
	ImagesGCDry bool `yaml:"images_gc_dry" toml:"images_gc_dry" json:"images_gc_dry"`

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
	// LanguageConfigs is populated when the expanded languages format is used.
	// It is excluded from decoding because both formats intentionally share the
	// public `languages` key.
	LanguageConfigs []ssgi18n.LanguageConfig `yaml:"-" toml:"-" json:"-"`
	I18n            ssgi18n.Config           `yaml:"i18n" toml:"i18n" json:"i18n"`

	// Taxonomies declares custom dynamic taxonomies and/or overrides the built-in
	// category/tag/series definitions (taxonomies-feature.md).
	Taxonomies map[string]taxonomy.DefinitionConfig `yaml:"taxonomies" toml:"taxonomies" json:"taxonomies"`

	// ExternalSources configures the unified external data system
	// (ssg-external-sources-implementation-plan.md).
	ExternalSources externalsource.Config `yaml:"external_sources" toml:"external_sources" json:"external_sources"`

	// Built-in server access control (config-only; SSO/LDAP deferred):
	// server_auth "basic"|"jwt", basic users as "login:$PASS_ENV", HS256 JWT
	// secret from the environment, IP allow/block lists (IPs or CIDRs) and a
	// per-IP token-bucket rate limiter.
	ServerAuth  string   `yaml:"server_auth" toml:"server_auth" json:"server_auth"`
	ServerUsers []string `yaml:"server_users" toml:"server_users" json:"server_users"`
	JWTSecret   string   `yaml:"jwt_secret" toml:"jwt_secret" json:"jwt_secret"`
	IPAllowlist []string `yaml:"ip_allowlist" toml:"ip_allowlist" json:"ip_allowlist"`
	IPBlocklist []string `yaml:"ip_blocklist" toml:"ip_blocklist" json:"ip_blocklist"`
	RateLimit   float64  `yaml:"rate_limit" toml:"rate_limit" json:"rate_limit"`
	RateBurst   int      `yaml:"rate_burst" toml:"rate_burst" json:"rate_burst"`

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
	// `seo_off` key / `--seo-off` flag still force SEO off, with a deprecation
	// warning at load time (GO-059).
	SEO    bool `yaml:"seo" toml:"seo" json:"seo"`
	SEOOff bool `yaml:"seo_off" toml:"seo_off" json:"seo_off"` // deprecated: use seo: false

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

	// ContentSources lists extra local Markdown roots merged into the site
	// alongside — or instead of — the primary `source` tree, so content that
	// already lives elsewhere (a docs/ folder, notes beside the code) needs no
	// copy. Empty by default, which keeps single-source builds unchanged.
	ContentSources []ContentSource `yaml:"content_sources" toml:"content_sources" json:"content_sources"`

	// LinkRewrites maps an href prefix in content to its replacement, so links
	// to repository files that the site never publishes (../examples/, a sample
	// config) can point at the repository instead of 404ing. Longest matching
	// prefix wins; empty by default (LINK-002).
	LinkRewrites map[string]string `yaml:"link_rewrites" toml:"link_rewrites" json:"link_rewrites"`

	// AutoExcerpt derives a missing excerpt from the opening paragraph of the
	// content, so listings, feeds and meta descriptions are not blank for
	// documents written without a "## Excerpt" section. Off by default,
	// because it changes those texts on an existing site (GO-057).
	AutoExcerpt bool `yaml:"auto_excerpt" toml:"auto_excerpt" json:"auto_excerpt"`

	// ShortcodeErrors decides what a shortcode whose template fails to render
	// leaves in the page: "" / "drop" (default, historical behaviour — a warning
	// and nothing in the page), "keep" (its raw source, so the gap is visible in
	// the output) or "strict" (keep, and fail the build). Issue #37.
	ShortcodeErrors string `yaml:"shortcode_errors" toml:"shortcode_errors" json:"shortcode_errors"`

	// Headers overrides or extends the generated Cloudflare Pages _headers file
	// per path pattern (e.g. "/*", "/api/*"): a pattern present here replaces
	// that pattern's default header block, unknown patterns are appended.
	// HeadersDefaultsOff drops the built-in security/cache blocks entirely.
	// Empty = the historical hardcoded output (GO-064).
	Headers            map[string]map[string]string `yaml:"headers" toml:"headers" json:"headers"`
	HeadersDefaultsOff bool                         `yaml:"headers_defaults_off" toml:"headers_defaults_off" json:"headers_defaults_off"`

	// Redirects declares real _redirects rules for Cloudflare Pages / Netlify:
	// exact paths, /old/* splats with :splat, and status 301/302/307/308/410.
	// Frontmatter aliases: are added as 301s automatically; exact chains
	// A→B→C are flattened to A→C at build time. AliasStubs (default true) also
	// writes the meta-refresh stub pages as a fallback for non-CF hosts; set it
	// false to keep only the _redirects entries (GO-063).
	Redirects  []RedirectRule `yaml:"redirects" toml:"redirects" json:"redirects"`
	AliasStubs *bool          `yaml:"alias_stubs" toml:"alias_stubs" json:"alias_stubs"`

	// Worker wires a Cloudflare Pages Functions directory (or a prebuilt
	// _worker.js) into the build output and generates _routes.json, so
	// transactional endpoints (Stripe, forms, dynamic pricing) live beside the
	// static site (GO-065). Empty = static-only build (unchanged).
	Worker WorkerConfig `yaml:"worker" toml:"worker" json:"worker"`

	// Other
	Quiet bool `yaml:"quiet" toml:"quiet" json:"quiet"`
}

// RedirectRule is one entry in the redirects: list; see Config.Redirects.
type RedirectRule struct {
	From   string `yaml:"from" toml:"from" json:"from"`
	To     string `yaml:"to" toml:"to" json:"to"`
	Status int    `yaml:"status" toml:"status" json:"status"`
	Force  bool   `yaml:"force" toml:"force" json:"force"`
}

// WorkerConfig wires a Cloudflare Pages Functions / Worker project into the
// build; see Config.Worker.
type WorkerConfig struct {
	// Dir is the source directory: a Pages Functions tree (mode "functions",
	// the default) or a directory holding a prebuilt _worker.js (mode "worker").
	Dir  string `yaml:"dir" toml:"dir" json:"dir"`
	Mode string `yaml:"mode" toml:"mode" json:"mode"`
	// RoutesInclude/RoutesExclude become _routes.json (default include
	// ["/api/*"]) so static assets bypass the Worker.
	RoutesInclude []string `yaml:"routes_include" toml:"routes_include" json:"routes_include"`
	RoutesExclude []string `yaml:"routes_exclude" toml:"routes_exclude" json:"routes_exclude"`
	// WranglerConfig points dev/deploy at a wrangler config outside the project
	// root; reused by the --wrangler watch runner (GO-054).
	WranglerConfig string `yaml:"wrangler_config" toml:"wrangler_config" json:"wrangler_config"`
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
	var expanded []ssgi18n.LanguageConfig
	switch ext {
	case ".yaml", ".yml":
		data, expanded, err = normalizeYAMLLanguages(data)
	case ".json":
		data, expanded, err = normalizeJSONLanguages(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing expanded languages: %w", err)
	}

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
	cfg.LanguageConfigs = expanded
	cfg.I18n = cfg.I18n.WithDefaults()

	// Apply minify_all
	if cfg.MinifyAll {
		cfg.MinifyHTML = true
		cfg.MinifyCSS = true
		cfg.MinifyJS = true
	}

	// Honour the deprecated seo_off key instead of silently ignoring it (GO-059).
	if cfg.SEOOff {
		cfg.SEO = false
		fmt.Fprintln(os.Stderr, "⚠️  Config key seo_off is deprecated; use seo: false")
	}

	warnUnknownKeys(path, data, ext)

	return cfg, nil
}

// warnUnknownKeys reports top-level configuration keys this binary does not
// know. Silently ignoring them makes a version mismatch invisible: a config
// using a newer key (content_sources, say) against an older ssg simply behaves
// as if the key were not there, and the resulting "missing source" is
// impossible to connect back to its cause (UX-002). Unknown keys are a warning,
// never an error — forward compatibility with newer configs is deliberate.
func warnUnknownKeys(path string, data []byte, ext string) {
	if ext != ".yaml" && ext != ".yml" {
		return // strict decoding is YAML-only for now; TOML/JSON keys pass through
	}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	var probe Config
	err := strict.Decode(&probe)
	if err == nil {
		return
	}
	for _, line := range strings.Split(err.Error(), "\n") {
		key, ok := unknownFieldName(line)
		if !ok {
			continue
		}
		fmt.Fprintf(os.Stderr,
			"⚠️  %s: unknown configuration key %q — ignored. Check the spelling, or upgrade ssg if the key is newer than this build (%s).\n",
			filepath.Base(path), key, docsURL)
	}
}

// ApplyWorkerWatchDefaults makes `worker:` the single source of truth for the
// dev runner: when a worker is configured and no explicit watch_runner is set,
// `wrangler dev` runs from the worker directory so the static preview and the
// Functions run side by side. Called only once watch mode is active (a plain
// one-shot build never starts a runner). Explicit watch_runner_* keys (GO-054)
// still win as overrides (GO-065).
func ApplyWorkerWatchDefaults(cfg *Config) {
	if cfg.Worker.Dir == "" || cfg.WatchRunner != "" {
		return
	}
	cfg.WatchRunner = "wrangler"
	if cfg.WatchRunnerDir == "" {
		cfg.WatchRunnerDir = cfg.Worker.Dir
	}
	if cfg.WatchRunnerConfig == "" {
		cfg.WatchRunnerConfig = cfg.Worker.WranglerConfig
	}
}

// docsURL points at the configuration reference named in the warning above.
const docsURL = "https://github.com/spagu/ssg/blob/main/docs/CONFIGURATION.md"

// unknownFieldNameRe matches yaml.v3's strict-decoding complaint, e.g.
// `line 12: field content_sources not found in type config.Config`.
var unknownFieldNameRe = regexp.MustCompile(`field ([A-Za-z0-9_.-]+) not found`)

// unknownFieldName extracts the offending key from one yaml.v3 error line.
func unknownFieldName(line string) (string, bool) {
	m := unknownFieldNameRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func normalizeYAMLLanguages(data []byte) ([]byte, []ssgi18n.LanguageConfig, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data, nil, err
	}
	if len(doc.Content) == 0 || len(doc.Content[0].Content) == 0 {
		return data, nil, nil
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "languages" {
			continue
		}
		value := root.Content[i+1]
		if value.Kind != yaml.SequenceNode || len(value.Content) == 0 || value.Content[0].Kind != yaml.MappingNode {
			return data, nil, nil
		}
		var expanded []ssgi18n.LanguageConfig
		if err := value.Decode(&expanded); err != nil {
			return data, nil, err
		}
		codes := make([]string, len(expanded))
		for j := range expanded {
			codes[j] = expanded[j].Code
		}
		var replacement yaml.Node
		if err := replacement.Encode(codes); err != nil {
			return data, nil, err
		}
		root.Content[i+1] = &replacement
		out, err := yaml.Marshal(&doc)
		return out, expanded, err
	}
	return data, nil, nil
}

func normalizeJSONLanguages(data []byte) ([]byte, []ssgi18n.LanguageConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return data, nil, err
	}
	v, ok := raw["languages"]
	if !ok {
		return data, nil, nil
	}
	var expanded []ssgi18n.LanguageConfig
	if err := json.Unmarshal(v, &expanded); err != nil || len(expanded) == 0 || expanded[0].Code == "" {
		return data, nil, nil
	}
	codes := make([]string, len(expanded))
	for i := range expanded {
		codes[i] = expanded[i].Code
	}
	raw["languages"], _ = json.Marshal(codes)
	out, err := json.Marshal(raw)
	return out, expanded, err
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
