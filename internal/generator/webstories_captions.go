package generator

// What a story says: short hooks a reader decides on, from what the post
// already declares — never its method text. Every phrase can be translated
// in the i18n catalog under web_stories.*.

import (
	"strconv"
	"strings"
	"unicode/utf8"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// storyCopy is the English shipped when the catalog has no translation.
var storyCopy = map[string]string{
	"ready_in":        "Ready in {{time}}",
	"cooks_in":        "Cooks in {{time}}",
	"makes":           "Makes {{yield}}",
	"servings":        "{{count}} servings",
	"watch":           "Watch as a Web Story",
	"cta":             defaultStoryCTA,
	"hub_title":       "Web Stories",
	"hub_description": "Web Stories from {{site}}",
}

// storyTagsShown is how many tags one caption names.
const storyTagsShown = 3

// storyText is one phrase in a language, its {{placeholders}} filled.
func (g *Generator) storyText(lang, key string, vars map[string]any) string {
	msg, ok := g.catalogString(lang, "web_stories."+key)
	if !ok {
		msg = storyCopy[key]
	}
	return ssgi18n.Interpolate(msg, vars)
}

// storyCaptions are the photo pages' captions, in order: the tags, how long
// a recipe takes, what it makes, then the excerpt.
func (g *Generator) storyCaptions(p models.Page, isPost bool, lang string) []string {
	var out []string
	if tags := storyTags(p.Tags); tags != "" {
		out = append(out, tags)
	}
	ld := g.mergedSchema(p, isPost)
	if hasLDType(ld, "Recipe") {
		if t := storyDuration(lang, ld["totalTime"]); t != "" {
			out = append(out, g.storyText(lang, "ready_in", map[string]any{"time": t}))
		} else if t := storyDuration(lang, ld["cookTime"]); t != "" {
			out = append(out, g.storyText(lang, "cooks_in", map[string]any{"time": t}))
		}
		if y := g.storyYield(lang, ld["recipeYield"]); y != "" {
			out = append(out, y)
		}
	}
	if ex := storyExcerpt(p, storyExcerptMaxRunes); ex != "" {
		out = append(out, ex)
	}
	return out
}

// storyTags names the first few tags.
func storyTags(tags []string) string {
	var shown []string
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			shown = append(shown, t)
		}
		if len(shown) == storyTagsShown {
			break
		}
	}
	return strings.Join(shown, " · ")
}

// storyDuration humanises an ISO 8601 duration the way the theme helper
// {{ duration }} does: PT1H25M is "1 hr 25 min". Anything else — a missing
// value, prose, a zero — is "".
func storyDuration(lang string, v interface{}) string {
	s, _ := v.(string)
	s = strings.ToUpper(strings.TrimSpace(s))
	if !isoDurationRe.MatchString(s) {
		return ""
	}
	out := tmplDuration(s, lang)
	if out == s || strings.HasPrefix(out, "0 ") {
		return ""
	}
	return out
}

// storyYield is "Makes 16 servings" from a recipeYield: a number, a phrase,
// or a list of either (the first counts).
func (g *Generator) storyYield(lang string, v interface{}) string {
	yields := ldTexts(v)
	if len(yields) == 0 {
		return ""
	}
	y := yields[0]
	if _, err := strconv.ParseFloat(y, 64); err == nil {
		y = g.storyText(lang, "servings", map[string]any{"count": y})
	}
	if strings.HasPrefix(strings.ToLower(y), "makes ") {
		return strings.ToUpper(y[:1]) + y[1:]
	}
	return g.storyText(lang, "makes", map[string]any{"yield": y})
}

// storyExcerpt is the post's own summary as plain text, cut at a word.
func storyExcerpt(p models.Page, max int) string {
	text := p.Excerpt
	if strings.TrimSpace(text) == "" {
		text = p.Description
	}
	text = strings.Join(strings.Fields(tmplDecodeHTML(tmplStripHTML(tmplStripShortcodes(text)))), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)[:max]
	cut := string(runes)
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-–—") + "…"
}

// storyDescription is the story's meta description: the excerpt, else the title.
func (g *Generator) storyDescription(p models.Page) string {
	if d := storyExcerpt(p, storyDescriptionMaxLen); d != "" {
		return d
	}
	return p.Title
}

// storyCTALabel is the last page's link text: web_stories.cta in the
// config, else the catalog's, else English.
func (g *Generator) storyCTALabel(lang string) string {
	if c := strings.TrimSpace(g.config.WebStories.CTA); c != "" {
		return c
	}
	return g.storyText(lang, "cta", nil)
}

// storyArticleURL is where the last page sends the reader: the article's
// canonical address.
func (g *Generator) storyArticleURL(p models.Page) string {
	if g.config.Domain != "" {
		return g.servedCanonical(p)
	}
	return g.storyHref(p.GetURL())
}
