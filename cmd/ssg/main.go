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
	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/theme"
	"github.com/spagu/ssg/internal/webp"
)

// Version is set by build flags
var Version = "dev"

func main() {
	args := os.Args[1:]

	cfg := loadConfig(args)
	parseFlags(args, cfg)
	validateRequiredFields(args, cfg)
	applyMinifyAll(cfg)
	setupTemplateEngine(cfg)
	downloadOnlineTheme(cfg)

	genCfg := createGeneratorConfig(cfg)

	if !runInitialBuild(genCfg, cfg) && !cfg.Watch && !cfg.HTTP {
		os.Exit(1)
	}

	if cfg.HTTP {
		go startServer(cfg.OutputDir, cfg.Port, cfg.Quiet)
	}

	runWatchOrServe(genCfg, cfg)
}

// applyMinifyAll sets all minify flags if minify_all is enabled
func applyMinifyAll(cfg *config.Config) {
	if cfg.MinifyAll {
		cfg.MinifyHTML = true
		cfg.MinifyCSS = true
		cfg.MinifyJS = true
	}
}

// runInitialBuild performs the initial site build, returns true on success
func runInitialBuild(genCfg generator.Config, cfg *config.Config) bool {
	if err := build(genCfg, cfg); err != nil {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		}
		return false
	}
	if !cfg.Quiet {
		fmt.Printf("✅ Site generated successfully to %s/\n", cfg.OutputDir)
	}
	return true
}

// runWatchOrServe handles watch mode loop or HTTP server blocking
func runWatchOrServe(genCfg generator.Config, cfg *config.Config) {
	if cfg.Mddb.Watch && cfg.Mddb.Enabled {
		runMddbWatchLoop(genCfg, cfg)
	} else if cfg.Watch {
		runWatchLoop(genCfg, cfg)
	} else if cfg.HTTP {
		select {}
	}
}

// runWatchLoop continuously watches for file changes and rebuilds
func runWatchLoop(genCfg generator.Config, cfg *config.Config) {
	if !cfg.Quiet {
		fmt.Println("👀 Watching for changes in content and templates...")
	}
	lastBuild := time.Now()

	for {
		time.Sleep(1 * time.Second)
		if hasChanges([]string{cfg.ContentDir, cfg.TemplatesDir}, lastBuild) {
			lastBuild = rebuildOnChange(genCfg, cfg)
		}
	}
}

// runMddbWatchLoop continuously polls MDDB checksum and rebuilds on changes
func runMddbWatchLoop(genCfg generator.Config, cfg *config.Config) {
	client, err := mddb.NewMddbClient(mddb.ClientConfig{
		URL:       cfg.Mddb.URL,
		Protocol:  cfg.Mddb.Protocol,
		APIKey:    cfg.Mddb.APIKey,
		Timeout:   cfg.Mddb.Timeout,
		BatchSize: cfg.Mddb.BatchSize,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error creating MDDB client: %v\n", err)
		return
	}

	interval := cfg.Mddb.WatchInterval
	if interval <= 0 {
		interval = 30
	}

	if !cfg.Quiet {
		fmt.Printf("👀 Watching MDDB collection '%s' for changes (interval: %ds)...\n",
			cfg.Mddb.Collection, interval)
	}

	var lastChecksum string

	// Get initial checksum
	checksumResp, err := client.Checksum(cfg.Mddb.Collection)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Could not get initial checksum: %v\n", err)
	} else {
		lastChecksum = checksumResp.Checksum
		if !cfg.Quiet {
			fmt.Printf("   📋 Initial checksum: %s (%d documents)\n",
				lastChecksum, checksumResp.DocumentCount)
		}
	}

	for {
		time.Sleep(time.Duration(interval) * time.Second)

		checksumResp, err := client.Checksum(cfg.Mddb.Collection)
		if err != nil {
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "⚠️  Checksum check failed: %v\n", err)
			}
			continue
		}

		if checksumResp.Checksum != lastChecksum {
			if !cfg.Quiet {
				fmt.Printf("\n🔄 MDDB content changed! Checksum: %s → %s (%d docs)\n",
					lastChecksum, checksumResp.Checksum, checksumResp.DocumentCount)
			}
			lastChecksum = checksumResp.Checksum
			rebuildOnChange(genCfg, cfg)
		}
	}
}

