package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// writeOut writes an output-relative HTML file for the check tests.
func writeOut(t *testing.T, g *Generator, rel, body string) {
	t.Helper()
	p := filepath.Join(g.config.OutputDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// capture runs fn with stdout redirected and returns what it printed.
func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := os.ReadFile("/dev/stdin")
	_ = b
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := r.Read(buf)
		sb.Write(buf[:n])
		if e != nil {
			break
		}
	}
	return sb.String(), err
}

// TestCheckImagesReportsOnlyMissingAlt covers #75's three states: a missing alt
// is a finding, alt="" is a valid decorative image and stays silent, and a real
// alt is fine. Getting the middle one wrong is the whole point of the issue.
func TestCheckImagesReportsOnlyMissingAlt(t *testing.T) {
	g := newTestGen(t, "")
	writeOut(t, g, "page/index.html", `<html><body>
		<img src="/no-alt.png">
		<img src="/deco.png" alt="">
		<img src="/ok.png" alt="A cat">
	</body></html>`)

	g.config.CheckImages = "warn"
	out, err := capture(t, g.checkImagesIfRequested)
	if err != nil {
		t.Fatalf("warn mode must not fail the build: %v", err)
	}
	if !strings.Contains(out, "/no-alt.png") {
		t.Errorf("missing alt not reported:\n%s", out)
	}
	for _, quiet := range []string{"/deco.png", "/ok.png"} {
		if strings.Contains(out, quiet) {
			t.Errorf("%s must not be reported (alt is present):\n%s", quiet, out)
		}
	}

	// strict-decorative opts in to reviewing alt="" as well, and fails.
	g.config.CheckImages = "strict-decorative"
	out, err = capture(t, g.checkImagesIfRequested)
	if err == nil {
		t.Error("strict-decorative must fail the build")
	}
	if !strings.Contains(out, "/deco.png") {
		t.Errorf(`strict-decorative must report alt="":\n%s`, out)
	}

	// Off by default.
	g.config.CheckImages = ""
	if out, err := capture(t, g.checkImagesIfRequested); err != nil || out != "" {
		t.Errorf("disabled check must be silent: %q, %v", out, err)
	}
}

// TestCheckImagesStrictFails: strict turns a missing alt into a build failure.
func TestCheckImagesStrictFails(t *testing.T) {
	g := newTestGen(t, "")
	writeOut(t, g, "a.html", `<html><body><img src="/x.png"></body></html>`)
	g.config.CheckImages = "strict"
	if _, err := capture(t, g.checkImagesIfRequested); err == nil {
		t.Error("strict must fail on a missing alt")
	}
	// The global strict flag escalates an otherwise-warn check (#62).
	g.config.CheckImages, g.config.Strict = "warn", true
	if _, err := capture(t, g.checkImagesIfRequested); err == nil {
		t.Error("global strict must escalate the check")
	}
}

// TestCheckMetaFindingsAndSkips covers #76: missing/empty title and description
// are findings, and noindex pages are skipped entirely.
func TestCheckMetaFindingsAndSkips(t *testing.T) {
	g := newTestGen(t, "")
	writeOut(t, g, "good.html", `<html><head><title>A title that is long enough to be sensible here</title>`+
		`<meta name="description" content="A description comfortably inside the window that search engines display in full."></head></html>`)
	writeOut(t, g, "empty.html", `<html><head><title>T</title><meta name="description" content=""></head></html>`)
	writeOut(t, g, "none.html", `<html><head><title>T</title></head></html>`)
	writeOut(t, g, "notitle.html", `<html><head><meta name="description" content="x"></head></html>`)
	writeOut(t, g, "404.html", `<html><head><meta name="robots" content="noindex, nofollow"></head></html>`)

	g.config.CheckMeta = "warn"
	out, err := capture(t, g.checkMetaIfRequested)
	if err != nil {
		t.Fatalf("warn must not fail: %v", err)
	}
	for _, want := range []string{"empty.html", "none.html", "notitle.html"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s should be reported:\n%s", want, out)
		}
	}
	if strings.Contains(out, "404.html") {
		t.Errorf("noindex pages must be skipped:\n%s", out)
	}
	if strings.Contains(out, "good.html") {
		t.Errorf("a complete page must not be reported:\n%s", out)
	}

	g.config.CheckMeta = "strict"
	if _, err := capture(t, g.checkMetaIfRequested); err == nil {
		t.Error("strict must fail when metadata is missing")
	}
}

