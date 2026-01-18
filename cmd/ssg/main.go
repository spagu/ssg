// Package main provides the entry point for the SSG (Static Site Generator) CLI tool.
// Usage: ssg <source> <template> <domain> [options]
// Example: ssg krowy.net.2026-01-13110345 simple krowy.net --zip --webp
// Example: ssg my-content my-template example.com --http --watch
// Example: ssg --config .ssg.yaml
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/engine"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/theme"
	"github.com/spagu/ssg/internal/webp"
)

// Version is set by build flags
var Version = "dev"

func main() {
	args := os.Args[1:]

	// Load configuration
	cfg := loadConfig(args)

	// Override with command line flags
	parseFlags(args, cfg)

	// Validate required fields
	validateRequiredFields(args, cfg)

	// Apply minify_all
	if cfg.MinifyAll {
		cfg.MinifyHTML = true
		cfg.MinifyCSS = true
		cfg.MinifyJS = true
	}

	// Setup template engine and theme
	setupTemplateEngine(cfg)
	downloadOnlineTheme(cfg)

	// Create generator config
	genCfg := createGeneratorConfig(cfg)

	// Initial build
	if err := build(genCfg, cfg); err != nil {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		}
		if !cfg.Watch && !cfg.HTTP {
			os.Exit(1)
		}
	} else if !cfg.Quiet {
		fmt.Printf("✅ Site generated successfully to %s/\n", cfg.OutputDir)
	}

	// Start HTTP server if requested
	if cfg.HTTP {
		go startServer(cfg.OutputDir, cfg.Port, cfg.Quiet)
	}

	// Watch mode
	if cfg.Watch {
		if !cfg.Quiet {
			fmt.Println("👀 Watching for changes in content and templates...")
		}
		lastBuild := time.Now()

		for {
			time.Sleep(1 * time.Second)

			if hasChanges([]string{cfg.ContentDir, cfg.TemplatesDir}, lastBuild) {
				if !cfg.Quiet {
					fmt.Println("\n🔄 Changes detected! Rebuilding...")
				}
				if err := build(genCfg, cfg); err != nil {
					if !cfg.Quiet {
						fmt.Fprintf(os.Stderr, "❌ Build error: %v\n", err)
						fmt.Println("⚠️  Fix the issue and save to retry...")
					}
				} else if !cfg.Quiet {
					fmt.Printf("✅ Rebuilt successfully\n")
				}
				lastBuild = time.Now()
				if !cfg.Quiet {
					fmt.Println("👀 Watching for changes...")
				}
			}
		}
	} else if cfg.HTTP {
		select {}
	}
}

// loadConfig loads configuration from file or returns defaults
func loadConfig(args []string) *config.Config {
	var configPath string

	// Look for --config flag
	for i, arg := range args {
		if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
		} else if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
		}
	}

	// If no --config, look for default config file
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	// Load config file if exists
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error loading config: %v\n", err)
			os.Exit(1)
		}
		return cfg
	}

	return config.DefaultConfig()
}

// validateRequiredFields validates and populates required config fields
func validateRequiredFields(args []string, cfg *config.Config) {
	if cfg.Source != "" && cfg.Template != "" && cfg.Domain != "" {
		return
	}

	// Check positional args
	var positionalArgs []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
		}
	}

	if len(positionalArgs) >= 3 {
		cfg.Source = positionalArgs[0]
		cfg.Template = positionalArgs[1]
		cfg.Domain = positionalArgs[2]
	} else if cfg.Source == "" || cfg.Template == "" || cfg.Domain == "" {
		printUsage()
		os.Exit(1)
	}
}

// setupTemplateEngine validates the template engine
func setupTemplateEngine(cfg *config.Config) {
	if cfg.Engine == "" {
		return
	}

	if _, err := engine.New(cfg.Engine); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	if !cfg.Quiet {
		fmt.Printf("🔧 Using template engine: %s\n", cfg.Engine)
	}
}

// downloadOnlineTheme downloads theme from URL if specified
func downloadOnlineTheme(cfg *config.Config) {
	if cfg.OnlineTheme == "" {
		return
	}

	themeDir := filepath.Join(cfg.TemplatesDir, cfg.Template)
	if !cfg.Quiet {
		fmt.Printf("🌐 Downloading theme from: %s\n", cfg.OnlineTheme)
	}

	if err := theme.Download(cfg.OnlineTheme, themeDir); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error downloading theme: %v\n", err)
		os.Exit(1)
	}
}

// createGeneratorConfig creates generator.Config from app config
func createGeneratorConfig(cfg *config.Config) generator.Config {
	return generator.Config{
		Source:       cfg.Source,
		Template:     cfg.Template,
		Domain:       cfg.Domain,
		ContentDir:   cfg.ContentDir,
		TemplatesDir: cfg.TemplatesDir,
		OutputDir:    cfg.OutputDir,
		SitemapOff:   cfg.SitemapOff,
		RobotsOff:    cfg.RobotsOff,
		MinifyHTML:   cfg.MinifyHTML,
		MinifyCSS:    cfg.MinifyCSS,
		MinifyJS:     cfg.MinifyJS,
		SourceMap:    cfg.SourceMap,
		Clean:        cfg.Clean,
		Quiet:        cfg.Quiet,
		Engine:       cfg.Engine,
	}
}

