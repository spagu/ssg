package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
)

// docInfo is a parsed docblock plus what PHPDoc says that JSDoc does not.
type docInfo struct {
	*apidoc.Comment
	varType string // @var Type
}

// parseDoc parses a docblock. PHPDoc writes types before names and without
// braces ("@param int $n text", "@return Doc text"), which apidoc reads as
// JSDoc; the tag lines are rewritten into that form first. The text of a
// "@var Type text" becomes the summary when the docblock has none, an
// unfenced @example is fenced as PHP, and @throws keeps its type in front
// of its text.
func parseDoc(raw string) *docInfo {
	if strings.TrimSpace(raw) == "" {
		return &docInfo{Comment: &apidoc.Comment{Doc: &apimodel.Doc{}}}
	}
	norm, varType, varText := normalizeDoc(raw)
	info := &docInfo{Comment: apidoc.Parse(norm), varType: varType}
	if info.Doc.Summary == "" {
		info.Doc.Summary = varText
	}
	for i, ex := range info.Doc.Examples {
		info.Doc.Examples[i] = strings.Replace(ex, "```js\n", "```php\n", 1) // apidoc's default fence
	}
	info.Doc.Throws = nil
	for _, t := range info.Throws {
		if s := strings.TrimSpace(t.Type + " " + t.Text); s != "" {
			info.Doc.Throws = append(info.Doc.Throws, s)
		}
	}
	return info
}

// normalizeDoc rewrites PHPDoc tag lines into the JSDoc form apidoc.Parse
// reads, turns {@see X} into {@link X}, and takes "@var Type [$name] text"
// out of the comment.
func normalizeDoc(raw string) (norm, varType, varText string) {
	raw = strings.ReplaceAll(raw, "{@see ", "{@link ")
	lines := strings.Split(raw, "\n")
	out := lines[:0]
	for _, line := range lines {
		head, tag, body := splitTagLine(line)
		switch tag {
		case "param":
			line = head + "@param " + paramTag(body)
		case "return", "returns":
			line = head + "@returns " + typedTag(body)
		case "throws", "throw":
			line = head + "@throws " + typedTag(body)
		case "var":
			typ, rest := phpType(body)
			name, text := splitWord(rest)
			if !strings.HasPrefix(name, "$") {
				text = strings.TrimSpace(rest)
			}
			varType, varText = typ, strings.TrimSpace(text)
			continue
		}
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return strings.Join(out, "\n"), varType, varText
}

// splitTagLine splits a comment line into its gutter, its block tag name
// and the text after the tag. tag is "" for a line holding no tag.
func splitTagLine(line string) (head, tag, body string) {
	t := strings.TrimLeft(line, " \t")
	t = strings.TrimLeft(strings.TrimPrefix(t, "*"), " \t")
	if !strings.HasPrefix(t, "@") {
		return line, "", ""
	}
	head = line[:len(line)-len(t)]
	name, rest := splitWord(t[1:])
	return head, name, rest
}

// paramTag rewrites "Type $name text" (or "$name text", "Type ...$args",
// "Type &$ref") as "{Type} name text".
func paramTag(body string) string {
	typ, rest := phpType(body)
	name, text := splitWord(rest)
	name = strings.TrimPrefix(strings.TrimPrefix(name, "&"), "$")
	if strings.HasPrefix(name, "...") {
		name = "..." + strings.TrimPrefix(name[3:], "$")
	}
	return strings.TrimSpace(braced(typ) + name + " " + text)
}

// typedTag rewrites "Type text" as "{Type} text".
func typedTag(body string) string {
	typ, rest := phpType(body)
	return strings.TrimSpace(braced(typ) + rest)
}

// braced wraps a type in braces followed by a space, or returns "".
func braced(typ string) string {
	if typ == "" {
		return ""
	}
	return "{" + typ + "} "
}

// phpType reads the type at the start of a tag's text: everything up to
// the first space outside brackets ("array<string, int>" is one type). A
// text starting with a variable, "...", "&" or a JSDoc "{" has no type.
func phpType(body string) (typ, rest string) {
	body = strings.TrimLeft(body, " \t")
	if body == "" || strings.ContainsRune("{&.", rune(body[0])) ||
		(body[0] == '$' && !strings.HasPrefix(body, "$this")) {
		return "", body
	}
	depth := 0
	for i := 0; i < len(body); i++ {
		switch c := body[i]; c {
		case '<', '(', '[', '{':
			depth++
		case '>', ')', ']', '}':
			depth--
		case ' ', '\t', '\n':
			if depth <= 0 && !strings.HasSuffix(body[:i], ":") && !strings.HasSuffix(body[:i], ",") {
				return body[:i], strings.TrimLeft(body[i:], " \t")
			}
		}
	}
	return body, ""
}

// splitWord splits s into its first space-delimited word and the rest.
func splitWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t")
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimLeft(s[i:], " \t")
	}
	return s, ""
}

// paramDoc returns the @param text and type for a parameter name.
func (d *docInfo) param(name string) (text, typ string) {
	for _, p := range d.Params {
		if p.Name == name {
			return p.Text, p.Type
		}
	}
	return "", ""
}

// returnType returns the @return type, "" when none was written.
func (d *docInfo) returnType() string {
	if d.Returns == nil {
		return ""
	}
	return d.Returns.Type
}
