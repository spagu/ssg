package engine

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Split breaks text into chunks of at most max characters, cutting between
// sentences where possible, then between words, and only as a last resort
// inside a word.
func Split(text string, max int) []string {
	var chunks []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			chunks = append(chunks, s)
		}
		cur.Reset()
	}
	for _, s := range sentences(text) {
		for _, piece := range hardSplit(s, max) {
			if utf8.RuneCountInString(cur.String())+utf8.RuneCountInString(piece)+1 > max {
				flush()
			}
			if cur.Len() > 0 {
				cur.WriteByte(' ')
			}
			cur.WriteString(piece)
		}
	}
	flush()
	return chunks
}

// sentences cuts after . ! ? … ; followed by whitespace, and at line breaks.
func sentences(text string) []string {
	var out []string
	runes := []rune(text)
	start := 0
	for i, r := range runes {
		end := r == '\n' ||
			(strings.ContainsRune(".!?…;", r) && i+1 < len(runes) && unicode.IsSpace(runes[i+1]))
		if end {
			out = appendTrimmed(out, string(runes[start:i+1]))
			start = i + 1
		}
	}
	return appendTrimmed(out, string(runes[start:]))
}

// hardSplit cuts a sentence longer than max at the last space before the
// limit, or exactly at the limit when there is no space.
func hardSplit(s string, max int) []string {
	var out []string
	runes := []rune(s)
	for len(runes) > max {
		cut := max
		if i := lastSpace(runes[:max]); i > 0 {
			cut = i
		}
		out = appendTrimmed(out, string(runes[:cut]))
		runes = runes[cut:]
	}
	return appendTrimmed(out, string(runes))
}

func lastSpace(r []rune) int {
	for i := len(r) - 1; i >= 0; i-- {
		if unicode.IsSpace(r[i]) {
			return i
		}
	}
	return -1
}

func appendTrimmed(list []string, s string) []string {
	if s = strings.TrimSpace(s); s != "" {
		list = append(list, s)
	}
	return list
}
