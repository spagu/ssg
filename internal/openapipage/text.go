package openapipage

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/openapi"
)

// writer accumulates one page's Markdown.
type writer struct {
	b    strings.Builder
	site *site
}

// line appends one formatted line. A strings.Builder cannot fail to write.
func (w *writer) line(format string, args ...any) {
	w.b.WriteString(fmt.Sprintf(format, args...) + "\n")
}

// String is the page so far.
func (w *writer) String() string { return w.b.String() }

// text writes a Markdown paragraph from the spec, as written.
func (w *writer) text(md string) {
	if md = strings.TrimSpace(md); md != "" {
		w.line("%s\n", md)
	}
}

// heading is raw HTML so the id is exactly the anchor other pages link to.
func (w *writer) heading(level int, id, title string) {
	w.line("<h%d id=\"%s\">%s</h%d>\n", level, id, title, level)
}

// Slug makes a name into an anchor or URL segment: lower case letters,
// digits and single hyphens.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "x"
	}
	return out
}

// schemaAnchor is a schema's id on the schemas page; the prefix keeps it
// apart from the page's other headings.
func schemaAnchor(name string) string { return "schema-" + Slug(name) }

func escape(s string) string { return html.EscapeString(s) }

// cell makes text safe inside a Markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(oneLine(s), "|", `\|`)
}

// oneLine joins a multi-line text into one line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// suffix is sep+s, or "" when s is empty.
func suffix(sep, s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return ""
	}
	return sep + s
}

// summary is the first non-empty text's first paragraph, for descriptions.
func summary(texts ...string) string {
	for _, t := range texts {
		if t = strings.TrimSpace(t); t != "" {
			first, _, _ := strings.Cut(t, "\n\n")
			return oneLine(first)
		}
	}
	return ""
}

func enumNote(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return " (one of `" + strings.Join(values, "`, `") + "`)"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// describeScheme says how a security scheme is used, in words.
func describeScheme(sc openapi.SecurityScheme) string {
	switch sc.Type {
	case "apiKey":
		return fmt.Sprintf("API key in the %s `%s`", sc.In, sc.ParamName)
	case "http":
		if strings.EqualFold(sc.Scheme, "basic") {
			return "HTTP Basic authentication"
		}
		return "HTTP " + sc.Scheme + " token" + suffix(" — ", sc.BearerFormat)
	case "oauth2":
		return "OAuth 2.0 bearer token"
	case "openIdConnect":
		return "OpenID Connect bearer token"
	}
	return sc.Type
}
