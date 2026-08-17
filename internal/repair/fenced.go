package repair

// The other way Markdown swallows markup: a code fence rather than four columns
// of indentation (#166).
//
// `repair` looked for indentation and reported a site clean while its front page
// carried a grey box a screen and a half tall, full of `</div>` and
// `<section class="elementor-section …">`. The cause was one fence the exporter
// emitted around a plugin's `<template>`, in the middle of a `<div>`: from that
// line on, the document renders as source.
//
// This is louder than the indented case and arrives from every exporter that
// converts a page builder's body to Markdown, so it has to be caught here and
// not only at the source.
//
// The discrimination that matters: an author writing about HTML fences it on
// purpose. Two shapes are reported and nothing else —
//
//   - a fence that never closes before the end of the document, whose content
//     carries markup. Nothing renders correctly after it, whatever it holds, and
//     there is no reading under which an author meant it.
//   - a BARE fence (no language) holding markup, in a document that carries raw
//     markup outside the fence as well. That second condition is what separates
//     a page-builder export from a prose document with one HTML example: a
//     deliberate sample is written as ```html, and it does not sit in a page
//     otherwise made of divs.
//
// The fix is the same shape as the indented one — drop the fence markers, leave
// the markup — and equally conservative: nothing here guesses at content.

import "strings"

// Kind says how a block was swallowed, so a report can name the cause and a fix
// can apply the right one.
type Kind int

const (
	// KindIndented is markup indented four columns or more. The zero value, so
	// a Finding built before fences were understood still means what it did.
	KindIndented Kind = iota
	// KindFenced is markup inside a code fence.
	KindFenced
)

// String names the kind for a report.
func (k Kind) String() string {
	if k == KindFenced {
		return "fenced as code"
	}
	return "indented as code"
}

// fence is one fenced region found in a document.
type fence struct {
	Open   int  // line index of the opening marker
	Close  int  // line index of the closing marker, or -1 when it never closes
	Bare   bool // no language after the marker
	Markup bool // the content carries an HTML tag
	First  int  // line index of the first content line, or -1 when empty
}

// scanFences walks a document's fenced regions and calls visit for each one that
// swallowed markup, with the indices of its markers.
func scanFences(content string, visit func(f fence)) {
	lines := strings.Split(content, "\n")
	body := frontMatterEnd(lines)
	fences := collectFences(lines, body)
	outside := markupOutsideFences(lines, body, fences)

	for _, f := range fences {
		switch {
		case f.Close < 0 && f.Markup:
			// Unclosed: everything after it renders as source.
			visit(f)
		case f.Close >= 0 && f.Bare && f.Markup && f.startsWithTag(lines) && outside:
			// A bare fence of markup in a document already full of markup.
			visit(f)
		}
	}
}

// startsWithTag reports whether the fence's first content line opens a tag,
// which is what an exported page-builder body looks like and what a sentence of
// prose does not.
func (f fence) startsWithTag(lines []string) bool {
	if f.First < 0 || f.First >= len(lines) {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(lines[f.First], " \t"), "<")
}

// collectFences finds every fenced region from body onwards, in order.
func collectFences(lines []string, body int) []fence {
	var out []fence
	open := -1
	for i := body; i < len(lines); i++ {
		if !fenceMarker(lines[i]) {
			continue
		}
		if open < 0 {
			open = i
			continue
		}
		out = append(out, newFence(lines, open, i))
		open = -1
	}
	if open >= 0 {
		out = append(out, newFence(lines, open, -1))
	}
	return out
}

// newFence describes one region: whether its marker carried a language, whether
// its content holds markup, and where that content starts.
func newFence(lines []string, open, close int) fence {
	f := fence{Open: open, Close: close, First: -1}
	f.Bare = fenceInfo(lines[open]) == ""

	end := close
	if end < 0 {
		end = len(lines)
	}
	for i := open + 1; i < end; i++ {
		if f.First < 0 && strings.TrimSpace(lines[i]) != "" {
			f.First = i
		}
		if markupRe.MatchString(lines[i]) {
			f.Markup = true
		}
	}
	return f
}

// fenceInfo returns the info string after a fence marker — "html" in "```html".
// An author naming the language meant the fence.
func fenceInfo(line string) string {
	t := strings.TrimLeft(line, " \t")
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, marker) {
			return strings.TrimSpace(strings.TrimLeft(t[len(marker):], "`~"))
		}
	}
	return ""
}

// markupOutsideFences reports whether the document carries raw markup outside
// every fence — the difference between a page-builder export and a prose
// document that happens to show one HTML example.
func markupOutsideFences(lines []string, body int, fences []fence) bool {
	inside := make(map[int]bool)
	for _, f := range fences {
		end := f.Close
		if end < 0 {
			end = len(lines) - 1
		}
		for i := f.Open; i <= end && i < len(lines); i++ {
			inside[i] = true
		}
	}
	for i := body; i < len(lines); i++ {
		if !inside[i] && markupRe.MatchString(lines[i]) {
			return true
		}
	}
	return false
}
