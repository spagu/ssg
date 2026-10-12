package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// toolsGen is a generator with one post, for the page-tool tests.
func toolsGen(t *testing.T, cfg func(*Config)) (*Generator, models.Page) {
	t.Helper()
	g := sectionGen(t.TempDir(), nil)
	post := models.Page{Title: "Soup", Slug: "soup", Link: "/soup/", Content: "<p>x</p>"}
	g.siteData.Posts = []models.Page{post}
	cfg(&g.config)
	return g, post
}

// recipeSchema is a complete Recipe with markup-significant characters.
func recipeSchema() map[string]interface{} {
	return map[string]interface{}{
		"@type": "Recipe", "name": `Tom "&" Jerry <3`, "recipeYield": "2", "prepTime": "PT10M",
		"recipeIngredient":   []interface{}{"1 l water", `a "pinch" of <b>salt</b> <2g`},
		"recipeInstructions": []interface{}{"Boil.", map[string]interface{}{"@type": "HowToStep", "text": "Salt & serve."}},
	}
}

const toolsPage = "<html><body><article><h1>Soup</h1><p>x</p></article></body></html>"

// TestPageToolsOffIsUnchanged: with both features off a page — slots and
// all — comes out exactly as a theme without slots would.
func TestPageToolsOffIsUnchanged(t *testing.T) {
	g, post := toolsGen(t, func(*Config) {})
	post.Schema = recipeSchema()
	in := "<html><body><h1>Soup</h1><p>" + printSlot + cookSlot + "</p></body></html>"
	if got := g.pageToolsHTMLString(in, &post, true); got != "<html><body><h1>Soup</h1><p></p></body></html>" {
		t.Errorf("off = %s", got)
	}
	if got := g.pageToolsHTMLString(toolsPage, nil, false); got != toolsPage {
		t.Errorf("no page = %s", got)
	}
}

// TestPrintPlacementAndSections: the block goes after the first </h1> on a
// post, not on a page unless sections says so, with the styles, footer and
// script once.
func TestPrintPlacementAndSections(t *testing.T) {
	g, post := toolsGen(t, func(c *Config) { c.Print = PrintOptions{Enabled: true} })
	got := g.pageToolsHTMLString(toolsPage, &post, true)
	for _, want := range []string{
		"</h1>\n" + `<span class="ssg-print" data-ssg-print hidden><button type="button" class="ssg-print-button" data-print="page" hidden>`,
		`<span class="ssg-tool-label">Print / PDF</span>`, "<style data-ssg-print-style>/*", printScriptAttr,
		`<div class="ssg-print-footer" data-ssg-print-footer data-printed="Printed" hidden></div>`, pageToolsStyleAttr,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `data-print="recipe"`) || strings.Contains(got, `<section class="ssg-recipe-card"`) {
		t.Error("a page without a recipe has no recipe button or card")
	}
	if !strings.HasSuffix(got, "\n</body></html>") {
		t.Error("the assets go before </body>")
	}
	if again := g.pageToolsHTMLString(got, &post, true); again != got {
		t.Error("placement and assets must not repeat")
	}
	page := models.Page{Title: "About", Link: "/about/", Content: "<p>a</p>"}
	if got := g.pageToolsHTMLString(toolsPage, &page, false); got != toolsPage {
		t.Error("pages are not in the default sections")
	}
	g.config.Print.Sections = []string{"pages"}
	if got := g.pageToolsHTMLString(toolsPage, &page, false); !strings.Contains(got, printScriptAttr) {
		t.Error("sections: pages selects pages")
	}
	empty := models.Page{Link: "/"}
	if got := g.pageToolsHTMLString(toolsPage, &empty, false); got != toolsPage {
		t.Error("a listing context has no content and gets no button")
	}
}

// TestPrintSlotAndAuto: a theme's slot wins over the auto placement; with
// auto off only a slot is filled.
func TestPrintSlotAndAuto(t *testing.T) {
	g, post := toolsGen(t, func(c *Config) { c.Print = PrintOptions{Enabled: true, NoAuto: true, Label: "Drukuj <PDF>"} })
	if got := g.pageToolsHTMLString(toolsPage, &post, true); got != toolsPage {
		t.Errorf("auto: false and no slot leaves the page alone: %s", got)
	}
	slotted := "<html><body><h1>Soup</h1><p class=meta>" + printSlot + "</p></body></html>"
	got := g.pageToolsHTMLString(slotted, &post, true)
	if !strings.Contains(got, `<p class=meta><span class="ssg-print" data-ssg-print hidden>`) || strings.Contains(got, printSlot) {
		t.Errorf("slot placement:\n%s", got)
	}
	if !strings.Contains(got, ">Drukuj &lt;PDF&gt;</span>") {
		t.Error("the label is escaped")
	}
}

