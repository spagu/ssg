package python

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// docParam is one documented parameter or attribute.
type docParam struct {
	name string // without "*" or "**"
	typ  string // type from the docstring, used when there is no annotation
	text string // Markdown
}

// docInfo is what a docstring says, before it is attached to a symbol.
type docInfo struct {
	doc        apimodel.Doc
	params     []*docParam
	attrs      []*docParam
	returnType string
	notes      []string // admonitions, appended to the body
}

// param returns the entry for a parameter name in list, adding it if new.
func param(list *[]*docParam, name string) *docParam {
	name = strings.TrimLeft(strings.TrimSpace(name), "*")
	for _, p := range *list {
		if p.name == name {
			return p
		}
	}
	p := &docParam{name: name}
	*list = append(*list, p)
	return p
}

// lookup returns the entry for name in list, or nil.
func lookup(list []*docParam, name string) *docParam {
	for _, p := range list {
		if p.name == name {
			return p
		}
	}
	return nil
}

// parseDocstring reads a docstring in Google, NumPy or reST style (or
// plain prose) into a docInfo. Styles may be mixed: each line is tried as
// a section header, a field or a directive before it counts as prose.
func parseDocstring(raw string) *docInfo {
	info := &docInfo{}
	lines := dedentDoc(raw)
	var free []string
	for i := 0; i < len(lines); {
		if n, ok := info.numpySection(lines, i); ok {
			i = n
			continue
		}
		if n, ok := info.googleSection(lines, i); ok {
			i = n
			continue
		}
		if n, ok := info.restField(lines, i); ok {
			i = n
			continue
		}
		if n, ok := info.directive(lines, i); ok {
			i = n
			continue
		}
		free = append(free, lines[i])
		i++
	}
	info.prose(free)
	return info
}

// dedentDoc splits a docstring into lines and removes the common
// indentation of all lines but the first (PEP 257), with leading and
// trailing blank lines.
func dedentDoc(raw string) []string {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(expandTabs(raw), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	lines[0] = strings.TrimSpace(lines[0])
	lines = append(lines[:1:1], dedent(lines[1:])...)
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// dedent removes the common leading spaces of non-blank lines.
func dedent(lines []string) []string {
	least := -1
	for _, l := range lines {
		if strings.TrimSpace(l) != "" && (least < 0 || indentOf(l) < least) {
			least = indentOf(l)
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if least > 0 && len(l) >= least {
			l = l[least:]
		}
		out[i] = l
	}
	return out
}

// indentOf counts a line's leading spaces.
func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

// expandTabs replaces tabs with spaces to the next multiple of eight.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := tabSize - col%tabSize
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case '\n':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

// emptyDoc reports whether a Doc carries nothing.
func emptyDoc(d *apimodel.Doc) bool {
	return d.Summary == "" && d.Body == "" && d.Returns == "" && len(d.Throws) == 0 &&
		len(d.Examples) == 0 && d.Deprecated == nil && d.Since == "" && len(d.See) == 0 &&
		d.Default == "" && len(d.Tags) == 0
}

// block returns the lines after index i that belong to it: blank lines and
// lines indented deeper than base, trailing blanks dropped. It returns
// them and the index after the block.
func block(lines []string, i, base int) ([]string, int) {
	j := i
	for j < len(lines) && (lines[j] == "" || indentOf(lines[j]) > base) {
		j++
	}
	end := j
	for end > i && lines[end-1] == "" {
		end--
	}
	return lines[i:end], j
}
