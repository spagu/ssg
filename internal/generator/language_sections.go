package generator

// Assigning a language to a content section rather than to a page (#182).
//
// A page can declare its own `lang:` and `languages:`/`default_language:` say
// what the site has, but there was no way to say "everything under this
// directory is German" in one place.
//
// That is the shape a migrated site arrives in, and it is the case that matters.
// A bilingual WordPress site keeps its languages in /de/ and /fr/ and says so
// nowhere a page carries: the language was a plugin's property of the *section*,
// not a field on the post. An export therefore produces a few hundred documents
// with no `lang` at all, and the only ways to give them one were to write it
// into every file — which the next migration overwrites — or to hand-edit after
// every build. A migration is not a one-off; it is run again whenever the source
// changes, so the fix has to live somewhere a re-migration does not touch.
//
//	language_sections:
//	  de: de
//	  fr/blog: fr
//	  home: en
//
// The spelling is deliberately the one the configuration already uses for
// output_encoding_sections and schema_defaults: keyed by the page's directory
// relative to the source, longest prefix wins, `home` for the site root. One
// prefix convention in the project rather than three.

import (
	"fmt"
	"strings"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// homeSectionKey is the reserved key for the site root — the only page that has
// no directory to be keyed by.
const homeSectionKey = "home"

// sectionValue resolves a section-keyed map for one content section: the entry
// whose key is the longest prefix of section, ignoring the reserved `home` key.
// Generic because three settings share the rule and would otherwise share three
// copies of the loop.
func sectionValue[T any](sections map[string]T, section string) (T, bool) {
	var best T
	bestLen := -1
	for prefix, v := range sections {
		if prefix == homeSectionKey {
			continue
		}
		p := strings.Trim(prefix, "/")
		if p != "" && (section == p || strings.HasPrefix(section, p+"/")) && len(p) > bestLen {
			best, bestLen = v, len(p)
		}
	}
	return best, bestLen >= 0
}

// sectionLanguage returns the language language_sections assigns to a page, or
// "" when no section claims it.
//
// Only consulted for a page that has not named a language itself: a per-file
// declaration is more specific than a per-section default and must win, the same
// rule applySourceCategory documents for categories.
func (g *Generator) sectionLanguage(page models.Page) string {
	if len(g.config.LanguageSections) == 0 {
		return ""
	}
	if strings.Trim(page.GetURL(), "/") == "" {
		if v, ok := g.config.LanguageSections[homeSectionKey]; ok {
			return strings.TrimSpace(v)
		}
	}
	section := g.contentSection(page)
	if section == "" {
		return ""
	}
	v, _ := sectionValue(g.config.LanguageSections, section)
	return strings.TrimSpace(v)
}

// warnUnconfiguredSectionLanguages reports a section assigning a language the
// site does not declare — once per section, not once per file. A misspelt code
// silently producing a language nobody configured is the failure this catches,
// and it is the same complaint validateI18nContent makes about a page.
//
// Silent when the site declares no languages at all: language_sections on a
// single-language site is unusual but not wrong, and there is nothing to check
// it against.
func (g *Generator) warnUnconfiguredSectionLanguages(languages []ssgi18n.LanguageConfig) {
	if len(g.config.LanguageSections) == 0 || len(languages) == 0 {
		return
	}
	known := make(map[string]bool, len(languages))
	for _, l := range languages {
		known[l.Code] = true
	}
	for _, section := range sortedKeys(g.config.LanguageSections) {
		lang := strings.TrimSpace(g.config.LanguageSections[section])
		if lang == "" || known[lang] {
			continue
		}
		fmt.Printf("   ⚠️  language_sections %q uses unconfigured language %q\n", section, lang)
	}
}
