// Package main provides the entry point for the SSG (Static Site Generator) CLI tool.
// Usage: ssg <source> <template> <domain> [options]
// Example: ssg krowy.net.2026-01-13110345 simple krowy.net --zip --webp
// Example: ssg my-content my-template example.com --http --watch
// Example: ssg --config .ssg.yaml
package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the IANA zone db so --timezone works in static/Windows builds (I18N-001)

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/deploy"
	"github.com/spagu/ssg/internal/engine"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/mddb"
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
		go startServer(cfg)
	}

	runWatchOrServe(genCfg, cfg)
}

// applyMinifyAll sets all minify flags if minify_all is enabled. config.Load
// performs the same expansion for file-based configs, but this call is NOT
// redundant: it is the only expansion point when --minify-all is given on the
// command line (parsed after config load) or when no config file exists (GO-046).
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

// runWatchLoop continuously watches for file changes and rebuilds. Rebuilds are
// gated on a content signature so that touch-only events (mtime bumped, bytes
// unchanged) do not trigger redundant work — a conservative first increment of
// incremental builds where any real change still triggers a full, correct rebuild
// (PLAT-006).
func runWatchLoop(genCfg generator.Config, cfg *config.Config) {
	if !cfg.Quiet {
		fmt.Println("👀 Watching for changes in content and templates...")
	}
	dirs := watchDirs(cfg)
	// One cache for the whole loop: unchanged files keep their hash between
	// polls, so a change event never re-reads the entire content tree (PERF-008).
	sigCache := newFileSigCache()
	lastBuild := time.Now()
	lastSig := sigCache.signature(dirs)

	for {
		time.Sleep(1 * time.Second)
		lastBuild, lastSig = watchIteration(dirs, sigCache, lastBuild, lastSig, func() {
			rebuildOnChange(genCfg, cfg)
		})
	}
}

// watchIteration runs one poll of the watch loop: detect changes, skip
// touch-only events, rebuild. It returns the updated lastBuild/lastSig pair.
// The build timestamp is taken BEFORE the rebuild runs, so files edited while
// the build is in progress are picked up on the next poll instead of being
// lost (GO-025).
func watchIteration(dirs []string, sigCache *fileSigCache, lastBuild time.Time, lastSig string, rebuild func()) (time.Time, string) {
	if !hasChanges(dirs, lastBuild) {
		return lastBuild, lastSig
	}
	sig := sigCache.signature(dirs)
	if sig == lastSig {
		// mtime changed but bytes did not — skip the rebuild (PLAT-006).
		return time.Now(), lastSig
	}
	buildStart := time.Now()
	rebuild()
	return buildStart, sig
}

