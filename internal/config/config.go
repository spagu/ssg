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
	Data     map[string]string `yaml:"data" toml:"data" json:"data"`             // Additional custom data
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
	RelativeLinks bool   `yaml:"relative_links" toml:"relative_links" json:"relative_links"`    // Convert absolute URLs to relative
	MinifyAll     bool   `yaml:"minify_all" toml:"minify_all" json:"minify_all"`
	MinifyHTML    bool   `yaml:"minify_html" toml:"minify_html" json:"minify_html"`
	MinifyCSS     bool   `yaml:"minify_css" toml:"minify_css" json:"minify_css"`
	MinifyJS      bool   `yaml:"minify_js" toml:"minify_js" json:"minify_js"`
	SourceMap     bool   `yaml:"sourcemap" toml:"sourcemap" json:"sourcemap"`

	// Shortcodes
	Shortcodes []Shortcode `yaml:"shortcodes" toml:"shortcodes" json:"shortcodes"`

	// Image Processing
	WebP        bool `yaml:"webp" toml:"webp" json:"webp"`
	WebPQuality int  `yaml:"webp_quality" toml:"webp_quality" json:"webp_quality"`

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
