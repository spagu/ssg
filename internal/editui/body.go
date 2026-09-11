package editui

// Click-to-edit for the body of a page (GO-102, phase 2).
//
// Phase 1 edited frontmatter, where the field a click means is written on the
// element that was clicked. The body is harder, and the reason is the hazard
// the ticket named first: the HTML on screen went through Markdown rendering,
// shortcode expansion, SEO injection, link rewriting, sanitisation and
// minification, and none of that has an inverse. "Take the text of the element
// and find it in the file" is the shape of every editor that eventually changes
// the wrong paragraph.
//
// So this does not look for the text in the file. It looks for the BLOCK: the
// source is split into the blocks Markdown itself is made of, each block's
// plain text is compared with the clicked text after normalising the things
// the build is known to change, and the edit only proceeds when exactly one
// block matches. Zero or several is a refusal that says the count.
//
// The save then goes through `content_edit`, whose contract is the same rule
// one level down: the anchor must appear exactly once in the file, or nothing
// is written. Two independent checks of the same property, because the failure
// they prevent is silent.

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Block is one editable piece of a document's body.
type Block struct {
	// Source is the block exactly as it appears in the file — the anchor a
	// save is made against.
	Source string
	// Text is its plain text, which is what the browser can compare against.
	Text string
	// Kind is what the block is, for the editor's label: paragraph, heading,
	// list-item, quote.
	Kind string
}

// blockSplitRe splits a body into paragraph-level chunks on blank lines.
var blockSplitRe = regexp.MustCompile(`\n[ \t]*\n`)

// fenceLineRe marks the start or end of a fenced code block.
var fenceLineRe = regexp.MustCompile("^\\s*(```|~~~)")

// inlineMarkRe strips the inline Markdown that decorates text without being
// part of it, so a block's plain text can be compared with what a browser
// rendered.
var inlineMarkRe = regexp.MustCompile(`(\*\*|__|\*|_|` + "`" + `)`)

// linkRe turns [text](href) into its text.
var linkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// imageRe removes ![alt](src) entirely: an image contributes no text a reader
// clicked on.
var imageRe = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)

// leadingMarkRe removes the marker that makes a line a heading, a quote or a
// list item.
var leadingMarkRe = regexp.MustCompile(`^\s{0,3}(#{1,6}\s+|>\s?|[-*+]\s+|\d+\.\s+)`)

// SplitBlocks breaks a document body into the blocks a person can click.
//
// Code fences are kept whole and marked as code: their text is the code, and
// editing it through a click-to-edit box would be a worse experience than
// opening the file. Lists are split per item, because that is what a reader
// clicks.
func SplitBlocks(body string) []Block {
	var out []Block
	for _, chunk := range splitOutsideFences(body) {
		// Trim the blank lines around a block but nothing inside it: the source
		// is going to be used as an edit anchor, and an anchor carrying the
		// newline that happened to precede it is one the next save may not find.
		chunk = strings.Trim(chunk, " \t\n")
		if chunk == "" {
			continue
		}
		if fenceLineRe.MatchString(chunk) {
			out = append(out, Block{Source: chunk, Text: chunk, Kind: "code"})
			continue
		}
		out = append(out, splitListItems(chunk)...)
	}
	return out
}

// splitOutsideFences splits on blank lines without cutting a fenced block in
// half, because a fence may contain them.
func splitOutsideFences(body string) []string {
	if !strings.Contains(body, "```") && !strings.Contains(body, "~~~") {
		return blockSplitRe.Split(body, -1)
	}
	var out []string
	var current []string
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceLineRe.MatchString(line) {
			inFence = !inFence
			current = append(current, line)
			continue
		}
		if !inFence && strings.TrimSpace(line) == "" {
			out = append(out, strings.Join(current, "\n"))
			current = nil
			continue
		}
		current = append(current, line)
	}
	return append(out, strings.Join(current, "\n"))
}

// splitListItems turns a list into one block per item, and leaves anything else
// as one block.
func splitListItems(chunk string) []Block {
	lines := strings.Split(chunk, "\n")
	if !isListLine(lines[0]) {
		return []Block{{Source: chunk, Text: plainText(chunk), Kind: kindOf(chunk)}}
	}
	var out []Block
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		src := strings.Join(current, "\n")
		out = append(out, Block{Source: src, Text: plainText(src), Kind: "list-item"})
		current = nil
	}
	for _, line := range lines {
		if isListLine(line) {
			flush()
		}
		current = append(current, line)
	}
	flush()
	return out
}

// isListLine reports whether a line starts a list item.
func isListLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}
	return leadingMarkRe.MatchString(line) && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ">")
}

// kindOf labels a block for the editor.
func kindOf(chunk string) string {
	trimmed := strings.TrimSpace(chunk)
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return "heading"
	case strings.HasPrefix(trimmed, ">"):
		return "quote"
	}
	return "paragraph"
}

// plainText renders a block's source as the text a browser would show.
func plainText(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		line = leadingMarkRe.ReplaceAllString(line, "")
		lines = append(lines, line)
	}
	s := strings.Join(lines, " ")
	s = imageRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = inlineMarkRe.ReplaceAllString(s, "")
	return normalizeText(s)
}

// normalizeText makes two pieces of text comparable across everything the
// build does to them on the way to a page.
//
// Whitespace collapses, because HTML collapses it. Smart punctuation folds back
// to its plain form, because `smart-punctuation` and `clean_special_chars`
// rewrite it (GO-086) and the browser is reading the result. Non-breaking
// spaces become spaces, because a typographic pass inserts them.
func normalizeText(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch r {
		case '‘', '’', 'ʼ':
			r = '\''
		case '“', '”':
			r = '"'
		case '–', '—', '−':
			r = '-'
		case '…':
			b.WriteString("...")
			lastSpace = false
			continue
		case ' ', ' ', ' ':
			r = ' '
		}
		if unicode.IsSpace(r) {
			if lastSpace {
				continue
			}
			lastSpace = true
			b.WriteByte(' ')
			continue
		}
		lastSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// FindBlock returns the one block of a body whose text matches, or an error
// saying why it will not guess.
//
// This is the whole safety property of click-to-edit, so it refuses in both
// directions: nothing matched, or several did.
func FindBlock(body, clicked string) (Block, error) {
	want := normalizeText(clicked)
	if want == "" {
		return Block{}, fmt.Errorf("there is no text here to edit")
	}
	var matches []Block
	for _, b := range SplitBlocks(body) {
		if b.Text == want {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Block{}, fmt.Errorf("this text is not a block of the source — it may be generated by the theme " +
			"rather than written in the file, or changed by the build on the way to the page")
	}
	return Block{}, fmt.Errorf("this text appears %d times in the file, so an edit here could change the wrong one — "+
		"edit it in the file, or make the passages different", len(matches))
}
