// pretty_html: tidies generated HTML for reading while leaving the content
// of whitespace-sensitive elements untouched (#438).

package generator

import "strings"

// prettifyHTMLString removes blank lines and trailing whitespace, normalising
// line endings; output always ends with a single newline.
//
// <pre>, <textarea>, <script> and <style> are copied byte for byte (#438):
// HTML renders the whitespace of the first two, and a blank line inside a
// script's template literal is part of the string. They are the elements
// minify already protects, so both passes share minPreserveTagRe. An element
// without its closing tag does not match and is tidied like any other markup,
// so a stray <pre> cannot swallow the rest of the page.
func prettifyHTMLString(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	line := ""
	last := 0
	for _, loc := range minPreserveTagRe.FindAllStringIndex(s, -1) {
		line = prettifyGap(&out, line, s[last:loc[0]])
		// The element stays on the line it opened on, so the markup before
		// it (indentation, an opening <div>) is kept with it.
		line += s[loc[0]:loc[1]]
		last = loc[1]
	}
	line = prettifyGap(&out, line, s[last:])
	writePrettyLine(&out, line)
	if out.Len() == 0 {
		return "\n"
	}
	return out.String()
}

// prettifyGap tidies markup that sits outside every preserved element. line is
// the unfinished line the gap continues; the gap's own unfinished last line is
// returned for the next element or gap to continue.
func prettifyGap(out *strings.Builder, line, gap string) string {
	gap = strings.ReplaceAll(gap, "\r\n", "\n")
	gap = strings.ReplaceAll(gap, "\r", "\n")
	parts := strings.Split(gap, "\n")
	line += parts[0]
	for _, part := range parts[1:] {
		writePrettyLine(out, line)
		line = part
	}
	return line
}

// writePrettyLine writes a finished line without its trailing whitespace, and
// drops it when nothing but whitespace is left.
func writePrettyLine(out *strings.Builder, line string) {
	trimmed := strings.TrimRight(line, " \t")
	if strings.TrimSpace(trimmed) == "" {
		return
	}
	out.WriteString(trimmed)
	out.WriteByte('\n')
}
