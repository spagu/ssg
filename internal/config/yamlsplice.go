package config

// Editing a config as text, guided by the parsed document (GO-101).
//
// Re-encoding a yaml.Node keeps comments and key order, which was enough while
// the only callers were a migration filling in blanks and a designer tool
// setting one flag. It is not enough for a person editing their own file: the
// encoder drops every blank line between sections and re-aligns trailing
// comments, so changing one setting rewrote three hundred lines of this
// project's own config and flattened the spacing that made it readable.
//
// So the document is parsed to find WHERE a value lives, and the change is
// spliced into the original text. One setting changed is one line changed;
// every other byte of the file is the byte it was.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// spliceSet replaces the value of an existing key, keeping the key's own text
// and its trailing comment.
func spliceSet(src []byte, keyNode, valNode *yaml.Node, value interface{}) ([]byte, error) {
	lines, trailing := splitLines(src)
	keyLine := keyNode.Line - 1
	if keyLine < 0 || keyLine >= len(lines) {
		return nil, fmt.Errorf("the file does not have line %d", keyNode.Line)
	}
	indent := indentOf(lines[keyLine])
	inline, block, err := renderValue(value, indent)
	if err != nil {
		return nil, err
	}
	head, comment, commentCol := keyLineParts(lines[keyLine], keyNode, valNode)
	rebuilt := head
	if inline != "" {
		rebuilt += " " + inline
	}
	if comment != "" {
		// Put the comment back where it was if the new value leaves room, so a
		// file whose comments line up in a column still does.
		if pad := commentCol - len(rebuilt); pad > 0 {
			rebuilt += strings.Repeat(" ", pad)
		} else {
			rebuilt += " "
		}
		rebuilt += comment
	}
	end := blockEnd(lines, keyLine, indent, true)
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:keyLine]...)
	out = append(out, rebuilt)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return join(out, trailing), nil
}

// spliceSetSeqEntry replaces one entry of a sequence, keeping its "-" marker.
func spliceSetSeqEntry(src []byte, entry *yaml.Node, value interface{}) ([]byte, error) {
	lines, trailing := splitLines(src)
	line := entry.Line - 1
	if line < 0 || line >= len(lines) {
		return nil, fmt.Errorf("the file does not have line %d", entry.Line)
	}
	inline, block, err := renderValue(value, indentOf(lines[line]))
	if err != nil {
		return nil, err
	}
	if inline == "" {
		// A list entry that is itself a mapping is written under its marker,
		// which the block form above does not know how to indent; refuse
		// rather than write something subtly wrong.
		_ = block
		return nil, fmt.Errorf("line %d: set a list entry to a scalar, or set the whole list", entry.Line)
	}
	text := inline
	dash := strings.Index(lines[line], "- ")
	if dash < 0 {
		// A flow sequence ([a, b, c]) has no marker to keep: rewrite the line
		// from the key it belongs to instead.
		return nil, fmt.Errorf("line %d is a flow sequence — set the whole list instead", entry.Line)
	}
	indent := indentOf(lines[line])
	end := blockEnd(lines, line, indent, false)
	out := make([]string, 0, len(lines))
	out = append(out, lines[:line]...)
	out = append(out, lines[line][:dash+2]+text)
	out = append(out, lines[end:]...)
	return join(out, trailing), nil
}

// spliceRemove deletes a key (with its value block) or a sequence entry. For a
// key, the comment block written directly above it goes too: it documents the
// setting that is going away.
func spliceRemove(src []byte, node *yaml.Node, isKey bool) ([]byte, error) {
	lines, trailing := splitLines(src)
	line := node.Line - 1
	if line < 0 || line >= len(lines) {
		return nil, fmt.Errorf("the file does not have line %d", node.Line)
	}
	start := line
	if isKey && node.HeadComment != "" {
		for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#") {
			start--
		}
	}
	end := blockEnd(lines, line, indentOf(lines[line]), isKey)
	out := append(append([]string{}, lines[:start]...), lines[end:]...)
	return join(out, trailing), nil
}

// insertNested writes segs as a nested block inside an existing mapping. The
// first segment may be new, and so may every one after it — a path is written
// whole rather than requiring its parents to be typed in by hand first.
func insertNested(src []byte, container, containerKey *yaml.Node, segs []pathSeg, value interface{}) ([]byte, error) {
	for _, s := range segs {
		if s.isIndex {
			return nil, fmt.Errorf("%s: a list entry must already exist", pathString(segs))
		}
	}
	lines, trailing := splitLines(src)
	at, indent, err := insertPoint(lines, container, containerKey)
	if err != nil {
		return nil, err
	}
	deepest := len(indent) + 2*(len(segs)-1)
	inline, valueBlock, err := renderValue(value, deepest)
	if err != nil {
		return nil, err
	}
	block := make([]string, 0, len(segs)+len(valueBlock))
	for i, s := range segs[:len(segs)-1] {
		block = append(block, indent+strings.Repeat("  ", i)+quoteKey(s.key)+":")
	}
	lastLine := indent + strings.Repeat("  ", len(segs)-1) + quoteKey(segs[len(segs)-1].key) + ":"
	if inline != "" {
		lastLine += " " + inline
	}
	block = append(block, lastLine)
	block = append(block, valueBlock...)

	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:at]...)
	out = append(out, block...)
	out = append(out, lines[at:]...)
	return join(out, trailing), nil
}

