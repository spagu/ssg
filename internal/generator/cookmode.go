package generator

// Cook Mode (1.8.72): a full-screen, one-step-at-a-time view of a recipe for
// the kitchen. The first screen is the ingredients as a checklist, then one
// large-text screen per step, then a finish screen; Next/Back, the arrow
// keys, a swipe or a tap on either half of the screen move between them. The
// screen is kept awake while it is open (Screen Wake Lock, where supported),
// and the step and the ticked ingredients survive a reload.
//
// It appears only on pages whose merged structured data is a Recipe with
// both recipeIngredient and recipeInstructions: the button carries that
// recipe as JSON in a data attribute, so the script needs no request. The
// placement follows Listen and Print — a theme's
// <span data-ssg-cook-slot></span>, else after the first </h1> unless
// cook_mode.auto is false — and an empty slot is removed.

import (
	_ "embed"
	"encoding/json"
	stdhtml "html"
	"strings"
)

//go:embed assets/cookmode.js
var cookScript string

//go:embed assets/cookmode.css
var cookStyle string

// CookModeOptions configures the Cook Mode button.
type CookModeOptions struct {
	Enabled bool
	Label   string // default "Cook mode"
	NoAuto  bool   // only fill a theme's slot; never place after the first </h1>
	// Labels overrides the dialog's texts: ingredients, step ("Step {n} of
	// {total}"), next, back, close, restart, done, finished.
	Labels map[string]string
}

const (
	// cookMarker is on the button, always followed by a space.
	cookMarker     = "data-ssg-cook"
	cookSlot       = `<span data-ssg-cook-slot></span>`
	cookScriptAttr = "data-ssg-cook-script"
)

// cookIcon is a knife and fork, decorative.
const cookIcon = `<svg class="ssg-tool-icon" aria-hidden="true" focusable="false" width="18" height="18" viewBox="0 0 24 24">` +
	`<path fill="currentColor" d="M11 9H9V2H7v7H5V2H3v7a4 4 0 0 0 3.75 3.97V22h2.5v-9.03A4 4 0 0 0 13 9V2h-2v7zm5-3v8h2.5v8H21V2a5 5 0 0 0-5 4z"/></svg>`

// cookRecipe is the recipe as the script reads it.
type cookRecipe struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
}

// cookBlock is the button for a complete recipe, hidden until the script
// shows it (no JavaScript, or no <dialog>, means no button).
func (g *Generator) cookBlock(r recipeData) string {
	label := strings.TrimSpace(g.config.CookMode.Label)
	if label == "" {
		label = "Cook mode"
	}
	attrs := cookMarker + ` aria-haspopup="dialog" data-recipe="` +
		jsonAttr(cookRecipe{Name: r.Name, Ingredients: r.Ingredients, Steps: r.Steps}) + `"`
	if len(g.config.CookMode.Labels) > 0 {
		attrs += ` data-labels="` + jsonAttr(g.config.CookMode.Labels) + `"`
	}
	return toolButton("ssg-cook", attrs, cookIcon, label)
}

// cookHTMLString places the button on a page with a complete recipe (nil
// otherwise), removes an empty slot, and adds the styles and script once.
func (g *Generator) cookHTMLString(s string, recipe *recipeData) string {
	if recipe != nil && !strings.Contains(s, " "+cookMarker+" ") {
		s = placeTool(s, cookSlot, g.cookBlock(*recipe), !g.config.CookMode.NoAuto)
	}
	s = strings.ReplaceAll(s, cookSlot, "")
	if !strings.Contains(s, " "+cookMarker+" ") || strings.Contains(s, cookScriptAttr) {
		return s
	}
	return insertBeforeBody(s, `<style data-ssg-cook-style>`+cookStyle+`</style><script `+cookScriptAttr+`>`+cookScript+`</script>`)
}

// jsonAttr is v as JSON, escaped for a double-quoted attribute. Marshal
// already writes <, > and & as \u escapes; the quotes are escaped here.
func jsonAttr(v interface{}) string {
	data, _ := json.Marshal(v) // strings, lists and maps of strings cannot fail
	return stdhtml.EscapeString(string(data))
}
