package admintheme

import (
	"strings"

	"github.com/spagu/ssg/internal/modules"
)

// CSS writes /core/theme.css for a computed theme: @font-face blocks, then
// the custom properties in the same three places tokens.css sets them — the
// light scheme on :root, the dark scheme under prefers-color-scheme (unless
// the page chose light) and under data-theme="dark" (the page's own switch).
// Loaded after a panel's tokens.css, each rule wins over its twin there by
// order alone (same selector, same specificity).
//
// Only validated values reach this writer, and it emits nothing but
// `--token: value;` lines and @font-face blocks built from their parts: token
// names come from Themed, colours are #rrggbb, font stacks were rebuilt by
// Stack, and paths passed modules.SiteFilePath, so no value can close a
// declaration, a block or a string. It returns nil when the theme sets no
// colour and no font: the shipped empty theme.css then stays as it is.
func CSS(t *modules.Theme) []byte {
	if t == nil || (len(t.Light) == 0 && t.FontDisplay == nil && t.FontUI == nil && len(t.FontFiles) == 0) {
		return nil
	}
	var b strings.Builder
	b.WriteString("/* The site's own look for the admin panels (GO-152), written by ssg from\n" +
		" * admin_theme: in the site's configuration. Do not edit: the next build\n" +
		" * replaces it. Change the configuration and rebuild instead. */\n")
	for _, f := range t.FontFiles {
		b.WriteString("\n@font-face {\n")
		decl(&b, "  ", "font-family", `"`+f.Family+`"`)
		decl(&b, "  ", "src", `url("`+f.Src+`") format("`+f.Format+`")`)
		decl(&b, "  ", "font-weight", f.Weight)
		decl(&b, "  ", "font-style", f.Style)
		decl(&b, "  ", "font-display", "swap")
		b.WriteString("}\n")
	}
	b.WriteString("\n:root {\n")
	if t.FontDisplay != nil {
		decl(&b, "  ", "--font-display", *t.FontDisplay)
	}
	if t.FontUI != nil {
		decl(&b, "  ", "--font-ui", *t.FontUI)
	}
	tokens(&b, "  ", t.Light)
	b.WriteString("}\n")
	if len(t.Dark) > 0 {
		b.WriteString("\n@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) {\n")
		tokens(&b, "    ", t.Dark)
		b.WriteString("  }\n}\n\n:root[data-theme=\"dark\"] {\n")
		tokens(&b, "  ", t.Dark)
		b.WriteString("}\n")
	}
	return []byte(b.String())
}

func decl(b *strings.Builder, indent, name, value string) {
	b.WriteString(indent + name + ": " + value + ";\n")
}

// tokens writes one scheme in Themed order (not map order), so the file is
// the same bytes every build.
func tokens(b *strings.Builder, indent string, scheme map[string]string) {
	for _, k := range Themed {
		if v, ok := scheme[k]; ok {
			decl(b, indent, "--"+k, v)
		}
	}
}