func parseFlags(args []string, cfg *config.Config) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--zip" || arg == "-zip":
			cfg.Zip = true
		case arg == "--webp" || arg == "-webp":
			cfg.WebP = true
		case strings.HasPrefix(arg, "--webp-quality="):
			if q, err := strconv.Atoi(strings.TrimPrefix(arg, "--webp-quality=")); err == nil && q >= 1 && q <= 100 {
				cfg.WebPQuality = q
			}
		case arg == "--webp-quality" && i+1 < len(args):
			i++
			if q, err := strconv.Atoi(args[i]); err == nil && q >= 1 && q <= 100 {
				cfg.WebPQuality = q
			}
		case arg == "--watch" || arg == "-watch":
			cfg.Watch = true
		case arg == "--http" || arg == "-http":
			cfg.HTTP = true
		case strings.HasPrefix(arg, "--port="):
			if port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil {
				cfg.Port = port
			}
		case arg == "--port" && i+1 < len(args):
			i++
			if port, err := strconv.Atoi(args[i]); err == nil {
				cfg.Port = port
			}
		case strings.HasPrefix(arg, "--content-dir="):
			cfg.ContentDir = strings.TrimPrefix(arg, "--content-dir=")
		case strings.HasPrefix(arg, "--templates-dir="):
			cfg.TemplatesDir = strings.TrimPrefix(arg, "--templates-dir=")
		case strings.HasPrefix(arg, "--output-dir="):
			cfg.OutputDir = strings.TrimPrefix(arg, "--output-dir=")
		case arg == "--content-dir" && i+1 < len(args):
			i++
			cfg.ContentDir = args[i]
		case arg == "--templates-dir" && i+1 < len(args):
			i++
			cfg.TemplatesDir = args[i]
		case arg == "--output-dir" && i+1 < len(args):
			i++
			cfg.OutputDir = args[i]
		case arg == "--sitemap-off":
			cfg.SitemapOff = true
		case arg == "--robots-off":
			cfg.RobotsOff = true
		case arg == "--minify-all":
			cfg.MinifyAll = true
		case arg == "--minify-html":
			cfg.MinifyHTML = true
		case arg == "--minify-css":
			cfg.MinifyCSS = true
		case arg == "--minify-js":
			cfg.MinifyJS = true
		case arg == "--sourcemap":
			cfg.SourceMap = true
		case arg == "--clean":
			cfg.Clean = true
		case arg == "--quiet" || arg == "-q":
			cfg.Quiet = true
		case strings.HasPrefix(arg, "--engine="):
			cfg.Engine = strings.TrimPrefix(arg, "--engine=")
		case arg == "--engine" && i+1 < len(args):
			i++
			cfg.Engine = args[i]
		case strings.HasPrefix(arg, "--online-theme="):
			cfg.OnlineTheme = strings.TrimPrefix(arg, "--online-theme=")
		case arg == "--online-theme" && i+1 < len(args):
			i++
			cfg.OnlineTheme = args[i]
		case arg == "--version" || arg == "-v":
			fmt.Printf("ssg version %s\n", Version)
			os.Exit(0)
		case arg == "--help" || arg == "-h":
			printUsage()
			os.Exit(0)
		case strings.HasPrefix(arg, "--config"):
			// Skip, already processed
			if arg == "--config" {
				i++
			}
		}
	}
}

func startServer(dir string, port int, quiet bool) {
	addr := fmt.Sprintf(":%d", port)
	if !quiet {
		fmt.Printf("🌐 Starting HTTP server at http://localhost%s\n", addr)
		fmt.Printf("   Serving files from: %s/\n", dir)
	}

	fs := http.FileServer(http.Dir(dir))
	http.Handle("/", fs)

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
	}
}

func build(genCfg generator.Config, cfg *config.Config) error {
	gen, err := generator.New(genCfg)
	if err != nil {
		return fmt.Errorf("initializing generator: %w", err)
	}

	if err := gen.Generate(); err != nil {
		return fmt.Errorf("generating site: %w", err)
	}

	// Convert images to WebP if requested (using native Go library)
	if cfg.WebP {
		if !cfg.Quiet {
			fmt.Printf("🖼️  Converting images to WebP (quality: %d)...\n", cfg.WebPQuality)
		}

		opts := webp.ConvertOptions{
			Quality: cfg.WebPQuality,
			Quiet:   cfg.Quiet,
		}

		converted, saved, err := webp.ConvertDirectory(cfg.OutputDir, opts)
		if err != nil {
			return fmt.Errorf("converting to WebP: %w", err)
		}

		if err := webp.UpdateReferences(cfg.OutputDir); err != nil {
			return fmt.Errorf("updating image references: %w", err)
		}

		if !cfg.Quiet {
			savedMB := float64(saved) / (1024 * 1024)
			fmt.Printf("   📊 Converted %d images, saved %.1f MB\n", converted, savedMB)
			fmt.Println("✅ Images converted to WebP")
		}
	}

	// Create ZIP if requested
	if cfg.Zip {
		zipFileName := fmt.Sprintf("%s.zip", cfg.Domain)
		if err := createZip(cfg.OutputDir, zipFileName); err != nil {
			return fmt.Errorf("creating ZIP: %w", err)
		}

		if !cfg.Quiet {
			if info, err := os.Stat(zipFileName); err == nil {
				sizeMB := float64(info.Size()) / (1024 * 1024)
				fmt.Printf("📦 Created deployment package: %s (%.1f MB)\n", zipFileName, sizeMB)
				if sizeMB > 25 {
					fmt.Printf("⚠️  Warning: File exceeds Cloudflare Pages 25MB limit!\n")
				}
			}
		}
	}
	return nil
}

