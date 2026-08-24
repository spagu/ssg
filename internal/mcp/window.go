package mcp

// Context measured in characters, for lines that are not a unit of size (#204).
//
// find and edit report what they found by line: the matching line, the changed
// line, a few lines either side. That is right for source with lines and wrong
// for a minified stylesheet, where the file *is* one line — so "three lines of
// context" is the whole file, and three colour changes across two minified
// files cost 40–50k tokens. Exactly the cost the anchored edit was built to
// remove, back through a side door.
//
// A migrated theme is where this lands: many WordPress sites ship CSS and JS
// already minified, and a migration keeps what it fetched.
//
// The switch keys on the length of the line in hand, not on the file's name or
// extension. A hand-written stylesheet with one enormous selector list gets the
// same treatment, and a minified file that happens to have short lines is not
// punished for what it is called.

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// longLine is where a line stops being a sensible amount to print. Well
	// past any hand-formatted source line, well short of a minified bundle.
	longLine = 400
	// charContext is how much of a long line is shown either side of the match.
	charContext = 160
	// maxFragmentBytes caps any single fragment, whatever produced it. A cap
	// that is announced is a cap; one that silently truncates is a bug.
	maxFragmentBytes = 4096
)

// isLongLine reports whether a line is too long to print whole.
func isLongLine(line string) bool { return len(line) > longLine }

// charWindow returns the part of line around [from,to), the column range it
// covers, and whether anything was trimmed.
//
// Columns are 1-based character positions, which is what a human counts and
// what an editor shows. Offsets are bytes, because that is what a match
// returns.
func charWindow(line string, from, to int) (fragment string, fromCol, toCol int, trimmed bool) {
	from, to = clampSpan(from, to, len(line))
	start := snapToRune(line, from-charContext, -1)
	end := snapToRune(line, to+charContext, +1)

	fragment = line[start:end]
	if start > 0 {
		fragment = fmt.Sprintf("…(%d chars omitted)… ", utf8.RuneCountInString(line[:start])) + fragment
		trimmed = true
	}
	if end < len(line) {
		fragment += fmt.Sprintf(" …(%d chars omitted)…", utf8.RuneCountInString(line[end:]))
		trimmed = true
	}
	// Reported columns describe the match, not the window: the window is how
	// much was shown, the columns are where the thing actually is.
	return fragment, utf8.RuneCountInString(line[:from]) + 1, utf8.RuneCountInString(line[:to]), trimmed
}

// clampSpan keeps a byte span inside the line and non-empty.
func clampSpan(from, to, n int) (int, int) {
	from = max(0, min(from, n))
	to = max(from, min(to, n))
	return from, to
}

// snapToRune moves a byte offset onto a rune boundary, searching in dir.
//
// Slicing UTF-8 at an offset that is not a boundary produces a replacement
// character where there was a letter — a tool that corrupts text instead of
// trimming it. Widening the window is always safe; narrowing it could cut the
// match itself, so both directions move outward from the match.
func snapToRune(s string, at, dir int) int {
	at = max(0, min(at, len(s)))
	for at > 0 && at < len(s) && !utf8.RuneStart(s[at]) {
		at += dir
		at = max(0, min(at, len(s)))
	}
	return at
}

// capFragment bounds a fragment however it was produced, saying so out loud.
func capFragment(s string) string {
	if len(s) <= maxFragmentBytes {
		return s
	}
	end := snapToRune(s, maxFragmentBytes, -1)
	return fmt.Sprintf("%s\n…(%d more chars omitted)…", s[:end], utf8.RuneCountInString(s[end:]))
}

// locusOf formats a hit's location: "12" for a whole line, "12:40-96" when a
// column range narrows it.
func locusOf(from, to, fromCol, toCol int) string {
	span := fmt.Sprintf("%d-%d", from, to)
	if from == to {
		span = fmt.Sprintf("%d", from)
	}
	if fromCol > 0 {
		return fmt.Sprintf("%s:%d-%d", span, fromCol, toCol)
	}
	return span
}

// trimLine is used where a whole line is shown but must not be unbounded.
func trimLine(line string) string {
	if !isLongLine(line) {
		return line
	}
	frag, _, _, _ := charWindow(line, 0, 0)
	return strings.TrimSpace(frag)
}
