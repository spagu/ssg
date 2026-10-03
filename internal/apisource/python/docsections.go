package python

import (
	"regexp"
	"strings"
)

// sectionKinds maps a Google or NumPy section header, in lower case, to
// what it holds.
var sectionKinds = map[string]string{
	"args": "params", "arguments": "params", "parameters": "params", "params": "params",
	"keyword args": "params", "keyword arguments": "params", "other parameters": "params",
	"returns": "returns", "return": "returns", "yields": "returns", "yield": "returns",
	"raises": "raises", "raise": "raises", "exceptions": "raises",
	"example": "examples", "examples": "examples",
	"note": "note", "notes": "note", "warning": "note", "warnings": "note", "tip": "note",
	"see also": "see", "deprecated": "deprecated", "attributes": "attrs",
	"todo": "tag", "references": "tag", "methods": "tag", "warns": "tag",
}

// googleSection reads a Google-style section ("Args:" and an indented
// block) starting at line i.
func (info *docInfo) googleSection(lines []string, i int) (int, bool) {
	l := lines[i]
	if indentOf(l) != 0 || !strings.HasSuffix(l, ":") {
		return i, false
	}
	header := strings.TrimSuffix(l, ":")
	kind, ok := sectionKinds[strings.ToLower(header)]
	if !ok {
		return i, false
	}
	content, next := block(lines, i+1, 0)
	info.section(kind, header, dedent(content), true)
	return next, true
}

// numpySection reads a NumPy-style section (a header underlined with
// dashes, up to the next such header) starting at line i.
func (info *docInfo) numpySection(lines []string, i int) (int, bool) {
	if !isNumpyHeader(lines, i) {
		return i, false
	}
	j := i + 2
	for j < len(lines) && !isNumpyHeader(lines, j) {
		j++
	}
	end := j
	for end > i+2 && lines[end-1] == "" {
		end--
	}
	info.section(sectionKinds[strings.ToLower(lines[i])], lines[i], lines[i+2:end], false)
	return j, true
}

// isNumpyHeader reports whether line i is a known header underlined with
// at least three dashes.
func isNumpyHeader(lines []string, i int) bool {
	if i+1 >= len(lines) || indentOf(lines[i]) != 0 {
		return false
	}
	under := strings.TrimSpace(lines[i+1])
	_, known := sectionKinds[strings.ToLower(lines[i])]
	return known && len(under) >= 3 && strings.Trim(under, "-") == ""
}

// section files the content of one section by its kind.
func (info *docInfo) section(kind, header string, content []string, google bool) {
	text := markup(strings.Join(content, "\n"))
	switch kind {
	case "params", "attrs":
		list := &info.params
		if kind == "attrs" {
			list = &info.attrs
		}
		for _, it := range items(content, google) {
			for _, n := range it.names {
				p := param(list, n)
				p.typ, p.text = firstNonEmpty(it.typ, p.typ), it.text
			}
		}
	case "returns":
		info.returnsSection(content, google)
	case "raises":
		for _, it := range items(content, google) {
			info.doc.Throws = append(info.doc.Throws, joinNonEmpty(": ", strings.Join(it.names, ", "), it.text))
		}
	case "examples":
		info.doc.Examples = append(info.doc.Examples, exampleMarkdown(content))
	case "note":
		info.notes = append(info.notes, "**"+header+":** "+text)
	case "see":
		info.seeAlso(content)
	case "deprecated":
		info.doc.Deprecated = &text
	default:
		info.addTag(strings.ToLower(header), text)
	}
}

// item is one entry of a parameter-like section.
type item struct {
	names []string
	typ   string
	text  string
}

// googleItem matches "name (type): text" and "name: text".
var googleItem = regexp.MustCompile(`^(\*{0,2}[A-Za-z_][\w.]*)\s*(?:\((.*?)\))?\s*:\s*(.*)$`)

// items splits a section into entries: an entry starts at an unindented
// line; deeper lines continue it.
func items(content []string, google bool) []item {
	var out []item
	for i := 0; i < len(content); {
		head := content[i]
		if head == "" || indentOf(head) > 0 {
			i++
			continue
		}
		body, next := block(content, i+1, 0)
		i = next
		desc := markup(strings.TrimSpace(strings.Join(dedent(body), "\n")))
		it := item{names: []string{head}, text: desc}
		if m := googleItem.FindStringSubmatch(head); google && m != nil {
			it = item{names: []string{m[1]}, typ: cleanType(m[2]), text: joinNonEmpty("\n", markup(m[3]), desc)}
		} else if !google {
			name, typ := numpyHead(head)
			it.names, it.typ = strings.Split(name, ","), cleanType(typ)
		}
		for k, n := range it.names {
			it.names[k] = strings.TrimSpace(n)
		}
		out = append(out, it)
	}
	return out
}

// cleanType drops the "optional" and "default ..." notes from a docstring
// type ("int, optional" → "int").
func cleanType(t string) string {
	var keep []string
	for _, part := range splitText(t, ',') {
		p := strings.ToLower(strings.TrimSpace(part))
		if p != "" && p != "optional" && !strings.HasPrefix(p, "default") {
			keep = append(keep, strings.TrimSpace(part))
		}
	}
	return strings.Join(keep, ", ")
}

// splitText splits s at sep outside brackets.
func splitText(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// numpyHead splits a NumPy entry head "name : type" (the spaces are the
// convention; "name:type" is accepted when the head does not start with
// a role such as ":func:").
func numpyHead(head string) (name, typ string) {
	if name, typ, ok := strings.Cut(head, " : "); ok {
		return name, typ
	}
	if name, typ, ok := strings.Cut(head, ":"); ok && name != "" {
		return name, typ
	}
	return head, ""
}
