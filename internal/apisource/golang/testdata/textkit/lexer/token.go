// Package lexer holds the tokens.
package lexer

import tk "example.com/textkit"

// Token is one piece of text.
type Token struct {
	Text string
	Kind Kind
}

// Kind is what a token is.
type Kind string

// Back returns the lexer, see [tk.Lexer].
func Back() *tk.Lexer { return nil }
