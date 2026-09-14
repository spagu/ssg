package generator

// Letter case in a template (#280).
//
// A language switcher wants "EN PL RO" from `.Lang` values "en", "pl", "ro",
// and html/template has no case builtin, so a theme ended up carrying an
// if/else chain per language — the per-language hardcoding the i18n config
// exists to avoid. CSS `text-transform` is no substitute: it changes the glyphs
// but not the accessible name, and it cannot reach an attribute or a title.
//
// Named the way Hugo names them, so a theme ports over unchanged.

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// stringCaseFuncs returns the case helpers, for every template context.
func stringCaseFuncs() map[string]interface{} {
	return map[string]interface{}{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": tmplTitle,
	}
}

// tmplTitle capitalises the first letter of each word. Language-neutral on
// purpose: a template does not say which language a string is in, and the
// neutral rules are the ones that never surprise — "ijssel" stays "Ijssel"
// rather than becoming Dutch "IJssel" on one site and not on another.
func tmplTitle(s string) string {
	return cases.Title(language.Und).String(s)
}
