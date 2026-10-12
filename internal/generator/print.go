package generator

// Print / Save as PDF (1.8.72). A button that opens the browser's print
// dialog — which in every current browser offers "Save as PDF" — and a print
// stylesheet that turns any theme's article into a clean page: no header,
// navigation, sidebar, comments or buttons; the article full width; images
// kept whole; the address of every external link written after it; and a
// footer line with the page's URL and the date it was printed.
//
// On a page whose structured data is a Recipe a second button prints only a
// compact recipe card — title, yield and times, ingredients, numbered steps —
// built here from the merged schema into a hidden <section>, and shown by the
// print stylesheet while the script has marked <html> for it.
//
// Like Listen, the block goes into a theme's <span data-ssg-print-slot></span>
// or after the first </h1> (unless print.auto is false), a slot left empty is
// removed, and the stylesheet and script are added once, to pages that carry
// the block. No template function: a bundled theme is built by older ssg
// releases too.

import (
	_ "embed"
	stdhtml "html"
	"strings"
)

//go:embed assets/print.js
var printScript string

//go:embed assets/print.css
var printStyle string

// PrintOptions configures the Print / PDF button.
type PrintOptions struct {
	Enabled  bool
	Label    string   // default "Print / PDF"
	Sections []string // posts (default), pages
	NoAuto   bool     // only fill a theme's slot; never place after the first </h1>
	// Labels overrides the other texts: recipe, ingredients, steps, yield,
	// prep, cook, total, printed.
	Labels map[string]string
}

// printDefaults are the English texts a Labels entry replaces.
var printDefaults = map[string]string{
	"recipe": "Print recipe", "ingredients": "Ingredients", "steps": "Method",
	"yield": "Makes", "prep": "Prep", "cook": "Cook", "total": "Total", "printed": "Printed",
}

const (
	// printMarker is on the block; followed by a space, which tells it apart
	// from the slot, the script and the footer attributes it prefixes.
	printMarker = "data-ssg-print"
	printSlot   = `<span data-ssg-print-slot></span>`
	// printScriptAttr marks the injected script, so it is added once.
	printScriptAttr = "data-ssg-print-script"
)

// printIcon is a printer, decorative: the label says what the button does.
const printIcon = `<svg class="ssg-tool-icon" aria-hidden="true" focusable="false" width="18" height="18" viewBox="0 0 24 24">` +
	`<path fill="currentColor" d="M19 8H5a3 3 0 0 0-3 3v6h4v4h12v-4h4v-6a3 3 0 0 0-3-3zm-3 11H8v-5h8v5zm3-7a1 1 0 1 1 0-2 1 1 0 0 1 0 2zm-1-9H6v4h12V3z"/></svg>`

// printLabel is the configured text for key, or the English default.
func (g *Generator) printLabel(key string) string {
	return labelOr(g.config.Print.Labels, key, printDefaults[key])
}

// printBlock is the button group: "Print / PDF", and "Print recipe" when the
// page has a recipe card. Hidden until the script shows it.
func (g *Generator) printBlock(withRecipe bool) string {
	label := strings.TrimSpace(g.config.Print.Label)
	if label == "" {
		label = "Print / PDF"
	}
	b := `<span class="ssg-print" ` + printMarker + ` hidden>` +
		toolButton("ssg-print-button", `data-print="page"`, printIcon, label)
	if withRecipe {
		b += toolButton("ssg-print-button", `data-print="recipe"`, printIcon, g.printLabel("recipe"))
	}
	return b + `</span>`
}

// printExtras are the hidden elements the print stylesheet shows: the footer
// line the script fills in, and the recipe card when there is one.
func (g *Generator) printExtras(r *recipeData) string {
	out := `<div class="ssg-print-footer" data-ssg-print-footer data-printed="` +
		stdhtml.EscapeString(g.printLabel("printed")) + `" hidden></div>`
	if r != nil {
		out += g.recipeCardHTML(*r)
	}
	return out
}

// recipeCardHTML is the compact card "Print recipe" prints. Every value is
// escaped: it comes from front matter and config.
func (g *Generator) recipeCardHTML(r recipeData) string {
	var b strings.Builder
	b.WriteString(`<section class="ssg-recipe-card" data-ssg-recipe-card hidden>`)
	if r.Name != "" {
		b.WriteString(`<h1>` + stdhtml.EscapeString(r.Name) + `</h1>`)
	}
	var meta []string
	if r.Yield != "" {
		meta = append(meta, g.printLabel("yield")+": "+r.Yield)
	}
	for _, t := range r.Times {
		meta = append(meta, g.printLabel(t.Key)+": "+t.Value)
	}
	if len(meta) > 0 {
		b.WriteString(`<p class="ssg-recipe-meta">` + stdhtml.EscapeString(strings.Join(meta, " · ")) + `</p>`)
	}
	writeList := func(heading, tag string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(`<h2>` + stdhtml.EscapeString(heading) + `</h2><` + tag + `>`)
		for _, it := range items {
			b.WriteString(`<li>` + stdhtml.EscapeString(it) + `</li>`)
		}
		b.WriteString(`</` + tag + `>`)
	}
	writeList(g.printLabel("ingredients"), "ul", r.Ingredients)
	writeList(g.printLabel("steps"), "ol", r.Steps)
	b.WriteString(`</section>`)
	return b.String()
}

// printHTMLString places the block on a page in the print sections, removes
// an empty slot, and adds the stylesheet, the hidden extras and the script
// to a page that has the block. recipe is nil on a page without one.
func (g *Generator) printHTMLString(s string, active bool, recipe *recipeData) string {
	if active && !strings.Contains(s, " "+printMarker+" ") {
		s = placeTool(s, printSlot, g.printBlock(recipe != nil), !g.config.Print.NoAuto)
	}
	s = strings.ReplaceAll(s, printSlot, "")
	if !strings.Contains(s, " "+printMarker+" ") || strings.Contains(s, printScriptAttr) {
		return s
	}
	return insertBeforeBody(s, `<style data-ssg-print-style>`+printStyle+`</style>`+
		g.printExtras(recipe)+`<script `+printScriptAttr+`>`+printScript+`</script>`)
}