// rebuildOnChange handles rebuilding when changes are detected
func rebuildOnChange(genCfg generator.Config, cfg *config.Config) time.Time {
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
	if !cfg.Quiet {
		fmt.Println("👀 Watching for changes...")
	}
	return time.Now()
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
	// Convert shortcodes from config to generator format
	shortcodes := make([]generator.Shortcode, len(cfg.Shortcodes))
	for i, sc := range cfg.Shortcodes {
		shortcodes[i] = generator.Shortcode{
			Name:     sc.Name,
			Type:     sc.Type,
			Template: sc.Template,
			Title:    sc.Title,
			Text:     sc.Text,
			Url:      sc.Url,
			Logo:     sc.Logo,
			Legal:    sc.Legal,
			Ranking:  sc.Ranking,
			Tags:     sc.Tags,
			Data:     sc.Data,
		}
	}

	return generator.Config{
		Source:        cfg.Source,
		Template:      cfg.Template,
		Domain:        cfg.Domain,
		ContentDir:    cfg.ContentDir,
		TemplatesDir:  cfg.TemplatesDir,
		OutputDir:     cfg.OutputDir,
		SitemapOff:    cfg.SitemapOff,
		RobotsOff:     cfg.RobotsOff,
		PrettyHTML:    cfg.PrettyHTML,
		PostURLFormat: cfg.PostURLFormat,
		RelativeLinks: cfg.RelativeLinks,
		Shortcodes:    shortcodes,
		MinifyHTML:    cfg.MinifyHTML,
		MinifyCSS:     cfg.MinifyCSS,
		MinifyJS:      cfg.MinifyJS,
		SourceMap:     cfg.SourceMap,
		Clean:         cfg.Clean,
		Quiet:         cfg.Quiet,
		Engine:        cfg.Engine,
		Mddb: generator.MddbConfig{
			Enabled:       cfg.Mddb.Enabled,
			URL:           cfg.Mddb.URL,
			Protocol:      cfg.Mddb.Protocol,
			APIKey:        cfg.Mddb.APIKey,
			Collection:    cfg.Mddb.Collection,
			Lang:          cfg.Mddb.Lang,
			Timeout:       cfg.Mddb.Timeout,
			BatchSize:     cfg.Mddb.BatchSize,
			Watch:         cfg.Mddb.Watch,
			WatchInterval: cfg.Mddb.WatchInterval,
		},
	}
}

func parseFlags(args []string, cfg *config.Config) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		skip := parseBoolFlags(arg, cfg)
		if skip {
			continue
		}
		skip = parseSpecialFlags(arg)
		if skip {
			continue
		}
		i += parseValueFlags(args, i, cfg)
	}
}

// parseBoolFlags handles boolean flags, returns true if flag was handled
func parseBoolFlags(arg string, cfg *config.Config) bool {
	switch arg {
	case "--zip", "-zip":
		cfg.Zip = true
	case "--webp", "-webp":
		cfg.WebP = true
	case "--reconvert-images":
		cfg.ReconvertImages = true
	case "--watch", "-watch":
		cfg.Watch = true
	case "--http", "-http":
		cfg.HTTP = true
	case "--sitemap-off":
		cfg.SitemapOff = true
	case "--robots-off":
		cfg.RobotsOff = true
	case "--pretty-html", "--pretty":
		cfg.PrettyHTML = true
	case "--relative-links":
		cfg.RelativeLinks = true
	case "--minify-all":
		cfg.MinifyAll = true
	case "--minify-html":
		cfg.MinifyHTML = true
	case "--minify-css":
		cfg.MinifyCSS = true
	case "--minify-js":
		cfg.MinifyJS = true
	case "--sourcemap":
		cfg.SourceMap = true
	case "--clean":
		cfg.Clean = true
	case "--quiet", "-q":
		cfg.Quiet = true
	default:
		return false
	}
	return true
}

// parseSpecialFlags handles --version and --help
func parseSpecialFlags(arg string) bool {
	switch arg {
	case "--version", "-v":
		fmt.Printf("ssg version %s\n", Version)
		os.Exit(0)
	case "--help", "-h":
		printUsage()
		os.Exit(0)
	default:
		return false
	}
	return true
}

// parseValueFlags handles flags with values, returns number of args to skip
func parseValueFlags(args []string, i int, cfg *config.Config) int {
	arg := args[i]
	skip := 0

	// Handle --flag=value format
	if strings.Contains(arg, "=") {
		parseEqualFlags(arg, cfg)
		return 0
	}

	// Handle --flag value format
	skip = parseSeparateValueFlags(args, i, cfg)
	return skip
}