func hasChanges(dirs []string, lastBuild time.Time) bool {
	changed := false
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				if info.ModTime().After(lastBuild) {
					changed = true
					return io.EOF
				}
			}
			return nil
		})
		if changed {
			break
		}
	}
	return changed
}

func createZip(sourceDir, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	zipWriter := zip.NewWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == sourceDir {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("getting relative path: %w", err)
		}

		relPath = strings.ReplaceAll(relPath, string(os.PathSeparator), "/")

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("creating file header: %w", err)
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
			_, err = zipWriter.CreateHeader(header)
			return err
		}

		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("creating zip entry: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer func() { _ = file.Close() }()

		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("walking directory: %w", err)
	}

	return nil
}

func printUsage() {
	fmt.Println("SSG - Static Site Generator")
	fmt.Println("")
	fmt.Println("Usage: ssg <source> <template> <domain> [options]")
	fmt.Println("       ssg --config .ssg.yaml")
	fmt.Println("")
	fmt.Println("Arguments:")
	fmt.Println("  source    - Content source folder name (inside content-dir)")
	fmt.Println("  template  - Template name (inside templates-dir)")
	fmt.Println("  domain    - Target domain for the generated site")
	fmt.Println("")
	fmt.Println("Configuration:")
	fmt.Println("  --config=FILE          - Load config from YAML/TOML/JSON file")
	fmt.Println("                           Auto-detects: .ssg.yaml, .ssg.toml, .ssg.json")
	fmt.Println("")
	fmt.Println("Template Engine:")
	fmt.Println("  --engine=ENGINE        - Template engine (default: go)")
	fmt.Println("                           Available: go, pongo2 (jinja2), mustache, handlebars")
	fmt.Println("  --online-theme=URL     - Download theme from URL (GitHub, GitLab, or direct ZIP)")
	fmt.Println("                           Example: --online-theme=https://github.com/user/hugo-theme")
	fmt.Println("")
	fmt.Println("Server & Development:")
	fmt.Println("  --http                 - Start built-in HTTP server")
	fmt.Println("  --port=PORT            - HTTP server port (default: 8888)")
	fmt.Println("  --watch                - Watch for changes and rebuild automatically")
	fmt.Println("  --clean                - Clean output directory before build")
	fmt.Println("")
	fmt.Println("Output Control:")
	fmt.Println("  --sitemap-off          - Disable sitemap.xml generation")
	fmt.Println("  --robots-off           - Disable robots.txt generation")
	fmt.Println("  --minify-all           - Minify HTML, CSS, and JS")
	fmt.Println("  --minify-html          - Minify HTML output")
	fmt.Println("  --minify-css           - Minify CSS output")
	fmt.Println("  --minify-js            - Minify JS output")
	fmt.Println("  --sourcemap            - Include source maps in output")
	fmt.Println("")
	fmt.Println("Image Processing:")
	fmt.Println("  --webp                 - Convert images to WebP format (requires cwebp)")
	fmt.Println("  --webp-quality=N       - WebP compression quality 1-100 (default: 60)")
	fmt.Println("")
	fmt.Println("Deployment:")
	fmt.Println("  --zip                  - Create ZIP file for Cloudflare Pages")
	fmt.Println("")
	fmt.Println("Paths:")
	fmt.Println("  --content-dir=PATH     - Content directory (default: content)")
	fmt.Println("  --templates-dir=PATH   - Templates directory (default: templates)")
	fmt.Println("  --output-dir=PATH      - Output directory (default: output)")
	fmt.Println("")
	fmt.Println("Other:")
	fmt.Println("  --quiet, -q            - Suppress output (only exit codes)")
	fmt.Println("  --version, -v          - Show version")
	fmt.Println("  --help, -h             - Show this help")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ssg my-site simple example.com --http --watch")
	fmt.Println("  ssg my-site krowy example.com --clean --minify-all --zip")
	fmt.Println("  ssg my-site mytheme example.com --engine=pongo2")
	fmt.Println("  ssg my-site themename example.com --online-theme=https://github.com/user/hugo-theme")
	fmt.Println("  ssg --config .ssg.yaml --http --watch")
}
