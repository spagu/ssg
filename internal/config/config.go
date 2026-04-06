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
	HTTP  bool `yaml:"http" toml:"http" json:"http"`
	Port  int  `yaml:"port" toml:"port" json:"port"`
	Watch bool `yaml:"watch" toml:"watch" json:"watch"`
	Clean bool `yaml:"clean" toml:"clean" json:"clean"`

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
	SourceMap     bool   `yaml:"sourcemap" toml:"sourcemap" json:"sourcemap"`

	// Shortcodes
	Shortcodes []Shortcode `yaml:"shortcodes" toml:"shortcodes" json:"shortcodes"`

	// Variables defines custom variables available in all templates as {{.Vars.key}}
	// and exported as environment variables with SSG_ prefix (e.g. SSG_GTM).
	// Values starting with $ are resolved from the current environment (e.g. "$GTM_CODE").
	Variables map[string]interface{} `yaml:"variables" toml:"variables" json:"variables"`

	// PagesPath is the subdirectory name inside source for static pages (default: "pages")
	PagesPath string `yaml:"pages_path" toml:"pages_path" json:"pages_path"`
	// PostsPath is the subdirectory name inside source for blog posts (default: "posts")
	PostsPath string `yaml:"posts_path" toml:"posts_path" json:"posts_path"`

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

	// Deployment
	Zip bool `yaml:"zip" toml:"zip" json:"zip"`

	// Other
	Quiet bool `yaml:"quiet" toml:"quiet" json:"quiet"`
}

// DefaultConfig returns configuration with default values
func DefaultConfig() *Config {
	return &Config{
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
		Port:         8888,
		WebPQuality:  60,
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
