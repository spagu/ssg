package apidoc

import "strings"

// formatExample turns the text of an @example tag into Markdown. Text
// without a fenced code block is wrapped in a ```js block; a leading
// "<caption>…</caption>" becomes an emphasized line before the code.
// Indentation is preserved. Empty text yields "".
func formatExample(text string) string {
	caption, code := splitCaption(trimBlankLines(text))
	if code != "" && !hasFence(code) {
		code = "```js\n" + code + "\n```"
	}
	switch {
	case caption == "":
		return code
	case code == "":
		return "*" + caption + "*"
	}
	return "*" + caption + "*\n\n" + code
}

// splitCaption separates an optional leading <caption>…</caption> from the
// example code that follows it.
func splitCaption(text string) (caption, code string) {
	const open, closing = "<caption>", "</caption>"
	t := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(t, open) {
		return "", text
	}
	end := strings.Index(t, closing)
	if end < 0 {
		return "", text
	}
	rest := strings.TrimLeft(t[end+len(closing):], " \t")
	return strings.TrimSpace(t[len(open):end]), trimBlankLines(rest)
}
