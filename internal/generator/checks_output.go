package generator

// Build-time validation of the rendered HTML: image alt attributes (#75) and
// page metadata (#76). Both mirror the shape of check_links (SEO-005) — a mode
// of "" (off), "warn" or "strict", escalated to strict by the global strict flag
// — because they answer the same kind of question: the build succeeded, but is
// the output actually correct?
//
// Both check the OUTPUT, not the source, so an image written by a template and
// one written in Markdown are covered by the same pass. Neither ever invents
// content: they report what a human has to decide, which is the whole point.
// Silent whole-site failures are what motivated them — a theme interpolating a
// field that is always empty produces no warning anywhere today.

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	stdhtml "html"

	"golang.org/x/net/html"

	"github.com/spagu/ssg/internal/models"
)

// finding is one reported problem: the output file and what is wrong in it.
type finding struct {
	file   string // output-relative path
	detail string // the offending src / the missing tag
}

// walkOutputHTML calls visit for every generated HTML file, with its parsed tree
// and its output-relative path. Unreadable or unparseable files are skipped
// rather than failing the build — a check must not be the thing that breaks it.
func (g *Generator) walkOutputHTML(visit func(rel string, doc *html.Node)) error {
	root := g.config.OutputDir
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		f, e := os.Open(path) // #nosec G304 -- CLI reads its own output
		if e != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		doc, e := html.Parse(f)
		if e != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		visit(filepath.ToSlash(rel), doc)
		return nil
	})
}

// forEachElement walks a parsed document, calling visit for every element with
// the given tag name.
func forEachElement(doc *html.Node, tag string, visit func(*html.Node)) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			visit(n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

// attr returns an element's attribute value and whether the attribute is present
// at all. The distinction matters: a missing alt and an empty alt mean different
// things (#75).
func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// sortFindings orders findings so a report is stable between builds.
func sortFindings(f []finding) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].file != f[j].file {
			return f[i].file < f[j].file
		}
		return f[i].detail < f[j].detail
	})
}

// resolveMode returns the effective mode for a check, with the global strict
// flag (#62) escalating any enabled check to fatal.
func (g *Generator) resolveMode(mode string) string {
	if g.config.Strict {
		return "strict"
	}
	return mode
}

// report prints findings and returns an error when the mode is strict.
func (g *Generator) report(findings []finding, mode, label, okMsg, errFmt string) error {
	for _, f := range findings {
		fmt.Printf("   ⚠️  %s in %s → %s\n", label, f.file, f.detail)
	}
	if mode == "strict" && len(findings) > 0 {
		return fmt.Errorf(errFmt, len(findings))
	}
	if len(findings) == 0 && !g.config.Quiet {
		fmt.Println("   ✅ " + okMsg)
	}
	return nil
}

// checkImagesIfRequested validates image alt attributes over the built HTML
// (#75). It reports only images with NO alt attribute: `alt=""` is the correct
// WCAG treatment for a decorative image (a logo beside the site name that would
// otherwise be announced twice), so flagging it would train authors to add noise.
// Nothing is ever generated — an invented description reads as authoritative
// while being wrong, which is worse for a screen-reader user than silence.
//
// check_images: "strict-decorative" additionally reports `alt=""`, for authors
// who want to review every decorative decision. It is opt-in for that reason.
func (g *Generator) checkImagesIfRequested() error {
	mode := g.resolveMode(g.config.CheckImages)
	if mode == "" {
		return nil
	}
	g.log("🖼️  Checking image alt attributes...")

	reportDecorative := g.config.CheckImages == "strict-decorative"
	var findings []finding
	err := g.walkOutputHTML(func(rel string, doc *html.Node) {
		forEachElement(doc, "img", func(n *html.Node) {
			src, _ := attr(n, "src")
			if src == "" {
				src = "(no src)"
			}
			alt, present := attr(n, "alt")
			switch {
			case !present:
				findings = append(findings, finding{rel, src + " (no alt attribute)"})
			case reportDecorative && strings.TrimSpace(alt) == "":
				findings = append(findings, finding{rel, src + ` (alt="" — decorative)`})
			}
		})
	})
	if err != nil {
		return err
	}
	sortFindings(findings)
	if mode == "strict-decorative" {
		mode = "strict"
	}
	// "image alt" rather than "missing alt": the same line also reports a present
	// but empty alt under strict-decorative, where "missing" would contradict it.
	return g.report(findings, mode, "image alt", "every image has an alt attribute",
		"%d image(s) with an alt attribute to review")
}