// parseEqualFlags handles --flag=value format
func parseEqualFlags(arg string, cfg *config.Config) {
	switch {
	case strings.HasPrefix(arg, "--webp-quality="):
		if q, err := strconv.Atoi(strings.TrimPrefix(arg, "--webp-quality=")); err == nil && q >= 1 && q <= 100 {
			cfg.WebPQuality = q
		}
	case strings.HasPrefix(arg, "--port="):
		if port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil {
			cfg.Port = port
		}
	case strings.HasPrefix(arg, "--content-dir="):
		cfg.ContentDir = strings.TrimPrefix(arg, "--content-dir=")
	case strings.HasPrefix(arg, "--templates-dir="):
		cfg.TemplatesDir = strings.TrimPrefix(arg, "--templates-dir=")
	case strings.HasPrefix(arg, "--output-dir="):
		cfg.OutputDir = strings.TrimPrefix(arg, "--output-dir=")
	case strings.HasPrefix(arg, "--engine="):
		cfg.Engine = strings.TrimPrefix(arg, "--engine=")
	case strings.HasPrefix(arg, "--online-theme="):
		cfg.OnlineTheme = strings.TrimPrefix(arg, "--online-theme=")
	case strings.HasPrefix(arg, "--post-url-format="):
		cfg.PostURLFormat = strings.TrimPrefix(arg, "--post-url-format=")
	// MDDB flags
	case strings.HasPrefix(arg, "--mddb-url="):
		cfg.Mddb.URL = strings.TrimPrefix(arg, "--mddb-url=")
		cfg.Mddb.Enabled = true
	case strings.HasPrefix(arg, "--mddb-key="):
		cfg.Mddb.APIKey = strings.TrimPrefix(arg, "--mddb-key=")
	case strings.HasPrefix(arg, "--mddb-collection="):
		cfg.Mddb.Collection = strings.TrimPrefix(arg, "--mddb-collection=")
	case strings.HasPrefix(arg, "--mddb-lang="):
		cfg.Mddb.Lang = strings.TrimPrefix(arg, "--mddb-lang=")
	case strings.HasPrefix(arg, "--mddb-timeout="):
		if t, err := strconv.Atoi(strings.TrimPrefix(arg, "--mddb-timeout=")); err == nil && t > 0 {
			cfg.Mddb.Timeout = t
		}
	case strings.HasPrefix(arg, "--mddb-batch-size="):
		if b, err := strconv.Atoi(strings.TrimPrefix(arg, "--mddb-batch-size=")); err == nil && b > 0 {
			cfg.Mddb.BatchSize = b
		}
	case strings.HasPrefix(arg, "--mddb-protocol="):
		protocol := strings.TrimPrefix(arg, "--mddb-protocol=")
		if protocol == "http" || protocol == "grpc" {
			cfg.Mddb.Protocol = protocol
		}
	case arg == "--mddb-watch":
		cfg.Mddb.Watch = true
	case strings.HasPrefix(arg, "--mddb-watch-interval="):
		if i, err := strconv.Atoi(strings.TrimPrefix(arg, "--mddb-watch-interval=")); err == nil && i > 0 {
			cfg.Mddb.WatchInterval = i
		}
	}
}

// parseSeparateValueFlags handles --flag value format, returns skip count
func parseSeparateValueFlags(args []string, i int, cfg *config.Config) int {
	arg := args[i]
	if i+1 >= len(args) {
		return handleConfigSkip(arg)
	}

	nextArg := args[i+1]
	switch arg {
	case "--webp-quality":
		if q, err := strconv.Atoi(nextArg); err == nil && q >= 1 && q <= 100 {
			cfg.WebPQuality = q
		}
		return 1
	case "--port":
		if port, err := strconv.Atoi(nextArg); err == nil {
			cfg.Port = port
		}
		return 1
	case "--content-dir":
		cfg.ContentDir = nextArg
		return 1
	case "--templates-dir":
		cfg.TemplatesDir = nextArg
		return 1
	case "--output-dir":
		cfg.OutputDir = nextArg
		return 1
	case "--engine":
		cfg.Engine = nextArg
		return 1
	case "--online-theme":
		cfg.OnlineTheme = nextArg
		return 1
	case "--post-url-format":
		cfg.PostURLFormat = nextArg
		return 1
	case "--config":
		return 1 // Skip, already processed
	// MDDB flags
	case "--mddb-url":
		cfg.Mddb.URL = nextArg
		cfg.Mddb.Enabled = true
		return 1
	case "--mddb-key":
		cfg.Mddb.APIKey = nextArg
		return 1
	case "--mddb-collection":
		cfg.Mddb.Collection = nextArg
		return 1
	case "--mddb-lang":
		cfg.Mddb.Lang = nextArg
		return 1
	case "--mddb-timeout":
		if t, err := strconv.Atoi(nextArg); err == nil && t > 0 {
			cfg.Mddb.Timeout = t
		}
		return 1
	case "--mddb-batch-size":
		if b, err := strconv.Atoi(nextArg); err == nil && b > 0 {
			cfg.Mddb.BatchSize = b
		}
		return 1
	case "--mddb-protocol":
		if nextArg == "http" || nextArg == "grpc" {
			cfg.Mddb.Protocol = nextArg
		}
		return 1
	case "--mddb-watch-interval":
		if i, err := strconv.Atoi(nextArg); err == nil && i > 0 {
			cfg.Mddb.WatchInterval = i
		}
		return 1
	}
	return 0
}

