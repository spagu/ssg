// Package migrate turns an existing site (WordPress today; Drupal and static
// crawls later) into a ready-to-build ssg project. Providers are BUILT-IN
// only — one file per provider behind this interface, compiled into the ssg
// binary (owner decision 2026-08-12: no external provider binaries, no
// plugins). A provider may delegate the actual data pull to an external tool
// discovered like cwebp (e.g. wordpress → wpexporter); that is a tool
// dependency of the strategy, not an extension mechanism.
package migrate

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	LookPath func(file string) (string, error)
	Run      func(name string, args []string, quiet bool) error
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