// Default advisory ranges: roughly what search engines display in full before
// truncating. They are only defaults — meta_limits in config overrides each one,
// because the right length depends on the site and on which engines matter, and
// an explicit 0 switches a bound off entirely (#76).
const (
	defaultTitleMin       = 30
	defaultTitleMax       = 60
	defaultDescriptionMin = 70
	defaultDescriptionMax = 160
)

// limitOr returns the configured bound, or def when it is unset. An explicit 0
// is honoured — that is how a bound is disabled.
func limitOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// lengthFinding reports a rune length outside [min, max], skipping a bound set
// to 0. Returns "" when the value is within range or there is nothing to check.
func lengthFinding(what, value string, min, max int) string {
	n := len([]rune(strings.TrimSpace(value)))
	if n == 0 {
		return "" // emptiness is a hard finding elsewhere, not a length warning
	}
	switch {
	case min > 0 && n < min:
		return fmt.Sprintf("%s is %d characters (shorter than %d)", what, n, min)
	case max > 0 && n > max:
		return fmt.Sprintf("%s is %d characters (longer than %d)", what, n, max)
	}
	return ""
}

// checkMetaIfRequested validates rendered page metadata (#76): a page that is
// indexable must have a non-empty <title> and a non-empty meta description.
//
// This exists because the failure mode is invisible. A theme interpolating the
// wrong field — `.Excerpt` where it meant `.Description` — emits an empty tag on
// every page, forever, with no warning: the generator did exactly what the
// template asked. The data needed to catch it is right there, since SSG parsed a
// description for each page and then wrote an empty one out.
//
// noindex pages are skipped. A 404 page carrying `noindex, nofollow` legitimately
// has no description, and a check that flagged it would be worked around
// immediately, which costs more than it catches.
func (g *Generator) checkMetaIfRequested() error {
	mode := g.resolveMode(g.config.CheckMeta)
	if mode == "" {
		return nil
	}
	g.log("🏷️  Checking page metadata...")

	lim := g.config.MetaLimits
	titleMin, titleMax := limitOr(lim.TitleMin, defaultTitleMin), limitOr(lim.TitleMax, defaultTitleMax)
	descMin, descMax := limitOr(lim.DescriptionMin, defaultDescriptionMin), limitOr(lim.DescriptionMax, defaultDescriptionMax)

	var findings, warnings []finding
	err := g.walkOutputHTML(func(rel string, doc *html.Node) {
		if isNoindexDoc(doc) {
			return
		}
		title := elementText(doc, "title")
		if strings.TrimSpace(title) == "" {
			findings = append(findings, finding{rel, "no <title>"})
		} else if w := lengthFinding("title", title, titleMin, titleMax); w != "" {
			warnings = append(warnings, finding{rel, w})
		}
		desc, present := metaContent(doc, "description")
		switch {
		case !present:
			findings = append(findings, finding{rel, "no meta description"})
		case strings.TrimSpace(desc) == "":
			findings = append(findings, finding{rel, "empty meta description"})
		default:
			// Length is advisory: outside the display window is worth knowing,
			// but it is never a build failure.
			if w := lengthFinding("meta description", desc, descMin, descMax); w != "" {
				warnings = append(warnings, finding{rel, w})
			}
		}
	})
	if err != nil {
		return err
	}
	sortFindings(findings)
	sortFindings(warnings)
	for _, w := range warnings {
		fmt.Printf("   ℹ️  %s\n", w.detail+" in "+w.file)
	}
	return g.report(findings, mode, "metadata", "every indexable page has a title and description",
		"%d page(s) with missing metadata")
}

// isNoindexDoc reports whether a document asks robots not to index it.
func isNoindexDoc(doc *html.Node) bool {
	robots, ok := metaContent(doc, "robots")
	return ok && strings.Contains(strings.ToLower(robots), "noindex")
}

