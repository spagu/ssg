package main

// `ssg migrate <provider> <url>` — one command from a live site to a working
// ssg project. With --watch --http it runs the owner-specified LIVE flow
// (GO-100, 2026-08-12): 1) scaffold if the project is missing and start the
// watch+http server FIRST, announcing its address; 2) pull the data
// incrementally so auto-reload (GO-090) shows the site filling up in the
// browser; 3) finish with an honest report and the next step — `ssg mcp` and
// an AI agent rebuilding the source site's look. Without those flags it is a
// plain batch: fetch, build once, report.
//
// `migrate` is a fully claimed verb (unlike `cache`, which dispatches only on
// known nouns): the silent fallthrough to positional <source> <template>
// <domain> args cost real confusion. A source directory literally named
// "migrate" still builds via `--source=migrate`.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/migrate"
)

// migrateFetch is the provider-call seam so command-level tests never spawn
// wpexporter or touch the network.
var migrateFetch = func(p migrate.Provider, rawURL string, opts migrate.Options) (*migrate.Report, error) {
	return p.Fetch(rawURL, opts)
}

// migrateBlock parks the live mode after the report so the server keeps
// serving; tests replace it to return.
var migrateBlock = func() { select {} }

type migrateFlags struct {
	content  []string
	watch    bool
	http     bool
	quiet    bool
	noCrawl  bool
	allMedia bool
	source   string
}

func runMigrate(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printMigrateUsage()
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] == "--list" || args[0] == "list" {
		for _, p := range migrate.Providers() {
			fmt.Printf("   %s@%s — %s\n", p.Name(), p.Version(), p.Description())
		}
		return 0
	}

	provider, ok := migrate.Lookup(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "❌ unknown migration provider %q — available: %s\n",
			args[0], strings.Join(migrateProviderNames(), ", "))
		return 2
	}
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "❌ missing site URL\n\n")
		printMigrateUsage()
		return 2
	}
	rawURL := args[1]
	u, err := migrate.ValidateURL(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 2
	}
	flags, code := parseMigrateFlags(args[2:])
	if code >= 0 {
		return code
	}
	return executeMigrate(provider, rawURL, u.Hostname(), flags)
}

// executeMigrate runs the shared prologue (scaffold + config) and then the
// batch or live epilogue.
func executeMigrate(provider migrate.Provider, rawURL, host string, flags migrateFlags) int {
	source := flags.source
	if source == "" {
		source = strings.TrimPrefix(host, "www.")
	}
	if config.FindConfigFile() == "" {
		created, skipped, err := writeScaffold(migrateScaffold(source, host))
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ scaffolding project: %v\n", err)
			return 1
		}
		reportScaffold(created, skipped)
	}

	cfg := loadConfig(nil)
	cfg.Watch = cfg.Watch || flags.watch
	cfg.HTTP = cfg.HTTP || flags.http
	if flags.quiet {
		cfg.Quiet = true
	}
	if cfg.Source == "" {
		cfg.Source = source
	}
	applyMinifyAll(cfg)
	setupTemplateEngine(cfg)
	downloadOnlineTheme(cfg)

	opts := migrate.Options{
		Content:  flags.content,
		Dest:     filepath.Join(cfg.ContentDir, cfg.Source),
		Quiet:    cfg.Quiet,
		NoCrawl:  flags.noCrawl,
		AllMedia: flags.allMedia,
	}
	genCfg := createGeneratorConfig(cfg)
	if !cfg.Watch && !cfg.HTTP {
		return migrateBatch(provider, rawURL, opts, genCfg, cfg)
	}
	return migrateLive(provider, rawURL, opts, genCfg, cfg)
}