// TestMetaLimitsAreConfigurable covers the configurable advisory ranges: they are
// warnings only, defaults apply when unset, and an explicit 0 disables a bound.
func TestMetaLimitsAreConfigurable(t *testing.T) {
	g := newTestGen(t, "")
	writeOut(t, g, "p.html", `<html><head><title>Short</title>`+
		`<meta name="description" content="Also short."></head></html>`)
	g.config.CheckMeta = "warn"

	// Defaults: both are under the minimum, so both warn — but the build passes.
	out, err := capture(t, g.checkMetaIfRequested)
	if err != nil {
		t.Fatalf("length problems must never fail the build: %v", err)
	}
	if !strings.Contains(out, "title is 5 characters") || !strings.Contains(out, "meta description is 11 characters") {
		t.Errorf("default ranges should warn on both:\n%s", out)
	}

	// Explicit 0 disables a bound; a raised max flags what was previously fine.
	zero, small := 0, 3
	g.config.MetaLimits = models.MetaLimits{TitleMin: &zero, DescriptionMin: &zero, DescriptionMax: &small}
	out, _ = capture(t, g.checkMetaIfRequested)
	if strings.Contains(out, "title is 5 characters") {
		t.Errorf("title_min: 0 must disable the lower bound:\n%s", out)
	}
	if !strings.Contains(out, "longer than 3") {
		t.Errorf("description_max should now flag the value:\n%s", out)
	}
}

