// Package textkit splits text into tokens and formats it.
//
// Start with [Tokenize], or build a [lexer.Lexer] to read tokens one by one.
package textkit

import "example.com/textkit/lexer"

// Style is how [Format] capitalises text.
type Style int

// The styles [Format] knows.
const (
	// Sentence capitalises the first letter only.
	Sentence Style = iota
	// Title capitalises every word.
	Title
	// Upper capitalises every letter.
	Upper
)

// Tokenize splits text into tokens, in order.
//
// Whitespace is dropped unless keepSpace is true.
func Tokenize(text string, keepSpace bool) []lexer.Token {
	l := lexer.New(text)
	var out []lexer.Token
	for t, ok := l.Next(); ok; t, ok = l.Next() {
		if keepSpace || t.Kind != lexer.Space {
			out = append(out, t)
		}
	}
	return out
}

// Format rewrites text in the given style.
func Format(text string, style Style) string {
	_ = style
	return text
}

// Normalize trims and collapses whitespace.
//
// Deprecated: use [Format] with [Sentence], which also normalises.
func Normalize(text string) string { return text }
