package generator

// A recipe as plain text, read from the page's merged JSON-LD (1.8.72). The
// print card and Cook Mode both need the same thing — the title, what it
// makes, how long it takes, the ingredients and the numbered steps — and
// both read it from the structured data the page already publishes, so a
// recipe is written once, in `schema:`, and never repeated for a component.
//
// recipeInstructions comes in three shapes in the wild: plain strings,
// HowToStep objects ({"@type":"HowToStep","text":…}) and HowToSection
// objects grouping steps under itemListElement. All three flatten to one
// list of step texts. HTML a WordPress import left in a value is reduced to
// its text.

import (
	stdhtml "html"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// recipeTime is one of a recipe's durations, labelled.
type recipeTime struct {
	Key   string // prep, cook, total — the label key
	Value string // "1 h 30 min"
}

// recipeData is what a Recipe node holds that a reader needs at the stove.
type recipeData struct {
	Name        string
	Yield       string
	Times       []recipeTime
	Ingredients []string
	Steps       []string
}

// complete reports whether the recipe can be cooked step by step: it has
// both ingredients and instructions.
func (r recipeData) complete() bool {
	return len(r.Ingredients) > 0 && len(r.Steps) > 0
}

// pageRecipe returns the recipe in a page's merged schema, and false when
// the page is not a Recipe.
func (g *Generator) pageRecipe(page models.Page, isPost bool) (recipeData, bool) {
	lang := page.Lang
	if lang == "" {
		lang = g.currentLang
	}
	return recipeOf(g.mergedSchema(page, isPost), lang)
}

// recipeOf extracts a recipe from a JSON-LD node, and false when the node's
// @type is not Recipe. Times are written for people in lang.
func recipeOf(ld map[string]interface{}, lang string) (recipeData, bool) {
	if !hasLDType(ld, "Recipe") {
		return recipeData{}, false
	}
	r := recipeData{
		Name:        ldText(ld["name"]),
		Yield:       strings.Join(ldTexts(ld["recipeYield"]), ", "),
		Ingredients: ldTexts(ld["recipeIngredient"]),
		Steps:       ldSteps(ld["recipeInstructions"]),
	}
	for _, k := range []string{"prep", "cook", "total"} {
		if v := tmplDuration(ldText(ld[k+"Time"]), lang); v != "" {
			r.Times = append(r.Times, recipeTime{Key: k, Value: v})
		}
	}
	return r, true
}

// ldText is a scalar value as plain text: tags dropped, entities decoded,
// whitespace collapsed. Anything but a string or a number is empty.
func ldText(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(stdhtml.UnescapeString(tmplStripHTML(t))), " ")
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return ""
}

// ldTexts is a value that may be one scalar or a list of them, as texts.
func ldTexts(v interface{}) []string {
	var out []string
	for _, item := range ldList(v) {
		if s := ldText(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ldList is a value as a list: a list as is, one value as a list of one.
func ldList(v interface{}) []interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case []interface{}:
		return t
	case []string:
		out := make([]interface{}, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	}
	return []interface{}{v}
}

// ldSteps flattens recipeInstructions into step texts. A single string is
// split on blank lines and line breaks, so a whole method pasted into one
// field still becomes steps.
func ldSteps(v interface{}) []string {
	var out []string
	for _, item := range ldList(v) {
		switch t := item.(type) {
		case string:
			for _, line := range strings.Split(strings.ReplaceAll(t, "\r\n", "\n"), "\n") {
				if s := ldText(line); s != "" {
					out = append(out, s)
				}
			}
		case map[string]interface{}:
			if hasLDType(t, "HowToSection") {
				out = append(out, ldSteps(t["itemListElement"])...)
				continue
			}
			s := ldText(t["text"])
			if s == "" {
				s = ldText(t["name"])
			}
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
