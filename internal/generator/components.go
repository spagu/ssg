package generator

// Typed content components in the build (GO-093).
//
// The engine lives in internal/components; this is where it meets a site:
// loading the directory once, rendering calls inside the same content pipeline
// the legacy shortcodes run in, copying a component's assets only onto the
// pages that used it, and publishing the manifest that tells a person — or an
// agent — what content may call.

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/components"
)

// componentAssetPrefix is where a component's assets land in the output.
const componentAssetPrefix = "/components"

// componentMarkerAttr is stamped on a rendered component's first element so a
// finished page can say which components it contains, and is taken back out
// before the page ships.
//
// The alternative was to track usage per output path through a concurrent
// render, which is bookkeeping that has to stay correct forever. This is a
// fact the page carries about itself, read once and erased — the same shape as
// the editing attributes in GO-102.
const componentMarkerAttr = "data-ssg-component"

// componentMarkerRe matches a marker together with the space before it, so
// removing one restores the byte-for-byte original.
var componentMarkerRe = regexp.MustCompile(`\s` + componentMarkerAttr + `="[a-z0-9_-]+"`)

// firstTagRe finds the opening tag of a rendered component, where the marker
// goes.
var firstTagRe = regexp.MustCompile(`^\s*<([a-zA-Z][a-zA-Z0-9-]*)`)

// loadComponents reads the site's component directory. A site without one
// builds exactly as before: the set is nil and every method on it is a no-op.
func (g *Generator) loadComponents(funcs template.FuncMap) error {
	dir := g.config.ComponentsDir
	if dir == "" {
		dir = components.DefaultDir
	}
	set, warnings, err := components.Load(dir, funcs)
	if err != nil {
		return fmt.Errorf("loading components: %w", err)
	}
	for _, w := range warnings {
		fmt.Printf("   ⚠️  %s\n", w)
	}
	g.components = set
	if set.Len() > 0 && !g.config.Quiet {
		fmt.Printf("   🧩 Loaded %d component(s): %s\n", set.Len(), strings.Join(set.Names(), ", "))
	}
	return nil
}

// renderComponents replaces component calls in a page's content.
//
// wrap protects the output from the sanitizer, exactly as shortcode output is
// protected: the markup came from the site's own template, while the props that
// went into it came from content and were escaped by html/template on the way.
func (g *Generator) renderComponents(content string, wrap func(string) string) string {
	if g.components.Len() == 0 {
		return content
	}
	// The site's own setting, empty included: `shortcode_errors` means the same
	// thing here as it does for a shortcode, and empty is its historical drop.
	policy := g.config.ShortcodeErrors
	marked := func(name, html string) string {
		return wrap(markComponent(html, name))
	}
	out, res := g.components.RenderMarked(content, g.siteData, policy, "", marked)
	for _, w := range res.Warnings {
		g.warnComponent(w)
	}
	if res.Err != nil {
		g.noteComponentError(res.Err)
	}
	g.noteComponentsUsed(res.Used)
	return out
}

// markComponent stamps a component's output with its name.
//
// A component whose markup starts with text rather than an element gets no
// marker and therefore no per-page assets — documented, and the reason to give
// a component with a stylesheet an element to hang it on.
func markComponent(html, name string) string {
	m := firstTagRe.FindStringSubmatchIndex(html)
	if m == nil {
		return html
	}
	at := m[3] // just past the tag name
	return html[:at] + ` ` + componentMarkerAttr + `="` + name + `"` + html[at:]
}

// stripComponentMarkers removes the bookkeeping from a finished page.
func stripComponentMarkers(s string) string {
	if !strings.Contains(s, componentMarkerAttr) {
		return s
	}
	return componentMarkerRe.ReplaceAllString(s, "")
}

// warnComponent reports a call that did not resolve, once per distinct message:
// the same broken call in a page rendered for three languages is one mistake.
func (g *Generator) warnComponent(msg string) {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	if g.componentWarned == nil {
		g.componentWarned = map[string]bool{}
	}
	if g.componentWarned[msg] {
		return
	}
	g.componentWarned[msg] = true
	fmt.Printf("   ⚠️  %s\n", msg)
}

// noteComponentError records the first strict-mode failure.
func (g *Generator) noteComponentError(err error) {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	if g.componentErr == nil {
		g.componentErr = err
	}
}

// componentError returns the strict-mode failure, if any.
func (g *Generator) componentError() error {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	return g.componentErr
}

// noteComponentsUsed records which components the build rendered, so only their
// assets are copied.
func (g *Generator) noteComponentsUsed(names []string) {
	if len(names) == 0 {
		return
	}
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	if g.componentsUsed == nil {
		g.componentsUsed = map[string]bool{}
	}
	for _, name := range names {
		g.componentsUsed[name] = true
	}
}

