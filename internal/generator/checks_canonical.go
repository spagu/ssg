package generator

// An empty canonical is a silent whole-site failure (#247).
//
// `<link rel="canonical" href=""/>` is never correct for any site, and nothing
// reported it: the build succeeds, check_links is clean — there is no link to
// follow — and check_meta looks at the title and the description. It reaches
// production and comes back weeks later in a crawl report.
//
// The cause is always the same shape: a template names a value its context does
// not carry. Go templates resolve a missing map key to nil, which renders empty
// rather than failing, so one typo in one theme file ships an empty canonical on
// every page it renders. That is how #245 stayed invisible — ten archives on a
// live site, every one of them href="".
//
// So this warns unconditionally, with no mode and no configuration to enable. A
// canonical that disagrees with the permalink needs judgement and therefore an
// opt-in (sitemap_prune_canonical); an empty one needs none. It stays a warning:
// the site is publishable, and a build that refused to finish over a theme bug
// would be worked around rather than fixed.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// emptyURLTags are the head tags whose whole job is to name the page's own
// address. They are written from one value in every theme, so they fail
// together and are worth reporting together.
var (
	canonicalLinkRe = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?canonical\b[^>]*>`)
	selfURLMetaRe   = regexp.MustCompile(`(?is)<meta\b[^>]*\b(?:og:url|twitter:url)\b[^>]*>`)
	tagAttrRe       = regexp.MustCompile(`(?is)\b(href|content)\s*=\s*("[^"]*"|'[^']*'|[^\s"'>]+)`)
)

// noteEmptyCanonical records outputPath if its finished HTML names its own
// address with an empty value. Called once per rendered document, from the two
// write paths, with the bytes that are about to hit disk — so it sees what a
// crawler will see, after every transform and whichever template engine
// produced it.
func (g *Generator) noteEmptyCanonical(outputPath, s string) {
	head := documentHead(s)
	// The overwhelmingly common case is a page with a perfectly good canonical,
	// so decide that with a substring search before running any expression.
	if !strings.Contains(head, "canonical") && !strings.Contains(head, "og:url") &&
		!strings.Contains(head, "twitter:url") {
		return
	}
	var empty []string
	if tag := canonicalLinkRe.FindString(head); tag != "" && selfURLValue(tag) == "" {
		empty = append(empty, `<link rel="canonical">`)
	}
	for _, tag := range selfURLMetaRe.FindAllString(head, -1) {
		if selfURLValue(tag) != "" {
			continue
		}
		name := "og:url"
		if strings.Contains(strings.ToLower(tag), "twitter:url") {
			name = "twitter:url"
		}
		empty = append(empty, name)
	}
	if len(empty) == 0 {
		return
	}
	rel := outputPath
	if r, err := filepath.Rel(g.config.OutputDir, outputPath); err == nil {
		rel = r
	}
	g.canonicalMu.Lock()
	defer g.canonicalMu.Unlock()
	if g.emptyCanonicals == nil {
		g.emptyCanonicals = map[string]string{}
	}
	g.emptyCanonicals[filepath.ToSlash(rel)] = strings.Join(empty, ", ")
}

// documentHead narrows the search to <head>, where these tags belong. A
// canonical URL printed in the body — in a code sample documenting this very
// problem, for instance — is text, not a claim about the page.
func documentHead(s string) string {
	if i := strings.Index(strings.ToLower(s), "</head>"); i >= 0 {
		return s[:i]
	}
	return s
}

// selfURLValue returns the address a canonical/og:url/twitter:url tag names.
// An attribute that is absent altogether reads the same as an empty one: either
// way the tag claims an address and supplies none.
func selfURLValue(tag string) string {
	m := tagAttrRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(m[2], `"'`))
}

// resetEmptyCanonicals clears the record at the start of a build, so a watch-mode
// rebuild reports the site as it is now rather than every page that was ever
// wrong in this process.
func (g *Generator) resetEmptyCanonicals() {
	g.canonicalMu.Lock()
	defer g.canonicalMu.Unlock()
	g.emptyCanonicals = nil
}

// emptyCanonicalReportLimit is how many files are named before the rest are
// counted. The failure is usually site-wide, and a warning that scrolls the
// whole build out of the terminal teaches people to ignore warnings.
const emptyCanonicalReportLimit = 5

// reportEmptyCanonicals says, once per build, which pages name their own address
// with nothing.
func (g *Generator) reportEmptyCanonicals() {
	g.canonicalMu.Lock()
	defer g.canonicalMu.Unlock()
	if len(g.emptyCanonicals) == 0 || g.config.Quiet {
		return
	}
	files := make([]string, 0, len(g.emptyCanonicals))
	for file := range g.emptyCanonicals {
		files = append(files, file)
	}
	sort.Strings(files)
	fmt.Printf("   ⚠️  %d page(s) name their own URL with an empty value\n", len(files))
	for i, file := range files {
		if i == emptyCanonicalReportLimit {
			fmt.Printf("      …and %d more\n", len(files)-i)
			break
		}
		fmt.Printf("      %s → %s\n", file, g.emptyCanonicals[file])
	}
	fmt.Println("      A template naming a value its context does not carry renders empty rather than failing.")
	fmt.Println("      Archives carry .CanonicalURL; pages and posts carry it too. See docs/TEMPLATES.md.")
}
