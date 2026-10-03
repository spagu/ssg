package dts

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// tokKind is the lexical class of a token.
type tokKind int

// The token classes of a declaration file.
const (
	tokEOF      tokKind = iota
	tokIdent            // names and keywords, including #private names
	tokPunct            // operators and brackets: { } < > ( ) [ ] ; , : ? = => ... | & .
	tokString           // "text" or 'text', quotes included
	tokNumber           // 1, 0x1F, 1_000, 10n
	tokTemplate         // `text ${T}`, backticks included
)

// token is one lexeme with its byte span, line and the documentation comment
// written immediately before it.
type token struct {
	kind  tokKind
	text  string
	start int  // byte offset in the source
	end   int  // byte offset after the token
	line  int  // 1-based
	nl    bool // a line break precedes it
	doc   string
}

// is reports whether the token is the punctuation or identifier s.
func (t token) is(s string) bool {
	return (t.kind == tokPunct || t.kind == tokIdent) && t.text == s
}

// lexer turns declaration source into tokens.
type lexer struct {
	src    string
	pos    int
	line   int
	nl     bool
	doc    string
	tokens []token
}

// tokenize splits src into tokens, attaching to each the /** */ comment that
// directly precedes it. An unterminated comment, string or template is an
// error naming its line.
func tokenize(src string) ([]token, error) {
	lx := &lexer{src: src, line: 1}
	for {
		if err := lx.skipTrivia(); err != nil {
			return nil, err
		}
		if lx.pos >= len(src) {
			lx.emit(tokEOF, lx.pos)
			return lx.tokens, nil
		}
		if err := lx.next(); err != nil {
			return nil, err
		}
	}
}

// emit appends a token spanning start..pos and resets the pending comment.
func (lx *lexer) emit(kind tokKind, start int) {
	lx.tokens = append(lx.tokens, token{
		kind: kind, text: lx.src[start:lx.pos], start: start, end: lx.pos,
		line: lx.line - countLines(lx.src[start:lx.pos]), nl: lx.nl, doc: lx.doc,
	})
	lx.nl, lx.doc = false, ""
}

// next lexes one token at lx.pos.
func (lx *lexer) next() error {
	start := lx.pos
	c := lx.src[lx.pos]
	switch {
	case c == '"' || c == '\'':
		return lx.quoted(start, c)
	case c == '`':
		return lx.template(start)
	case c >= '0' && c <= '9':
		lx.pos = lx.scanWord(lx.pos)
		lx.emit(tokNumber, start)
	case isIdentStart(lx.src, lx.pos):
		lx.pos = lx.scanWord(lx.pos + 1)
		lx.emit(tokIdent, start)
	default:
		lx.pos += punctLen(lx.src[lx.pos:])
		lx.emit(tokPunct, start)
	}
	return nil
}

// quoted lexes a string literal ending at the matching quote.
func (lx *lexer) quoted(start int, q byte) error {
	for i := start + 1; i < len(lx.src); i++ {
		switch lx.src[i] {
		case '\\':
			i++
		case '\n':
			return fmt.Errorf("line %d: unterminated string", lx.line)
		case q:
			lx.pos = i + 1
			lx.emit(tokString, start)
			return nil
		}
	}
	return fmt.Errorf("line %d: unterminated string", lx.line)
}

// template lexes a template literal type, skipping ${…} holes by brace depth.
func (lx *lexer) template(start int) error {
	depth := 0
	for i := start + 1; i < len(lx.src); i++ {
		switch c := lx.src[i]; {
		case c == '\\':
			i++
		case c == '\n':
			lx.line++
		case c == '$' && i+1 < len(lx.src) && lx.src[i+1] == '{':
			depth++
			i++
		case c == '}' && depth > 0:
			depth--
		case c == '`' && depth == 0:
			lx.pos = i + 1
			lx.emit(tokTemplate, start)
			return nil
		}
	}
	return fmt.Errorf("line %d: unterminated template literal", lx.line)
}

// scanWord returns the offset after the identifier or number characters at i.
func (lx *lexer) scanWord(i int) int {
	for i < len(lx.src) {
		r, size := utf8.DecodeRuneInString(lx.src[i:])
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return i
		}
		i += size
	}
	return i
}

// isIdentStart reports whether an identifier (or #private name) starts at i.
func isIdentStart(src string, i int) bool {
	r, _ := utf8.DecodeRuneInString(src[i:])
	if r == '#' {
		return i+1 < len(src) && isIdentStart(src, i+1)
	}
	return r == '_' || r == '$' || unicode.IsLetter(r)
}

// punctLen is the length of the punctuation at the start of s. Only the
// tokens a type or declaration needs are joined; ">" always stands alone so
// nested generics close one level at a time.
func punctLen(s string) int {
	for _, p := range []string{"...", "=>"} {
		if len(s) >= len(p) && s[:len(p)] == p {
			return len(p)
		}
	}
	_, size := utf8.DecodeRuneInString(s)
	return size
}

// countLines counts line breaks in s.
func countLines(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}
