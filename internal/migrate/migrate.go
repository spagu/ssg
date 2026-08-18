// Package migrate turns an existing site (WordPress today; Drupal and static
// crawls later) into a ready-to-build ssg project. Providers are BUILT-IN
// only — one file per provider behind this interface, compiled into the ssg
// binary (owner decision 2026-08-12: no external provider binaries, no
// plugins). A provider may delegate the actual data pull to an external tool
// discovered like cwebp (e.g. wordpress → wpexporter); that is a tool
// dependency of the strategy, not an extension mechanism.
package migrate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Options carries everything a provider needs to fetch a site. The exec
// seams (LookPath, Run) default to the real thing and exist so tests never
// spawn processes or touch the network.
type Options struct {
	// Content lists the requested content kinds (pages, posts, media, ...).
	// Empty means the provider's full default export.
	Content []string
	// Dest is the destination content directory (content/<source>). The
	// provider writes ssg's native layout there: pages/, posts/, media/,
	// metadata.json.
	Dest  string
	Quiet bool
	// NoCrawl skips the extra per-page fetch that collects SEO metadata and
	// the site's marketing/analytics wiring — faster, but the migrated site
	// arrives without its tracking ids, social profiles and icons.
	NoCrawl bool
	// AllMedia downloads the source site's entire media library instead of only
	// the files its content references. Off by default: the library is mostly
	// renditions the generator recreates itself (#130).
	AllMedia bool
	// CustomTypes selects the theme's own post types by slug (Services,
	// Portfolio, Team). Empty takes whatever the engine exports by default;
	// NoCustomTypes skips them entirely (#130).
	CustomTypes   []string
	NoCustomTypes bool
	// Auth carries credentials to the engine. WordPress gates menus and
	// settings behind edit_theme_options, so a public export comes back
	// without navigation (#132). These are forwarded to the engine and never
	// written to the project's config — a password in a file that gets
	// committed is a worse problem than a missing menu.
	AuthUser  string
	AuthPass  string
	AuthToken string

	// UserAgent and RateLimit reach the engine's own flags of the same names
	// (#171). Bot protection in front of WordPress is ordinary, and the engine
	// already diagnoses it precisely — "try --user-agent with a browser's
	// string, --rate-limit to slow the crawl" — but that advice was printed
	// inside a run `ssg migrate` had no way to act on. RateLimit is the engine's
	// unit: milliseconds between requests.
	UserAgent string
	RateLimit int
	// EngineArgs are handed to the engine verbatim, after everything ssg
	// derives. The engine will grow flags this list cannot predict, and without
	// a pass-through every one of them is unreachable from `ssg migrate` until
	// ssg ships a release naming it.
	EngineArgs []string

	// EnginePath names the engine binary explicitly, overriding every search.
	// Set from --engine or SSG_WPEXPORTER: the snap bundles its own copy, and
	// without a way to say "use this one" an operator who installed a newer
	// engine had no way to reach it (#160).
	EnginePath string

	LookPath func(file string) (string, error)
	Run      func(name string, args []string, quiet bool) error
	// Version returns the engine's version banner. A provider gates flags the
	// installed engine may not know on it (#137).
	Version func(bin string) string
}

func (o Options) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

func (o Options) run(name string, args []string) error {
	if o.Run != nil {
		return o.Run(name, args, o.Quiet)
	}
	return runStreaming(name, args, o.Quiet)
}

