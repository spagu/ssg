package generator

// Extras gaps: link-checker error/skip behaviour (SEO-005), pretty-URL target
// resolution (#87), bundling failures (ASSET-002) and the i18n search-index
// write path (PLAT-004).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// TestCheckLinksStrictFlagEscalation: the global --strict flag turns link
// checking fatal even when check_links itself is unset — a renamed slug must
// fail loudly, not pass because nobody opted in.
func TestCheckLinksStrictFlagEscalation(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"), `<html><body><a href="/gone/">x</a></body></html>`)
	g := &Generator{config: Config{OutputDir: out, Quiet: true, Strict: true}}
	if err := g.checkLinksIfRequested(); err == nil || !strings.Contains(err.Error(), "broken internal link") {
		t.Fatalf("strict flag must fail on a broken link, got %v", err)
	}
}

// TestCheckLinksErrorAndSkip: a missing output tree is a real error, while a
// single unreadable HTML file is skipped — the checker reports link problems,
// it does not invent build failures.
func TestCheckLinksErrorAndSkip(t *testing.T) {
	g := &Generator{config: Config{OutputDir: filepath.Join(t.TempDir(), "missing"), Quiet: true, CheckLinks: "warn"}}
	if err := g.checkLinksIfRequested(); err == nil {
		t.Error("a missing output dir must error")
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads anything; the unreadable-file skip cannot trigger")
	}
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "locked.html"), `<html><a href="/gone/">x</a></html>`)
	mustWrite(t, filepath.Join(out, "ok.html"), `<html></html>`)
	if err := os.Chmod(filepath.Join(out, "locked.html"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(out, "locked.html"), 0o644) }) // #nosec G302 -- restoring perms on a test temp file
	g2 := &Generator{config: Config{OutputDir: out, Quiet: true}}
	broken, err := g2.checkLinks()
	if err != nil || len(broken) != 0 {
		t.Errorf("unreadable file must be skipped: %v, %v", broken, err)
	}
}

// TestCheckLinksSortWithinFile: two dead links in one file report in a stable
// href order, so reports diff cleanly between builds.
func TestCheckLinksSortWithinFile(t *testing.T) {
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "index.html"),
		`<html><body><a href="/zzz/">z</a><a href="/aaa/">a</a></body></html>`)
	g := &Generator{config: Config{OutputDir: out, Quiet: true}}
	broken, err := g.checkLinks()
	if err != nil || len(broken) != 2 {
		t.Fatalf("broken = %v, err = %v", broken, err)
	}
	if broken[0].href != "/aaa/" || broken[1].href != "/zzz/" {
		t.Errorf("findings not sorted by href: %v", broken)
	}
}

// TestExtractRefsMissingFile: an unopenable path reports the error to the
// caller (which then skips the file).
func TestExtractRefsMissingFile(t *testing.T) {
	if _, err := extractRefs(filepath.Join(t.TempDir(), "nope.html")); err == nil {
		t.Fatal("missing file must error")
	}
}

// TestRefTargetExistsPrettyForms: on a pretty-URL host, "/docs/x.html" is
// served by docs/x/index.html and an existing plain target still resolves.
func TestRefTargetExistsPrettyForms(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "swagger", "index.html"), "<html></html>")
	// The plain form: target exists as-is.
	if !refTargetExistsPretty(filepath.Join(root, "docs", "swagger"), false) {
		t.Error("existing directory target must resolve")
	}
	// The ".html names a directory" form the host normalizes (#87).
	if !refTargetExistsPretty(filepath.Join(root, "docs", "swagger.html"), false) {
		t.Error(".html link to a directory-served page must resolve")
	}
	if refTargetExistsPretty(filepath.Join(root, "docs", "missing.html"), false) {
		t.Error("a genuinely missing page must not resolve")
	}
}

// TestBundleWriteErrors: a bundle the filesystem refuses fails the build —
// templates already reference the bundle URL, so a missing artifact is a 404
// on every page.
func TestBundleWriteErrors(t *testing.T) {
	// With no usable output root every bundle path is out of bounds.
	g := &Generator{config: Config{OutputDir: "", Quiet: true,
		Bundles: map[string][]string{"b.js": {"a.js"}}}}
	if err := g.bundleIfRequested(); err == nil {
		t.Error("a bundle path outside the output root must error")
	}
	// A file where the bundle's directory should be blocks MkdirAll.
	out := t.TempDir()
	mustWrite(t, filepath.Join(out, "sub"), "not a dir")
	g2 := &Generator{config: Config{OutputDir: out, Quiet: true,
		Bundles: map[string][]string{"sub/b.js": {"a.js"}}}}
	if err := g2.bundleIfRequested(); err == nil {
		t.Error("a blocked bundle directory must error")
	}
}

// TestSearchIndexI18nWriteErrors: per-language search indexes surface their
// write failures instead of silently shipping a site without search.
func TestSearchIndexI18nWriteErrors(t *testing.T) {
	newGen := func(out string) *Generator {
		g := &Generator{config: Config{OutputDir: out, Quiet: true, SearchIndex: true,
			DefaultLanguage: "en"}, siteData: &models.SiteData{}}
		g.config.I18n.Enabled = true
		g.siteData.Languages = []ssgi18n.LanguageConfig{{Code: "en"}, {Code: "pl"}}
		return g
	}
	// The default language writes at the root; a directory squatting on the
	// index filename fails that write.
	out1 := t.TempDir()
	mustMkdir(t, filepath.Join(out1, "search-index.json"))
	if err := newGen(out1).generateSearchIndex(); err == nil {
		t.Error("a directory at search-index.json must error")
	}
	// A file squatting on a language prefix fails that language's MkdirAll.
	out2 := t.TempDir()
	mustWrite(t, filepath.Join(out2, "pl"), "not a dir")
	if err := newGen(out2).generateSearchIndex(); err == nil {
		t.Error("a blocked language prefix must error")
	}
}
