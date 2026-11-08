package ogcard

import (
	"strings"

	"golang.org/x/image/font"
)

const ellipsis = "…"

// width is the advance of s in whole pixels.
func width(f font.Face, s string) int {
	return font.MeasureString(f, s).Ceil()
}

// wrap breaks text into lines no wider than maxW, at spaces where it can and
// inside a word only when the word alone is wider than the line.
func wrap(f font.Face, text string, maxW int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if width(f, next) <= maxW {
			line = next
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = word
		for width(f, line) > maxW {
			head, rest := splitAt(f, line, maxW)
			lines = append(lines, head)
			line = rest
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// splitAt cuts s at the last rune that still fits in maxW, keeping at least
// one rune on the line so a single glyph wider than the line cannot loop.
func splitAt(f font.Face, s string, maxW int) (string, string) {
	runes := []rune(s)
	n := 1
	for n < len(runes) && width(f, string(runes[:n+1])) <= maxW {
		n++
	}
	return string(runes[:n]), string(runes[n:])
}

// clampLines keeps at most n lines; when it drops some, the last kept line
// ends with an ellipsis.
func clampLines(f font.Face, lines []string, n, maxW int) []string {
	if len(lines) <= n {
		return lines
	}
	lines = lines[:n]
	lines[n-1] = withEllipsis(f, lines[n-1], maxW)
	return lines
}

// fitLine is s on one line: whole if it fits, otherwise cut with an ellipsis.
func fitLine(f font.Face, s string, maxW int) string {
	if width(f, s) <= maxW {
		return s
	}
	return withEllipsis(f, s, maxW)
}

// withEllipsis shortens s until s+"…" fits in maxW.
func withEllipsis(f font.Face, s string, maxW int) string {
	runes := []rune(strings.TrimSpace(s))
	for len(runes) > 0 && width(f, string(runes)+ellipsis) > maxW {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ,.;:-") + ellipsis
}
