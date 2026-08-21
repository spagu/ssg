package generator

// #182: a language assigned to a content section rather than to every page.

import (
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// langSectionGen builds a bilingual generator whose content root is dir — the
// shape a migrated WordPress site arrives in: languages declared, and hundreds
// of documents carrying none.
func langSectionGen(dir string, sections map[string]string) *Generator {
	return &Generator{
		config: Config{
			Domain: "example.com", ContentDir: dir, Source: "wp",
			Languages: []string{"en", "de", "fr"}, DefaultLanguage: "en",
			LanguageSections: sections, Quiet: true,
		},
		siteData: &models.SiteData{},
	}
}

// pageIn returns a page whose source directory is the named section.
func pageIn(root, slug string, section ...string) models.Page {
	dirs := append([]string{root, "wp"}, section...)
	return models.Page{Slug: slug, Type: "page", SourceDir: filepath.Join(dirs...)}
}

// TestASectionAssignsItsLanguage is the reported case: /de/ is German and
// nothing in the exported documents says so.
func TestASectionAssignsItsLanguage(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de"})

	if got := g.sectionLanguage(pageIn(root, "impressum", "de")); got != "de" {
		t.Errorf("sectionLanguage = %q, want de", got)
	}
	if got := g.sectionLanguage(pageIn(root, "about", "en")); got != "" {
		t.Errorf("an unlisted section must claim nothing, got %q", got)
	}
}

// TestTheLongestSectionPrefixWins: the same rule schema_defaults and
// output_encoding_sections use, so the project has one prefix convention.
func TestTheLongestSectionPrefixWins(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de", "de/blog": "fr"})
	if got := g.sectionLanguage(pageIn(root, "post", "de", "blog")); got != "fr" {
		t.Errorf("sectionLanguage = %q, want the longer key's value", got)
	}
	if got := g.sectionLanguage(pageIn(root, "impressum", "de")); got != "de" {
		t.Errorf("the shorter key still covers its own section: %q", got)
	}
}

// TestTheHomeKeyNamesTheRootPage: the root has no directory to be keyed by, so
// it gets the reserved key — and that key must not leak onto anything else.
func TestTheHomeKeyNamesTheRootPage(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"home": "en", "de": "de"})

	if got := g.sectionLanguage(models.Page{Link: "/"}); got != "en" {
		t.Errorf("home = %q, want en", got)
	}
	if got := g.sectionLanguage(pageIn(root, "impressum", "de")); got != "de" {
		t.Errorf("an ordinary page = %q", got)
	}
}

// TestContentSourcesResolveViaTheContentRoot: content that does not sit under
// the source directory still has a section, mirroring sectionSchema's fallback.
func TestContentSourcesResolveViaTheContentRoot(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"extra/fr": "fr"})
	page := models.Page{Slug: "bonjour", Type: "page",
		SourceDir: filepath.Join(root, "extra", "fr")}
	if got := g.sectionLanguage(page); got != "fr" {
		t.Errorf("sectionLanguage = %q, want fr via the content root", got)
	}
	// Content nowhere near the site is placed nowhere, not somewhere wrong.
	outside := models.Page{Slug: "x", SourceDir: filepath.Join(t.TempDir(), "elsewhere")}
	if got := g.sectionLanguage(outside); got != "" {
		t.Errorf("unplaceable content = %q, want empty", got)
	}
}

// TestLanguageSectionsAreIgnoredWhenUnconfigured keeps the feature free for
// every site that does not use it.
func TestLanguageSectionsAreIgnoredWhenUnconfigured(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, nil)
	if got := g.sectionLanguage(pageIn(root, "x", "de")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// finalizePages runs the real finalize pass over pages and returns them.
func finalizePages(t *testing.T, g *Generator, pages ...models.Page) []models.Page {
	t.Helper()
	g.siteData.Pages = pages
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatalf("finalizeLoadedContent: %v", err)
	}
	return g.siteData.Pages
}

// TestAPagesOwnLangBeatsItsSection: a per-file declaration is the most specific
// statement anyone made — the rule applySourceCategory documents for categories.
func TestAPagesOwnLangBeatsItsSection(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de"})
	g.config.I18n = ssgi18n.Config{Enabled: true, InvalidLanguage: "warn"}

	declared := pageIn(root, "manifest", "de")
	declared.Lang = "fr"
	silent := pageIn(root, "impressum", "de")
	elsewhere := pageIn(root, "about", "pages")

	got := finalizePages(t, g, declared, silent, elsewhere)
	if got[0].Lang != "fr" {
		t.Errorf("frontmatter lang = %q, want fr — the file must win", got[0].Lang)
	}
	if got[1].Lang != "de" {
		t.Errorf("section lang = %q, want de", got[1].Lang)
	}
	if got[2].Lang != "en" {
		t.Errorf("unclaimed page = %q, want default_language", got[2].Lang)
	}
}

