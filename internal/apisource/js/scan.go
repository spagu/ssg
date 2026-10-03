package js

import (
	"bytes"
	"sort"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// token is one significant token of a source file: no whitespace, no
// comments.
type token struct {
	text  string
	off   int  // byte offset in the source
	depth int  // bracket depth; an opening bracket carries the outer depth
	nl    bool // a line break comes before it
	doc   int  // index in scan.docs of the doc comment right before it, or -1
}

// docComment is one /** ... */ block.
type docComment struct {
	raw  string // text between "/**" and "*/"
	line int    // 1-based line of "/**"
}

// scan is the token view of a source file: where things are, and which
// documentation comment belongs to which token.
type scan struct {
	toks  []token
	docs  []docComment
	lines []int // byte offset of each line start
}

// scanner holds the running state of scanSource.
type scanner struct {
	s       *scan
	lex     *js.Lexer
	off     int
	depth   int
	nl      bool
	pending int // doc comment waiting for the next token, or -1
	prev    js.TokenType
}

// scanSource tokenises src. A lexical error ends the scan early; the parser
// reports the error, and the tokens read so far still serve.
func scanSource(src []byte) *scan {
	sc := &scanner{
		s:       &scan{lines: lineStarts(src)},
		lex:     js.NewLexer(parse.NewInputBytes(bytes.Clone(src))),
		pending: -1,
	}
	for sc.step() {
	}
	return sc.s
}

// step reads one token; it returns false at the end of input or on error.
func (sc *scanner) step() bool {
	tt, data := sc.lex.Next()
	if (tt == js.DivToken || tt == js.DivEqToken) && !operandEnd(sc.prev) {
		tt, data = sc.lex.RegExp()
	}
	if tt == js.ErrorToken {
		return false
	}
	start := sc.off
	sc.off += len(data)
	switch tt {
	case js.WhitespaceToken:
	case js.LineTerminatorToken:
		sc.nl = true
	case js.CommentToken, js.CommentLineTerminatorToken:
		sc.comment(data, start, tt == js.CommentLineTerminatorToken)
	default:
		sc.emit(tt, string(data), start)
	}
	return true
}

// comment records a doc comment as pending, or clears the pending one: only
// whitespace may separate a doc comment from what it documents.
func (sc *scanner) comment(data []byte, start int, hasNewline bool) {
	sc.nl = sc.nl || hasNewline
	sc.pending = -1
	if !bytes.HasPrefix(data, []byte("/**")) || len(data) < 5 { // "/**/" is not a doc comment
		return
	}
	sc.s.docs = append(sc.s.docs, docComment{raw: string(data[3 : len(data)-2]), line: sc.s.line(start)})
	sc.pending = len(sc.s.docs) - 1
}

// emit appends a significant token, tracking bracket depth.
func (sc *scanner) emit(tt js.TokenType, text string, start int) {
	if closes(tt) {
		sc.depth--
	}
	sc.s.toks = append(sc.s.toks, token{text: text, off: start, depth: sc.depth, nl: sc.nl, doc: sc.pending})
	if opens(tt) {
		sc.depth++
	}
	sc.nl, sc.pending, sc.prev = false, -1, tt
}

// opens reports whether a token opens a bracket.
func opens(tt js.TokenType) bool {
	switch tt {
	case js.OpenBraceToken, js.OpenParenToken, js.OpenBracketToken, js.TemplateStartToken, js.TemplateMiddleToken:
		return true
	}
	return false
}

// closes reports whether a token closes a bracket.
func closes(tt js.TokenType) bool {
	switch tt {
	case js.CloseBraceToken, js.CloseParenToken, js.CloseBracketToken, js.TemplateMiddleToken, js.TemplateEndToken:
		return true
	}
	return false
}

// operandEnd reports whether a token can end an operand, so that a "/"
// after it divides; after anything else a "/" starts a regular expression.
func operandEnd(tt js.TokenType) bool {
	switch tt {
	case js.CloseParenToken, js.CloseBracketToken, js.CloseBraceToken, js.StringToken, js.TemplateToken,
		js.TemplateEndToken, js.RegExpToken, js.PrivateIdentifierToken, js.ThisToken, js.SuperToken,
		js.TrueToken, js.FalseToken, js.NullToken:
		return true
	}
	return js.IsNumeric(tt) || (js.IsIdentifier(tt) && !js.IsReservedWord(tt))
}

// lineStarts returns the byte offset of each line start.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// line returns the 1-based line of a byte offset.
func (s *scan) line(off int) int {
	return sort.Search(len(s.lines), func(i int) bool { return s.lines[i] > off })
}

// text returns the text of token i, or "" when i is out of range.
func (s *scan) text(i int) string {
	if i < 0 || i >= len(s.toks) {
		return ""
	}
	return s.toks[i].text
}

// tokenLine returns the 1-based line of token i, or 0 when there is none.
func (s *scan) tokenLine(i int) int {
	if i < 0 || i >= len(s.toks) {
		return 0
	}
	return s.line(s.toks[i].off)
}
