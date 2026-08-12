package migrate

// The wordpress provider orchestrates wpexporter — the default and only v1
// engine (owner decision 2026-08-12). ssg does not reimplement the WordPress
// REST crawl; it discovers the binary like cwebp, maps --content selections
// to wpexporter flags and builds the project around the export, whose
// markdown layout (pages/, posts/, media/, metadata.json) IS ssg's native
// content model.

import (
	"fmt"
	"sort"
	"strings"
)

const (
	wordpressVersion = "1.0.0"
	wpexporterBinary = "wpexporter"
)

// wpContentKinds maps each selectable kind to the wpexporter flag that turns
// it OFF. When --content is given, every kind absent from the selection is
// disabled; an empty selection keeps wpexporter's full default export.
var wpContentKinds = map[string]string{
	"pages":    "--no-pages",
	"posts":    "--no-posts",
	"media":    "--no-media",
	"tags":     "--no-tags",
	"users":    "--no-users",
	"menus":    "--no-menus",
	"products": "--no-products",
}

// wpUnsupportedKinds are kinds a user may reasonably ask for that the engine
// cannot deliver yet. They are reported as skipped — never silently dropped.
var wpUnsupportedKinds = map[string]string{
	"comments": "wpexporter's REST export does not export comments yet",
}

type wordpressProvider struct{}

func (wordpressProvider) Name() string    { return "wordpress" }
func (wordpressProvider) Version() string { return wordpressVersion }
func (wordpressProvider) Description() string {
	return "WordPress site via wpexporter (REST API; pages, posts, media, tags, users, menus, products)"
}

// Fetch runs wpexporter against rawURL, writing ssg-native markdown into
// opts.Dest, and counts what landed.
func (p wordpressProvider) Fetch(rawURL string, opts Options) (*Report, error) {
	if _, err := ValidateURL(rawURL); err != nil {
		return nil, err
	}
	bin, err := opts.lookPath(wpexporterBinary)
	if err != nil {
		return nil, fmt.Errorf(`wpexporter not found in PATH — the wordpress migration engine is a separate tool.
Install it with one of:
   snap install wpexporter
   go install github.com/tradik/wpexporter@latest
   https://github.com/tradik/wpexporter/releases`)
	}

	args, skipped, err := wpexporterArgs(rawURL, opts)
	if err != nil {
		return nil, err
	}
	if runErr := opts.run(bin, args); runErr != nil {
		return nil, fmt.Errorf("wpexporter failed: %w", runErr)
	}

	rep := &Report{Provider: p.Name() + "@" + p.Version(), Skipped: skipped}
	rep.Pages, rep.Posts, rep.Media = countExport(opts.Dest)
	for _, kind := range skipped {
		rep.Warnings = append(rep.Warnings, kind+": "+wpUnsupportedKinds[kind])
	}
	return rep, nil
}

// wpexporterArgs translates a --content selection into wpexporter's CLI.
// Unknown kinds are a hard error (a typo must not silently export
// everything); unsupported-but-known kinds come back as skipped.
func wpexporterArgs(rawURL string, opts Options) (args, skipped []string, err error) {
	args = []string{"export", "-u", rawURL, "-f", "markdown", "-o", opts.Dest, "--link-style", "root"}
	if opts.Quiet {
		args = append(args, "-q")
	}
	if len(opts.Content) == 0 {
		return args, nil, nil
	}

	requested := map[string]bool{}
	for _, kind := range opts.Content {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			continue
		}
		if _, unsupported := wpUnsupportedKinds[kind]; unsupported {
			skipped = append(skipped, kind)
			continue
		}
		if _, ok := wpContentKinds[kind]; !ok {
			return nil, nil, fmt.Errorf("unknown content kind %q — valid kinds: %s",
				kind, strings.Join(wpKindNames(), ", "))
		}
		requested[kind] = true
	}
	// Deterministic flag order keeps runs reproducible (and tests golden).
	for _, kind := range wpKindNames() {
		if flag, selectable := wpContentKinds[kind]; selectable && !requested[kind] {
			args = append(args, flag)
		}
	}
	return args, skipped, nil
}

// wpKindNames lists every kind --content accepts (supported + recognised),
// sorted for stable help texts and error messages.
func wpKindNames() []string {
	names := make([]string, 0, len(wpContentKinds)+len(wpUnsupportedKinds))
	for k := range wpContentKinds {
		names = append(names, k)
	}
	for k := range wpUnsupportedKinds {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
