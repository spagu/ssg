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
	"strconv"
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
	// "comments" is what the site's readers wrote (wpexporter 1.8.5+): the one
	// part of a site its owner did not author and cannot rewrite. They land in
	// content/<source>/comments.json, addressed by page URL (#134).
	"comments": "--no-comments",
}

// wpMetadataKinds are NOT content: tags, authors and menus describe the site
// around the content. Asking for "pages,posts,media" and silently losing the
// navigation, the category names and the post authors is not what anyone
// means — a migrated site would come up without a menu. They ship unless
// explicitly excluded with --no-<kind>, in which case the report says so.
var wpMetadataKinds = map[string]bool{"tags": true, "users": true, "menus": true}

// wpUnsupportedKinds are kinds a user may reasonably ask for that the engine
// cannot deliver yet. They are reported as skipped — never silently dropped.
// (Empty since #134 gave comments a real export; the machinery stays because
// the next recognised-but-undeliverable kind must degrade, not error.)
var wpUnsupportedKinds = map[string]string{}

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
	// The newest engine that can actually be executed, not merely the first on
	// PATH — inside a snap that was always the bundled copy (#160).
	choice, err := selectEngine(opts)
	if err != nil {
		return nil, err
	}
	bin, banner := choice.Path, choice.Banner

	// Record what this engine can be asked to do BEFORE anything reads opts:
	// the argument builder and the report both depend on it (#137).
	if err := checkEngineVersion(banner, opts.Quiet); err != nil {
		return nil, err
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
			"   The engine reported %s; ssg migrate is built against %s or newer.\n"+
			"   If the failure names an unknown flag, upgrade it: snap refresh\n"+
			"   static-site-generator, or go install .../cmd/wpexporter@latest",
			runErr, engineLabel(bin, banner), engineVersionString(minimumEngine))
	}

	rep := &Report{Provider: p.Name() + "@" + p.Version(), Skipped: skipped,
		Engine: engineLabel(bin, banner)}
	// An engine that was found and could not be run is worth a line: it is the
	// answer to "I installed a newer wpexporter and nothing changed" (#160).
	rep.Warnings = append(rep.Warnings, engineSelectionNotes(choice)...)
	rep.Pages, rep.Posts, rep.Media = countExport(opts.Dest)
	rep.Comments = countComments(opts.Dest)
	rep.Menus = countMenus(opts.Dest)
	for _, kind := range skipped {
		rep.Warnings = append(rep.Warnings, kind+": "+wpUnsupportedKinds[kind])
	}
	if w := menusWarning(rep.Menus, opts); w != "" {
		rep.Warnings = append(rep.Warnings, w)
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
	args = append(args, authArgs(opts)...)
	args = append(args, customTypeArgs(opts)...)
	args = append(args, engineTuningArgs(opts)...)
	if len(opts.Content) == 0 {
		return append(args, opts.EngineArgs...), nil, nil
	}

	sel, err := parseContentSelection(opts.Content)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, disableFlags(sel)...)
	// Verbatim last, so an operator can override anything ssg derived.
	return append(args, opts.EngineArgs...), sel.skipped, nil
}