// handleConfigSkip handles --config at end of args
func handleConfigSkip(arg string) int {
	if arg == "--config" {
		return 0 // No next arg to skip
	}
	return 0
}

func startServer(dir string, port int, quiet bool) {
	addr := fmt.Sprintf(":%d", port)
	if !quiet {
		fmt.Printf("🌐 Starting HTTP server at http://localhost%s\n", addr)
		fmt.Printf("   Serving files from: %s/\n", dir)
	}

	fs := http.FileServer(http.Dir(dir))
	mux := http.NewServeMux()
	mux.Handle("/", fs)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
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
		opts := webp.ConvertOptions{
			Quality: cfg.WebPQuality,
			Quiet:   cfg.Quiet,
			Force:   cfg.ReconvertImages,
		}

		converted, saved, err := webp.ConvertDirectory(cfg.OutputDir, opts)
		if err != nil {
			return fmt.Errorf("converting to WebP: %w", err)
		}

		if err := webp.UpdateReferences(cfg.OutputDir); err != nil {
			return fmt.Errorf("updating image references: %w", err)
		}

		if !cfg.Quiet && converted > 0 {
			savedMB := float64(saved) / (1024 * 1024)
			fmt.Printf("   📊 Converted %d images, saved %.1f MB\n", converted, savedMB)
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
	zipFile, err := os.Create(zipFileName) // #nosec G304 -- CLI tool creates user's output file
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

		file, err := os.Open(path) // #nosec G304 -- CLI tool reads user's output files
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
	fmt.Println("       ssg --mddb-url=http://localhost:8080 --mddb-collection=blog <template> <domain>")
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
	fmt.Println("MDDB Content Source (https://github.com/tradik/mddb):")
	fmt.Println("  --mddb-url=URL         - MDDB server URL (enables mddb mode)")
	fmt.Println("                           HTTP: http://localhost:11023")
	fmt.Println("                           gRPC: localhost:11024")
	fmt.Println("  --mddb-protocol=PROTO  - Connection protocol: http (default) or grpc")
	fmt.Println("  --mddb-collection=NAME - Collection name for pages/posts")
	fmt.Println("  --mddb-key=KEY         - API key for authentication (optional)")
	fmt.Println("  --mddb-lang=LANG       - Language filter (e.g., en_US, pl_PL)")
	fmt.Println("  --mddb-timeout=SEC     - Request timeout in seconds (default: 30)")
	fmt.Println("  --mddb-batch-size=N    - Batch size for pagination (default: 1000)")
	fmt.Println("  --mddb-watch           - Watch MDDB for changes and rebuild automatically")
	fmt.Println("  --mddb-watch-interval=SEC - Watch interval in seconds (default: 30)")
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
	fmt.Println("  --pretty-html          - Prettify HTML (remove all blank lines)")
	fmt.Println("  --relative-links       - Convert absolute URLs to relative links")
	fmt.Println("  --post-url-format=FMT  - Post URL format: 'date' (default: /YYYY/MM/DD/slug/)")
	fmt.Println("                           or 'slug' (/slug/ using slug or link field)")
	fmt.Println("  --minify-all           - Minify HTML, CSS, and JS")
	fmt.Println("  --minify-html          - Minify HTML output")
	fmt.Println("  --minify-css           - Minify CSS output")
	fmt.Println("  --minify-js            - Minify JS output")
	fmt.Println("  --sourcemap            - Include source maps in output")
	fmt.Println("")
	fmt.Println("Image Processing:")
	fmt.Println("  --webp                 - Convert images to WebP format (requires cwebp)")
	fmt.Println("  --webp-quality=N       - WebP compression quality 1-100 (default: 60)")
	fmt.Println("  --reconvert-images     - Force reconvert even if WebP already exists")
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
	fmt.Println("")
	fmt.Println("MDDB Examples:")
	fmt.Println("  # Fetch content from MDDB server (HTTP)")
	fmt.Println("  ssg --mddb-url=http://localhost:11023 --mddb-collection=blog krowy example.com")
	fmt.Println("")
	fmt.Println("  # Use gRPC connection (faster)")
	fmt.Println("  ssg --mddb-url=localhost:11024 --mddb-protocol=grpc --mddb-collection=blog krowy example.com")
	fmt.Println("")
	fmt.Println("  # With language filter and API key")
	fmt.Println("  ssg --mddb-url=https://mddb.example.com --mddb-collection=site \\")
	fmt.Println("      --mddb-lang=en_US --mddb-key=secret krowy example.com")
}