// TestCheckOrphans covers #77, including the two traps the issue calls out: a
// canonical <link> must not count as an inbound link, and a page linking to
// itself is still an orphan.
func TestCheckOrphans(t *testing.T) {
	g := newTestGen(t, "")
	// Home links to reachable/ only. Every page canonicalises to itself.
	writeOut(t, g, "index.html", `<html><head><link rel="canonical" href="/"></head><body><a href="/reachable/">R</a></body></html>`)
	writeOut(t, g, "reachable/index.html", `<html><head><link rel="canonical" href="/reachable/"></head><body>r</body></html>`)
	writeOut(t, g, "orphan/index.html", `<html><head><link rel="canonical" href="/orphan/"></head><body><a href="/orphan/">self</a></body></html>`)
	writeOut(t, g, "hidden/index.html", `<html><head><meta name="robots" content="noindex"></head><body>h</body></html>`)

	g.config.CheckOrphans = "warn"
	out, err := capture(t, g.checkOrphansIfRequested)
	if err != nil {
		t.Fatalf("warn must not fail: %v", err)
	}
	if !strings.Contains(out, "orphan/index.html") {
		t.Errorf("an unlinked page must be reported (a self-link does not count):\n%s", out)
	}
	if strings.Contains(out, "reachable") {
		t.Errorf("a linked page must not be reported:\n%s", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("noindex pages must be skipped:\n%s", out)
	}
	if strings.Contains(out, "→ no inbound links\n") && strings.Contains(out, " index.html →") {
		t.Errorf("the site root must never be reported as an orphan:\n%s", out)
	}

	g.config.CheckOrphans = "strict"
	if _, err := capture(t, g.checkOrphansIfRequested); err == nil {
		t.Error("strict must fail when an orphan exists")
	}
}

// TestLinkTarget: hrefs resolve to the output file that serves them.
func TestLinkTarget(t *testing.T) {
	g := newTestGen(t, "")
	cases := map[string]string{
		"/":               "index.html",
		"/docs/":          "docs/index.html",
		"/docs/x.html":    "docs/x.html",
		"/docs/?a=1#frag": "docs/index.html",
		"#only-fragment":  "",
		"../up/":          "up/index.html",
		"sibling/":        "a/sibling/index.html",
	}
	for href, want := range cases {
		if got := g.linkTarget(href, "a/index.html"); got != want {
			t.Errorf("linkTarget(%q) = %q, want %q", href, got, want)
		}
	}
}

// TestFillMetaDescription covers #76's fallback: an existing non-empty tag wins,
// an empty one is rewritten in place (not duplicated), and a missing one is added.
func TestFillMetaDescription(t *testing.T) {
	const desc = "From the front matter."

	// A theme-provided description is left alone.
	existing := `<html><head><meta name="description" content="Theme wrote this"></head></html>`
	if got := fillMetaDescription(existing, desc); got != existing {
		t.Errorf("a non-empty description must not be replaced: %q", got)
	}

	// An empty one is replaced in place — exactly one tag survives.
	empty := `<html><head><meta name="description" content=""></head></html>`
	got := fillMetaDescription(empty, desc)
	if strings.Count(got, "name=\"description\"") != 1 {
		t.Errorf("must not end up with two description tags: %q", got)
	}
	if !strings.Contains(got, desc) {
		t.Errorf("front-matter description not used: %q", got)
	}

	// A missing one is injected before </head>.
	none := `<html><head><title>T</title></head><body></body></html>`
	got = fillMetaDescription(none, desc)
	if !strings.Contains(got, desc) || !strings.Contains(got, "</head>") {
		t.Errorf("description not injected into head: %q", got)
	}

	// Nothing to fall back to ⇒ untouched.
	if got := fillMetaDescription(none, "   "); got != none {
		t.Errorf("an empty front-matter description must change nothing: %q", got)
	}
}

// TestSameURL: canonical comparison tolerates a missing host and trailing slash,
// which is what keeps every page from looking non-self-canonical (#78).
func TestSameURL(t *testing.T) {
	same := [][2]string{
		{"/docs/intro/", "https://example.com/docs/intro"},
		{"https://example.com/a", "https://example.com/a/"},
		{"/", "https://example.com/"},
		{"//example.com/a/", "https://example.com/a"},
	}
	for _, c := range same {
		if !sameURL(c[0], c[1]) {
			t.Errorf("sameURL(%q, %q) = false, want true", c[0], c[1])
		}
	}
	differ := [][2]string{
		{"/a/", "https://example.com/b/"},
		{"https://other.test/a", "https://example.com/a"},
	}
	for _, c := range differ {
		if sameURL(c[0], c[1]) {
			t.Errorf("sameURL(%q, %q) = true, want false", c[0], c[1])
		}
	}
}

// TestExcludedFromContent covers #74: globs match the full path, the
// content-relative path and the bare filename, and ** crosses directories.
func TestExcludedFromContent(t *testing.T) {
	g := newTestGen(t, "")
	g.config.ContentDir, g.config.Source = "content", "site"

	g.config.ContentExclude = []string{"**/examples/**"}
	if !g.excludedFromContent(filepath.Join("content", "site", "docs", "examples", "s.md")) {
		t.Error("** must cross directory separators")
	}
	if g.excludedFromContent(filepath.Join("content", "site", "docs", "real.md")) {
		t.Error("a normal page must not be excluded")
	}

	g.config.ContentExclude = []string{"sample-*.md"}
	if !g.excludedFromContent(filepath.Join("content", "site", "sample-one.md")) {
		t.Error("a bare filename pattern must match")
	}

	// No patterns ⇒ nothing excluded, and the fast path is taken.
	g.config.ContentExclude = nil
	if g.excludedFromContent(filepath.Join("content", "site", "anything.md")) {
		t.Error("no patterns must exclude nothing")
	}
}

// TestMatchGlob pins the glob semantics on their own.
func TestMatchGlob(t *testing.T) {
	yes := [][2]string{
		{"*.md", "a.md"}, {"docs/*.md", "docs/a.md"}, {"docs/**", "docs/a/b.md"},
		{"**/x/**", "a/b/x/c.md"}, {"a?c.md", "abc.md"},
	}
	for _, c := range yes {
		if !matchGlob(c[0], c[1]) {
			t.Errorf("matchGlob(%q, %q) = false, want true", c[0], c[1])
		}
	}
	no := [][2]string{
		{"*.md", "docs/a.md"}, // * must not cross a separator
		{"docs/*.md", "docs/a/b.md"},
		{"[", "anything"}, // a malformed pattern matches nothing rather than panicking
	}
	for _, c := range no {
		if matchGlob(c[0], c[1]) {
			t.Errorf("matchGlob(%q, %q) = true, want false", c[0], c[1])
		}
	}
}

// TestSitemapSelfExclusion covers #78: a noindex page is always dropped, while a
// page whose canonical points elsewhere is dropped only when the site opts in.
// The default matters — the shipped `simple` theme emits a canonical that does
// not match the post permalink, so pruning on canonical by default would silently
// remove real posts from the sitemap over a theme bug.
func TestSitemapSelfExclusion(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "ex.com"

	writeOut(t, g, "hidden/index.html", `<html><head><meta name="robots" content="noindex"></head></html>`)
	writeOut(t, g, "moved/index.html", `<html><head><link rel="canonical" href="https://ex.com/elsewhere/"></head></html>`)
	writeOut(t, g, "fine/index.html", `<html><head><link rel="canonical" href="/fine/"></head></html>`)

	page := func(slug string) models.Page {
		return models.Page{Slug: slug, Type: "page", Status: "publish"}
	}

	// noindex is pruned with no configuration at all.
	if !g.excludesFromSitemap(page("hidden")) {
		t.Error("a noindex page must be excluded from the sitemap")
	}
	// A self-canonical page stays, whatever the setting.
	if g.excludesFromSitemap(page("fine")) {
		t.Error("a self-canonical page must stay in the sitemap")
	}
	// Canonical mismatch is ignored by default...
	if g.excludesFromSitemap(page("moved")) {
		t.Error("canonical pruning must be opt-in")
	}
	// ...and honoured once opted into. The memo is per output path, so use a
	// fresh generator rather than re-reading a cached verdict.
	g2 := newTestGen(t, "")
	g2.config.Domain, g2.config.SitemapPruneCanonical = "ex.com", true
	writeOut(t, g2, "moved/index.html", `<html><head><link rel="canonical" href="https://ex.com/elsewhere/"></head></html>`)
	if !g2.excludesFromSitemap(page("moved")) {
		t.Error("sitemap_prune_canonical must drop a non-self-canonical page")
	}
}

// TestSitemapExclusionIsPerURL covers #88, a regression from #78: exclusion was
// decided per page and read from one output file, so a source emitting more than
// one URL had every one of them judged by that single verdict. A page slugged
// "index" emits both "/" and "/index/", so a theme marking the duplicate noindex
// silently removed the site root — worse than the duplicate it was fixing.
func TestSitemapExclusionIsPerURL(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "ex.com"
	// The root is indexable; the /index/ duplicate is not.
	writeOut(t, g, "index.html", `<html><head><link rel="canonical" href="/"></head><body>home</body></html>`)
	writeOut(t, g, "index/index.html", `<html><head><meta name="robots" content="noindex, follow"><link rel="canonical" href="/"></head></html>`)

	page := models.Page{Slug: "index", Type: "page", Status: "publish"}

	// The URL "/" is served by the root index.html and belongs in the sitemap.
	if g.excludesFromSitemapAt(page, indexHTMLName) {
		t.Error("the site root must not be excluded because a duplicate URL is noindex")
	}
	// The URL "/index/" is served by index/index.html and does not.
	if !g.excludesFromSitemapAt(page, page.GetOutputPath()) {
		t.Error("the noindex duplicate must be excluded")
	}

	// Same split with canonical pruning enabled: the root is self-canonical.
	g2 := newTestGen(t, "")
	g2.config.Domain, g2.config.SitemapPruneCanonical = "ex.com", true
	writeOut(t, g2, "index.html", `<html><head><link rel="canonical" href="/"></head></html>`)
	writeOut(t, g2, "index/index.html", `<html><head><link rel="canonical" href="/"></head></html>`)
	if g2.excludesFromSitemapAt(page, indexHTMLName) {
		t.Error("a self-canonical root must stay in the sitemap under canonical pruning")
	}
	if !g2.excludesFromSitemapAt(page, page.GetOutputPath()) {
		t.Error("a duplicate canonicalising to / must be pruned")
	}
}

// TestURLForOutputFile: the URL an output file is served at, which is what the
// canonical comparison must use.
func TestURLForOutputFile(t *testing.T) {
	cases := map[string]string{
		"index.html":      "/",
		"docs/index.html": "/docs/",
		"a/b/index.html":  "/a/b/",
		"feed.xml":        "/feed.xml",
		"validator.html":  "/validator.html",
	}
	for in, want := range cases {
		if got := urlForOutputFile(in); got != want {
			t.Errorf("urlForOutputFile(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCheckRedirects covers #87: links that resolve fine against the output but
// which a pretty-URL host answers with a redirect. Nothing is broken — which is
// why check_links passes them — but each costs a visitor a round trip, and one
// such link in shared chrome puts every page on the site through one.
func TestCheckRedirects(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Domain = "ex.com"
	writeOut(t, g, "docs/intro/index.html", `<html><body>x</body></html>`)
	writeOut(t, g, "page.html", `<html><body>
		<a href="/docs/intro.html">ext</a>
		<a href="/docs/intro">noslash</a>
		<a href="/docs/intro/">correct</a>
		<a href="/feed.xml">feed</a>
		<a href="https://example.com/out">external</a>
	</body></html>`)

	// Off without pretty_urls: on a plain object store the extensionless form is a
	// genuine 404, so reporting it as "a redirect" would be wrong.
	g.config.CheckRedirects = "warn"
	out, err := capture(t, g.checkRedirectsIfRequested)
	if err != nil {
		t.Fatalf("warn must not fail: %v", err)
	}
	if !strings.Contains(out, "pretty_urls") {
		t.Errorf("without pretty_urls the check should explain why it skipped:\n%s", out)
	}

	g.config.PrettyURLs = models.PrettyStripSlash
	out, err = capture(t, g.checkRedirectsIfRequested)
	if err != nil {
		t.Fatalf("warn must not fail: %v", err)
	}
	for _, want := range []string{"/docs/intro.html  →  /docs/intro/", "/docs/intro  →  /docs/intro/"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing finding %q:\n%s", want, out)
		}
	}
	// The already-final form, a non-page file and an external link are all silent.
	if strings.Contains(out, `"/docs/intro/"`) || strings.Contains(out, "feed.xml") || strings.Contains(out, "example.com") {
		t.Errorf("reported something that costs no redirect:\n%s", out)
	}

	g.config.CheckRedirects = "strict"
	if _, err := capture(t, g.checkRedirectsIfRequested); err == nil {
		t.Error("strict must fail when links only resolve through a redirect")
	}
}

// TestRedirectTargetOf pins the shapes a pretty-URL host rewrites.
//
// The mode has to be set explicitly: since #103 the destination depends on it,
// because a host that strips the extension without adding a slash would itself
// redirect the slashed form this case expects.
func TestRedirectTargetOf(t *testing.T) {
	g := newTestGen(t, "")
	g.config.PrettyURLs = models.PrettyStripSlash
	writeOut(t, g, "docs/intro/index.html", "x")
	cases := map[string]string{
		"/docs/intro.html":       "/docs/intro/",
		"/docs/intro/index.html": "/docs/intro/",
		"/docs/intro":            "/docs/intro/",
		"/docs/intro/":           "", // already final
		"/feed.xml":              "", // not a page
		"/nowhere":               "", // no directory there — check_links' finding, not ours
		"/":                      "",
	}
	for href, want := range cases {
		if got := g.redirectTargetOf(href); got != want {
			t.Errorf("redirectTargetOf(%q) = %q, want %q", href, got, want)
		}
	}
	// A query or fragment is carried onto the reported destination.
	if got := g.redirectTargetOf("/docs/intro?a=1"); got != "/docs/intro/?a=1" {
		t.Errorf("query not carried: %q", got)
	}
}

// TestPrettyURLsMakesLinkCheckAgree: with pretty_urls the link checker stops
// contradicting the host in the other direction, so an author is not pushed to
// restructure content around a limitation of the checker.
func TestPrettyURLsMakesLinkCheckAgree(t *testing.T) {
	g := newTestGen(t, "")
	g.refCache = map[string]bool{}
	// A FLAT page: docs/swagger.html, with no directory of that name. This is the
	// shape the issue hit — a directory-backed page already resolved.
	writeOut(t, g, "docs/swagger.html", "x")
	writeOut(t, g, "docs/intro/index.html", "x")
	dir := g.config.OutputDir

	if g.refResolves("/docs/swagger", dir) {
		t.Error("without pretty_urls an extensionless link to a flat page does not resolve")
	}
	g.config.PrettyURLs = models.PrettyStripSlash
	g.refCache = map[string]bool{} // the verdict is cached per path
	if !g.refResolves("/docs/swagger", dir) {
		t.Error("with pretty_urls the host strips .html and serves it — not a broken link")
	}
	// A directory-backed page resolves either way.
	g.refCache = map[string]bool{}
	if !g.refResolves("/docs/intro", dir) {
		t.Error("a directory-backed page must resolve")
	}
	g.refCache = map[string]bool{}
	if g.refResolves("/docs/missing", dir) {
		t.Error("a genuinely missing page must still be reported")
	}
}