// TestPrintRecipeCard: a recipe page gets "Print recipe" and an escaped,
// hidden card; labels are configurable.
func TestPrintRecipeCard(t *testing.T) {
	g, post := toolsGen(t, func(c *Config) {
		c.Print = PrintOptions{Enabled: true, Labels: map[string]string{"steps": "Sposób", "yield": " "}}
	})
	post.Schema = recipeSchema()
	got := g.pageToolsHTMLString(toolsPage, &post, true)
	for _, want := range []string{
		`data-print="recipe" hidden>`, `<span class="ssg-tool-label">Print recipe</span>`,
		`<section class="ssg-recipe-card" data-ssg-recipe-card hidden><h1>Tom &#34;&amp;&#34; Jerry &lt;3</h1>`,
		`<p class="ssg-recipe-meta">Makes: 2 · Prep: 10 min</p>`,
		`<h2>Ingredients</h2><ul><li>1 l water</li><li>a &#34;pinch&#34; of salt &lt;2g</li></ul>`,
		`<h2>Sposób</h2><ol><li>Boil.</li><li>Salt &amp; serve.</li></ol></section>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, cookScriptAttr) {
		t.Error("cook mode is off")
	}
	bare := g.recipeCardHTML(recipeData{Steps: []string{"Only."}})
	if bare != `<section class="ssg-recipe-card" data-ssg-recipe-card hidden><h2>Sposób</h2><ol><li>Only.</li></ol></section>` {
		t.Errorf("a card without a name, meta or ingredients = %s", bare)
	}
}

// TestCookMode: only a complete Recipe gets the button, with its data
// escaped into an attribute; the slot, auto and label rules match Print.
func TestCookMode(t *testing.T) {
	g, post := toolsGen(t, func(c *Config) { c.CookMode = CookModeOptions{Enabled: true} })
	if got := g.pageToolsHTMLString(toolsPage, &post, true); got != toolsPage {
		t.Error("no recipe, no button")
	}
	post.Schema = map[string]interface{}{"@type": "Recipe", "recipeIngredient": []interface{}{"x"}}
	if got := g.pageToolsHTMLString(toolsPage, &post, true); got != toolsPage {
		t.Error("a recipe without instructions cannot be cooked step by step")
	}
	post.Schema = recipeSchema()
	got := g.pageToolsHTMLString(toolsPage, &post, true)
	for _, want := range []string{
		"</h1>\n" + `<button type="button" class="ssg-cook" data-ssg-cook aria-haspopup="dialog" data-recipe="{&#34;name&#34;:&#34;Tom \&#34;\u0026\&#34; Jerry \u003c3&#34;`,
		`&#34;steps&#34;:[&#34;Boil.&#34;,&#34;Salt \u0026 serve.&#34;]}" hidden>`,
		`<span class="ssg-tool-label">Cook mode</span>`, "<style data-ssg-cook-style>", cookScriptAttr, pageToolsStyleAttr,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `data-labels="`) || strings.Contains(got, printScriptAttr) {
		t.Error("no labels configured, and print is off")
	}
	if again := g.pageToolsHTMLString(got, &post, true); again != got {
		t.Error("the button and script must not repeat")
	}
	g.config.CookMode = CookModeOptions{Enabled: true, NoAuto: true, Label: "Gotuj", Labels: map[string]string{"next": `"Dalej"`}}
	if got := g.pageToolsHTMLString(toolsPage, &post, true); got != toolsPage {
		t.Error("auto: false and no slot leaves the page alone")
	}
	got = g.pageToolsHTMLString("<h1>S</h1><p>"+cookSlot+"</p>", &post, true)
	if !strings.HasPrefix(got, `<h1>S</h1><p><button type="button" class="ssg-cook" data-ssg-cook`) ||
		!strings.Contains(got, `data-labels="{&#34;next&#34;:&#34;\&#34;Dalej\&#34;&#34;}"`) || !strings.Contains(got, ">Gotuj</span>") {
		t.Errorf("slot, labels and label:\n%s", got)
	}
}

