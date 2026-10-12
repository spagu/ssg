package generator

// The reader's page tools after the title (1.8.72): Print / PDF and Cook
// Mode, placed by the same rules as Listen. One render-transform hook runs
// both, so the recipe in the page's structured data is read once.

import (
	_ "embed"
	stdhtml "html"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

//go:embed assets/pagetools.css
var pageToolsStyle string

// pageToolsStyleAttr marks the buttons' shared stylesheet, added once.
const pageToolsStyleAttr = "data-ssg-tools-style"

// pageToolsHTMLString adds Cook Mode and Print to a rendered page. It runs
// before Listen, and each places itself right after the first </h1>, so the
// buttons read Listen, Print, Cook mode in that order. Slots the features
// leave empty are removed, so a site with both off is unchanged.
func (g *Generator) pageToolsHTMLString(s string, page *models.Page, isPost bool) string {
	var printing bool
	var recipe *recipeData
	if page != nil && page.Content != "" {
		printing = g.config.Print.Enabled && sectionSelected(g.config.Print.Sections, isPost)
		if printing || g.config.CookMode.Enabled {
			if r, ok := g.pageRecipe(*page, isPost); ok && (len(r.Ingredients) > 0 || len(r.Steps) > 0) {
				recipe = &r
			}
		}
	}
	var cook *recipeData
	if g.config.CookMode.Enabled && recipe != nil && recipe.complete() {
		cook = recipe
	}
	s = g.cookHTMLString(s, cook)
	s = g.printHTMLString(s, printing, recipe)
	if (strings.Contains(s, " "+cookMarker+" ") || strings.Contains(s, " "+printMarker+" ")) &&
		!strings.Contains(s, pageToolsStyleAttr) {
		s = insertBeforeBody(s, `<style `+pageToolsStyleAttr+`>`+pageToolsStyle+`</style>`)
	}
	return s
}

// placeTool puts block into the theme's slot, or — when auto — after the
// first </h1>. Without auto a page with no slot is left alone.
func placeTool(s, slot, block string, auto bool) string {
	if !auto && !strings.Contains(s, slot) {
		return s
	}
	return placeBlock(s, slot, block)
}

// toolButton is a page-tool button, hidden until its script shows it. attrs
// are trusted attribute markup; label is escaped.
func toolButton(class, attrs, icon, label string) string {
	return `<button type="button" class="` + class + `" ` + attrs + ` hidden>` + icon +
		`<span class="ssg-tool-label">` + stdhtml.EscapeString(label) + `</span></button>`
}

// labelOr is the configured text for key, or def.
func labelOr(labels map[string]string, key, def string) string {
	if v := strings.TrimSpace(labels[key]); v != "" {
		return v
	}
	return def
}

// insertBeforeBody adds markup before the last </body>, or at the end of a
// fragment that has none.
func insertBeforeBody(s, add string) string {
	if i := strings.LastIndex(s, "</body>"); i >= 0 {
		return s[:i] + add + "\n" + s[i:]
	}
	return s + add
}
