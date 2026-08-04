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
