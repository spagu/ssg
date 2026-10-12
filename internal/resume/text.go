package resume

import "strings"

// markdownEscapes are the characters that turn prose into markup.
var markdownEscapes = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
	"<", "&lt;", ">", "&gt;", "#", `\#`, "|", `\|`, "~", `\~`,
)

// Escape renders plain text as itself in Markdown: a "C#" or a "<b>" in a
// CV is prose, not markup.
func Escape(s string) string {
	lines := strings.Split(markdownEscapes.Replace(strings.TrimSpace(s)), "\n")
	for i, l := range lines {
		lines[i] = escapeLineStart(l)
	}
	return strings.Join(lines, "\n")
}

// escapeLineStart keeps a line that opens like a list item or a setext
// underline ("- ", "+ ", "1. ", "===") from becoming one.
func escapeLineStart(l string) string {
	body := strings.TrimLeft(l, " \t")
	indent := l[:len(l)-len(body)]
	if body == "" {
		return l
	}
	switch body[0] {
	case '-', '+', '=':
		return indent + `\` + body
	}
	digits := len(body) - len(strings.TrimLeft(body, "0123456789"))
	if digits > 0 && digits < len(body) && (body[digits] == '.' || body[digits] == ')') {
		return indent + body[:digits] + `\` + body[digits:]
	}
	return l
}

// urlEscapes keep a link target inside its parentheses.
var urlEscapes = strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E")

// Link is text linked to href when href is a web URL, and the text alone
// otherwise: a javascript: or relative link never reaches the page.
func Link(text, href string) string {
	t := Escape(text)
	if t == "" {
		t = Escape(href)
	}
	if !WebURL(href) {
		return t
	}
	return "[" + t + "](" + urlEscapes.Replace(href) + ")"
}

// Span is a date range as written: "2019-06 – present", "2015 – 2019".
func Span(start, end Date) string {
	switch {
	case start == "" && end == "":
		return ""
	case start == "":
		return string(end)
	case end == "":
		return string(start) + " – present"
	}
	return string(start) + " – " + string(end)
}
