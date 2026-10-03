package dts

import (
	"fmt"
	"strings"
)

// skipTrivia skips whitespace and comments before the next token, keeping
// the text of the last /** */ comment so the token can carry it.
func (lx *lexer) skipTrivia() error {
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '\n':
			lx.line++
			lx.nl = true
			lx.pos++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			lx.pos++
		case strings.HasPrefix(lx.src[lx.pos:], "\uFEFF"):
			lx.pos += len("\uFEFF")
		case strings.HasPrefix(lx.src[lx.pos:], "//"):
			lx.lineComment()
		case strings.HasPrefix(lx.src[lx.pos:], "/*"):
			if err := lx.blockComment(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

// lineComment skips a // comment up to, not including, the line break.
func (lx *lexer) lineComment() {
	end := strings.IndexByte(lx.src[lx.pos:], '\n')
	if end < 0 {
		lx.pos = len(lx.src)
		return
	}
	lx.pos += end
}

// blockComment skips a /* */ comment; a /** */ one becomes the pending
// documentation comment ("/**/" is an empty plain comment, not documentation).
func (lx *lexer) blockComment() error {
	body := lx.src[lx.pos+2:]
	end := strings.Index(body, "*/")
	if end < 0 {
		return fmt.Errorf("line %d: unterminated comment", lx.line)
	}
	text := body[:end]
	if strings.HasPrefix(text, "*") {
		lx.doc = text[1:]
	}
	n := countLines(text)
	lx.line += n
	if n > 0 {
		lx.nl = true
	}
	lx.pos += 2 + end + 2
	return nil
}
