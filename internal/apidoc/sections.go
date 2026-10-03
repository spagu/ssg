package apidoc

import (
	"strings"
	"unicode"
)

// rawTag is one block tag as written: its name without "@" and its text,
// which runs until the next block tag and may span several lines.
type rawTag struct {
	name string
	text string
}

// splitSections separates the description (the text before the first block
// tag) from the block tags. A line starting with "@" inside a fenced code
// block is code, not a tag.
func splitSections(lines []string) (string, []rawTag) {
	var desc []string
	var tags []rawTag
	var cur []string // lines of the tag being read
	flush := func() {
		if len(tags) > 0 {
			tags[len(tags)-1].text = strings.Join(cur, "\n")
		}
	}
	var f fence
	for _, line := range lines {
		if !f.open() {
			if name, rest, ok := tagStart(line); ok {
				flush()
				tags = append(tags, rawTag{name: name})
				cur = []string{rest}
				continue
			}
		}
		f.update(line)
		if len(tags) == 0 {
			desc = append(desc, line)
		} else {
			cur = append(cur, line)
		}
	}
	flush()
	return strings.Join(desc, "\n"), tags
}

// tagStart recognises a block tag at the start of line: "@name rest".
func tagStart(line string) (name, rest string, ok bool) {
	t := strings.TrimLeft(line, " \t")
	if len(t) < 2 || t[0] != '@' || !unicode.IsLetter(rune(t[1])) {
		return "", "", false
	}
	n := 1
	for n < len(t) && isNameChar(t[n]) {
		n++
	}
	return t[1:n], strings.TrimLeft(t[n:], " \t"), true
}

// isNameChar reports whether c may appear in a block tag name.
func isNameChar(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// splitDescription splits a description into its first paragraph (the
// summary) and the remaining paragraphs (the body). A blank line inside a
// fenced code block does not end the summary.
func splitDescription(desc string) (summary, body string) {
	lines := strings.Split(trimBlankLines(desc), "\n")
	var f fence
	for i, line := range lines {
		f.update(line)
		if !f.open() && strings.TrimSpace(line) == "" {
			return strings.TrimSpace(strings.Join(lines[:i], "\n")),
				trimBlankLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), ""
}
