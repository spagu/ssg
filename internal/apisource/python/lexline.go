package python

import "strings"

// tabSize is how far a tab advances the indentation column (Python uses 8).
const tabSize = 8

// indentation measures the indentation of a new logical line and emits
// INDENT or DEDENT tokens. A blank or comment-only line is consumed whole
// and reported as false: it does not take part in indentation.
func (lx *lexer) indentation() bool {
	col, p := lx.measure()
	lx.pos = p
	if p >= len(lx.src) || lx.src[p] == '#' || lx.atNewline(p) {
		for lx.pos < len(lx.src) && !lx.atNewline(lx.pos) {
			lx.pos++
		}
		if lx.pos < len(lx.src) {
			lx.skipNewline()
		}
		lx.lineStart = true
		return false
	}
	lx.indent(col)
	return true
}

// measure returns the indentation column of the line at lx.pos and the
// offset of its first non-blank character.
func (lx *lexer) measure() (col, p int) {
	for p = lx.pos; p < len(lx.src); p++ {
		switch lx.src[p] {
		case ' ':
			col++
		case '\t':
			col = (col/tabSize + 1) * tabSize
		case '\f':
			col = 0
		default:
			return col, p
		}
	}
	return col, p
}

// indent compares a line's column with the open blocks.
func (lx *lexer) indent(col int) {
	top := lx.indents[len(lx.indents)-1]
	if col > top {
		lx.indents = append(lx.indents, col)
		lx.toks = append(lx.toks, token{kind: tIndent, line: lx.line, start: lx.pos, end: lx.pos})
		return
	}
	for col < lx.indents[len(lx.indents)-1] {
		lx.indents = lx.indents[:len(lx.indents)-1]
		lx.toks = append(lx.toks, token{kind: tDedent, line: lx.line, start: lx.pos, end: lx.pos})
	}
	if col != lx.indents[len(lx.indents)-1] {
		lx.diags = append(lx.diags, lexDiag{lx.line, "inconsistent dedent: the line matches no enclosing block"})
		lx.indents = append(lx.indents, col)
	}
}

// operators are the multi-character operators, longest first.
var operators = []string{
	"**=", "//=", ">>=", "<<=", "...", "->", "**", "//", "==", "!=", "<=", ">=", ":=",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "@=", "<<", ">>",
}

// op scans an operator or bracket, tracking bracket nesting.
func (lx *lexer) op() {
	start := lx.pos
	for _, o := range operators {
		if strings.HasPrefix(lx.src[lx.pos:], o) {
			lx.pos += len(o)
			lx.emit(tOp, start, lx.line)
			return
		}
	}
	switch c := lx.src[lx.pos]; c {
	case '(', '[', '{':
		lx.brackets = append(lx.brackets, lx.line)
	case ')', ']', '}':
		if len(lx.brackets) == 0 {
			lx.diags = append(lx.diags, lexDiag{lx.line, "unmatched closing bracket " + string(c)})
		} else {
			lx.brackets = lx.brackets[:len(lx.brackets)-1]
		}
	}
	lx.pos++
	lx.emit(tOp, start, lx.line)
}

// finish reports brackets left open, closes the last logical line and
// every open block, and appends EOF.
func (lx *lexer) finish() {
	if len(lx.brackets) > 0 {
		lx.diags = append(lx.diags, lexDiag{lx.brackets[0], "unterminated bracket: opened here and never closed"})
	}
	if lx.openLine() {
		lx.toks = append(lx.toks, token{kind: tNewline, line: lx.line, start: lx.pos, end: lx.pos})
	}
	for range lx.indents[1:] {
		lx.toks = append(lx.toks, token{kind: tDedent, line: lx.line, start: lx.pos, end: lx.pos})
	}
	lx.toks = append(lx.toks, token{kind: tEOF, line: lx.line, start: lx.pos, end: lx.pos})
}