// TestASectionLanguageReachesPrefixAndTranslations: assigning the language has
// to happen before translation grouping and LangPrefix, or the feature changes
// a field nothing downstream reads.
func TestASectionLanguageReachesPrefixAndTranslations(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de"})
	g.config.I18n = ssgi18n.Config{Enabled: true, InvalidLanguage: "warn"}

	got := finalizePages(t, g, pageIn(root, "impressum", "de"))
	if got[0].LangPrefix != "de" {
		t.Errorf("LangPrefix = %q, want de", got[0].LangPrefix)
	}
	if got[0].Locale == "" {
		t.Error("a section-assigned language must resolve a locale")
	}
	if got[0].TranslationKey == "" {
		t.Error("a section-assigned language must still be grouped for translation")
	}
	if out := got[0].GetOutputPath(); out != filepath.Join("de", "impressum") {
		t.Errorf("output path = %q, want the language prefix applied once", out)
	}
}

// TestAnExplicitLinkIsNotPrefixedTwice is the migration guard: an export that
// already wrote `link: /de/impressum/` into the file must not become
// /de/de/impressum/ once the section assigns the same language. Link is the
// highest-precedence URL source and GetOutputPath honours it whole.
func TestAnExplicitLinkIsNotPrefixedTwice(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de"})
	g.config.I18n = ssgi18n.Config{Enabled: true, InvalidLanguage: "warn"}

	page := pageIn(root, "impressum", "de")
	page.Link = "/de/impressum/"
	got := finalizePages(t, g, page)

	if got[0].Lang != "de" || got[0].LangPrefix != "de" {
		t.Fatalf("lang = %q prefix = %q", got[0].Lang, got[0].LangPrefix)
	}
	if out := got[0].GetOutputPath(); out != filepath.Join("de", "impressum") {
		t.Errorf("output path = %q — an explicit link must not be prefixed again", out)
	}
	if url := got[0].GetURL(); url != "/de/impressum/" {
		t.Errorf("URL = %q", url)
	}
}

// TestAnUnconfiguredSectionLanguageWarnsOnce: naming a language the site does
// not declare is worth one warning about the section, not one per file under it.
func TestAnUnconfiguredSectionLanguageWarnsOnce(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de", "es": "es"})
	g.config.I18n = ssgi18n.Config{Enabled: true, InvalidLanguage: "warn"}

	out := captureBuildOutput(t, func() {
		finalizePages(t, g,
			pageIn(root, "uno", "es"), pageIn(root, "dos", "es"), pageIn(root, "tres", "es"))
	})
	if n := strings.Count(out, `language_sections "es"`); n != 1 {
		t.Fatalf("warned %d times, want exactly one:\n%s", n, out)
	}
	if strings.Contains(out, `language_sections "de"`) {
		t.Errorf("a configured language must not warn:\n%s", out)
	}
}

// TestNoLanguagesMeansNothingToCheckAgainst: language_sections on a
// single-language site is unusual but not wrong, and there is nothing to
// validate it against.
func TestNoLanguagesMeansNothingToCheckAgainst(t *testing.T) {
	root := t.TempDir()
	g := langSectionGen(root, map[string]string{"de": "de"})
	g.config.Languages = nil
	out := captureBuildOutput(t, func() {
		g.warnUnconfiguredSectionLanguages(nil)
		g.warnUnconfiguredSectionLanguages([]ssgi18n.LanguageConfig{{Code: "de"}})
	})
	if out != "" {
		t.Errorf("nothing to warn about, got %q", out)
	}
}

// TestSectionValueIgnoresTheReservedHomeKey: `home` is a page, not a prefix,
// and must never be matched as one.
func TestSectionValueIgnoresTheReservedHomeKey(t *testing.T) {
	if v, ok := sectionValue(map[string]string{"home": "en"}, "home"); ok {
		t.Errorf("the reserved key matched as a prefix: %q", v)
	}
	if v, ok := sectionValue(map[string]string{"/de/": "de"}, "de/blog"); !ok || v != "de" {
		t.Errorf("a key with slashes must still match: %q %v", v, ok)
	}
	if _, ok := sectionValue(map[string]string{"": "x"}, "de"); ok {
		t.Error("an empty key must claim nothing")
	}
}
