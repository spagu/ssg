package php

import (
	"fmt"
	"strings"
)

// tokKind is what a token is.
type tokKind int

// The kinds of token the scanner produces. Ordinary comments and
// whitespace produce none.
const (
	tokEOF    tokKind = iota // past the last token
	tokIdent                 // a name or keyword, qualified ones included: Foo\Bar, \Baz
	tokVar                   // $name
	tokString                // a quoted string, heredoc, nowdoc or backtick command
	tokNumber                // 42, 0x1F, 1_000, 3.14
	tokPunct                 // an operator or bracket
	tokDoc                   // a /** */ docblock; text is what lies between /** and */
	tokAttr                  // a whole #[...] attribute
)

// token is one lexical unit with its byte span and the line it starts on.
type token struct {
	kind       tokKind
	text       string
	start, end int
	line       int
}

// is reports whether the token is the punctuation p.
func (t token) is(p string) bool { return t.kind == tokPunct && t.text == p }

// keyword reports whether the token is the name k, ignoring case as PHP does.
func (t token) keyword(k string) bool { return t.kind == tokIdent && strings.EqualFold(t.text, k) }

// lexError is a file the scanner cannot read to its end.
type lexError struct {
	line int
	msg  string
}

func (e *lexError) Error() string { return fmt.Sprintf("line %d: %s", e.line, e.msg) }

// lexer holds the scanning state of one file.
type lexer struct {
	src  string
	pos  int
	line int
	toks []token
}

// multiPunct are the operators kept as one token; anything else is a
// single character.
var multiPunct = []string{"...", "?->", "::", "=>", "->", "??"}

// tokenize scans a PHP file into tokens, or reports where it could not.
func tokenize(src string) ([]token, error) {
	lx := &lexer{src: src, line: 1}
	for lx.pos < len(lx.src) {
		lx.skipHTML()
		if err := lx.php(); err != nil {
			return nil, err
		}
	}
	return lx.toks, nil
}

// advance moves to byte offset to, counting the newlines passed.
func (lx *lexer) advance(to int) {
	lx.line += strings.Count(lx.src[lx.pos:to], "\n")
	lx.pos = to
}

// emit records the token src[lx.pos:end] and moves past it.
func (lx *lexer) emit(kind tokKind, end int) {
	lx.emitText(kind, end, lx.src[lx.pos:end])
}

// emitText records a token spanning lx.pos..end with the given text.
func (lx *lexer) emitText(kind tokKind, end int, text string) {
	lx.toks = append(lx.toks, token{kind: kind, text: text, start: lx.pos, end: end, line: lx.line})
	lx.advance(end)
}

// skipHTML moves past inline HTML to just after the next opening tag
// (<?php, <?= or <?), or to the end of the file.
func (lx *lexer) skipHTML() {
	i := strings.Index(lx.src[lx.pos:], "<?")
	if i < 0 {
		lx.advance(len(lx.src))
		return
	}
	to := lx.pos + i + 2
	switch {
	case len(lx.src) >= to+3 && strings.EqualFold(lx.src[to:to+3], "php"):
		to += 3
	case to < len(lx.src) && lx.src[to] == '=':
		to++
	}
	lx.advance(to)
}

// php scans PHP code until a closing ?> or the end of the file.
func (lx *lexer) php() error {
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		rest := lx.src[lx.pos:]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			lx.advance(lx.pos + 1)
		case strings.HasPrefix(rest, "?>"):
			lx.advance(lx.pos + 2)
			return nil
		case strings.HasPrefix(rest, "#["):
			if err := lx.attribute(); err != nil {
				return err
			}
		case c == '#' || strings.HasPrefix(rest, "//"):
			lx.lineComment()
		case strings.HasPrefix(rest, "/*"):
			if err := lx.blockComment(); err != nil {
				return err
			}
		case c == '\'' || c == '"' || c == '`':
			if err := lx.quoted(c); err != nil {
				return err
			}
		case strings.HasPrefix(rest, "<<<"):
			if err := lx.heredoc(); err != nil {
				return err
			}
		default:
			lx.word(c)
		}
	}
	return nil
}

// word scans a variable, name, number or operator starting with c.
func (lx *lexer) word(c byte) {
	switch {
	case c == '$' && lx.pos+1 < len(lx.src) && isNameStart(lx.src[lx.pos+1]):
		lx.emit(tokVar, lx.nameEnd(lx.pos+1))
	case isNameStart(c) || c == '\\':
		lx.emit(tokIdent, lx.nameEnd(lx.pos))
	case isDigit(c):
		end := lx.pos
		for end < len(lx.src) && (isNameChar(lx.src[end]) || lx.src[end] == '.') {
			end++
		}
		lx.emit(tokNumber, end)
	default:
		for _, p := range multiPunct {
			if strings.HasPrefix(lx.src[lx.pos:], p) {
				lx.emit(tokPunct, lx.pos+len(p))
				return
			}
		}
		lx.emit(tokPunct, lx.pos+1)
	}
}

// nameEnd returns where a (possibly qualified) name starting at i ends.
func (lx *lexer) nameEnd(i int) int {
	for i < len(lx.src) && (isNameChar(lx.src[i]) || lx.src[i] == '\\') {
		i++
	}
	return i
}

// isNameStart reports whether c can start a PHP name; bytes of multi-byte
// UTF-8 characters count as letters, as in PHP.
func isNameStart(c byte) bool {
	return c == '_' || c >= 0x80 || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

// isNameChar reports whether c can continue a PHP name.
func isNameChar(c byte) bool { return isNameStart(c) || isDigit(c) }

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
