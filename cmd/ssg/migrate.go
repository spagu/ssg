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
	"strconv"
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
	// Credentials for the source CMS. Held here and handed to the engine —
	// never written into .ssg.yaml, which is a file people commit (#132).
	authUser      string
	authPass      string
	authToken     string
	customTypes   []string
	noCustomTypes bool
	// host and port address the live-mode server. `migrate` parses its own
	// flags, so the server flags every other command accepts were rejected
	// here — `--watch --http --port 8889` died on "unknown flag" after the
	// project had already been scaffolded (#135). 0 / "" means "as configured".
	host string
	port int
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
	if flags.host != "" {
		cfg.Host = flags.host
	}
	if flags.port > 0 {
		cfg.Port = flags.port
	}
	if cfg.Source == "" {
		cfg.Source = source
	}
	applyMinifyAll(cfg)
	setupTemplateEngine(cfg)
	downloadOnlineTheme(cfg)

	opts := migrate.Options{
		Content:       flags.content,
		Dest:          filepath.Join(cfg.ContentDir, cfg.Source),
		Quiet:         cfg.Quiet,
		NoCrawl:       flags.noCrawl,
		AuthUser:      flags.authUser,
		AuthPass:      flags.authPass,
		AuthToken:     flags.authToken,
		CustomTypes:   flags.customTypes,
		NoCustomTypes: flags.noCustomTypes,
		AllMedia:      flags.allMedia,
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
		// Claimed in the foreground: the address printed below must be the one
		// the server took, port walk included (#135).
		startServerAsync(cfg)
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
		case arg == "--no-custom-types":
			f.noCustomTypes = true
		case strings.HasPrefix(arg, "--auth-user="):
			f.authUser = strings.TrimPrefix(arg, "--auth-user=")
		case arg == "--auth-user" && i+1 < len(args):
			f.authUser = args[i+1]
			i++
		case strings.HasPrefix(arg, "--auth-pass="):
			f.authPass = strings.TrimPrefix(arg, "--auth-pass=")
		case arg == "--auth-pass" && i+1 < len(args):
			f.authPass = args[i+1]
			i++
		case strings.HasPrefix(arg, "--auth-token="):
			f.authToken = strings.TrimPrefix(arg, "--auth-token=")
		case arg == "--auth-token" && i+1 < len(args):
			f.authToken = args[i+1]
			i++
		case strings.HasPrefix(arg, "--custom-types="):
			f.customTypes = splitContentList(strings.TrimPrefix(arg, "--custom-types="))
		case arg == "--custom-types" && i+1 < len(args):
			f.customTypes = splitContentList(args[i+1])
			i++
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
		case strings.HasPrefix(arg, "--host="):
			f.host = strings.TrimPrefix(arg, "--host=")
		case arg == "--host" && i+1 < len(args):
			f.host = args[i+1]
			i++
		case strings.HasPrefix(arg, "--port="):
			if code := f.setPort(strings.TrimPrefix(arg, "--port=")); code >= 0 {
				return f, code
			}
		case arg == "--port" && i+1 < len(args):
			if code := f.setPort(args[i+1]); code >= 0 {
				return f, code
			}
			i++
		default:
			fmt.Fprintf(os.Stderr, "❌ unknown flag %q\n", arg)
			if hint := engineFlagHint(arg); hint != "" {
				fmt.Fprintf(os.Stderr, "   %s\n", hint)
			}
			fmt.Fprintln(os.Stderr)
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

// setPort parses a --port value. A returned code >= 0 means stop with it: a
// port that is not a number is a typo worth reporting, not a silent fallback
// to 8888 in a command whose whole point was choosing the port.
func (f *migrateFlags) setPort(value string) int {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 0 || port > 65535 {
		fmt.Fprintf(os.Stderr, "❌ invalid --port %q — expected 0-65535\n", value)
		return 2
	}
	f.port = port

	return -1
}

// engineFlagKinds maps the engine's own --no-<kind> flags to the ssg way of
// asking for the same thing. Passing wpexporter's flags to `ssg migrate` is an
// easy mistake — they appear in the engine's help, and the two tools sit one
// command apart — and the bare "unknown flag" left the operator guessing (#134).
var engineFlagKinds = map[string]string{
	"--no-custom-types": "custom",
	"--no-comments":     "comments",
	"--no-posts":        "posts",
	"--no-pages":        "pages",
	"--no-media":        "media",
	"--no-products":     "products",
}

// engineFlagHint explains how to express an engine flag as a --content
// selection, for the flags where that is what the operator meant.
func engineFlagHint(arg string) string {
	kind, ok := engineFlagKinds[strings.SplitN(arg, "=", 2)[0]]
	if !ok {
		return ""
	}

	return fmt.Sprintf("That is a wpexporter flag. ssg selects kinds instead: --content leaves %s out "+
		"unless you list it (so --content pages,posts already excludes %s).", kind, kind)
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
	// Comments are reported only when there are some: a zero on every migration
	// of a site that never had comments reads as a failure to fetch them (#134).
	if report.Comments > 0 {
		fmt.Printf("   💬 %d reader comments in comments.json, addressed by page URL\n", report.Comments)
	}
	// Navigation is an editorial arrangement the content cannot rebuild, so its
	// arrival is worth stating; its absence is warned about separately (#132).
	if report.Menus > 0 {
		fmt.Printf("   🧭 %d navigation menu(s) — themes read them as .Site.Menus.<location>\n", report.Menus)
	}
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
                     types: Services, Portfolio, ...), comments, products, tags,
                     users, menus. Site metadata (tags, users, menus) always
                     ships — a site without its navigation is not a migration;
                     exclude it explicitly with no-menus / no-tags / no-users.
                     Naming --content at all opts every unlisted kind OUT, so
                     --content pages,posts leaves the theme's own types and the
                     readers' comments behind: list them to keep them.
   --watch --http    LIVE mode: scaffold + server first, then watch the data load
   --host ADDR       live-mode bind address (default: 127.0.0.1)
   --port PORT       live-mode port (default: 8888; a busy port shifts forward
                     and the address actually served is announced)
   --source NAME     content source directory name (default: the site's host)
   --all-media       download the whole media library, not just the files the
                     content references (the default keeps a migration small:
                     WordPress stores a dozen renditions of every upload and ssg
                     generates its own)
   --no-crawl        skip the SEO/marketing crawl (faster; no tracking ids,
                     social profiles or icons in metadata.json)
   --auth-user U --auth-pass P   credentials for the source CMS
   --auth-token T    bearer token instead of the pair. WordPress gates menus
                     and settings behind edit_theme_options, so without these
                     a migrated site arrives with no navigation
   --custom-types a,b  the theme's own post types (Services, Portfolio, Team)
   --no-custom-types   skip them entirely
   --quiet, -q       suppress progress output

Credentials are handed to the engine and never written to .ssg.yaml.
`)
}