// watchDirs returns the directories watched for changes (content, templates, data).
func watchDirs(cfg *config.Config) []string {
	dirs := []string{cfg.ContentDir, cfg.TemplatesDir}
	if cfg.DataDir != "" {
		dirs = append(dirs, cfg.DataDir)
	}
	return dirs
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

// rebuildOnChange handles rebuilding when changes are detected. It no longer
// returns a completion timestamp: the watch loop stamps lastBuild before the
// build starts so mid-build edits are not lost (GO-025).
func rebuildOnChange(genCfg generator.Config, cfg *config.Config) {
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
}

// configFlag selects the configuration file; it is consumed by loadConfig before
// the regular flag parsing runs.
const configFlag = "--config"

// loadConfig loads configuration from file or returns defaults
func loadConfig(args []string) *config.Config {
	var configPath string

	// Look for --config flag
	for i, arg := range args {
		if strings.HasPrefix(arg, configFlag+"=") {
			configPath = strings.TrimPrefix(arg, configFlag+"=")
		} else if arg == configFlag && i+1 < len(args) {
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

	positionalArgs := positionalArgsOf(args)

	if len(positionalArgs) >= 3 {
		cfg.Source = positionalArgs[0]
		cfg.Template = positionalArgs[1]
		cfg.Domain = positionalArgs[2]
	} else if cfg.Source == "" || cfg.Template == "" || cfg.Domain == "" {
		printUsage()
		os.Exit(1)
	}
}

// positionalArgsOf extracts positional arguments, skipping flags and the values
// consumed by space-separated value flags so e.g. "--engine go" never leaks "go"
// into <source>/<template>/<domain> (GO-036).
func positionalArgsOf(args []string) []string {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		if separateValueFlags[arg] && i+1 < len(args) {
			i++ // skip the flag's value (GO-036)
		}
	}
	return positional
}

// setupTemplateEngine validates the template engine and deploy target, exiting on error.
func setupTemplateEngine(cfg *config.Config) {
	if err := validateTemplateEngine(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	if cfg.Deploy != "" && !deploy.Supported(cfg.Deploy) {
		fmt.Fprintf(os.Stderr, "❌ Error: unknown --deploy provider %q (supported: %s)\n",
			cfg.Deploy, strings.Join(deploy.SupportedProviders(), ", "))
		os.Exit(1)
	}

	if cfg.Engine != "" && !cfg.Quiet {
		fmt.Printf("🔧 Using template engine: %s\n", cfg.Engine)
	}
}

// validateTemplateEngine checks that the requested template engine is supported.
// All four back-ends now render for real (GO-007): the generator loads the theme's
// templates through the selected engine. Alt-engine themes must be authored in
// that engine's syntax (pongo2/mustache/handlebars have no Go FuncMap/inheritance).
func validateTemplateEngine(cfg *config.Config) error {
	if cfg.Engine == "" {
		return nil
	}
	switch strings.ToLower(cfg.Engine) {
	case engine.EngineGo,
		engine.EnginePongo2, "jinja2", "django",
		engine.EngineMustache,
		engine.EngineHandlebars, "hbs":
		return nil
	default:
		return fmt.Errorf("unknown template engine: %s (supported: go, pongo2, mustache, handlebars)", cfg.Engine)
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
		Source:            cfg.Source,
		Template:          cfg.Template,
		Domain:            cfg.Domain,
		ContentDir:        cfg.ContentDir,
		TemplatesDir:      cfg.TemplatesDir,
		OutputDir:         cfg.OutputDir,
		SitemapOff:        cfg.SitemapOff,
		RobotsOff:         cfg.RobotsOff,
		PrettyHTML:        cfg.PrettyHTML,
		PostURLFormat:     cfg.PostURLFormat,
		PageFormat:        cfg.PageFormat,
		RelativeLinks:     cfg.RelativeLinks,
		Shortcodes:        shortcodes,
		ShortcodeBrackets: cfg.ShortcodeBrackets,
		MinifyHTML:        cfg.MinifyHTML,
		MinifyCSS:         cfg.MinifyCSS,
		MinifyJS:          cfg.MinifyJS,
		SourceMap:         cfg.SourceMap,
		Clean:             cfg.Clean,
		Quiet:             cfg.Quiet,
		Engine:            cfg.Engine,
		Variables:         cfg.Variables,
		PagesPath:         cfg.PagesPath,
		PostsPath:         cfg.PostsPath,
		StaticDir:         cfg.StaticDir,
		DataDir:           cfg.DataDir,
		RewriteMdLinks:    cfg.RewriteMdLinks,
		PreserveSlugCase:  cfg.PreserveSlugCase,
		Permalinks:        cfg.Permalinks,
		LastmodFromGit:    cfg.LastmodFromGit,
		Fingerprint:       cfg.Fingerprint,
		SCSS:              cfg.SCSS,
		SassBinary:        cfg.SassBinary,
		Timezone:          cfg.Timezone,
		LanguageTimezones: cfg.LanguageTimezones,
		Math:              cfg.Math,
		Paginate:          cfg.Paginate,
		Languages:         cfg.Languages,
		DefaultLanguage:   cfg.DefaultLanguage,
		LanguageConfigs:   cfg.LanguageConfigs,
		I18n:              cfg.I18n,
		Taxonomies:        cfg.Taxonomies,
		ExternalSources:   cfg.ExternalSources,
		Hooks:             cfg.Hooks,
		Feed:              cfg.Feed,
		FeedItems:         cfg.FeedItems,
		FeedFullContent:   cfg.FeedFullContent,
		Highlight:         cfg.Highlight,
		HighlightStyle:    cfg.HighlightStyle,
		TOC:               cfg.TOC,
		TOCDepth:          cfg.TOCDepth,
		SEO:               cfg.SEO,
		CheckLinks:        cfg.CheckLinks,
		Bundles:           cfg.Bundles,
		Outputs:           cfg.Outputs,
		SearchIndex:       cfg.SearchIndex,
		SanitizeHTML:      cfg.SanitizeHTML,
		Mddb: generator.MddbConfig{
			Enabled:    cfg.Mddb.Enabled,
			URL:        cfg.Mddb.URL,
			Protocol:   cfg.Mddb.Protocol,
			APIKey:     cfg.Mddb.APIKey,
			Collection: cfg.Mddb.Collection,
			Lang:       cfg.Mddb.Lang,
			Timeout:    cfg.Mddb.Timeout,
			BatchSize:  cfg.Mddb.BatchSize,
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

// parseBoolFlags handles boolean flags, returns true if flag was handled. Simple
// on/off toggles are table-driven (name → target field) to keep this small and DRY.
func parseBoolFlags(arg string, cfg *config.Config) bool {
	if arg == "--check-links" { // the one toggle that sets a string mode, not a bool
		cfg.CheckLinks = "warn"
		return true
	}
	if arg == "--seo-off" { // deprecated no-op: SEO injection is opt-in since v1.8.2
		cfg.SEO = false
		return true
	}
	toggles := map[string]*bool{
		"--zip": &cfg.Zip, "-zip": &cfg.Zip,
		"--targz": &cfg.TarGz, "--tarxz": &cfg.TarXz,
		"--tls-auto": &cfg.TLSAuto, "--gzip": &cfg.Gzip, "--http3": &cfg.HTTP3,
		"--sanitize-html": &cfg.SanitizeHTML,
		"--webp":          &cfg.WebP, "-webp": &cfg.WebP,
		"--webp-keep-original": &cfg.WebPKeepOriginal,
		"--reconvert-images":   &cfg.ReconvertImages,
		"--watch":              &cfg.Watch, "-watch": &cfg.Watch,
		"--http": &cfg.HTTP, "-http": &cfg.HTTP,
		"--sitemap-off": &cfg.SitemapOff, "--robots-off": &cfg.RobotsOff,
		"--pretty-html": &cfg.PrettyHTML, "--pretty": &cfg.PrettyHTML,
		"--relative-links": &cfg.RelativeLinks,
		"--minify-all":     &cfg.MinifyAll,
		"--minify-html":    &cfg.MinifyHTML, "--minify-css": &cfg.MinifyCSS, "--minify-js": &cfg.MinifyJS,
		"--sourcemap": &cfg.SourceMap, "--fingerprint": &cfg.Fingerprint,
		"--scss":             &cfg.SCSS,
		"--lastmod-from-git": &cfg.LastmodFromGit,
		"--math":             &cfg.Math, "--feed": &cfg.Feed,
		"--highlight": &cfg.Highlight, "--toc": &cfg.TOC,
		"--search-index": &cfg.SearchIndex, "--seo": &cfg.SEO,
		"--mddb-watch": &cfg.Mddb.Watch, // bool flag, not an =value flag (GO-018)
		"--clean":      &cfg.Clean,
		"--quiet":      &cfg.Quiet, "-q": &cfg.Quiet,
		// External sources (docs/EXTERNAL_SOURCES.md)
		"--offline":                  &cfg.ExternalSources.Offline,
		"--refresh-external-sources": &cfg.ExternalSources.Refresh,
		"--clear-external-cache":     &cfg.ExternalSources.ClearCache,
	}
	if target, ok := toggles[arg]; ok {
		*target = true
		return true
	}
	return false
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

// parseIntList parses a comma-separated list of positive integers (e.g. "480,960").
// Invalid or non-positive entries are skipped.
func parseIntList(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// splitCSV splits a comma-separated string into trimmed, non-empty tokens.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// setPermalink records a permalink pattern for a content type, initializing the map.
func setPermalink(cfg *config.Config, typ, pattern string) {
	if pattern == "" {
		return
	}
	if cfg.Permalinks == nil {
		cfg.Permalinks = make(map[string]string)
	}
	cfg.Permalinks[typ] = pattern
}

// stringEqualFlags maps simple "--flag=" prefixes to the string field they set,
// keeping parseEqualFlags free of one near-identical case per plain string option.
func stringEqualFlags(cfg *config.Config) map[string]*string {
	return map[string]*string{
		"--host=":             &cfg.Host,
		"--tls-cert=":         &cfg.TLSCert,
		"--tls-key=":          &cfg.TLSKey,
		"--tls-domain=":       &cfg.TLSDomain,
		"--mem-limit=":        &cfg.MemLimit,
		"--content-dir=":      &cfg.ContentDir,
		"--templates-dir=":    &cfg.TemplatesDir,
		"--output-dir=":       &cfg.OutputDir,
		"--static-dir=":       &cfg.StaticDir,
		"--data-dir=":         &cfg.DataDir,
		"--image-sizes-attr=": &cfg.ImageSizesAttr,
		"--sass-binary=":      &cfg.SassBinary,
		"--highlight-style=":  &cfg.HighlightStyle,
		"--default-language=": &cfg.DefaultLanguage,
		"--timezone=":         &cfg.Timezone,
		"--engine=":           &cfg.Engine,
		"--online-theme=":     &cfg.OnlineTheme,
		"--post-url-format=":  &cfg.PostURLFormat,
		"--deploy=":           &cfg.Deploy,
		"--deploy-project=":   &cfg.DeployProject,
		"--deploy-branch=":    &cfg.DeployBranch,
		"--deploy-target=":    &cfg.DeployTarget,
		"--mddb-key=":         &cfg.Mddb.APIKey,
		"--mddb-collection=":  &cfg.Mddb.Collection,
		"--mddb-lang=":        &cfg.Mddb.Lang,
		"--external-source=":  &cfg.ExternalSources.Only,
	}
}

// setIntEqual parses "--flag=N"; when arg carries prefix it applies the value if it
// passes the [minVal, maxVal] range (maxVal <= 0 means no upper bound) and returns
// true to signal the flag was recognised.
func setIntEqual(arg, prefix string, minVal, maxVal int, apply func(int)) bool {
	if !strings.HasPrefix(arg, prefix) {
		return false
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(arg, prefix)); err == nil && n >= minVal && (maxVal <= 0 || n <= maxVal) {
		apply(n)
	}
	return true
}

// parseEqualFlags handles --flag=value format, dispatching to focused helpers so no
// single function carries the whole option table.
func parseEqualFlags(arg string, cfg *config.Config) {
	for prefix, target := range stringEqualFlags(cfg) {
		if strings.HasPrefix(arg, prefix) {
			*target = strings.TrimPrefix(arg, prefix)
			return
		}
	}
	if parseIntEqualFlags(arg, cfg) || parseMddbEqualFlags(arg, cfg) {
		return
	}
	parseMiscEqualFlags(arg, cfg)
}

// parseIntEqualFlags handles numeric --flag=N options; returns true when recognised.
func parseIntEqualFlags(arg string, cfg *config.Config) bool {
	switch {
	case setIntEqual(arg, "--webp-quality=", 1, 100, func(n int) { cfg.WebPQuality = n }):
	case setIntEqual(arg, "--max-conns=", 0, 0, func(n int) { cfg.MaxConns = n }):
	case setIntEqual(arg, "--paginate=", 0, 0, func(n int) { cfg.Paginate = n }):
	case setIntEqual(arg, "--feed-items=", 1, 0, func(n int) { cfg.FeedItems = n }):
	case setIntEqual(arg, "--toc-depth=", 1, 0, func(n int) { cfg.TOCDepth = n }):
	case strings.HasPrefix(arg, "--port="):
		if port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil {
			cfg.Port = port
		}
	default:
		return false
	}
	return true
}

// parseMddbEqualFlags handles the --mddb-* options; returns true when recognised.
func parseMddbEqualFlags(arg string, cfg *config.Config) bool {
	switch {
	case setIntEqual(arg, "--mddb-timeout=", 1, 0, func(n int) { cfg.Mddb.Timeout = n }):
	case setIntEqual(arg, "--mddb-batch-size=", 1, 0, func(n int) { cfg.Mddb.BatchSize = n }):
	case setIntEqual(arg, "--mddb-watch-interval=", 1, 0, func(n int) { cfg.Mddb.WatchInterval = n }):
	// --mddb-watch is a boolean toggle handled by parseBoolFlags; a case here was
	// unreachable because only args containing "=" ever get this far (GO-018).
	case strings.HasPrefix(arg, "--mddb-url="):
		cfg.Mddb.URL = strings.TrimPrefix(arg, "--mddb-url=")
		cfg.Mddb.Enabled = true
	case strings.HasPrefix(arg, "--mddb-protocol="):
		if p := strings.TrimPrefix(arg, "--mddb-protocol="); p == "http" || p == "grpc" {
			cfg.Mddb.Protocol = p
		}
	default:
		return false
	}
	return true
}

// parseMiscEqualFlags handles the remaining validated/list --flag=value options.
func parseMiscEqualFlags(arg string, cfg *config.Config) {
	switch {
	case strings.HasPrefix(arg, "--image-sizes="):
		cfg.ImageSizes = parseIntList(strings.TrimPrefix(arg, "--image-sizes="))
	case strings.HasPrefix(arg, "--permalink-post="):
		setPermalink(cfg, "post", strings.TrimPrefix(arg, "--permalink-post="))
	case strings.HasPrefix(arg, "--permalink-page="):
		setPermalink(cfg, "page", strings.TrimPrefix(arg, "--permalink-page="))
	case strings.HasPrefix(arg, "--check-links="):
		if v := strings.TrimPrefix(arg, "--check-links="); v == "warn" || v == "strict" {
			cfg.CheckLinks = v
		}
	case strings.HasPrefix(arg, "--outputs="):
		cfg.Outputs = splitCSV(strings.TrimPrefix(arg, "--outputs="))
	case strings.HasPrefix(arg, "--languages="):
		cfg.Languages = splitCSV(strings.TrimPrefix(arg, "--languages="))
	case strings.HasPrefix(arg, "--page-format="):
		if pf := strings.TrimPrefix(arg, "--page-format="); pf == "directory" || pf == "flat" || pf == "both" {
			cfg.PageFormat = pf
		}
	}
}

// separateValueFlags lists the flags that consume the following argument when
// given in "--flag value" form. Shared by parseSeparateValueFlags and the
// positional-argument scanner so flag values are never miscounted as
// positionals (GO-036).
var separateValueFlags = map[string]bool{
	"--webp-quality": true, "--port": true, configFlag: true,
	"--content-dir": true, "--templates-dir": true, "--output-dir": true,
	"--engine": true, "--online-theme": true,
	"--post-url-format": true, "--page-format": true,
	"--mddb-url": true, "--mddb-key": true, "--mddb-collection": true,
	"--mddb-lang": true, "--mddb-timeout": true, "--mddb-batch-size": true,
	"--mddb-protocol": true, "--mddb-watch-interval": true,
}

// stringSeparateFlags maps "--flag value" plain string options to their target
// field, mirroring stringEqualFlags so parseSeparateValueFlags stays small.
func stringSeparateFlags(cfg *config.Config) map[string]*string {
	return map[string]*string{
		"--content-dir":     &cfg.ContentDir,
		"--templates-dir":   &cfg.TemplatesDir,
		"--output-dir":      &cfg.OutputDir,
		"--engine":          &cfg.Engine,
		"--online-theme":    &cfg.OnlineTheme,
		"--post-url-format": &cfg.PostURLFormat,
		"--mddb-key":        &cfg.Mddb.APIKey,
		"--mddb-collection": &cfg.Mddb.Collection,
		"--mddb-lang":       &cfg.Mddb.Lang,
	}
}

// setIntSeparate parses a "--flag N" value and applies it when it passes the
// [minVal, maxVal] range (maxVal <= 0 means no upper bound).
func setIntSeparate(value string, minVal, maxVal int, apply func(int)) {
	if n, err := strconv.Atoi(value); err == nil && n >= minVal && (maxVal <= 0 || n <= maxVal) {
		apply(n)
	}
}

// parseSeparateValueFlags handles --flag value format, returns skip count.
// Unknown flags, and known flags at the end of the argument list (no value to
// consume), skip nothing (GO-046: the old handleConfigSkip helper was a
// guaranteed no-op and has been removed in favour of this guard).
func parseSeparateValueFlags(args []string, i int, cfg *config.Config) int {
	arg := args[i]
	if !separateValueFlags[arg] || i+1 >= len(args) {
		return 0
	}

	nextArg := args[i+1]
	if target, ok := stringSeparateFlags(cfg)[arg]; ok {
		*target = nextArg
		return 1
	}
	switch arg {
	case "--webp-quality":
		setIntSeparate(nextArg, 1, 100, func(n int) { cfg.WebPQuality = n })
	case "--port":
		setIntSeparate(nextArg, 0, 0, func(n int) { cfg.Port = n })
	case "--page-format":
		if nextArg == "directory" || nextArg == "flat" || nextArg == "both" {
			cfg.PageFormat = nextArg
		}
	case configFlag:
		// Skip the value; --config was already processed by loadConfig.
	case "--mddb-url":
		cfg.Mddb.URL = nextArg
		cfg.Mddb.Enabled = true
	case "--mddb-timeout":
		setIntSeparate(nextArg, 1, 0, func(n int) { cfg.Mddb.Timeout = n })
	case "--mddb-batch-size":
		setIntSeparate(nextArg, 1, 0, func(n int) { cfg.Mddb.BatchSize = n })
	case "--mddb-protocol":
		if nextArg == "http" || nextArg == "grpc" {
			cfg.Mddb.Protocol = nextArg
		}
	case "--mddb-watch-interval":
		setIntSeparate(nextArg, 1, 0, func(n int) { cfg.Mddb.WatchInterval = n })
	}
	return 1
}

// resolveListenAddr computes the dev-server bind address and a user-facing URL.
// SEC-012: it defaults to loopback and flags exposure when an all-interfaces
// address is requested. net.JoinHostPort brackets IPv6 literals so --host=::1
// yields a valid listen address (GO-034).
func resolveListenAddr(host string, port int) (addr, url string, exposed bool) {
	if host == "" {
		host = "127.0.0.1"
	}
	portStr := strconv.Itoa(port)
	addr = net.JoinHostPort(host, portStr)
	display := host
	if host == "0.0.0.0" || host == "::" {
		display = "127.0.0.1"
		exposed = true
	}
	url = fmt.Sprintf("http://%s", net.JoinHostPort(display, portStr))
	return addr, url, exposed
}

func build(genCfg generator.Config, cfg *config.Config) error {
	gen, err := generator.New(genCfg)
	if err != nil {
		return fmt.Errorf("initializing generator: %w", err)
	}
	if err := gen.Generate(); err != nil {
		return fmt.Errorf("generating site: %w", err)
	}
	if err := runWebP(cfg); err != nil {
		return err
	}
	if err := runArchives(cfg); err != nil {
		return err
	}
	return runDeploy(cfg)
}

// runWebP converts output images to WebP and rewrites references when --webp is set.
func runWebP(cfg *config.Config) error {
	if !cfg.WebP {
		return nil
	}
	opts := webp.ConvertOptions{
		Quality:      cfg.WebPQuality,
		Quiet:        cfg.Quiet,
		Force:        cfg.ReconvertImages,
		Sizes:        cfg.ImageSizes,
		KeepOriginal: cfg.WebPKeepOriginal,
	}
	converted, saved, err := webp.ConvertDirectory(cfg.OutputDir, opts)
	if err != nil {
		return fmt.Errorf("converting to WebP: %w", err)
	}
	if err := webp.UpdateReferences(cfg.OutputDir); err != nil {
		return fmt.Errorf("updating image references: %w", err)
	}
	// Emit responsive srcset/sizes for images that have variants (ASSET-004).
	if err := webp.EmitSrcset(cfg.OutputDir, cfg.ImageSizes, cfg.ImageSizesAttr); err != nil {
		return fmt.Errorf("emitting responsive srcset: %w", err)
	}
	if !cfg.Quiet && converted > 0 {
		fmt.Printf("   📊 Converted %d images, saved %.1f MB\n", converted, float64(saved)/(1024*1024))
	}
	return nil
}

// runArchives creates the requested deployment archives (ZIP, tar.gz, tar.xz).
func runArchives(cfg *config.Config) error {
	if cfg.Zip {
		if err := createZipArchive(cfg); err != nil {
			return err
		}
	}
	if cfg.TarGz {
		if err := makeArchive(cfg, "tar.gz", createTarGz); err != nil {
			return err
		}
	}
	if cfg.TarXz {
		if err := makeArchive(cfg, "tar.xz", createTarXz); err != nil {
			return err
		}
	}
	return nil
}

// createZipArchive builds <domain>.zip and reports its size (warning past 25 MB).
func createZipArchive(cfg *config.Config) error {
	zipFileName := fmt.Sprintf("%s.zip", cfg.Domain)
	if err := createZip(cfg.OutputDir, zipFileName); err != nil {
		return fmt.Errorf("creating ZIP: %w", err)
	}
	if cfg.Quiet {
		return nil
	}
	info, err := os.Stat(zipFileName) // #nosec G703 -- CLI tool checks its own output
	if err != nil {
		return nil
	}
	sizeMB := float64(info.Size()) / (1024 * 1024)
	fmt.Printf("📦 Created deployment package: %s (%.1f MB)\n", zipFileName, sizeMB)
	if sizeMB > 25 {
		fmt.Printf("⚠️  Warning: File exceeds Cloudflare Pages 25MB limit!\n")
	}
	return nil
}

// runDeploy publishes the output tree to the configured provider (v1.8.1). No-op when
// --deploy is unset.
func runDeploy(cfg *config.Config) error {
	if cfg.Deploy == "" {
		return nil
	}
	url, err := deploy.Run(context.Background(), deploy.Options{
		Provider: cfg.Deploy,
		Dir:      cfg.OutputDir,
		Project:  cfg.DeployProject,
		Branch:   cfg.DeployBranch,
		Target:   cfg.DeployTarget,
		Quiet:    cfg.Quiet,
	})
	if err != nil {
		return fmt.Errorf("deploy to %s: %w", cfg.Deploy, err)
	}
	if !cfg.Quiet && url != "" {
		fmt.Printf("🚀 Deployed to %s\n", url)
	}
	return nil
}

// makeArchive builds a <domain>.<ext> archive from the output directory and reports
// its size (v1.8.1).
func makeArchive(cfg *config.Config, ext string, fn func(src, out string) error) error {
	name := fmt.Sprintf("%s.%s", cfg.Domain, ext)
	if err := fn(cfg.OutputDir, name); err != nil {
		return fmt.Errorf("creating %s: %w", ext, err)
	}
	if !cfg.Quiet {
		if info, err := os.Stat(name); err == nil { // #nosec G703 -- CLI checks its own output
			fmt.Printf("📦 Created deployment package: %s (%.1f MB)\n", name, float64(info.Size())/(1024*1024))
		}
	}
	return nil
}

func hasChanges(dirs []string, lastBuild time.Time) bool {
	changed := false
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error { // #nosec G703 -- CLI tool scans user's content dirs
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

// createZip builds a ZIP archive of sourceDir at zipFileName. Errors from the
// writer Close (which emits the ZIP central directory) and from the file Close
// are propagated so a corrupt archive is never reported as success (GO-024).
func createZip(sourceDir, zipFileName string) error {
	zipFile, err := os.Create(zipFileName) // #nosec G304,G703 -- CLI tool creates user's output file
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	if err := writeZip(sourceDir, zipFile); err != nil {
		_ = zipFile.Close()
		return err
	}
	if err := zipFile.Close(); err != nil {
		return fmt.Errorf("closing zip file: %w", err)
	}
	return nil
}

// writeZip streams every entry under sourceDir into a ZIP archive written to w.
// The central directory is written by zip.Writer.Close, so its error must be
// returned — swallowing it would deploy a truncated archive with exit code 0
// (GO-024).
func writeZip(sourceDir string, w io.Writer) error {
	zipWriter := zip.NewWriter(w)

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error { // #nosec G703 -- CLI tool walks user's output dir
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		return zipAddEntry(zipWriter, sourceDir, path, info)
	})
	if err != nil {
		_ = zipWriter.Close()
		return fmt.Errorf("walking directory: %w", err)
	}
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("finalizing zip: %w", err)
	}
	return nil
}

// zipAddEntry writes one file or directory entry (relative to sourceDir) into zw.
func zipAddEntry(zw *zip.Writer, sourceDir, path string, info os.FileInfo) error {
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
		_, err = zw.CreateHeader(header)
		return err
	}

	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("creating zip entry: %w", err)
	}

	file, err := os.Open(path) // #nosec G304,G122 -- CLI tool reads user's output files
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(writer, file)
	return err
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
	fmt.Println("                           Supported: go, pongo2, mustache, handlebars")
	fmt.Println("                           (alt engines load the theme's own templates verbatim)")
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
	fmt.Println("  --host=ADDR            - Dev server bind address (default: 127.0.0.1; use 0.0.0.0 to expose)")
	fmt.Println("  --port=PORT            - HTTP server port (default: 8888)")
	fmt.Println("  --watch                - Watch for changes and rebuild automatically")
	fmt.Println("  --clean                - Clean output directory before build")
	fmt.Println("")
	fmt.Println("Public Server Hardening (TLS/HTTP2/HTTP3, opt-in):")
	fmt.Println("  --tls-cert=FILE        - TLS certificate (PEM); with --tls-key enables HTTPS + HTTP/2")
	fmt.Println("  --tls-key=FILE         - TLS private key (PEM), paired with --tls-cert")
	fmt.Println("  --tls-auto             - Automatic Let's Encrypt certificates (needs --tls-domain, port 443)")
	fmt.Println("  --tls-domain=HOST      - Domain(s) for autocert (comma-separated)")
	fmt.Println("  --http3                - Advertise & serve HTTP/3 (QUIC) alongside HTTP/2 (requires TLS)")
	fmt.Println("  --gzip                 - gzip-compress responses when the client accepts it")
	fmt.Println("  --max-conns=N          - Cap simultaneous connections (0 = unlimited)")
	fmt.Println("  --mem-limit=SIZE       - Soft memory limit, e.g. 512MiB, 1GiB (runtime GC target)")
	fmt.Println("")
	fmt.Println("Output Control:")
	fmt.Println("  --sitemap-off          - Disable sitemap.xml generation")
	fmt.Println("  --robots-off           - Disable robots.txt generation")
	fmt.Println("  --pretty-html          - Prettify HTML (remove all blank lines)")
	fmt.Println("  --relative-links       - Convert absolute URLs to relative links")
	fmt.Println("  --post-url-format=FMT  - Post URL format: 'date' (default: /YYYY/MM/DD/slug/)")
	fmt.Println("                           or 'slug' (/slug/ using slug or link field)")
	fmt.Println("  --page-format=FMT      - Page output format:")
	fmt.Println("                           'directory' (default: slug/index.html)")
	fmt.Println("                           'flat' (slug.html)")
	fmt.Println("                           'both' (slug/index.html AND slug.html)")
	fmt.Println("  --minify-all           - Minify HTML, CSS, and JS")
	fmt.Println("  --minify-html          - Minify HTML output")
	fmt.Println("  --minify-css           - Minify CSS output")
	fmt.Println("  --minify-js            - Minify JS output")
	fmt.Println("  --sourcemap            - Emit v3 source maps (*.js.map/*.css.map) for minified JS/CSS")
	fmt.Println("  --fingerprint          - Content-hash CSS/JS names + manifest for immutable caching")
	fmt.Println("  --scss                 - Compile *.scss via dart-sass before bundling/minify (optional tool)")
	fmt.Println("  --sass-binary=PATH     - Explicit dart-sass binary (default: `sass` from PATH)")
	fmt.Println("  --lastmod-from-git     - Derive sitemap <lastmod> from each file's last git commit")
	fmt.Println("  --permalink-post=PAT   - Post URL pattern, tokens :year :month :day :slug :category")
	fmt.Println("  --permalink-page=PAT   - Page URL pattern (same tokens)")
	fmt.Println("")
	fmt.Println("Authoring:")
	fmt.Println("  --math                 - Render math: inject KaTeX only on pages containing $$…$$")
	fmt.Println("  --sanitize-html        - Sanitize raw HTML in markdown via bluemonday UGC policy (FE-005)")
	fmt.Println("  --seo                  - Inject OpenGraph/Twitter/JSON-LD into pages lacking their own")
	fmt.Println("                           (opt-in since v1.8.2; --seo-off is a deprecated no-op)")
	fmt.Println("  --timezone=ZONE        - IANA zone for content dates in permalinks/templates (e.g. Europe/Warsaw);")
	fmt.Println("                           per-language overrides via language_timezones: in .ssg.yaml")
	fmt.Println("")
	fmt.Println("Image Processing:")
	fmt.Println("  --webp                 - Convert images to WebP format (requires cwebp)")
	fmt.Println("  --webp-quality=N       - WebP compression quality 1-100 (default: 60)")
	fmt.Println("  --webp-keep-original   - Keep originals next to .webp files (safe for themes")
	fmt.Println("                           with hardcoded .png/.jpg refs); default replaces them")
	fmt.Println("  --reconvert-images     - Force reconvert even if WebP already exists")
	fmt.Println("  --image-sizes=A,B,C    - Responsive widths (px) → WebP variants + srcset (e.g. 480,960,1600)")
	fmt.Println("  --image-sizes-attr=VAL - Value of the generated sizes attribute (default: 100vw)")
	fmt.Println("")
	fmt.Println("Deployment:")
	fmt.Println("  --zip                  - Create ZIP archive of the output tree")
	fmt.Println("  --targz                - Create gzip-compressed tarball (.tar.gz) of the output tree")
	fmt.Println("  --tarxz                - Create xz-compressed tarball (.tar.xz) of the output tree")
	fmt.Println("")
	fmt.Println("Publish (native deploy — credentials/secrets come from the environment):")
	fmt.Println("  --deploy=PROVIDER      - cloudflare | github-pages | netlify | vercel | ftp | sftp")
	fmt.Println("  --deploy-project=NAME  - Pages/site/project name (cloudflare, netlify, vercel)")
	fmt.Println("  --deploy-branch=BRANCH - Target branch (cloudflare, github-pages; default gh-pages)")
	fmt.Println("  --deploy-target=URL    - ftp://user@host/path, sftp://user@host/path, or a git remote")
	fmt.Println("    cloudflare  → CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID")
	fmt.Println("    github-pages→ GITHUB_TOKEN (https remotes) or an SSH key")
	fmt.Println("    netlify     → NETLIFY_AUTH_TOKEN (+ site via --deploy-project/NETLIFY_SITE_ID)")
	fmt.Println("    vercel      → VERCEL_TOKEN, VERCEL_ORG_ID (+ project via --deploy-project)")
	fmt.Println("    ftp         → FTP_USERNAME, FTP_PASSWORD")
	fmt.Println("    sftp        → SSH_USERNAME, SSH_PASSWORD or SSH_KEY_FILE (host in known_hosts)")
	fmt.Println("")
	fmt.Println("Paths:")
	fmt.Println("  --content-dir=PATH     - Content directory (default: content)")
	fmt.Println("  --templates-dir=PATH   - Templates directory (default: templates)")
	fmt.Println("  --output-dir=PATH      - Output directory (default: output)")
	fmt.Println("  --static-dir=PATH      - Static passthrough directory copied verbatim to output (default: static)")
	fmt.Println("  --data-dir=PATH        - Data files dir (*.yaml|*.json) exposed as .Data.* (default: data)")
	fmt.Println("")
	fmt.Println("External sources (docs/EXTERNAL_SOURCES.md):")
	fmt.Println("  --offline                    - Serve external sources from the disk cache only")
	fmt.Println("  --refresh-external-sources   - Force re-fetch, ignoring fresh cache entries")
	fmt.Println("  --clear-external-cache       - Wipe the external-source disk cache before the build")
	fmt.Println("  --external-source=NAME       - Narrow --refresh-external-sources to one source")
	fmt.Println("")
	fmt.Println("Other:")
	fmt.Println("  --quiet, -q            - Suppress output (only exit codes)")
	fmt.Println("  --version, -v          - Show version")
	fmt.Println("  --help, -h             - Show this help")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ssg my-site simple example.com --http --watch")
	fmt.Println("  ssg my-site krowy example.com --clean --minify-all --zip --targz --tarxz")
	fmt.Println("  ssg my-site simple example.com --http --port=443 --tls-cert=cert.pem --tls-key=key.pem --http3 --gzip")
	fmt.Println("  ssg my-site simple example.com --http --port=443 --tls-auto --tls-domain=example.com --max-conns=1024 --mem-limit=512MiB")
	fmt.Println("  ssg my-site mytheme example.com --engine=go")
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
