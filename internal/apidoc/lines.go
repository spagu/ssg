package apidoc

import "strings"

// commentLines turns the raw text of a comment into lines without the
// leading " * " gutter. Line endings are normalised to "\n" and trailing
// whitespace is dropped. Lines written without a gutter lose their common
// indentation, so code inside fenced blocks keeps its relative indentation.
func commentLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	var bare []int // indexes of lines written without a gutter
	for i, line := range lines {
		line = strings.TrimRight(line, " \t")
		if i == 0 {
			line = strings.TrimLeft(line, " \t")
		} else if stripped, ok := stripGutter(line); ok {
			line = stripped
		} else {
			bare = append(bare, i)
		}
		lines[i] = line
	}
	dedent(lines, bare)
	return lines
}

// stripGutter removes the leading whitespace, the "*" and one following
// space or tab of a gutter line. ok is false when the line has no gutter.
func stripGutter(line string) (string, bool) {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, "*") {
		return line, false
	}
	if len(t) == 1 {
		return "", true
	}
	switch t[1] {
	case ' ', '\t':
		return t[2:], true
	case '@':
		return t[1:], true
	}
	return line, false
}

// dedent removes from the lines at idx the indentation they all share.
func dedent(lines []string, idx []int) {
	common := -1
	for _, i := range idx {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		n := len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	for _, i := range idx {
		if len(lines[i]) >= common && common > 0 {
			lines[i] = lines[i][common:]
		}
	}
}

// trimBlankLines drops blank lines at both ends of s but keeps the
// indentation of the first remaining line (it may be code).
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), " \t")
}

// fence tracks whether a sequence of lines is inside a fenced code block
// (``` or ~~~). The zero value is outside any block.
type fence struct {
	char byte // '`' or '~' of the open block, 0 when closed
	size int  // length of the opening run
}

// open reports whether the tracker is inside a fenced block.
func (f *fence) open() bool { return f.char != 0 }

// update feeds one line to the tracker and reports whether the line is a
// fence delimiter (opening or closing).
func (f *fence) update(line string) bool {
	char, size, info := fenceMarker(line)
	if size == 0 {
		return false
	}
	if !f.open() {
		f.char, f.size = char, size
		return true
	}
	if char == f.char && size >= f.size && info == "" {
		f.char, f.size = 0, 0
		return true
	}
	return false
}

// fenceMarker returns the delimiter character, the run length (0 when the
// line is no delimiter) and the trimmed text after the run.
func fenceMarker(line string) (byte, int, string) {
	t := strings.TrimLeft(line, " \t")
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return 0, 0, ""
	}
	n := 0
	for n < len(t) && t[n] == t[0] {
		n++
	}
	if n < 3 {
		return 0, 0, ""
	}
	return t[0], n, strings.TrimSpace(t[n:])
}

// hasFence reports whether s contains a fenced code block delimiter.
func hasFence(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if _, n, _ := fenceMarker(line); n > 0 {
			return true
		}
	}
	return false
}

// dedentTail removes the indentation shared by the lines after the first:
// the continuation lines of a tag whose first line follows the tag name.
func dedentTail(s string) string {
	lines := strings.Split(s, "\n")
	idx := make([]int, 0, len(lines))
	for i := 1; i < len(lines); i++ {
		idx = append(idx, i)
	}
	dedent(lines, idx)
	return strings.Join(lines, "\n")
}