// TestPageToolsOrder: Listen, Print, Cook mode read in that order after the
// title, all three placed automatically.
func TestPageToolsOrder(t *testing.T) {
	g, post := toolsGen(t, func(c *Config) {
		c.Print = PrintOptions{Enabled: true}
		c.CookMode = CookModeOptions{Enabled: true}
		c.Listen = ListenOptions{Enabled: true}
	})
	post.Schema = recipeSchema()
	got := g.listenHTMLString(g.pageToolsHTMLString(toolsPage, &post, true), &post)
	l, p, c := strings.Index(got, `class="ssg-listen"`), strings.Index(got, `class="ssg-print"`), strings.Index(got, `class="ssg-cook"`)
	if l < 0 || l >= p || p >= c {
		t.Errorf("order listen=%d print=%d cook=%d", l, p, c)
	}
	if strings.Count(got, "<style "+pageToolsStyleAttr) != 1 {
		t.Error("the shared button styles are added once")
	}
	if got := insertBeforeBody("<p>frag</p>", "<b>x</b>"); got != "<p>frag</p><b>x</b>" {
		t.Errorf("a fragment gets the assets at the end: %s", got)
	}
}

// The scripts are inlined into <script> and the styles into <style>: a
// closing tag inside either would end the element early.
func TestPageToolsAreInlineSafe(t *testing.T) {
	for name, src := range map[string]string{"print.js": printScript, "cookmode.js": cookScript} {
		if src == "" || strings.Contains(strings.ToLower(src), "</script") {
			t.Errorf("%s is empty or contains a closing script tag", name)
		}
	}
	for name, src := range map[string]string{"print.css": printStyle, "cookmode.css": cookStyle, "pagetools.css": pageToolsStyle} {
		if src == "" || strings.Contains(strings.ToLower(src), "</style") {
			t.Errorf("%s is empty or contains a closing style tag", name)
		}
	}
	if !strings.Contains(cookStyle, "prefers-reduced-motion") || !strings.Contains(cookStyle, "prefers-color-scheme: dark") ||
		!strings.Contains(pageToolsStyle, "min-height: 44px") || !strings.Contains(printStyle, "@media print") {
		t.Error("reduced motion, dark mode, 44px targets and the print media query must stay")
	}
}

// The scripts' pure helpers, run in Node where it exists.
func TestPageToolsRuntime(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	for name, src := range map[string]string{"print.cjs": printScript, "cook.cjs": cookScript} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := `
const assert = require('assert');
const p = require('./print.cjs'), c = require('./cook.cjs');
assert.strictEqual(p.cleanURL('https://example.com/a/?x=1#step'), 'https://example.com/a/?x=1');
assert.strictEqual(p.cleanURL('https://example.com/a/'), 'https://example.com/a/');
const d = new Date(Date.UTC(2026, 9, 12, 12));
assert.ok(/2026/.test(p.formatDate(d, 'en-GB')), p.formatDate(d, 'en-GB'));
assert.strictEqual(p.formatDate(d, 'not a language!'), '2026-10-12');
assert.strictEqual(p.footerText('https://example.com/a/#x', 'Printed', '12 October 2026'),
  'https://example.com/a/ — Printed 12 October 2026');
const r = {ingredients: ['a', 'b', 'c'], steps: ['one', 'two']};
const L = c.labelsWith({next: 'Dalej', back: ' ', bogus: 'x', step: 'Krok {n} z {total}'});
assert.strictEqual(L.next, 'Dalej'); assert.strictEqual(L.back, 'Back'); assert.ok(!('bogus' in L));
assert.strictEqual(c.labelsWith(null).close, 'Close');
assert.strictEqual(c.lastScreen(r), 3);
assert.deepStrictEqual([c.clampScreen(-2, r), c.clampScreen('2', r), c.clampScreen(9, r), c.clampScreen('x', r)], [0, 2, 3, 0]);
assert.deepStrictEqual([0, 1, 2, 3].map(i => c.progressText(i, r, L)), ['Ingredients', 'Krok 1 z 2', 'Krok 2 z 2', 'All done']);
assert.deepStrictEqual(c.parseState('{"screen":2,"checked":[0,2,2,7,-1,1.5,"1"]}', r), {screen: 2, checked: [0, 2]});
assert.deepStrictEqual(c.parseState('{"screen":40}', r), {screen: 3, checked: []});
assert.deepStrictEqual(c.parseState('not json', r), {screen: 0, checked: []});
assert.deepStrictEqual(c.parseState(null, r), {screen: 0, checked: []});
assert.strictEqual(c.storageKey('/soup/', 0), 'ssg-cook:/soup/');
assert.strictEqual(c.storageKey('/soup/', 2), 'ssg-cook:/soup/#2');
assert.deepStrictEqual([c.swipeDirection(-80, 10), c.swipeDirection(80, 10), c.swipeDirection(30, 0), c.swipeDirection(-80, 70)], [1, -1, 0, 0]);
assert.deepStrictEqual([c.tapDirection(300, 400), c.tapDirection(100, 400)], [1, -1]);
console.log('ok');
`
	cmd := exec.Command(node, "-e", check) // #nosec G204 -- test harness
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("node: %v\n%s", err, out)
	}
}
