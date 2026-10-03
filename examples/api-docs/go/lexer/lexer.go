// Package lexer reads text one token at a time.
package lexer

// Kind is what a [Token] is.
type Kind int

// Token kinds.
const (
	Word Kind = iota
	Number
	Punct
	Space
)

// Token is one piece of the input.
type Token struct {
	Kind  Kind   // what the token is
	Text  string // the token as written
	Start int    // byte offset of the first character
}

// Lexer reads tokens from a string.
type Lexer struct {
	src string
	pos int
}

// New returns a Lexer over src.
func New(src string) *Lexer { return &Lexer{src: src} }

// Next returns the next token, and false at the end of the input.
func (l *Lexer) Next() (Token, bool) {
	if l.pos >= len(l.src) {
		return Token{}, false
	}
	t := Token{Kind: Word, Text: l.src[l.pos : l.pos+1], Start: l.pos}
	l.pos++
	return t, true
}
