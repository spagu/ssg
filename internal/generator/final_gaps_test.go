package generator

// Last-mile gaps: constructor validation (I18N), finalize-time language
// defaults, index pagination failures, asset-copy failure modes and the
// OpenGraph locale alternates.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// TestNewValidatesI18n: New rejects a broken i18n setup up front — a missing
// default language and an unparseable translation catalog both fail before any
// build work happens.
func TestNewValidatesI18n(t *testing.T) {
	cfg := Config{Domain: "example.com", Languages: []string{"en"}, Quiet: true}
	cfg.I18n.Enabled = true
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "default_language") {
		t.Fatalf("missing default language must fail New, got %v", err)
	}

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "en.yaml"), "key: [unclosed")
	cfg.DefaultLanguage = "en"
	cfg.I18n.TranslationsDir = dir
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "parsing catalog") {
		t.Fatalf("broken catalog must fail New, got %v", err)
	}
}

// TestFinalizeLoadedContentI18nDefaults: a page without a language gets the
// default; an unconfigured language keeps its code as locale (so the warning
// names it); translation keys are generated when absent — and a collision
// found at the end fails the build.
func TestFinalizeLoadedContentI18nDefaults(t *testing.T) {
	g := newTestGen(t, "")
	g.config.I18n = g.config.I18n.WithDefaults()
	g.config.I18n.Enabled = true
	g.config.I18n.InvalidLanguage = "warn"
	g.config.Languages = []string{"en"}
	g.config.DefaultLanguage = "en"
	g.siteData.Pages = []models.Page{
		{Slug: "a", SourceFile: "a.md"},                // no lang → default
		{Slug: "b", SourceFile: "b.xx.md", Lang: "xx"}, // unconfigured lang
	}
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatal(err)
	}
	if g.siteData.Pages[0].Lang != "en" || g.siteData.Pages[0].TranslationKey != "a" {
		t.Errorf("defaults not applied: %+v", g.siteData.Pages[0])
	}
	if g.siteData.Pages[1].Locale != "xx" {
		t.Errorf("unconfigured language must keep its code as locale, got %q", g.siteData.Pages[1].Locale)
	}

	// Two pages emitting the same path: finalize surfaces the collision.
	g.siteData.Pages = []models.Page{
		{Slug: "same", Type: "page", SourceFile: "a.md", Lang: "en", TranslationKey: "k1"},
		{Slug: "same", Type: "page", SourceFile: "b.md", Lang: "en", TranslationKey: "k2"},
	}
	if err := g.finalizeLoadedContent(); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("output collision must fail finalize, got %v", err)
	}
}

// TestGenerateSeriesWriteError: a blocked /series/ directory fails the series
// landing pages loudly.
func TestGenerateSeriesWriteError(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{{Slug: "p", Series: "Saga"}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "series"), "in the way")
	if err := g.generateSeries(); err == nil {
		t.Fatal("a blocked series directory must error")
	}
}

// TestGenerateAuthorsSkipsUnsluggable: an author whose name slugifies to
// nothing has no URL — skipped rather than emitting /author//.
func TestGenerateAuthorsSkipsUnsluggable(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Authors = map[int]models.Author{5: {ID: 5, Name: "!!!"}}
	g.siteData.Posts = []models.Page{{Slug: "p", Author: 5}}
	slugs, err := g.generateAuthors()
	if err != nil || len(slugs) != 0 {
		t.Fatalf("unsluggable author must be skipped: %v, %v", slugs, err)
	}
}

// TestGenerateIndexFailures: index pagination failures propagate per language
// — a blocked prefix directory, a blocked page/N directory, and a template
// that cannot render.
func TestGenerateIndexFailures(t *testing.T) {
	// Blocked language prefix directory (i18n branch).
	g := newTestGen(t, "")
	g.config.I18n.Enabled = true
	g.config.DefaultLanguage = "pl"
	g.siteData.Languages = []ssgi18n.LanguageConfig{{Code: "en"}}
	mustWrite(t, filepath.Join(g.config.OutputDir, "en"), "in the way")
	if err := g.generateSite(); err == nil || !strings.Contains(err.Error(), "generating index") {
		t.Fatalf("blocked prefix must fail the index, got %v", err)
	}

	// Blocked page/2 directory once pagination kicks in.
	g2 := newTestGen(t, `{{define "index.html"}}ok{{end}}`)
	g2.config.Paginate = 1
	g2.siteData.Posts = []models.Page{{Slug: "a"}, {Slug: "b"}}
	mustWrite(t, filepath.Join(g2.config.OutputDir, "page"), "in the way")
	if err := g2.generateIndex(); err == nil {
		t.Fatal("blocked pagination directory must error")
	}

	// A failing index template surfaces per page.
	g3 := newTestGen(t, `{{define "index.html"}}{{.Nope}}{{end}}`)
	g3.config.Paginate = 1
	g3.siteData.Posts = []models.Page{{Slug: "a"}, {Slug: "b"}}
	if err := g3.generateIndex(); err == nil {
		t.Fatal("failing index template must error")
	}
}

// TestPageURLWithPrefixSections: section-prefixed pagination URLs keep the
// prefix on every page.
func TestPageURLWithPrefixSections(t *testing.T) {
	if got := pageURLWithPrefix("en", 1); got != "/en/" {
		t.Errorf("page 1 = %q, want /en/", got)
	}
	if got := pageURLWithPrefix("en", 2); got != "/en/page/2/" {
		t.Errorf("page 2 = %q, want /en/page/2/", got)
	}
}

