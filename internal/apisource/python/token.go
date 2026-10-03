package python

import "strings"

// tokKind is the class of a token.
type tokKind int

// The token classes the scanner emits. Comments are dropped.
const (
	tEOF tokKind = iota
	tName
	tNumber
	tString
	tOp
	tNewline
	tIndent
	tDedent
)

// token is one lexical unit: its class, its source text and where it is.
type token struct {
	kind       tokKind
	text       string // source text; empty for NEWLINE, INDENT, DEDENT and EOF
	line       int    // 1-based line of the first character
	start, end int    // byte offsets in the source
}

// is reports whether the token is an operator or name with exactly text.
func (t token) is(text string) bool {
	return (t.kind == tOp || t.kind == tName) && t.text == text
}

// lexDiag is one problem the scanner found, at a line.
type lexDiag struct {
	line int
	msg  string
}

// lexer holds the scanning state of one source file.
type lexer struct {
	src       string
	pos       int
	line      int
	toks      []token
	indents   []int // indentation columns of the open blocks; [0] is 0
	brackets  []int // lines of the open brackets, innermost last
	lineStart bool  // the next character starts a logical line
	diags     []lexDiag
}

// tokenize scans Python source into tokens, ending with EOF. It never
// fails: problems become diagnostics and scanning goes on.
func tokenize(src string) ([]token, []lexDiag) {
	lx := &lexer{src: strings.TrimPrefix(src, "\ufeff"), line: 1, indents: []int{0}, lineStart: true}
	for lx.pos < len(lx.src) {
		if lx.lineStart && len(lx.brackets) == 0 {
			lx.lineStart = false
			if !lx.indentation() {
				continue
			}
		}
		lx.next()
	}
	lx.finish()
	return lx.toks, lx.diags
}

// next scans one token, or skips whitespace, a comment or a continuation.
func (lx *lexer) next() {
	c := lx.src[lx.pos]
	switch {
	case c == ' ' || c == '\t' || c == '\f':
		lx.pos++
	case c == '#':
		for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' && lx.src[lx.pos] != '\r' {
			lx.pos++
		}
	case c == '\\' && lx.atNewline(lx.pos+1):
		lx.pos++
		lx.skipNewline()
	case c == '\n' || c == '\r':
		lx.newline()
	case isIdentStart(c):
		lx.name()
	case isDigit(c) || (c == '.' && lx.pos+1 < len(lx.src) && isDigit(lx.src[lx.pos+1])):
		lx.number()
	case c == '"' || c == '\'':
		lx.str(lx.pos)
	default:
		lx.op()
	}
}

// emit appends a token spanning src[start:lx.pos].
func (lx *lexer) emit(kind tokKind, start, line int) {
	lx.toks = append(lx.toks, token{kind: kind, text: lx.src[start:lx.pos], line: line, start: start, end: lx.pos})
}

// atNewline reports whether a line break starts at offset i.
func (lx *lexer) atNewline(i int) bool {
	return i < len(lx.src) && (lx.src[i] == '\n' || lx.src[i] == '\r')
}

// skipNewline consumes one line break ("\n", "\r\n" or "\r").
func (lx *lexer) skipNewline() {
	if lx.src[lx.pos] == '\r' && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] == '\n' {
		lx.pos++
	}
	lx.pos++
	lx.line++
}

// newline ends a logical line unless brackets are open (implicit joining)
// or the line held no token.
func (lx *lexer) newline() {
	if len(lx.brackets) == 0 && lx.openLine() {
		lx.toks = append(lx.toks, token{kind: tNewline, line: lx.line, start: lx.pos, end: lx.pos})
	}
	lx.skipNewline()
	lx.lineStart = len(lx.brackets) == 0
}

// openLine reports whether tokens were emitted since the last NEWLINE.
func (lx *lexer) openLine() bool {
	if len(lx.toks) == 0 {
		return false
	}
	k := lx.toks[len(lx.toks)-1].kind
	return k != tNewline && k != tIndent && k != tDedent
}

// name scans an identifier or keyword, or a prefixed string literal.
func (lx *lexer) name() {
	start := lx.pos
	for lx.pos < len(lx.src) && isIdentChar(lx.src[lx.pos]) {
		lx.pos++
	}
	if lx.pos < len(lx.src) && (lx.src[lx.pos] == '"' || lx.src[lx.pos] == '\'') &&
		stringPrefixes[strings.ToLower(lx.src[start:lx.pos])] {
		lx.str(start)
		return
	}
	lx.emit(tName, start, lx.line)
}

// number scans a numeric literal, loosely: digits, letters, "_", "." and
// an exponent sign.
func (lx *lexer) number() {
	start := lx.pos
	lx.pos++
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		prev := lx.src[lx.pos-1]
		sign := (c == '+' || c == '-') && (prev == 'e' || prev == 'E') && !strings.ContainsAny(lx.src[start:lx.pos], "xX")
		if !isIdentChar(c) && c != '.' && !sign {
			break
		}
		lx.pos++
	}
	lx.emit(tNumber, start, lx.line)
}

// isIdentStart reports whether c can begin an identifier (non-ASCII bytes
// are taken as letters).
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// isIdentChar reports whether c can continue an identifier.
func isIdentChar(c byte) bool { return isIdentStart(c) || isDigit(c) }

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
