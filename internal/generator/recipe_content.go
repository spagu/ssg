package generator

// A recipe's ingredients and method from the post itself (1.8.72). A recipe
// post already says them in prose — a heading "Ingredients" over a list, a
// heading "Method" over numbered steps — and a site used to run a script
// that copied both into schema: for every post. When a Recipe has neither
// recipeIngredient nor recipeInstructions, ssg reads them from those
// sections. The page's own schema: still wins, field by field.

import (
	"regexp"
	"strings"
)

// recipeHeadings are the section names read, lower case, per role. A site
// adds its own with recipe_headings.
var recipeHeadings = map[string][]string{
	"ingredients": {"ingredients", "składniki", "zutaten", "ingrédients", "ingredientes", "ingredienti"},
	"instructions": {"method", "instructions", "directions", "preparation", "steps",
		"przygotowanie", "sposób przygotowania", "wykonanie", "zubereitung", "préparation", "preparación", "preparazione"},
}

var (
	// atxHeadingRe is "## Name"; boldHeadingRe a paragraph that is only bold
	// text ("**Ingredients**"), which exported posts use as often.
	atxHeadingRe  = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
	boldHeadingRe = regexp.MustCompile(`^\*\*(.+?)\*\*:?\s*$`)
	listItemRe    = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(.+)$`)
	recipeLinkRe  = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	mdEmphasisRe  = regexp.MustCompile(`(\*\*|__|\*|_|` + "`" + `)`)
)

// fillRecipeFromContent sets recipeIngredient and recipeInstructions on a
// Recipe that has neither field, from the page's Markdown sections.
func (g *Generator) fillRecipeFromContent(ld map[string]interface{}, content string) {
	if !hasLDType(ld, "Recipe") {
		return
	}
	for role, prop := range map[string]string{"ingredients": "recipeIngredient", "instructions": "recipeInstructions"} {
		if hasLDProp(ld, prop) {
			continue
		}
		if items := recipeSection(content, g.recipeHeadingNames(role)); len(items) > 0 {
			list := make([]interface{}, len(items))
			for i, it := range items {
				list[i] = it
			}
			ld[prop] = list
		}
	}
}

// recipeHeadingNames is the built-in names for a role plus the site's own.
func (g *Generator) recipeHeadingNames(role string) []string {
	names := append([]string{}, recipeHeadings[role]...)
	for _, n := range g.config.RecipeHeadings[role] {
		names = append(names, strings.ToLower(strings.TrimSpace(n)))
	}
	return names
}

// recipeSection returns the items under the first heading named in names:
// list items, or — under a heading with no list — its paragraphs. It ends at
// the next heading of the same or a higher level (or the next bold heading).
func recipeSection(content string, names []string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start, level := -1, 0
	for i, line := range lines {
		if name, lvl, ok := headingOf(line); ok && matchesHeading(name, names) {
			start, level = i+1, lvl
			break
		}
	}
	if start < 0 {
		return nil
	}
	var items, paragraphs []string
	var para []string
	flush := func() {
		if len(para) > 0 {
			paragraphs = append(paragraphs, plainInline(strings.Join(para, " ")))
			para = nil
		}
	}
	for _, line := range lines[start:] {
		if _, lvl, ok := headingOf(line); ok {
			if lvl <= level || level == boldLevel {
				break // the next section
			}
			flush() // a sub-heading ("**Step 2**") separates steps
			continue
		}
		if m := listItemRe.FindStringSubmatch(line); m != nil {
			flush()
			items = append(items, plainInline(m[1]))
			continue
		}
		if t := strings.TrimSpace(line); t == "" {
			flush()
		} else if !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "!") && !strings.HasPrefix(t, "<") {
			para = append(para, t)
		}
	}
	flush()
	if len(items) > 0 {
		return items
	}
	return paragraphs
}

// boldLevel is the level given to a bold-paragraph heading: below every ATX
// level, so a "**Step 1**" inside "## Method" separates steps rather than
// ending the section, while a section that itself began at a bold heading
// ends at the next heading of any kind.
const boldLevel = 7

// headingOf reports a line's heading text and level.
func headingOf(line string) (string, int, bool) {
	line = strings.TrimSpace(line)
	if m := atxHeadingRe.FindStringSubmatch(line); m != nil {
		return m[2], len(m[1]), true
	}
	if m := boldHeadingRe.FindStringSubmatch(line); m != nil {
		return m[1], boldLevel, true
	}
	return "", 0, false
}

// matchesHeading reports whether a heading names a section: equal to a name,
// or starting with it ("Ingredients for 16 servings").
func matchesHeading(heading string, names []string) bool {
	h := strings.ToLower(strings.TrimSpace(plainInline(heading)))
	h = strings.TrimRight(h, ":")
	for _, n := range names {
		if h == n || strings.HasPrefix(h, n+" ") || strings.HasPrefix(h, n+":") {
			return true
		}
	}
	return false
}

// plainInline drops Markdown inline syntax: links keep their text, emphasis
// and code marks go.
func plainInline(s string) string {
	s = recipeLinkRe.ReplaceAllString(s, "$1")
	s = mdEmphasisRe.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}
