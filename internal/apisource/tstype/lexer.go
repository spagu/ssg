package tstype

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenKind classifies a token.
type tokenKind int

// The token kinds the type grammar needs.
const (
	tokEOF      tokenKind = iota
	tokIdent              // names and keywords: string, keyof, Foo
	tokNumber             // 42, 0x1F, 1.5, 10n
	tokString             // 'a' or "a", quotes kept
	tokTemplate           // `a${B}`, backticks kept
	tokPunct              // | & ( ) [ ] { } < > , ; : ? ! = * . ... => + -
)

// token is one lexeme with its byte span in the source.
type token struct {
	kind       tokenKind
	text       string
	start, end int
	// hasSubst is set on a template literal that contains ${...}.
	hasSubst bool
}

// is reports whether the token is the punctuation or identifier text s.
func (t token) is(s string) bool {
	return (t.kind == tokPunct || t.kind == tokIdent) && t.text == s
}

// lexer turns source text into tokens; whitespace is dropped because the
// verbatim renderer recovers spacing from the token spans.
type lexer struct {
	src string
	pos int
}

// lex tokenizes src, ending with a tokEOF token.
func lex(src string) ([]token, error) {
	l := &lexer{src: src}
	var toks []token
	for {
		l.skipSpace()
		if l.pos >= len(l.src) {
			return append(toks, token{kind: tokEOF, start: l.pos, end: l.pos}), nil
		}
		start := l.pos
		kind, err := l.next()
		if err != nil {
			return nil, err
		}
		t := token{kind: kind, text: l.src[start:l.pos], start: start, end: l.pos}
		t.hasSubst = kind == tokTemplate && strings.Contains(t.text, "${")
		toks = append(toks, t)
	}
}

// skipSpace advances past whitespace, including newlines.
func (l *lexer) skipSpace() {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(r) {
			return
		}
		l.pos += size
	}
}

// next scans one token starting at l.pos and returns its kind.
func (l *lexer) next() (tokenKind, error) {
	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	switch {
	case isIdentRune(r) && !unicode.IsDigit(r):
		l.scanWhile(isIdentRune)
		return tokIdent, nil
	case r >= '0' && r <= '9':
		l.scanWhile(func(r rune) bool { return isIdentRune(r) || r == '.' })
		return tokNumber, nil
	case r == '\'' || r == '"':
		return tokString, l.scanString(byte(r))
	case r == '`':
		return tokTemplate, l.scanTemplate()
	}
	for _, p := range []string{"...", "=>"} {
		if strings.HasPrefix(l.src[l.pos:], p) {
			l.pos += len(p)
			return tokPunct, nil
		}
	}
	if strings.ContainsRune("|&()[]{}<>,;:?!=*.+-", r) {
		l.pos += size
		return tokPunct, nil
	}
	return tokEOF, fmt.Errorf("unexpected character %q at offset %d", r, l.pos)
}

// isIdentRune reports whether r may appear in an identifier.
func isIdentRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// scanWhile advances while ok holds for the next rune.
func (l *lexer) scanWhile(ok func(rune) bool) {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !ok(r) {
			return
		}
		l.pos += size
	}
}

// scanString scans a quoted string whose opening quote is at l.pos.
func (l *lexer) scanString(quote byte) error {
	start := l.pos
	for l.pos++; l.pos < len(l.src); l.pos++ {
		switch l.src[l.pos] {
		case '\\':
			l.pos++
		case quote:
			l.pos++
			return nil
		}
	}
	return fmt.Errorf("unterminated string at offset %d", start)
}

// scanTemplate scans a template literal whose backtick is at l.pos,
// including any ${...} substitutions, which may nest strings and templates.
func (l *lexer) scanTemplate() error {
	start := l.pos
	for l.pos++; l.pos < len(l.src); l.pos++ {
		switch {
		case l.src[l.pos] == '\\':
			l.pos++
		case l.src[l.pos] == '`':
			l.pos++
			return nil
		case strings.HasPrefix(l.src[l.pos:], "${"):
			l.pos += 2
			if err := l.scanSubstitution(); err != nil {
				return err
			}
			l.pos-- // the loop step moves past the closing brace
		}
	}
	return fmt.Errorf("unterminated template literal at offset %d", start)
}

// scanSubstitution scans the body of ${...} up to and past its closing brace.
func (l *lexer) scanSubstitution() error {
	start := l.pos
	for depth := 1; l.pos < len(l.src); {
		c := l.src[l.pos]
		var err error
		switch c {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				l.pos++
				return nil
			}
		case '\'', '"':
			err = l.scanString(c)
		case '`':
			err = l.scanTemplate()
		}
		if err != nil {
			return err
		}
		if c != '\'' && c != '"' && c != '`' {
			l.pos++
		}
	}
	return fmt.Errorf("unterminated substitution at offset %d", start)
}