// engineTuningArgs carries the two flags bot protection actually needs (#171).
//
// The engine already diagnoses a Cloudflare block precisely and names these two
// by name — but it printed that advice inside a run started by `ssg migrate`,
// which had no way to act on it. The operator was told to try a flag by a tool
// they were not running, and the tool they were running could not pass it on.
func engineTuningArgs(opts Options) []string {
	var args []string
	if ua := strings.TrimSpace(opts.UserAgent); ua != "" {
		args = append(args, "--user-agent", ua)
	}
	// Milliseconds between requests, which is the engine's own unit. Zero means
	// "not set" there too, so it is passed only when asked for.
	if opts.RateLimit > 0 {
		args = append(args, "--rate-limit", strconv.Itoa(opts.RateLimit))
	}
	return args
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

// authArgs forwards credentials to the engine. A token and a user/password
// pair are both accepted because a site may offer either; the token wins, the
// way an Authorization header would.
func authArgs(opts Options) []string {
	if opts.AuthToken != "" {
		return []string{"--auth-token", opts.AuthToken}
	}
	var args []string
	if opts.AuthUser != "" {
		args = append(args, "--auth-user", opts.AuthUser)
	}
	if opts.AuthPass != "" {
		args = append(args, "--auth-pass", opts.AuthPass)
	}
	return args
}

// customTypeArgs selects the theme's own post types (#130).
func customTypeArgs(opts Options) []string {
	if opts.NoCustomTypes {
		return []string{"--no-custom-types"}
	}
	if len(opts.CustomTypes) > 0 {
		return []string{"--custom-types", strings.Join(opts.CustomTypes, ",")}
	}
	return nil
}

// menusWarning explains an export that came back without navigation. Silence
// here is how a migrated site goes live with no menu and nothing in the run
// saying why: WordPress refuses /wp/v2/menus to an anonymous caller, and the
// engine reports success regardless (#132).
func menusWarning(menus int, opts Options) string {
	if menus > 0 || excludedMenus(opts) {
		return ""
	}
	if opts.AuthToken == "" && opts.AuthUser == "" {
		return "menus: not readable without authentication — WordPress gates them behind " +
			"edit_theme_options; re-run with --auth-user/--auth-pass or --auth-token"
	}
	return "menus: none came back even with credentials — the account may lack edit_theme_options, " +
		"or the site may define no menus"
}

// excludedMenus reports whether the run asked for no menus, in which case
// their absence is the answer, not a problem.
func excludedMenus(opts Options) bool {
	for _, kind := range opts.Content {
		if strings.EqualFold(strings.TrimSpace(kind), "no-menus") {
			return true
		}
	}
	return false
}

// minimumEngine is the oldest wpexporter a migration runs against.
//
// Every export this provider asks for depends on it: --ssg-sections and
// --assisted-crawl (1.8.1), --relevant-media-only (1.8.2), --no-comments
// (1.8.5), --custom-types (1.8.4), and the fixes to ordered lists, term slugs,
// post-loop pages and shortcode expansion that landed through 1.8.11. Running
// an older one produces an export that looks complete and is not, which is the
// expensive kind of wrong — so it is refused before anything is written rather
// than after the project is scaffolded and, in live mode, the server is up.
var minimumEngine = [3]int{1, 8, 11}

// withEngine records what the installed engine can be asked to do. An
// unreadable version banner is treated as an old engine: skipping a flag costs
// a line in the report, sending an unknown one costs the whole migration.
// checkEngineVersion refuses an engine older than the minimum, naming what was
// found and how to move past it. An unreadable banner is NOT refused: a
// wrapper or a fork that prints something else is not proof of an old engine,
// and blocking a working setup over a formatting difference would be worse
// than the risk. It is reported instead.
func checkEngineVersion(banner string, quiet bool) error {
	major, minor, patch := parseSemver(banner)
	if major+minor+patch == 0 {
		if !quiet {
			fmt.Fprintf(os.Stderr, "⚠️  could not read the wpexporter version (%s needs %s or newer)\n",
				"ssg migrate", engineVersionString(minimumEngine))
		}
		return nil
	}
	if atLeast(banner, minimumEngine[0], minimumEngine[1], minimumEngine[2]) {
		return nil
	}
	return fmt.Errorf(`wpexporter %d.%d.%d is too old — ssg migrate needs %s or newer.
Every export it asks for depends on fixes released up to that version, so an
older engine produces an export that looks complete and is not. Upgrade with:
   snap refresh static-site-generator   (the snap bundles the engine)
   go install github.com/tradik/wpexporter/cmd/wpexporter@latest`,
		major, minor, patch, engineVersionString(minimumEngine))
}

// engineVersionString renders a version triple for a message.
func engineVersionString(v [3]int) string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// engineLabel names the engine that ran and where it came from. A snap carries
// its own copy, so "wpexporter 1.8.4 (bundled with the snap)" is the only line
// that explains why upgrading the host's binary changed nothing (#140).
func engineLabel(bin, banner string) string {
	major, minor, patch := parseSemver(banner)
	version := "unknown version"
	if major+minor+patch > 0 {
		version = fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	if snap := os.Getenv("SNAP"); snap != "" && strings.HasPrefix(bin, snap) {
		return "wpexporter " + version + " (bundled with the snap)"
	}
	return "wpexporter " + version + " (" + bin + ")"
}
