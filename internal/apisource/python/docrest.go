package python

import (
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// googleReturn matches "type: text" on the first line of a Google Returns
// section.
var googleReturn = regexp.MustCompile(`^([\w.]+(?:\[.*?\])?(?:\s*\|\s*[\w.]+(?:\[.*?\])?)*):\s+(.*)$`)

// returnsSection reads a Returns or Yields section.
func (info *docInfo) returnsSection(content []string, google bool) {
	if len(content) == 0 {
		return
	}
	var typ, text string
	switch its := items(content, false); {
	case google:
		text = strings.Join(content, "\n")
		if m := googleReturn.FindStringSubmatch(content[0]); m != nil {
			typ, text = m[1], strings.Join(append([]string{m[2]}, dedent(content[1:])...), "\n")
		}
		text = markup(strings.TrimSpace(text))
	case len(its) == 1:
		typ, text = firstNonEmpty(its[0].typ, its[0].names[0]), its[0].text
	default:
		var lines []string
		for _, it := range its {
			lines = append(lines, "- "+joinNonEmpty(": ", "`"+strings.Join(it.names, ", ")+"`", it.text))
		}
		text = strings.Join(lines, "\n")
	}
	info.returnType = firstNonEmpty(info.returnType, typ)
	info.doc.Returns = joinNonEmpty("\n\n", info.doc.Returns, text)
}

// seeAlso reads "name : text" or "a, b" lines into See entries; a line
// that names nothing is kept as text.
func (info *docInfo) seeAlso(content []string) {
	for _, it := range items(content, false) {
		desc := joinNonEmpty(" ", markup(strings.TrimSpace(it.typ)), it.text)
		for _, n := range it.names {
			name := strings.Trim(roleTarget.ReplaceAllString(n, "$1"), "`")
			if !refName.MatchString(name) {
				info.doc.See = append(info.doc.See, markup(joinNonEmpty(": ", n, desc)))
				continue
			}
			info.doc.See = append(info.doc.See, joinNonEmpty(": ", "{@link "+name+"}", desc))
		}
	}
}

// refName matches a name a See entry can link to.
var refName = regexp.MustCompile(`^[A-Za-z_][\w.]*$`)

// roleTarget unwraps ":role:`target`" to its target.
var roleTarget = regexp.MustCompile("^(?::\\w+)?:\\w+:`~?([^`]+)`$")

// addTag keeps a section the model has no field for.
func (info *docInfo) addTag(name, text string) {
	info.doc.Tags = append(info.doc.Tags, apimodel.Tag{Name: strings.ReplaceAll(name, " ", "-"), Text: text})
}

// restFieldRe matches a reST field: ":name arg: text".
var restFieldRe = regexp.MustCompile(`^:(\w+)(?:\s+([^:]+?))?:(?:\s+(.*))?$`)

// restField reads a reST/Sphinx field list entry starting at line i.
func (info *docInfo) restField(lines []string, i int) (int, bool) {
	m := restFieldRe.FindStringSubmatch(lines[i])
	if m == nil {
		return i, false
	}
	cont, next := block(lines, i+1, 0)
	field, arg := strings.ToLower(m[1]), strings.TrimSpace(m[2])
	text := markup(joinNonEmpty("\n", m[3], strings.TrimSpace(strings.Join(dedent(cont), "\n"))))
	switch field {
	case "param", "parameter", "arg", "argument", "key", "keyword", "ivar", "cvar", "var":
		list := &info.params
		if strings.HasSuffix(field, "var") {
			list = &info.attrs
		}
		typ, name := "", arg
		if k := strings.LastIndex(arg, " "); k >= 0 {
			typ, name = arg[:k], arg[k+1:]
		}
		p := param(list, name)
		p.typ, p.text = firstNonEmpty(typ, p.typ), text
	case "type":
		param(&info.params, arg).typ = text
	case "vartype":
		param(&info.attrs, arg).typ = text
	case "returns", "return", "yields", "yield":
		info.doc.Returns = joinNonEmpty("\n\n", info.doc.Returns, text)
	case "rtype", "ytype":
		info.returnType = text
	case "raises", "raise", "except", "exception":
		info.doc.Throws = append(info.doc.Throws, joinNonEmpty(": ", arg, text))
	default:
		info.addTag(field, joinNonEmpty(" ", arg, text))
	}
	return next, true
}

// directiveRe matches a reST directive: ".. name:: argument".
var directiveRe = regexp.MustCompile(`^\.\.\s+([\w-]+)::\s*(.*)$`)

// admonitions are the directives shown as a labelled paragraph.
var admonitions = map[string]string{
	"note": "Note", "warning": "Warning", "tip": "Tip", "important": "Important",
	"attention": "Attention", "caution": "Caution", "danger": "Danger", "hint": "Hint",
}

// directive reads a version or admonition directive starting at line i.
// Code directives are left to prose.
func (info *docInfo) directive(lines []string, i int) (int, bool) {
	m := directiveRe.FindStringSubmatch(lines[i])
	if m == nil || indentOf(lines[i]) != 0 {
		return i, false
	}
	cont, next := block(lines, i+1, 0)
	name, arg := strings.ToLower(m[1]), strings.TrimSpace(m[2])
	text := markup(strings.TrimSpace(strings.Join(dedent(cont), "\n")))
	switch {
	case name == "deprecated":
		d := text
		if arg != "" {
			d = strings.TrimSpace("Since " + arg + ". " + text)
		}
		info.doc.Deprecated = &d
	case name == "versionadded":
		info.doc.Since = arg
	case name == "versionchanged":
		info.addTag(name, joinNonEmpty(" ", arg, text))
	case name == "seealso":
		info.seeAlso(append([]string{arg}, dedent(cont)...))
	case admonitions[name] != "":
		info.notes = append(info.notes, "**"+admonitions[name]+":** "+markup(joinNonEmpty(" ", arg, text)))
	default:
		return i, false
	}
	return next, true
}

// joinNonEmpty joins the non-empty parts with sep.
func joinNonEmpty(sep string, parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, sep)
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
