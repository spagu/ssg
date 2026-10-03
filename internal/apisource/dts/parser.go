package dts

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// syntaxError is raised inside a statement and recovered at the statement
// boundary, so one bad declaration does not lose the rest of the file.
type syntaxError struct {
	line int
	msg  string
}

// parser reads the statements of one declaration file.
type parser struct {
	toks  []token
	pos   int
	file  *file
	diags []apisource.Diagnostic
}

// parseFile parses the source of the file at path. A tokenizer error loses
// the file and is returned; a bad statement becomes a diagnostic and the
// parser carries on with the next one.
func parseFile(path, src string) (*file, []apisource.Diagnostic, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, nil, err
	}
	f := &file{path: path, top: newScope(), imports: map[string]importRef{}, ambients: map[string]*scope{}}
	p := &parser{toks: toks, file: f}
	f.doc = p.fileDoc()
	for top := (&env{scope: f.top, file: f}); ; p.advance() {
		p.parseBody(top)
		if p.peek().kind == tokEOF {
			break
		}
		p.diags = append(p.diags, apisource.Diagnostic{Severity: apisource.Warning, File: path, Line: p.peek().line, Message: "unexpected \"}\""})
	}
	return f, p.diags, nil
}

// fileDoc returns the file's overview comment: a leading /** */ comment
// that names itself with a file-level tag such as @packageDocumentation.
// The comment is then taken off the first token, so it does not document
// the first declaration too.
func (p *parser) fileDoc() string {
	doc := p.toks[0].doc
	for _, tag := range []string{"@packageDocumentation", "@module", "@file", "@fileoverview"} {
		if strings.Contains(doc, tag) {
			p.toks[0].doc = ""
			return doc
		}
	}
	return ""
}

// peek returns the current token.
func (p *parser) peek() token { return p.peekAt(0) }

// peekAt returns the token n places ahead, EOF past the end.
func (p *parser) peekAt(n int) token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return p.toks[len(p.toks)-1]
}

// advance consumes and returns the current token; EOF is never consumed.
func (p *parser) advance() token {
	t := p.peek()
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

// accept consumes the token if it is s.
func (p *parser) accept(s string) bool {
	if p.peek().is(s) {
		p.pos++
		return true
	}
	return false
}

// expect consumes s or raises a syntax error.
func (p *parser) expect(s string) token {
	if !p.peek().is(s) {
		p.fail("expected %q, found %s", s, describe(p.peek()))
	}
	return p.advance()
}

// expectName consumes an identifier, string or number used as a name.
func (p *parser) expectName() token {
	t := p.peek()
	if t.kind != tokIdent && t.kind != tokString && t.kind != tokNumber {
		p.fail("expected a name, found %s", describe(t))
	}
	return p.advance()
}

// fail raises a syntax error at the current token.
func (p *parser) fail(format string, args ...any) {
	panic(syntaxError{line: p.peek().line, msg: fmt.Sprintf(format, args...)})
}

// describe names a token for an error message.
func describe(t token) string {
	if t.kind == tokEOF {
		return "end of file"
	}
	return fmt.Sprintf("%q", t.text)
}

// endOfStatement consumes a closing ";" if present; a line break or a "}"
// ends a statement too.
func (p *parser) endOfStatement() {
	if p.accept(";") || p.peek().is("}") || p.peek().nl || p.peek().kind == tokEOF {
		return
	}
	p.fail("expected \";\", found %s", describe(p.peek()))
}

// unquote strips the quotes of a string token's text.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