// runStreaming executes the fetched tool, streaming its progress to the
// terminal so a live migration shows what is being pulled.
func runStreaming(name string, args []string, quiet bool) error {
	// #nosec G204 -- name is the LookPath-resolved path of a fixed tool name
	// (e.g. "wpexporter") and args are built internally from a validated URL
	// and known flags; nothing is passed through a shell.
	cmd := exec.Command(name, args...)
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// Report is a provider's honest summary: what landed on disk, what was
// requested but skipped (never silently), and anything worth a warning.
type Report struct {
	Provider string // "wordpress@1.0.0" — diagnostics for bug reports
	Pages    int
	Posts    int
	Media    int
	// Comments is what the site's readers wrote, as counted in the export's
	// comments.json (#134).
	Comments int
	// Menus counts the navigation menus the export brought back (#132).
	Menus int
	// Engine is the version banner of the tool that actually ran. The snap
	// bundles its own copy, so a host upgrade changes nothing — and without
	// this line nobody could tell which one produced the export (#140).
	Engine   string
	Skipped  []string // requested kinds the engine cannot deliver
	Warnings []string
}

// Provider migrates one kind of source site into ssg's native content model.
type Provider interface {
	Name() string
	Version() string
	Description() string
	// Fetch pulls the site at rawURL into opts.Dest and reports what landed.
	Fetch(rawURL string, opts Options) (*Report, error)
}

// registry holds the built-in providers, keyed by Name().
var registry = map[string]Provider{
	"wordpress": wordpressProvider{},
}

// Lookup returns the named provider.
func Lookup(name string) (Provider, bool) {
	p, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	return p, ok
}

// Providers returns every built-in provider, sorted by name.
func Providers() []Provider {
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ValidateURL checks that rawURL is an absolute http(s) URL with a host —
// the one shape every provider can work with.
func ValidateURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL %q must start with http:// or https://", rawURL)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL %q has no host", rawURL)
	}
	return u, nil
}

// countExport walks the destination directory and counts what a fetch left
// behind: markdown pages, markdown posts and media files.
func countExport(dest string) (pages, posts, media int) {
	count := func(sub string, mdOnly bool) int {
		n := 0
		_ = filepath.Walk(filepath.Join(dest, sub), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil //nolint:nilerr // a missing subtree simply counts zero
			}
			if !mdOnly || strings.EqualFold(filepath.Ext(path), ".md") {
				n++
			}
			return nil
		})
		return n
	}
	return count("pages", true), count("posts", true), count("media", false)
}

// countComments reads how many reader comments the export left behind.
//
// The count is stated by the export itself (comments.json's `total`) rather
// than recomputed here: the file is one record per comment across every page,
// and a migration report that walked it would be counting the same thing the
// engine already counted. A missing or unreadable file counts zero — comments
// are one kind among several, and their absence never fails a migration (#134).
func countComments(dest string) int {
	raw, err := os.ReadFile(filepath.Join(dest, "comments.json")) // #nosec G304 -- the tool's own export directory
	if err != nil {
		return 0
	}

	var file struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return 0
	}

	return file.Total
}

// countMenus reads how many navigation menus the export brought back. A
// metadata.json without the key is the normal shape of an unauthenticated run,
// not an error (#132).
func countMenus(dest string) int {
	raw, err := os.ReadFile(filepath.Join(dest, "metadata.json")) // #nosec G304 -- the tool's own export directory
	if err != nil {
		return 0
	}
	var file struct {
		Menus []json.RawMessage `json:"menus"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return 0
	}
	return len(file.Menus)
}

// versionOutput asks a tool for its version. Kept on Options so tests never
// exec anything, and so a provider can gate a flag on the engine's age.
func (o Options) versionOutput(bin string) string {
	if o.Version != nil {
		return o.Version(bin)
	}
	// #nosec G204 -- bin is the LookPath-resolved path of a fixed tool name
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// parseSemver pulls the first x.y.z out of a tool's version banner
// ("wpexporter version v1.8.4 (8c1aacb, built …)"). An unreadable banner
// yields zeros, which every capability check treats as "too old to assume".
func parseSemver(s string) (major, minor, patch int) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch
}

// atLeast reports whether a version banner names a version >= the given one.
func atLeast(banner string, major, minor, patch int) bool {
	haveMajor, haveMinor, havePatch := parseSemver(banner)
	if haveMajor != major {
		return haveMajor > major
	}
	if haveMinor != minor {
		return haveMinor > minor
	}
	return havePatch >= patch
}

var semverRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