// migrateBatch: fetch everything, build once, report.
func migrateBatch(provider migrate.Provider, rawURL string, opts migrate.Options,
	genCfg generator.Config, cfg *config.Config) int {
	report, err := migrateFetch(provider, rawURL, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return 1
	}
	// The config is completed BEFORE the build, so the first render already
	// carries the site's own title, description and palette (#128).
	applied := applyMigratedIdentity(config.FindConfigFile(), opts.Dest)
	genCfg, cfg = reloadAfterIdentity(cfg, genCfg)
	if err := build(genCfg, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ building migrated site: %v\n", err)
		return 1
	}
	printMigrateReport(report, rawURL)
	printMigratedIdentity(applied)
	return 0
}

// reloadAfterIdentity re-reads the config so keys just written by
// applyMigratedIdentity take effect in this same run. A config that no longer
// loads is left alone — the build proceeds with what it already had.
func reloadAfterIdentity(cfg *config.Config, genCfg generator.Config) (generator.Config, *config.Config) {
	path := config.FindConfigFile()
	if path == "" {
		return genCfg, cfg
	}
	reloaded, err := loadConfigFile(path)
	if err != nil || reloaded == nil {
		return genCfg, cfg
	}
	genCfg.Title, genCfg.Description, genCfg.Colors = reloaded.Title, reloaded.Description, reloaded.Colors
	cfg.Title, cfg.Description, cfg.Colors = reloaded.Title, reloaded.Description, reloaded.Colors
	return genCfg, cfg
}

// printMigratedIdentity reports the config keys the export filled in, so the
// operator sees that the file changed and what it now says.
func printMigratedIdentity(applied []string) {
	if len(applied) == 0 {
		return
	}
	fmt.Printf("\n🪪 Completed %s from the source site:\n", config.FindConfigFile())
	for _, line := range applied {
		fmt.Printf("   ✅ %s\n", line)
	}
}

// migrateLive is the owner-specified order: server FIRST with a visible
// address, then the data lands incrementally while watch rebuilds and
// auto-reload refreshes the browser, then the report — and the server keeps
// running until Ctrl+C.
func migrateLive(provider migrate.Provider, rawURL string, opts migrate.Options,
	genCfg generator.Config, cfg *config.Config) int {
	// An empty scaffold may not build yet — that is fine, watch recovers as
	// soon as the first content lands (same forgiveness as `ssg --watch`).
	runInitialBuild(genCfg, cfg)
	if cfg.HTTP {
		if autoReloadEnabled(cfg) {
			reloadHub = newLiveReloadHub()
		}
		go startServer(cfg)
	}
	if cfg.Watch {
		go runWatchLoop(genCfg, cfg)
	}
	if cfg.HTTP && !cfg.Quiet {
		_, url, _ := resolveListenAddr(cfg.Host, cfg.Port)
		fmt.Printf("\n🚚 Migrating %s — watch the site fill up live at %s\n\n", rawURL, url)
	}

	report, err := migrateFetch(provider, rawURL, opts)
	if err != nil {
		// Keep serving whatever already landed; the user sees the partial
		// site plus the error instead of a dead terminal.
		fmt.Fprintf(os.Stderr, "❌ %v\n   The server keeps running — press Ctrl+C to stop.\n", err)
		migrateBlock()
		return 1
	}
	applied := applyMigratedIdentity(config.FindConfigFile(), opts.Dest)
	genCfg, cfg = reloadAfterIdentity(cfg, genCfg)
	if !cfg.Watch {
		// --http without --watch: nothing rebuilds on its own, so build once
		// now that all content is on disk.
		if buildErr := build(genCfg, cfg); buildErr != nil {
			fmt.Fprintf(os.Stderr, "❌ building migrated site: %v\n", buildErr)
		}
	}
	printMigrateReport(report, rawURL)
	printMigratedIdentity(applied)
	if !cfg.Quiet {
		fmt.Println("   The server keeps running — press Ctrl+C to stop.")
	}
	migrateBlock()
	return 0
}

