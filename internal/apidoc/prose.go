package apidoc

import "strings"

// span is a byte range [start, end) of a Markdown text.
type span struct {
	start, end int
}

// proseRanges returns the ranges of md outside fenced code blocks and
// inline code spans: where inline links are looked for.
func proseRanges(md string) []span {
	var out []span
	var f fence
	segStart, off := -1, 0
	flush := func(end int) {
		if segStart >= 0 {
			out = append(out, splitCodeSpans(md, span{segStart, end})...)
			segStart = -1
		}
	}
	for _, line := range strings.SplitAfter(md, "\n") {
		wasOpen := f.open()
		if f.update(line) || wasOpen {
			flush(off)
		} else if segStart < 0 {
			segStart = off
		}
		off += len(line)
	}
	flush(off)
	return out
}

// splitCodeSpans returns the parts of md[r.start:r.end] outside inline code
// spans. A backtick run without a closing run of the same length is text.
func splitCodeSpans(md string, r span) []span {
	var out []span
	prose := r.start
	for i := r.start; i < r.end; {
		if md[i] != '`' {
			i++
			continue
		}
		n := runLength(md, i, r.end)
		closing := findRun(md, i+n, r.end, n)
		if closing < 0 {
			i += n
			continue
		}
		if i > prose {
			out = append(out, span{prose, i})
		}
		i = closing + n
		prose = i
	}
	if r.end > prose {
		out = append(out, span{prose, r.end})
	}
	return out
}

// runLength counts the backticks starting at md[i], not past end.
func runLength(md string, i, end int) int {
	n := 0
	for i+n < end && md[i+n] == '`' {
		n++
	}
	return n
}

// findRun returns the start of the first backtick run of exactly n in
// md[from:end], or -1.
func findRun(md string, from, end, n int) int {
	for i := from; i < end; {
		if md[i] != '`' {
			i++
			continue
		}
		m := runLength(md, i, end)
		if m == n {
			return i
		}
		i += m
	}
	return -1
}