// usedComponents lists what the build rendered, in a stable order.
func (g *Generator) usedComponents() []string {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	out := make([]string, 0, len(g.componentsUsed))
	for name := range g.componentsUsed {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// resetComponents clears the per-build record, so a watch rebuild reports this
// build's components rather than every one this process has rendered.
func (g *Generator) resetComponents() {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	g.componentsUsed = nil
	g.componentWarned = nil
	g.componentErr = nil
}

// writeComponentAssets copies the assets of every component the build used, and
// publishes the manifest.
//
// Only what was used: a site with twenty components and one in use ships one
// component's CSS, because the alternative is a stylesheet that grows with the
// library rather than with the page.
func (g *Generator) writeComponentAssets() error {
	if g.components.Len() == 0 {
		return nil
	}
	for _, name := range g.usedComponents() {
		c, ok := g.components.Get(name)
		if !ok || len(c.Assets) == 0 {
			continue
		}
		for _, rel := range c.Assets {
			src := filepath.Join(c.Dir, "assets", filepath.FromSlash(rel))
			dst := filepath.Join(g.config.OutputDir, "components", name, filepath.FromSlash(rel))
			// Every write the build makes goes through the same confinement,
			// including this one: the name is validated at load and the path
			// comes from a walk of the component's own directory, but a check
			// that only holds while two other things stay true is not a check.
			if err := g.ensureWithinOutput(dst); err != nil {
				return fmt.Errorf("component %q asset %s: %w", name, rel, err)
			}
			if err := copyComponentAsset(src, dst); err != nil {
				return fmt.Errorf("component %q asset %s: %w", name, rel, err)
			}
		}
	}
	return g.writeComponentManifest()
}

// writeComponentManifest publishes components.json — the contract a person or
// an agent reads before writing a call.
func (g *Generator) writeComponentManifest() error {
	data, err := g.components.EncodeManifest()
	if err != nil {
		return err
	}
	path := filepath.Join(g.config.OutputDir, components.ManifestFileName)
	if err := g.ensureWithinOutput(path); err != nil {
		return err
	}
	// #nosec G306,G703 -- a public build artifact under the confined output root
	return os.WriteFile(path, data, 0o644)
}

// copyComponentAsset copies one file, creating the directories it needs.
func copyComponentAsset(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	data, err := os.ReadFile(src) // #nosec G304 -- the project's own components directory
	if err != nil {
		return err
	}
	// #nosec G306,G703 -- a published asset under the confined output root
	return os.WriteFile(dst, data, 0o644)
}

// componentAssetTags is the markup a page needs for the components it used.
func (g *Generator) componentAssetTags(html string) string {
	if g.components.Len() == 0 || !strings.Contains(html, componentMarkerAttr) {
		return ""
	}
	var b strings.Builder
	for _, name := range g.components.Names() {
		if !strings.Contains(html, componentMarkerAttr+`="`+name+`"`) {
			continue
		}
		c, ok := g.components.Get(name)
		if !ok {
			continue
		}
		writeAssetTags(&b, html, name, c.StyleAssets(), `<link rel="stylesheet" href="%s">`)
		writeAssetTags(&b, html, name, c.ScriptAssets(), `<script src="%s" defer></script>`)
	}
	return b.String()
}

// writeAssetTags emits one tag per asset a page does not already carry. The
// check against the page is what keeps a theme that already links a component's
// stylesheet from getting a second copy of it.
func writeAssetTags(b *strings.Builder, html, name string, assets []string, tag string) {
	for _, asset := range assets {
		url := fmt.Sprintf("%s/%s/%s", componentAssetPrefix, name, asset)
		if strings.Contains(html, url) {
			continue
		}
		fmt.Fprintf(b, tag+"\n", url)
	}
}

// injectComponentAssets places a page's component assets in its head and then
// erases the markers that said which they were.
func (g *Generator) injectComponentAssets(s string) string {
	tags := g.componentAssetTags(s)
	if tags != "" {
		if i := strings.LastIndex(s, "</head>"); i >= 0 {
			s = s[:i] + tags + s[i:]
		} else {
			s = tags + s
		}
	}
	return stripComponentMarkers(s)
}

// componentsInContent names the components one page's source calls, in a
// stable order and without duplicates.
//
// It reads the content rather than the rendered HTML on purpose: the graph
// describes what a page IS, and a call that failed to resolve is still a call
// the author wrote — a reader asking "which pages use the pricing table" wants
// the page whose call is broken most of all.
func (g *Generator) componentsInContent(content string) []string {
	if g.components.Len() == 0 || !strings.Contains(content, "{{<") {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, call := range components.FindCalls(content) {
		if _, known := g.components.Get(call.Name); !known || seen[call.Name] {
			continue
		}
		seen[call.Name] = true
		out = append(out, call.Name)
	}
	sort.Strings(out)
	return out
}