// migrateScaffold is initScaffold minus the sample page and post — migrated
// content replaces them, and a leftover "Hello, World" on a freshly migrated
// site would read as data loss. writeScaffold still never overwrites.
func migrateScaffold(source, domain string) []scaffoldFile {
	samples := map[string]bool{
		filepath.Join("content", source, "pages", "index.md"):       true,
		filepath.Join("content", source, "posts", "hello-world.md"): true,
	}
	var out []scaffoldFile
	for _, f := range initScaffold(source, domain) {
		if !samples[f.path] {
			out = append(out, f)
		}
	}
	return out
}

func reportScaffold(created, skipped []string) {
	for _, f := range created {
		fmt.Printf("   ✅ created %s\n", f)
	}
	for _, f := range skipped {
		fmt.Printf("   ⏭️  kept existing %s\n", f)
	}
}

// parseMigrateFlags parses everything after `ssg migrate <provider> <url>`.
// A returned code >= 0 means stop with that exit code.
func parseMigrateFlags(args []string) (migrateFlags, int) {
	var f migrateFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--watch":
			f.watch = true
		case arg == "--http":
			f.http = true
		case arg == "--quiet" || arg == "-q":
			f.quiet = true
		case arg == "--no-crawl":
			f.noCrawl = true
		case arg == "--all-media":
			f.allMedia = true
		case strings.HasPrefix(arg, "--content="):
			f.content = splitContentList(strings.TrimPrefix(arg, "--content="))
		case arg == "--content" && i+1 < len(args):
			f.content = splitContentList(args[i+1])
			i++
		case strings.HasPrefix(arg, "--source="):
			f.source = strings.TrimPrefix(arg, "--source=")
		case arg == "--source" && i+1 < len(args):
			f.source = args[i+1]
			i++
		default:
			fmt.Fprintf(os.Stderr, "❌ unknown flag %q\n\n", arg)
			printMigrateUsage()
			return f, 2
		}
	}
	if strings.ContainsAny(f.source, "/\\") || strings.Contains(f.source, "..") {
		fmt.Fprintf(os.Stderr, "❌ invalid source name %q\n", f.source)
		return f, 2
	}
	return f, -1
}

func splitContentList(list string) []string {
	var out []string
	for _, kind := range strings.Split(list, ",") {
		if kind = strings.TrimSpace(kind); kind != "" {
			out = append(out, kind)
		}
	}
	return out
}

func migrateProviderNames() []string {
	var names []string
	for _, p := range migrate.Providers() {
		names = append(names, p.Name())
	}
	return names
}

func printMigrateReport(report *migrate.Report, rawURL string) {
	fmt.Printf("\n✅ Migration finished (%s): %d pages, %d posts, %d media files from %s\n",
		report.Provider, report.Pages, report.Posts, report.Media, rawURL)
	for _, w := range report.Warnings {
		fmt.Printf("   ⚠️  %s\n", w)
	}
	fmt.Println("\n➡️  Next step — let an AI agent rebuild the template on top of this content:")
	printMCPWiring()
}

func printMigrateUsage() {
	fmt.Print(`usage: ssg migrate <provider> <url> [flags]

   ssg migrate wordpress https://example.com --content pages,posts,media
   ssg migrate wordpress https://example.com --content pages,posts --watch --http
   ssg migrate --list

flags:
   --content a,b,c   content kinds to fetch (default: everything the provider offers).
                     Kinds: pages, posts, media, custom (a theme's own post
                     types: Services, Portfolio, ...), products, tags, users,
                     menus. Site metadata (tags, users, menus) always ships — a
                     site without its navigation is not a migration; exclude it
                     explicitly with no-menus / no-tags / no-users.
   --watch --http    LIVE mode: scaffold + server first, then watch the data load
   --source NAME     content source directory name (default: the site's host)
   --all-media       download the whole media library, not just the files the
                     content references (the default keeps a migration small:
                     WordPress stores a dozen renditions of every upload and ssg
                     generates its own)
   --no-crawl        skip the SEO/marketing crawl (faster; no tracking ids,
                     social profiles or icons in metadata.json)
   --quiet, -q       suppress progress output
`)
}
