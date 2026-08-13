package generator

// Site identity — name, tagline, palette — and the CSS custom properties a
// theme can build on (#128).
//
// A migration already learns all three from the source site: WordPress knows
// what it calls itself, and the theme's own stylesheet declares its colours.
// Before this they stopped at metadata.json, so a freshly migrated project
// rendered as an untitled site in the default palette and the operator retyped
// what the export had already collected.
//
// Configuration always wins. The export fills in only what the config leaves
// empty, so editing `.ssg.yaml` is never undone by a re-run of the migration.

import (
	"fmt"
	stdhtml "html"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// paletteRoles is the order colours are emitted in, so the generated custom
// properties are stable between builds. Roles outside this list follow,
// alphabetically — a theme may name its own.
var paletteRoles = []string{"primary", "secondary", "accent", "text", "background", "muted", "link"}

// applySiteIdentity resolves the site's title, description and palette from
// configuration first and the export's metadata second.
func applySiteIdentity(site *models.SiteData, metadata models.Metadata, cfg Config) {
	site.Title = firstNonEmpty(cfg.Title, metadata.Site.Name)
	site.Description = firstNonEmpty(cfg.Description, metadata.Site.Description)

	if len(cfg.Colors) > 0 {
		site.Colors = cfg.Colors
		return
	}
	if len(metadata.Marketing.Colors) > 0 {
		site.Colors = metadata.Marketing.Colors
	}
}

// firstNonEmpty returns the first value that is not empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// buildPaletteHead renders the site palette as CSS custom properties on :root,
// so a theme can style against --ssg-color-primary without the author copying
// hex codes into its stylesheet. Emitted once per page, and skipped entirely
// when the theme already declares the same variables.
func (g *Generator) buildPaletteHead(existing string) string {
	colors := g.siteData.Colors
	if len(colors) == 0 || strings.Contains(existing, "--ssg-color-") {
		return ""
	}
	var b strings.Builder
	b.WriteString("<style>:root{")
	for _, role := range paletteOrder(colors) {
		value := strings.TrimSpace(colors[role])
		if value == "" || !safeCSSValue(value) {
			continue
		}
		fmt.Fprintf(&b, "--ssg-color-%s:%s;", cssIdent(role), value)
	}
	b.WriteString("}</style>\n")
	// Nothing survived the value check: emit no empty style block.
	if !strings.Contains(b.String(), "--ssg-color-") {
		return ""
	}
	// The browser UI colour, when neither the theme nor the export declared
	// one: the site's primary colour is the honest answer, and a migrated site
	// otherwise shows the browser's default chrome next to its own palette.
	if g.siteData.Marketing.ThemeColor == "" && !strings.Contains(existing, `name="theme-color"`) {
		if c := paletteMetaThemeColor(colors); c != "" {
			fmt.Fprintf(&b, `<meta name="theme-color" content="%s">`+"\n", c)
		}
	}
	return b.String()
}

// paletteOrder lists the known roles first, then any theme-specific ones in
// alphabetical order, so output is deterministic.
func paletteOrder(colors map[string]string) []string {
	var rest []string
	known := map[string]bool{}
	for _, role := range paletteRoles {
		known[role] = true
	}
	for role := range colors {
		if !known[role] {
			rest = append(rest, role)
		}
	}
	sort.Strings(rest)

	out := make([]string, 0, len(colors))
	for _, role := range paletteRoles {
		if _, ok := colors[role]; ok {
			out = append(out, role)
		}
	}
	return append(out, rest...)
}

// safeCSSValue rejects anything that could close the declaration or the style
// element. The palette comes from a crawl of somebody else's site, so it is
// untrusted input by construction.
func safeCSSValue(v string) bool {
	return !strings.ContainsAny(v, "{};<>\"'\\") && len(v) <= 64
}

// cssIdent reduces a role name to the characters valid in a custom property.
func cssIdent(role string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(role) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// paletteMetaThemeColor returns the palette's primary colour for
// <meta name="theme-color">, escaped, when the export found no explicit one.
func paletteMetaThemeColor(colors map[string]string) string {
	v := strings.TrimSpace(colors["primary"])
	if v == "" || !safeCSSValue(v) {
		return ""
	}
	return stdhtml.EscapeString(v)
}