// insertPoint finds where a new entry of a mapping goes, and how far it is
// indented: after the last entry already there, aligned with it. An empty
// mapping falls back to two spaces past the key that owns it.
func insertPoint(lines []string, container, containerKey *yaml.Node) (int, string, error) {
	if len(container.Content) >= 2 {
		lastKey := container.Content[len(container.Content)-2]
		line := lastKey.Line - 1
		if line < 0 || line >= len(lines) {
			return 0, "", fmt.Errorf("the file does not have line %d", lastKey.Line)
		}
		indent := indentOf(lines[line])
		return blockEnd(lines, line, indent, true), strings.Repeat(" ", indent), nil
	}
	if containerKey == nil {
		return lastContentLine(lines) + 1, "", nil
	}
	line := containerKey.Line - 1
	if line < 0 || line >= len(lines) {
		return 0, "", fmt.Errorf("the file does not have line %d", containerKey.Line)
	}
	return line + 1, strings.Repeat(" ", indentOf(lines[line])+2), nil
}

// quoteKey wraps a key that YAML would otherwise misread.
func quoteKey(key string) string {
	if key == "" || strings.ContainsAny(key, ` :#{}[]&*!|>'"%@,`) {
		return `"` + strings.ReplaceAll(key, `"`, `\"`) + `"`
	}
	return key
}

// renderValue encodes a value for the position it is going into.
//
// A scalar comes back as inline text and sits on the key's line, so setting one
// setting changes exactly one line. A mapping or a list comes back as the block
// form on the lines below, indented past the key — because that is how a person
// writes them, and a config is read far more often than it is set.
func renderValue(value interface{}, indent int) (inline string, block []string, err error) {
	node, err := encodeNode(value)
	if err != nil {
		return "", nil, err
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return "", nil, err
	}
	if err := enc.Close(); err != nil {
		return "", nil, err
	}
	text := strings.TrimRight(b.String(), "\n")
	if node.Kind == yaml.ScalarNode {
		return text, nil, nil
	}
	if text == "{}" || text == "[]" { // an empty collection reads better inline
		return text, nil, nil
	}
	pad := strings.Repeat(" ", indent+2)
	for _, l := range strings.Split(text, "\n") {
		block = append(block, pad+l)
	}
	return "", block, nil
}

// keyLineParts splits a key's line into the text up to and including its colon,
// and the trailing comment, so a documented setting keeps its documentation.
func keyLineParts(line string, keyNode, valNode *yaml.Node) (head, comment string, commentCol int) {
	colon := keyColonIndex(line, keyNode)
	head = strings.TrimRight(line[:colon+1], " \t")
	if valNode == nil || valNode.Line != keyNode.Line {
		return head, "", 0
	}
	if valNode.LineComment == "" && keyNode.LineComment == "" {
		return head, "", 0
	}
	if i := strings.LastIndex(line, "#"); i > colon {
		return head, strings.TrimSpace(line[i:]), i
	}
	return head, "", 0
}

// keyColonIndex finds the colon that ends the key on its line, starting from
// the key's own column so a colon earlier in the line cannot fool it.
func keyColonIndex(line string, keyNode *yaml.Node) int {
	from := keyNode.Column - 1 + len(keyNode.Value)
	if from < 0 || from > len(line) {
		from = 0
	}
	if i := strings.Index(line[from:], ":"); i >= 0 {
		return from + i
	}
	return len(line) - 1
}

// blockEnd returns the line index just past the value block that starts on the
// given line: the lines below it that are indented further.
//
// takeDashes decides what a same-indent "- " line means. Under a KEY it is that
// key's list, written flush with it as YAML allows, and belongs to the block.
// Beside a list ENTRY it is the next entry, and does not — which is the whole
// difference between removing one entry and removing the list.
//
// Trailing blank and comment lines are left alone: the blank line separates
// sections, and a comment above a key belongs to that key.
func blockEnd(lines []string, startLine, keyIndent int, takeDashes bool) int {
	end := startLine + 1
	for end < len(lines) {
		trimmed := strings.TrimSpace(lines[end])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			end++
			continue
		}
		indent := indentOf(lines[end])
		if indent > keyIndent || (takeDashes && indent == keyIndent && strings.HasPrefix(trimmed, "- ")) {
			end++
			continue
		}
		break
	}
	for end > startLine+1 {
		trimmed := strings.TrimSpace(lines[end-1])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			end--
			continue
		}
		break
	}
	return end
}

// indentOf counts a line's leading spaces.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// lastContentLine is the last non-blank line, so an appended key lands against
// the content rather than after the file's trailing blank lines.
func lastContentLine(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

// splitLines splits the source into lines, reporting whether the file ended
// with a newline so joining can put back exactly what was there.
func splitLines(src []byte) ([]string, bool) {
	s := string(src)
	trailing := strings.HasSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n"), trailing
}

// join re-assembles the lines, restoring the file's final newline.
func join(lines []string, trailing bool) []byte {
	s := strings.Join(lines, "\n")
	if trailing {
		s += "\n"
	}
	return []byte(s)
}

// encodeNode turns a Go value into a YAML node, as an error rather than a
// panic. yaml.Node.Encode panics on a type it cannot represent — unlike
// yaml.Marshal, which recovers — and a config editor handed something odd by a
// caller should refuse the edit, not take the process down with it.
func encodeNode(value interface{}) (node *yaml.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			node, err = nil, fmt.Errorf("cannot write that value into YAML: %v", r)
		}
	}()
	node = &yaml.Node{}
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	return node, nil
}
