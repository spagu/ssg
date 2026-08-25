package generator

// The generated 404, in the site's own language when it has one (#209).
//
// A static host with no 404.html falls back to index.html for any unmatched
// path and answers 200 doing it, so every dead URL becomes a live page
// duplicating the home page. ssg fills that gap — but it filled it with English
// copy under a hardcoded `lang="en"`, so a Polish site served an English 404 to
// real visitors, which after a migration is what every old link produces.
//
// The attribute was not the bug. `lang` describes the language of the content,
// and the content was English, so declaring `pl` over it would have made a
// screen reader switch voice for text that had not changed — worse than leaving
// it alone. What was wrong was the copy, so this translates the copy and then
// labels the page with whichever language it actually used.

import (
	"fmt"
	"html/template"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
)

// notFoundKeys are the catalog entries this page reads.
var notFoundKeys = struct{ title, body, home string }{
	title: "not_found.title",
	body:  "not_found.body",
	home:  "not_found.home",
}

// notFoundCopy is everything the page says, plus the language it says it in.
type notFoundCopy struct {
	Title, Body, Home, Lang string
}

// englishNotFound is the copy shipped when nothing translates it. `{{site}}`
// rather than a printf verb, so a translator moves it where their grammar wants
// it instead of matching an argument order.
func englishNotFound(domain string) notFoundCopy {
	return notFoundCopy{
		Title: "404 — page not found",
		Body:  ssgi18n.Interpolate("That page does not exist on {{site}}.", map[string]any{"site": domain}),
		Home:  "Go to the home page",
		Lang:  "en",
	}
}

// notFoundText resolves the page's copy.
//
// All three strings must translate, or none do. A page carrying two translated
// lines, one English one and `lang="pl"` is worse than a wholly English page:
// the mislabelled part is the part a screen reader gets wrong, and nobody
// proof-reads a 404.
func (g *Generator) notFoundText() notFoundCopy {
	// The old body said "This site" when no domain was configured; keep that,
	// so a scaffolded project does not serve a sentence with a hole in it.
	site := g.config.Domain
	if site == "" {
		site = "This site"
	}
	fallback := englishNotFound(site)
	lang := g.currentLang
	if lang == "" {
		lang = g.config.DefaultLanguage
	}
	if g.catalog == nil || lang == "" {
		return fallback
	}

	title, okT := g.catalogString(lang, notFoundKeys.title)
	body, okB := g.catalogString(lang, notFoundKeys.body)
	home, okH := g.catalogString(lang, notFoundKeys.home)
	if !okT || !okB || !okH {
		return fallback
	}
	return notFoundCopy{
		Title: title,
		Body:  ssgi18n.Interpolate(body, map[string]any{"site": site}),
		Home:  home,
		Lang:  lang,
	}
}

// catalogString looks a key up without the missing-translation policy.
//
// translationValue applies `missing_translation`, which for a template is
// right — an author asked for that string. Here nobody asked: a site with no
// catalog would get a warning per string on every build, about copy it never
// wrote, for a page it may never serve.
func (g *Generator) catalogString(lang, key string) (string, bool) {
	value, ok := g.catalog.Lookup(lang, key)
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok && s != ""
}

// renderNotFound builds the page.
func renderNotFound(c notFoundCopy) string {
	esc := template.HTMLEscapeString
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>%s</title>
</head>
<body>
<h1>%s</h1>
<p>%s</p>
<p><a href="/">%s</a></p>
</body>
</html>
`, esc(c.Lang), esc(c.Title), esc(c.Title), esc(c.Body), esc(c.Home))
}
