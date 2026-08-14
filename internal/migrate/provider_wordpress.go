package migrate

// The wordpress provider orchestrates wpexporter — the default and only v1
// engine (owner decision 2026-08-12). ssg does not reimplement the WordPress
// REST crawl; it discovers the binary like cwebp, maps --content selections
// to wpexporter flags and builds the project around the export, whose
// markdown layout (pages/, posts/, media/, metadata.json) IS ssg's native
// content model.

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	wordpressVersion = "1.0.0"
	wpexporterBinary = "wpexporter"
)

// wpContentKinds maps each selectable kind to the wpexporter flag that turns
// it OFF. When --content is given, every CONTENT kind absent from the
// selection is disabled; an empty selection keeps wpexporter's full default
// export.
var wpContentKinds = map[string]string{
	"pages":    "--no-pages",
	"posts":    "--no-posts",
	"media":    "--no-media",
	"products": "--no-products",
	"tags":     "--no-tags",
	"users":    "--no-users",
	"menus":    "--no-menus",
	// "custom" is every post type a theme or plugin registered — Services,
	// Portfolio, Team (wpexporter 1.8.2+). They are content, so a --content
	// selection that does not name them turns them off like any other kind
	// (#130).
	"custom": "--no-custom-types",
}

// wpMetadataKinds are NOT content: tags, authors and menus describe the site
// around the content. Asking for "pages,posts,media" and silently losing the
// navigation, the category names and the post authors is not what anyone
// means — a migrated site would come up without a menu. They ship unless
// explicitly excluded with --no-<kind>, in which case the report says so.
var wpMetadataKinds = map[string]bool{"tags": true, "users": true, "menus": true}

// wpUnsupportedKinds are kinds a user may reasonably ask for that the engine
// cannot deliver yet. They are reported as skipped — never silently dropped.
var wpUnsupportedKinds = map[string]string{
	"comments": "wpexporter's REST export does not export comments yet",
}

// missingEngineMessage explains a missing engine truthfully for the way ssg was
// installed. Inside a snap ($SNAP set) the host's wpexporter is unreachable by
// design — strict confinement sees only the snap's own rootfs, and a snap
// cannot execute another snap — so "go install it" would be a lie: the snap
// ships its own copy, and a missing one means the snap is too old (#114).
func missingEngineMessage(snapDir string) string {
	if snapDir != "" {
		return `wpexporter is missing from this snap — the wordpress migration engine ships
inside it (since 1.8.29), because strict confinement cannot reach the host's
copy, not even the wpexporter snap (a snap cannot execute another snap).
   snap refresh static-site-generator
If it is already current, install ssg outside the snap so it can use your own
wpexporter: https://github.com/spagu/ssg/blob/main/docs/INSTALL.md`
	}
	return `wpexporter not found in PATH — the wordpress migration engine is a separate tool.
Install it with one of:
   go install github.com/tradik/wpexporter/cmd/wpexporter@latest
   snap install wpexporter
   https://github.com/tradik/wpexporter/releases`
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
		return nil, fmt.Errorf("%s", missingEngineMessage(os.Getenv("SNAP")))
	}

	args, skipped, err := wpexporterArgs(rawURL, opts)
	if err != nil {
		return nil, err
	}
	if runErr := opts.run(bin, args); runErr != nil {
		// The likeliest cause of an immediate failure is an engine older than
		// 1.8.1, which rejects --ssg-sections as an unknown flag; say so
		// instead of leaving the operator with a bare exit status.
		return nil, fmt.Errorf("wpexporter failed: %w\n"+
			"   If it is older than 1.8.1 it does not know --ssg-sections/--assisted-crawl —\n"+
			"   upgrade it, or re-run with --no-crawl for the crawl half only", runErr)
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
	// --ssg-sections (wpexporter 1.8.1) emits the "## Excerpt" / "## Content"
	// markers this parser reads and drops the duplicate leading H1; without it
	// every migrated page carried its title twice. --assisted-crawl fetches the
	// pages once more for SEO metadata and fills metadata.json's `marketing`
	// and `analytics` blocks (GTM/GA4 ids, social profiles, favicon, og:image),
	// which a migration needs and cannot reconstruct later.
	args = []string{"export", "-u", rawURL, "-f", "markdown", "-o", opts.Dest,
		"--link-style", "root", "--ssg-sections"}
	// Only the media the content actually points at. WordPress keeps a dozen
	// renditions of every upload and a theme demo's leftovers; one real site
	// exported 5,255 files (197 MB) of which 74 were referenced. ssg generates
	// its own responsive variants anyway, so the rest is weight in the
	// repository forever. --all-media takes the whole library (#130).
	if !opts.AllMedia {
		args = append(args, "--relevant-media-only")
	}
	if !opts.NoCrawl {
		args = append(args, "--assisted-crawl")
	}
	if opts.Quiet {
		args = append(args, "-q")
	}
	if len(opts.Content) == 0 {
		return args, nil, nil
	}

	sel, err := parseContentSelection(opts.Content)
	if err != nil {
		return nil, nil, err
	}
	return append(args, disableFlags(sel)...), sel.skipped, nil
}

// contentSelection is a parsed --content list: content kinds opted IN,
// metadata kinds opted OUT, and recognised-but-undeliverable kinds.
type contentSelection struct {
	requested map[string]bool
	excluded  map[string]bool
	skipped   []string
}

func parseContentSelection(list []string) (contentSelection, error) {
	sel := contentSelection{requested: map[string]bool{}, excluded: map[string]bool{}}
	for _, kind := range list {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			continue
		}
		// "--content pages,no-menus" opts a metadata kind OUT; content kinds
		// are opted IN by listing them.
		if off, isExclusion := strings.CutPrefix(kind, "no-"); isExclusion {
			if !wpMetadataKinds[off] {
				return sel, fmt.Errorf("no-%s cannot be excluded — only %s ship by default",
					off, strings.Join(metadataKindNames(), ", "))
			}
			sel.excluded[off] = true
			continue
		}
		if _, unsupported := wpUnsupportedKinds[kind]; unsupported {
			sel.skipped = append(sel.skipped, kind)
			continue
		}
		if _, ok := wpContentKinds[kind]; !ok {
			return sel, fmt.Errorf("unknown content kind %q — valid kinds: %s",
				kind, strings.Join(wpKindNames(), ", "))
		}
		sel.requested[kind] = true
	}
	return sel, nil
}

// disableFlags turns a selection into wpexporter's --no-* flags: content kinds
// not asked for are off, metadata kinds are on unless explicitly excluded.
// Sorted iteration keeps runs reproducible (and tests golden).
func disableFlags(sel contentSelection) []string {
	var flags []string
	for _, kind := range wpKindNames() {
		flag, selectable := wpContentKinds[kind]
		switch {
		case !selectable:
		case wpMetadataKinds[kind]:
			if sel.excluded[kind] {
				flags = append(flags, flag)
			}
		case !sel.requested[kind]:
			flags = append(flags, flag)
		}
	}
	return flags
}

// metadataKindNames lists the always-on kinds, sorted for stable messages.
func metadataKindNames() []string {
	names := make([]string, 0, len(wpMetadataKinds))
	for k := range wpMetadataKinds {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
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