// metaContent returns the content of <meta name="..."> and whether the tag is
// present. A tag with no content attribute counts as present-but-empty, which is
// exactly the silent failure this check exists to surface.
func metaContent(doc *html.Node, name string) (string, bool) {
	var content string
	var found bool
	forEachElement(doc, "meta", func(n *html.Node) {
		if found {
			return
		}
		if v, ok := attr(n, "name"); ok && strings.EqualFold(v, name) {
			found = true
			content, _ = attr(n, "content")
		}
	})
	return content, found
}

// elementText returns the concatenated text of the first element with the given
// tag name.
func elementText(doc *html.Node, tag string) string {
	var out string
	var done bool
	forEachElement(doc, tag, func(n *html.Node) {
		if done {
			return
		}
		done = true
		var sb strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				sb.WriteString(c.Data)
			}
		}
		out = sb.String()
	})
	return out
}

// checkOrphansIfRequested reports published, indexable pages that nothing on the
// site links to (#77). At the end of a build SSG is the only thing that knows
// every page it emitted and every link it wrote, so the set difference is nearly
// free — and freshest — here.
//
// Three rules decide whether this is useful or silently useless:
//
//   - Count only <a href>. Every page links to itself through
//     <link rel="canonical">, so counting all refs makes nothing an orphan and
//     the check passes happily on a site full of them.
//   - Ignore a page linking to itself, for the same reason.
//   - Skip noindex pages. They are not in search results, so an inbound link
//     buys them nothing; a 404 page would otherwise be reported every build.
func (g *Generator) checkOrphansIfRequested() error {
	mode := g.resolveMode(g.config.CheckOrphans)
	if mode == "" {
		return nil
	}
	g.log("🔍 Checking for orphan pages...")

	linked := map[string]bool{}
	candidates := map[string]bool{}
	err := g.walkOutputHTML(func(rel string, doc *html.Node) {
		// The site root is the entry point: nothing links to "/" and nothing needs
		// to, so reporting it would be a false positive on every site.
		if !isNoindexDoc(doc) && rel != indexHTMLName {
			candidates[rel] = true
		}
		forEachElement(doc, "a", func(n *html.Node) {
			href, ok := attr(n, "href")
			if !ok || !isInternalRef(href) {
				return
			}
			if target := g.linkTarget(href, rel); target != "" && target != rel {
				linked[target] = true
			}
		})
	})
	if err != nil {
		return err
	}

	var findings []finding
	for page := range candidates {
		if !linked[page] {
			findings = append(findings, finding{page, "no inbound links"})
		}
	}
	sortFindings(findings)
	return g.report(findings, mode, "orphan page", "every indexable page is linked from somewhere",
		"%d orphan page(s)")
}

// linkTarget resolves an <a href> to the output-relative HTML file it points at,
// or "" when it points outside the output tree. Directory-style links resolve to
// the index.html they serve, so "/docs/" and "/docs/index.html" are one page.
func (g *Generator) linkTarget(href, fromRel string) string {
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		href = href[:i]
	}
	if href == "" {
		return ""
	}
	var rel string
	if strings.HasPrefix(href, "/") {
		rel = strings.TrimPrefix(href, "/")
	} else {
		rel = path.Join(path.Dir(filepath.ToSlash(fromRel)), href)
	}
	rel = path.Clean(rel)
	switch {
	case rel == "." || rel == "/" || rel == "":
		return indexHTMLName
	case strings.HasSuffix(rel, ".html"):
		return rel
	}
	// A directory-style link is served by its index.html — but where the host
	// strips extensions, "/validator" is served by validator.html and there is
	// no directory at all (#107). Comparing the pre-normalisation path made
	// every such page an orphan while check_links resolved the very same links.
	dirIndex := path.Join(rel, indexHTMLName)
	if g.config.PrettyURLs.Enabled() && !g.outputFileExists(dirIndex) && g.outputFileExists(rel+".html") {
		return rel + ".html"
	}
	return dirIndex
}

