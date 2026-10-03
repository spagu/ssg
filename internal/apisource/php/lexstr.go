package php

import "strings"

// lineComment skips a // or # comment, which ends at the end of the line
// or just before a closing ?>.
func (lx *lexer) lineComment() {
	end := len(lx.src)
	if i := strings.IndexByte(lx.src[lx.pos:], '\n'); i >= 0 {
		end = lx.pos + i
	}
	if i := strings.Index(lx.src[lx.pos:end], "?>"); i >= 0 {
		end = lx.pos + i
	}
	lx.advance(end)
}

// blockComment skips a /* */ comment, or records a /** */ docblock.
func (lx *lexer) blockComment() error {
	i := strings.Index(lx.src[lx.pos+2:], "*/")
	if i < 0 {
		return &lexError{line: lx.line, msg: "unterminated comment"}
	}
	end := lx.pos + 2 + i + 2
	body := lx.src[lx.pos+2 : end-2]
	if strings.HasPrefix(body, "*") && len(body) > 1 {
		lx.emitText(tokDoc, end, body[1:])
		return nil
	}
	lx.advance(end)
	return nil
}

// quoted scans a string delimited by q ('…', "…" or `…`). A backslash
// escapes the next byte, which covers \' and \" and \\.
func (lx *lexer) quoted(q byte) error {
	end, ok := skipQuoted(lx.src, lx.pos)
	if !ok {
		return &lexError{line: lx.line, msg: "unterminated string " + string(q)}
	}
	lx.emit(tokString, end)
	return nil
}

// skipQuoted returns the offset just past the string opening at src[i],
// and false when it is never closed.
func skipQuoted(src string, i int) (int, bool) {
	q := src[i]
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case q:
			return j + 1, true
		}
	}
	return 0, false
}

// heredoc scans <<<ID … ID, <<<"ID" … ID and the nowdoc <<<'ID' … ID. The
// closing identifier may be indented (PHP 7.3) and must not be followed
// by a name character. "<<<" without an identifier is an operator.
func (lx *lexer) heredoc() error {
	i := lx.pos + 3
	for i < len(lx.src) && (lx.src[i] == ' ' || lx.src[i] == '\t') {
		i++
	}
	quote := byte(0)
	if i < len(lx.src) && (lx.src[i] == '"' || lx.src[i] == '\'') {
		quote = lx.src[i]
		i++
	}
	nameEnd := i
	for nameEnd < len(lx.src) && isNameChar(lx.src[nameEnd]) {
		nameEnd++
	}
	id := lx.src[i:nameEnd]
	if quote != 0 && nameEnd < len(lx.src) && lx.src[nameEnd] == quote {
		nameEnd++
	}
	nl := strings.IndexByte(lx.src[nameEnd:], '\n')
	if id == "" || !isNameStart(id[0]) || nl < 0 || strings.TrimSpace(lx.src[nameEnd:nameEnd+nl]) != "" {
		lx.emit(tokPunct, lx.pos+2)
		return nil
	}
	end, ok := heredocEnd(lx.src, nameEnd+nl+1, id)
	if !ok {
		return &lexError{line: lx.line, msg: "unterminated heredoc " + id}
	}
	lx.emit(tokString, end)
	return nil
}

// heredocEnd finds the line from offset i on that closes a heredoc named
// id, and returns the offset just past the identifier.
func heredocEnd(src string, i int, id string) (int, bool) {
	for i <= len(src) {
		line := src[i:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, id) {
			after := len(line) - len(trimmed) + len(id)
			if after >= len(line) || !isNameChar(line[after]) {
				return i + after, true
			}
		}
		if i+len(line) >= len(src) {
			break
		}
		i += len(line) + 1
	}
	return 0, false
}

// attribute records a whole #[...] attribute. Brackets nest, and strings
// inside it may hold brackets.
func (lx *lexer) attribute() error {
	depth := 0
	for i := lx.pos + 1; i < len(lx.src); i++ {
		switch lx.src[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				lx.emit(tokAttr, i+1)
				return nil
			}
		case '\'', '"':
			end, ok := skipQuoted(lx.src, i)
			if !ok {
				return &lexError{line: lx.line, msg: "unterminated string in attribute"}
			}
			i = end - 1
		}
	}
	return &lexError{line: lx.line, msg: "unterminated attribute"}
}