// TestCopyAssetsBlockedTargets: js/images directories blocked in the output
// are errors; a blocked media tree only warns (media is best-effort).
func TestCopyAssetsBlockedTargets(t *testing.T) {
	newAssetGen := func(kind string) *Generator {
		g := newTestGen(t, "")
		tmp := t.TempDir()
		g.config.TemplatesDir = tmp
		g.config.Template = "simple"
		mustWrite(t, filepath.Join(tmp, "simple", kind, "a.file"), "x")
		mustWrite(t, filepath.Join(g.config.OutputDir, kind), "in the way")
		return g
	}
	if err := newAssetGen("js").copyAssets(); err == nil {
		t.Error("blocked js directory must error")
	}
	if err := newAssetGen("images").copyAssets(); err == nil {
		t.Error("blocked images directory must error")
	}

	g := newTestGen(t, "")
	tmp := t.TempDir()
	g.config.ContentDir, g.config.Source = tmp, "site"
	mustWrite(t, filepath.Join(tmp, "site", "media", "m.png"), "x")
	mustWrite(t, filepath.Join(g.config.OutputDir, "media"), "in the way")
	if err := g.copyAssets(); err != nil {
		t.Errorf("blocked media must warn, not fail: %v", err)
	}
}

// TestCopyDirNestedFailure: a nested destination blocked by a file propagates
// out of the recursive copy.
func TestCopyDirNestedFailure(t *testing.T) {
	g := newTestGen(t, "")
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "sub", "f.txt"), "x")
	dst := t.TempDir()
	mustWrite(t, filepath.Join(dst, "sub"), "in the way")
	if err := g.copyDir(src, dst); err == nil {
		t.Fatal("a blocked nested directory must error")
	}
}

// TestCopyColocatedAssetFailures: an unusable output directory is an error; a
// single uncopyable asset is a warning that keeps the rest coming.
func TestCopyColocatedAssetFailures(t *testing.T) {
	g := newTestGen(t, "")
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "pic.png"), "img")

	blocked := filepath.Join(t.TempDir(), "blocked")
	mustWrite(t, blocked, "not a dir")
	if err := g.copyColocatedAssets(src, blocked, "see pic.png"); err == nil {
		t.Error("a file where the output dir should be must error")
	}

	out := t.TempDir()
	mustMkdir(t, filepath.Join(out, "pic.png")) // directory squats on the asset name
	if err := g.copyColocatedAssets(src, out, "see pic.png"); err != nil {
		t.Errorf("one uncopyable asset must warn, not fail: %v", err)
	}
}

// TestBuildOpenGraphLocaleAlternates: translations of a localized page are
// advertised as og:locale:alternate — the current one is not.
func TestBuildOpenGraphLocaleAlternates(t *testing.T) {
	g := newTestGen(t, "")
	page := models.Page{Title: "T", Slug: "t", Locale: "en-US", Translations: []models.TranslationLink{
		{Lang: "en", Locale: "en-US", IsCurrent: true},
		{Lang: "pl", Locale: "pl-PL"},
	}}
	og := g.buildOpenGraph(page, false)
	if !strings.Contains(og, `og:locale:alternate" content="pl_PL"`) {
		t.Errorf("missing alternate locale:\n%s", og)
	}
	if strings.Count(og, "og:locale:alternate") != 1 {
		t.Errorf("current locale must not be its own alternate:\n%s", og)
	}
}

// TestParseShortcodeTemplateSyntaxError: a template that does not parse warns
// and disables the shortcode instead of crashing the build.
func TestParseShortcodeTemplateSyntaxError(t *testing.T) {
	g := newTestGen(t, "")
	bad := filepath.Join(t.TempDir(), "sc.html")
	mustWrite(t, bad, "{{ unterminated")
	if tmpl := g.parseShortcodeTemplate(bad); tmpl != nil {
		t.Fatal("unparseable shortcode template must yield nil")
	}
}

// TestEnsureCategoryAllocatesIDs: a new category gets max+1 even with sparse
// IDs, and a nil category map is created on demand.
func TestEnsureCategoryAllocatesIDs(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Categories[7] = models.Category{ID: 7, Name: "Old", Slug: "old"}
	if id := g.ensureCategory("Fresh"); id != 8 {
		t.Errorf("new category id = %d, want 8", id)
	}
	g2 := newTestGen(t, "")
	g2.siteData.Categories = nil
	if id := g2.ensureCategory("First"); id != 1 {
		t.Errorf("first category id = %d, want 1", id)
	}
	if id := g2.ensureCategory("  "); id != 0 {
		t.Errorf("blank category id = %d, want 0", id)
	}
}

// TestGenerateCloudflareHeadersBlocked: a directory squatting on _headers
// fails the Cloudflare file generation.
func TestGenerateCloudflareHeadersBlocked(t *testing.T) {
	g := newTestGen(t, "")
	mustMkdir(t, filepath.Join(g.config.OutputDir, "_headers"))
	if err := g.generateCloudflareFiles(); err == nil || !strings.Contains(err.Error(), "_headers") {
		t.Fatalf("blocked _headers must error, got %v", err)
	}
}

// TestWriteAliasStubDanglingTarget: an alias landing on a dangling symlink
// fails the write and only warns — aliases are conveniences, not build risks.
func TestWriteAliasStubDanglingTarget(t *testing.T) {
	g := newTestGen(t, "")
	mustMkdir(t, filepath.Join(g.config.OutputDir, "d"))
	mustSymlinkOrSkip(t, "/nonexistent-ssg-dir/index.html",
		filepath.Join(g.config.OutputDir, "d", "index.html"))
	g.writeAliasStub("dangling", "d", "/target/") // must not panic or create the target
	if _, err := os.Stat("/nonexistent-ssg-dir"); err == nil {
		t.Fatal("the dangling target must not have been created")
	}
}