// outputFileExists reports whether an output-relative path is a file that was
// written.
func (g *Generator) outputFileExists(rel string) bool {
	info, err := os.Stat(filepath.Join(g.config.OutputDir, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

// excludesFromSitemap decides whether a page belongs in sitemap.xml, consulting
// both its front matter and the HTML that was actually rendered for it (#78).
//
// Front matter alone is not enough: `noindex` and the canonical URL are usually
// theme decisions, written by the template long after the front matter is read.
// A sitemap that lists a page whose own markup says "do not index this URL", or
// that names a different URL as canonical, asks a crawler for exactly what the
// page declines — Search Console reports both as errors. The sitemap is written
// after rendering, so by this point the answer is on disk.
func (g *Generator) excludesFromSitemap(page models.Page) bool {
	return g.excludesFromSitemapAt(page, page.GetOutputPath())
}

// excludesFromSitemapAt is the same decision for ONE emitted URL, judged against
// the file actually served at it.
//
// Exclusion has to be per URL, not per page: a source file can emit more than one
// URL — a page slugged "index" produces both "/" and "/index/" — and reading one
// page's verdict from one file applied it to every URL it emitted. A theme
// marking the "/index/" duplicate noindex therefore also dropped the site root,
// which is far worse than the duplicate it was fixing, and silently (#88).
func (g *Generator) excludesFromSitemapAt(page models.Page, relFile string) bool {
	if excludeFromSitemap(page) {
		return true
	}
	return g.renderedExcludesItself(page, relFile)
}

// renderedExcludesItself reports whether a page's own output marks it noindex or
// canonicalises to a different URL. Results are memoized per output path; an
// unreadable page is not excluded, since a missing file is not a statement.
func (g *Generator) renderedExcludesItself(page models.Page, outputPath string) bool {
	// The path names the page's directory ("docs/intro"), not the file that serves
	// it, so resolve to the index.html actually written there.
	rel := strings.TrimPrefix(outputPath, "/")
	if !strings.HasSuffix(rel, ".html") {
		rel = path.Join(rel, indexHTMLName)
	}
	if rel == "" || rel == indexHTMLName && page.GetOutputPath() == "" {
		return false
	}
	g.sitemapSelfMu.Lock()
	defer g.sitemapSelfMu.Unlock()
	if v, ok := g.sitemapSelf[rel]; ok {
		return v
	}
	excluded := false
	f, err := os.Open(filepath.Join(g.config.OutputDir, filepath.FromSlash(rel))) // #nosec G304 -- CLI reads its own output
	if err == nil {
		defer func() { _ = f.Close() }()
		if doc, perr := html.Parse(f); perr == nil {
			// noindex is unambiguous: the page itself says do not index this URL.
			// A mismatched canonical is not — far more often it is a theme whose
			// canonical disagrees with the permalink than a deliberate exclusion,
			// and silently dropping real pages from the sitemap over a theme bug
			// is too big a hammer for a default. Opt in with
			// sitemap_prune_canonical.
			excluded = isNoindexDoc(doc)
			if !excluded && g.config.SitemapPruneCanonical {
				// Compare against the URL this file is served at, which for the
				// site root is "/" rather than the page's own permalink (#88).
				excluded = canonicalPointsElsewhere(doc, httpsScheme+g.config.Domain+urlForOutputFile(rel))
			}
		}
	}
	if g.sitemapSelf == nil {
		g.sitemapSelf = map[string]bool{}
	}
	g.sitemapSelf[rel] = excluded
	return excluded
}

// canonicalPointsElsewhere reports whether the document names a canonical URL
// other than its own. A missing canonical is not a contradiction — plenty of
// themes omit it — so only an explicit, differing one excludes the page.
func canonicalPointsElsewhere(doc *html.Node, own string) bool {
	var canonical string
	forEachElement(doc, "link", func(n *html.Node) {
		if canonical != "" {
			return
		}
		if rel, ok := attr(n, "rel"); ok && strings.EqualFold(strings.TrimSpace(rel), "canonical") {
			canonical, _ = attr(n, "href")
		}
	})
	canonical = strings.TrimSpace(canonical)
	if canonical == "" || own == "" {
		return false
	}
	return !sameURL(canonical, own)
}

// sameURL compares two URLs ignoring scheme and a trailing slash. When either
// side is host-less it compares paths only, because a canonical written as
// "/docs/intro/" is the overwhelmingly common form and means the same page as
// "https://example.com/docs/intro/". Getting this wrong excludes every page from
// the sitemap, since each one then looks like it canonicalises elsewhere.
func sameURL(a, b string) bool {
	ua, erra := url.Parse(strings.TrimSpace(a))
	ub, errb := url.Parse(strings.TrimSpace(b))
	if erra != nil || errb != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	trim := func(p string) string {
		if p == "/" {
			return ""
		}
		return strings.TrimSuffix(p, "/")
	}
	if ua.Host == "" || ub.Host == "" {
		return trim(ua.Path) == trim(ub.Path)
	}
	return ua.Host == ub.Host && trim(ua.Path) == trim(ub.Path)
}

// metaDescriptionRe matches a meta description tag with non-empty content. A
// present-but-empty tag does not count: that is exactly the silent failure the
// --seo fallback exists to repair (#76).
var metaDescriptionRe = regexp.MustCompile(`(?is)<meta[^>]+name\s*=\s*["']description["'][^>]*>`)

// fillMetaDescription makes the page carry desc as its meta description when the
// rendered HTML has none or only an empty one (#76). An existing non-empty tag is
// left alone — the theme meant it. An empty one is rewritten in place, and a
// missing one is added before </head>.
func fillMetaDescription(s, desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return s
	}
	tags := metaDescriptionRe.FindAllStringIndex(s, -1)
	for _, loc := range tags {
		tag := s[loc[0]:loc[1]]
		if m := metaContentAttrRe.FindStringSubmatch(tag); m != nil && strings.TrimSpace(m[1]) != "" {
			return s // the theme already provides one
		}
	}
	replacement := fmt.Sprintf("<meta name=\"description\" content=%q>", stdhtml.EscapeString(desc))
	if len(tags) > 0 {
		loc := tags[0]
		return s[:loc[0]] + replacement + s[loc[1]:]
	}
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		return s[:i] + replacement + "\n" + s[i:]
	}
	return s
}

// metaContentAttrRe extracts the content attribute of a meta tag.
var metaContentAttrRe = regexp.MustCompile(`(?is)content\s*=\s*["']([^"']*)["']`)

// excludedFromContent reports whether a Markdown file matches content_exclude
// and must not be loaded as a page (#74).
//
// content_dir is scanned recursively and every .md becomes a page, with no way
// to say "this file is data, not content". A sample that documents a different
// tool's front-matter format is correct for its own purpose yet invalid as a
// page: the build warns, drops it, and exits 0. Moving the file out of
// content_dir is the only workaround today, which is a real cost when the sample
// belongs beside the documentation referencing it.
//
// Patterns are matched against the project-relative path with forward slashes,
// against the path relative to the content root, and against the bare filename,
// so "docs/examples/**", "examples/*.md" and "sample-*.md" all behave the way
// they read. "**" matches across directory separators; other syntax is
// filepath.Match.
func (g *Generator) excludedFromContent(entryPath string) bool {
	if len(g.config.ContentExclude) == 0 {
		return false
	}
	slashed := filepath.ToSlash(entryPath)
	candidates := []string{slashed, path.Base(slashed)}
	if root := filepath.ToSlash(filepath.Join(g.config.ContentDir, g.config.Source)); root != "" {
		if rel, err := filepath.Rel(root, entryPath); err == nil {
			candidates = append(candidates, filepath.ToSlash(rel))
		}
	}
	for _, pattern := range g.config.ContentExclude {
		for _, c := range candidates {
			if matchGlob(pattern, c) {
				return true
			}
		}
	}
	return false
}

// matchGlob is filepath.Match plus "**", which filepath.Match does not support:
// a "**" segment matches across directory separators, so "docs/**" covers
// "docs/a/b.md" and not only "docs/b.md".
func matchGlob(pattern, name string) bool {
	if !strings.Contains(pattern, "**") {
		ok, err := path.Match(pattern, name)
		return err == nil && ok
	}
	// Translate the pattern into a regexp, escaping everything except the glob
	// metacharacters we support.
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			sb.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			sb.WriteString(".*")
			i++
		case pattern[i] == '*':
			sb.WriteString("[^/]*")
		case pattern[i] == '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	return err == nil && re.MatchString(name)
}

// urlForOutputFile is the URL an output file is served at: "a/b/index.html" is
// "/a/b/", and the root "index.html" is "/".
func urlForOutputFile(rel string) string {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if rel == indexHTMLName {
		return "/"
	}
	if dir := path.Dir(rel); strings.HasSuffix(rel, "/"+indexHTMLName) || path.Base(rel) == indexHTMLName {
		if dir == "." {
			return "/"
		}
		return "/" + dir + "/"
	}
	return "/" + rel
}

// checkRedirectsIfRequested reports internal links the host would redirect
// rather than serve (#87).
//
// Nothing here is broken — which is exactly why check_links passes them — but
// each one costs a visitor a round trip and a crawler a hop of budget, and the
// cost multiplies: one ".html" link in shared chrome puts every page on the site
// through a redirect. It is invisible locally, because resolution against the
// output directory is not how the host answers.
//
// The rules are host behaviour, not ours, so this only runs with pretty_urls set:
// on a plain object store nothing is rewritten and "/docs/intro" is a genuine 404
// rather than a redirect, so reporting it would be wrong.
func (g *Generator) checkRedirectsIfRequested() error {
	mode := g.resolveMode(g.config.CheckRedirects)
	if mode == "" {
		return nil
	}
	if !g.config.PrettyURLs.Enabled() {
		fmt.Println("   ⚠️  check_redirects needs pretty_urls to know how the host serves URLs — skipping")
		return nil
	}
	g.log("↪️  Checking for links that only resolve via a redirect...")

	var findings []finding
	err := g.walkOutputHTML(func(rel string, doc *html.Node) {
		for _, tag := range []string{"a", "link"} {
			forEachElement(doc, tag, func(n *html.Node) {
				href, ok := attr(n, "href")
				if !ok || !isInternalRef(href) {
					return
				}
				if final := g.redirectTargetOf(href); final != "" {
					findings = append(findings, finding{rel, href + "  →  " + final})
				}
			})
		}
	})
	if err != nil {
		return err
	}
	sortFindings(findings)
	return g.report(findings, mode, "redirected link", "no link needs a redirect",
		"%d internal link(s) that only resolve through a redirect")
}

// redirectTargetOf returns the URL a pretty-URL host would answer with, or ""
// when the link is already the final form. Two shapes cost a redirect:
//
//	/docs/intro.html  →  /docs/intro/   (the extension is stripped)
//	/docs/intro       →  /docs/intro/   (the slash is appended)
//
// A file that is not a page — a feed, an image, a stylesheet — is served as-is
// and is never a finding.
func (g *Generator) redirectTargetOf(href string) string {
	// The mode decides the destination, not a fixed assumption (#103): under
	// `strip` the host serves /docs/intro with no trailing slash, so suggesting
	// /docs/intro/ would name a URL that host would itself redirect.
	if served := g.config.PrettyURLs.ServedURL(href); served != href {
		return served
	}
	clean, suffix := splitURLSuffix(href)
	if clean == "" || clean == "/" {
		return ""
	}
	// Extensionless and slashless costs a redirect only where the host appends
	// the slash, and only if a directory is actually served there — otherwise it
	// is a genuinely missing page, which is check_links' finding to make.
	if g.config.PrettyURLs == models.PrettyStripSlash &&
		!strings.HasSuffix(clean, "/") && path.Ext(clean) == "" {
		if g.outputDirExists(clean) {
			return clean + "/" + suffix
		}
	}
	return ""
}

// splitURLSuffix separates a query/fragment so it can be carried onto the
// reported destination.
func splitURLSuffix(href string) (string, string) {
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		return href[:i], href[i:]
	}
	return href, ""
}

// outputDirExists reports whether a root-relative link names a directory holding
// an index.html in the built output.
func (g *Generator) outputDirExists(clean string) bool {
	if !strings.HasPrefix(clean, "/") {
		return false // relative links are resolved by check_links, not modelled here
	}
	p := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.TrimPrefix(clean, "/")), indexHTMLName)
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
