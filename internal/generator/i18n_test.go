package generator

import (
	"strings"
	"testing"
	"time"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

func TestI18nTranslationRegistryAndRouting(t *testing.T) {
	g, err := New(Config{Domain: "example.com", Languages: []string{"pl", "en"}, DefaultLanguage: "pl", I18n: ssgi18n.Config{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Pages = []models.Page{
		{Title: "O nas", Slug: "o-nas", Lang: "pl", TranslationKey: "about", Type: "page", SourceFile: "about.pl.md"},
		{Title: "About", Slug: "about", Lang: "en", TranslationKey: "about", Type: "page", SourceFile: "about.en.md"},
	}
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatal(err)
	}
	pl, en := g.siteData.Pages[0], g.siteData.Pages[1]
	if pl.GetURL() != "/o-nas/" || en.GetURL() != "/en/about/" {
		t.Fatalf("URLs: %s, %s", pl.GetURL(), en.GetURL())
	}
	if len(pl.Translations) != 2 || g.translationURL("en", pl) != "/en/about/" {
		t.Fatalf("translations: %#v", pl.Translations)
	}
	if tags := string(g.hreflangTags(pl)); !strings.Contains(tags, `hreflang="x-default"`) {
		t.Fatalf("hreflang: %s", tags)
	}
}

func TestI18nPrefixDefaultAndDuplicate(t *testing.T) {
	g, err := New(Config{Languages: []string{"pl"}, DefaultLanguage: "pl", I18n: ssgi18n.Config{Enabled: true, PrefixDefaultLanguage: true}})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Pages = []models.Page{{Slug: "a", Lang: "pl", TranslationKey: "same", SourceFile: "a.md"}, {Slug: "b", Lang: "pl", TranslationKey: "same", SourceFile: "b.md"}}
	if err := g.finalizeLoadedContent(); err == nil {
		t.Fatal("expected duplicate translation error")
	}
	if got := g.siteData.Pages[0].GetURL(); got != "/pl/a/" {
		t.Fatalf("default URL = %q", got)
	}
}

func TestLegacyLanguageRoutingUnchanged(t *testing.T) {
	g, err := New(Config{Languages: []string{"pl", "en"}, DefaultLanguage: "pl"})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Pages = []models.Page{{Slug: "about", Lang: "en", Type: "page"}}
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatal(err)
	}
	if got := g.siteData.Pages[0].GetURL(); got != "/en/about/" {
		t.Fatalf("legacy URL = %q", got)
	}
	if g.siteData.Pages[0].TranslationKey != "" {
		t.Fatal("legacy build must not synthesize translation keys")
	}
}

// TestI18nMdLinksLanguageAware verifies §13: the active-language translation
// wins, an explicit language-suffixed link is preserved, the content-fallback
// chain applies only when enabled, and resolution is deterministic.
func TestI18nMdLinksLanguageAware(t *testing.T) {
	newG := func(contentFallback bool) *Generator {
		g, err := New(Config{Domain: "example.com", Languages: []string{"pl", "en"}, DefaultLanguage: "pl",
			I18n: ssgi18n.Config{Enabled: true, ContentFallback: contentFallback,
				FallbackLanguages: map[string][]string{"pl": {"en"}}}})
		if err != nil {
			t.Fatal(err)
		}
		g.siteData.Pages = []models.Page{
			{Title: "Instalacja", Slug: "instalacja", Lang: "pl", TranslationKey: "install", Type: "page", SourceFile: "installation.pl.md"},
			{Title: "Installation", Slug: "installation", Lang: "en", TranslationKey: "install", Type: "page", SourceFile: "installation.en.md"},
			{Title: "English only", Slug: "guide", Lang: "en", TranslationKey: "guide", Type: "page", SourceFile: "guide.en.md"},
		}
		if err := g.finalizeLoadedContent(); err != nil {
			t.Fatal(err)
		}
		return g
	}

	g := newG(true)
	m := g.buildMdLinkMap()

	// Active language wins: pl page linking installation → /instalacja/.
	g.currentLang = "pl"
	out := g.rewriteMdLinks(`<a href="installation.md">x</a>`, m)
	if !strings.Contains(out, `href="/instalacja/"`) {
		t.Errorf("pl resolution = %q", out)
	}
	// Same link from an en page → /en/installation/.
	g.currentLang = "en"
	out = g.rewriteMdLinks(`<a href="installation.md">x</a>`, m)
	if !strings.Contains(out, `href="/en/installation/"`) {
		t.Errorf("en resolution = %q", out)
	}
	// Explicit cross-language link is preserved as-is (target file's language).
	g.currentLang = "pl"
	out = g.rewriteMdLinks(`<a href="installation.en.md">x</a>`, m)
	if !strings.Contains(out, `href="/en/installation/"`) {
		t.Errorf("explicit cross-language link = %q", out)
	}
	// Missing pl translation + fallback ON → en URL (single-entry keys resolve
	// directly; a multi-language key without the active language uses the chain).
	out = g.rewriteMdLinks(`<a href="guide.md">x</a>`, m)
	if !strings.Contains(out, `href="/en/guide/"`) {
		t.Errorf("content fallback = %q", out)
	}
	// Deterministic across many rebuilt maps (the old flat map was random).
	for i := 0; i < 50; i++ {
		g.currentLang = "pl"
		again := g.rewriteMdLinks(`<a href="installation.md">x</a>`, g.buildMdLinkMap())
		if !strings.Contains(again, `href="/instalacja/"`) {
			t.Fatalf("iteration %d nondeterministic: %q", i, again)
		}
	}

	// Fallback OFF: multi-language key without the active language stays as-is
	// (and warns once); explicit single-language links still resolve.
	g2 := newG(false)
	m2 := g2.buildMdLinkMap()
	g2.currentLang = "pl"
	// force a multi-lang key miss: drop pl variant so "installation" keys keep only en?
	// Instead use a synthetic multi-language key with no pl entry:
	m2["shared.md"] = map[string]string{"en": "/en/shared/", "de": "/de/shared/"}
	out = g2.rewriteMdLinks(`<a href="shared.md">x</a>`, m2)
	if !strings.Contains(out, `href="shared.md"`) {
		t.Errorf("fallback disabled should leave link untouched, got %q", out)
	}
	// warn dedupe: second call does not add a second entry
	_ = g2.rewriteMdLinks(`<a href="shared.md">x</a>`, m2)
	if len(g2.mdLinkWarned) != 1 {
		t.Errorf("expected exactly one warn entry, got %d", len(g2.mdLinkWarned))
	}
}

// TestXDefaultFallsBackToDefaultRoot verifies §9: a translation group without
// a default-language variant points x-default at the default-language root.
func TestXDefaultFallsBackToDefaultRoot(t *testing.T) {
	g, err := New(Config{Domain: "example.com", Languages: []string{"pl", "en", "de"}, DefaultLanguage: "pl",
		I18n: ssgi18n.Config{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Pages = []models.Page{
		{Title: "Guide", Slug: "guide", Lang: "en", TranslationKey: "guide", Type: "page", SourceFile: "guide.en.md"},
		{Title: "Anleitung", Slug: "anleitung", Lang: "de", TranslationKey: "guide", Type: "page", SourceFile: "guide.de.md"},
	}
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatal(err)
	}
	tags := string(g.hreflangTags(g.siteData.Pages[0]))
	if !strings.Contains(tags, `hreflang="x-default" href="https://example.com/"`) {
		t.Errorf("x-default should point at the default-language root, got:\n%s", tags)
	}
}

// TestTranslationValuePolicies covers the t helper: hits, dictionary fallback,
// non-string values, interpolation and every missing-translation policy.
func TestTranslationValuePolicies(t *testing.T) {
	newG := func(policy string, fallback bool) *Generator {
		g, err := New(Config{Languages: []string{"pl", "en"}, DefaultLanguage: "pl",
			I18n: ssgi18n.Config{Enabled: true, MissingTranslation: policy, DictionaryFallback: fallback,
				FallbackLanguages: map[string][]string{"pl": {"en"}}}})
		if err != nil {
			t.Fatal(err)
		}
		g.catalog = &ssgi18n.Catalog{Messages: map[string]map[string]any{
			"pl": {"nav": map[string]any{"home": "Strona główna"}, "bad": 42},
			"en": {"only_en": "English", "count": "{{n}} items"},
		}}
		g.currentLang = "pl"
		return g
	}

	g := newG("warn", true)
	if v, err := g.translationValue("nav.home"); err != nil || v != "Strona główna" {
		t.Errorf("direct hit = %q, %v", v, err)
	}
	if v, err := g.translationValue("only_en"); err != nil || v != "English" {
		t.Errorf("dictionary fallback = %q, %v", v, err)
	}
	if v, err := g.translationValue("count", map[string]any{"n": 5}); err != nil || v != "5 items" {
		t.Errorf("interpolation = %q, %v", v, err)
	}
	if _, err := g.translationValue("bad"); err == nil {
		t.Error("non-string translation must error")
	}
	// currentLang empty → DefaultLanguage.
	g.currentLang = ""
	if v, _ := g.translationValue("nav.home"); v != "Strona główna" {
		t.Errorf("default-language fallback = %q", v)
	}

	// Fallback disabled: en-only key misses from pl.
	noFB := newG("warn", false)
	if v, err := noFB.translationValue("only_en"); err != nil || v != "only_en" {
		t.Errorf("warn policy should return the key, got %q, %v", v, err)
	}
	if _, err := newG("error", false).translationValue("only_en"); err == nil {
		t.Error("error policy must fail")
	}
	if v, err := newG("empty", false).translationValue("only_en"); err != nil || v != "" {
		t.Errorf("empty policy = %q, %v", v, err)
	}
	if v, err := newG("fallback", false).translationValue("only_en"); err != nil || v != "only_en" {
		t.Errorf("fallback policy = %q, %v", v, err)
	}
}

// TestI18nHelperEdges covers pageFromAny, translationURL misses, languageURL
// and localizeDate presets/timezones.
func TestI18nHelperEdges(t *testing.T) {
	g, err := New(Config{Languages: []string{"pl", "en"}, DefaultLanguage: "pl",
		LanguageTimezones: map[string]string{"en": "America/New_York"},
		I18n:              ssgi18n.Config{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}

	// pageFromAny variants.
	page := models.Page{Slug: "x", Translations: []models.TranslationLink{{Lang: "en", URL: "/en/x/"}}}
	if got := g.translationURL("en", page); got != "/en/x/" {
		t.Errorf("value page = %q", got)
	}
	if got := g.translationURL("en", &page); got != "/en/x/" {
		t.Errorf("pointer page = %q", got)
	}
	var nilPage *models.Page
	if got := g.translationURL("en", nilPage); got != "" {
		t.Errorf("nil pointer = %q", got)
	}
	if got := g.translationURL("en", 42); got != "" {
		t.Errorf("non-page = %q", got)
	}
	if got := g.translationURL("de", page); got != "" {
		t.Errorf("missing lang = %q", got)
	}

	// languageURL.
	if g.languageURL("pl") != "/" || g.languageURL("en") != "/en/" {
		t.Errorf("languageURL = %q / %q", g.languageURL("pl"), g.languageURL("en"))
	}

	// localizeDate: presets, unknown preset, per-language zone.
	when := time.Date(2026, 3, 7, 23, 30, 0, 0, time.UTC)
	g.currentLang = "pl"
	if got := g.localizeDate(when, "short"); got != "2026-03-07" {
		t.Errorf("short = %q", got)
	}
	if got := g.localizeDate(when, "full"); !strings.Contains(got, "March") {
		t.Errorf("full = %q", got)
	}
	if g.localizeDate(when, "???") != g.localizeDate(when, "medium") {
		t.Error("unknown preset must fall back to medium")
	}
	// en zone America/New_York: 23:30 UTC = 18:30 EST → still 7 March.
	g.currentLang = "en"
	if got := g.localizeDate(when, "short"); got != "2026-03-07" {
		t.Errorf("zoned short = %q", got)
	}

	// languagePages filter.
	pages := []models.Page{{Slug: "a", Lang: "pl"}, {Slug: "b", Lang: "en"}}
	if got := languagePages(pages, "en"); len(got) != 1 || got[0].Slug != "b" {
		t.Errorf("languagePages = %+v", got)
	}
}
