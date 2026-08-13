// Package repair finds and fixes source Markdown that renders as literal text
// instead of as the markup it is.
//
// The failure it exists for: a WordPress page built with Elementor (or any
// other page builder) exports as tab-indented `<div>` soup. The exporter turns
// `</p>` into a blank line, the blank line ends the surrounding HTML block per
// CommonMark, and every following tab-indented line is then four columns deep —
// an INDENTED CODE BLOCK. The build succeeds, the page ships, and the visitor
// reads `</div>` in monospace where the content should be.
//
// Nothing here guesses at content. A block is repaired only when it is both
// indented enough to be code AND made of markup — the one combination an author
// never writes on purpose, and the exact shape a broken export leaves behind.
package repair

import (
	"regexp"
	"strings"
)

// tabWidth is CommonMark's: a tab advances to the next multiple of four, and
// four columns of indentation open a code block.
const tabWidth = 4

// markupRe matches a raw HTML tag — opening, closing or self-closing. Used to
// tell a broken export from a legitimate indented code block: a code sample
// about HTML is possible, which is why the block ALSO has to have been produced
// by an export that indented its markup (see Finding.Kind).
var markupRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9-]*(\s[^<>]*)?/?>`)

// listMarkerRe matches the start of a list item, whose continuation lines are
// legitimately indented and must never be dedented.
var listMarkerRe = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s`)

// Finding is one repairable block: where it starts and what it looks like.
type Finding struct {
	// Line is the 1-based line number in the file, front matter included, so
	// the report points at something an editor can jump to.
	Line int
	// Lines is how many lines the block spans.
	Lines int
	// Sample is the block's first line, trimmed and truncated, so a report
	// shows what was found without printing the whole div soup.
	Sample string
}

// sampleWidth keeps a finding's sample to one terminal line.
const sampleWidth = 60

// Scan reports every indented block of raw markup in a Markdown source.
// Front matter and fenced code blocks are never reported: YAML indentation is
// structural, and a fence means the author asked for verbatim text.
func Scan(content string) []Finding {
	var findings []Finding
	scanBlocks(content, func(start, count int, lines []string) {
		findings = append(findings, Finding{
			Line:   start + 1,
			Lines:  count,
			Sample: truncate(strings.TrimSpace(lines[start]), sampleWidth),
		})
	})
	return findings
}

// Apply repairs the content, returning it with every indented markup block
// dedented, plus the number of lines it changed. Content with nothing to repair
// comes back byte-identical, so a caller can skip the write.
func Apply(content string) (string, int) {
	lines := strings.Split(content, "\n")
	changed := 0

	scanBlocks(content, func(start, count int, _ []string) {
		for i := start; i < start+count; i++ {
			if dedented := strings.TrimLeft(lines[i], " \t"); dedented != lines[i] {
				lines[i] = dedented
				changed++
			}
		}
	})
	if changed == 0 {
		return content, 0
	}
	return strings.Join(lines, "\n"), changed
}

// scanBlocks walks the body's blank-line-separated blocks and calls visit for
// each one that is both indented like code and made of markup. Both Scan and
// Apply are this walk with a different visitor, so a finding and a fix can
// never disagree about what is broken.
func scanBlocks(content string, visit func(start, count int, lines []string)) {
	lines := strings.Split(content, "\n")
	body := frontMatterEnd(lines)

	inFence := false
	prevBlockIsList := false

	for i := body; i < len(lines); {
		line := lines[i]
		if fenceMarker(line) {
			inFence = !inFence
			i++
			continue
		}
		if inFence || strings.TrimSpace(line) == "" {
			i++
			continue
		}

		start := i
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !fenceMarker(lines[i]) {
			i++
		}
		block := lines[start:i]

		if !prevBlockIsList && indentColumns(block[0]) >= tabWidth && containsMarkup(block) {
			visit(start, len(block), lines)
		}
		// A list's continuation lines are indented on purpose; a block that
		// follows one may be part of it, so it is left alone even if it looks
		// like markup.
		prevBlockIsList = listMarkerRe.MatchString(block[0])
	}
}

// frontMatterEnd returns the index of the first body line, skipping a leading
// `---` front-matter block. Its indentation is YAML structure, not prose.
func frontMatterEnd(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i + 1
		}
	}
	// Unterminated front matter: treat the whole file as front matter rather
	// than rewriting what may be structured data.
	return len(lines)
}

// fenceMarker reports whether a line opens or closes a fenced code block.
func fenceMarker(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// indentColumns measures a line's leading whitespace in columns, expanding tabs
// to the next tab stop the way CommonMark does.
func indentColumns(line string) int {
	cols := 0
	for _, r := range line {
		switch r {
		case ' ':
			cols++
		case '\t':
			cols += tabWidth - cols%tabWidth
		default:
			return cols
		}
	}
	return cols
}

// containsMarkup reports whether any line in the block carries an HTML tag.
func containsMarkup(block []string) bool {
	for _, line := range block {
		if markupRe.MatchString(line) {
			return true
		}
	}
	return false
}

// truncate shortens s to n runes with an ellipsis, so a report stays readable.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
