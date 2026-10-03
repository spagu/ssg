package tstype

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// maxDepth bounds nesting so hostile input cannot exhaust the stack.
const maxDepth = 200

// parseError carries a syntax error through the recursive descent; Parse
// recovers it at the boundary, so no other panic escapes the package.
type parseError struct{ err error }

// parser is a recursive-descent parser over a token slice.
type parser struct {
	source  string // the trimmed source the token spans index into
	toks    []token
	pos     int
	depth   int
	resolve Resolver
	scope   []string // type parameters in scope, innermost last
}

// Parse parses a type expression into a TypeRef tree. resolve may be nil.
// On a syntax error it returns a TypeVerbatim node holding the trimmed source
// text AND a non-nil error (callers keep the verbatim node and report the error).
func Parse(expr string, resolve Resolver) (*apimodel.TypeRef, error) {
	src := strings.TrimSpace(expr)
	t, err := parse(src, resolve)
	if err != nil {
		return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: src}, err
	}
	return t, nil
}

// parse runs the lexer and the parser, turning a parseError panic into an error.
func parse(src string, resolve Resolver) (t *apimodel.TypeRef, err error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{source: src, toks: toks, resolve: resolve}
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(parseError)
			if !ok {
				panic(r)
			}
			t, err = nil, pe.err
		}
	}()
	if p.peek().kind == tokEOF {
		p.fail("empty type expression")
	}
	t = p.parseType()
	p.accept("=") // JSDoc optional marker: T=
	for p.accept(";") {
	}
	if p.peek().kind != tokEOF {
		p.fail("unexpected %q", p.peek().text)
	}
	return t, nil
}

// fail aborts parsing with a positioned syntax error.
func (p *parser) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(parseError{fmt.Errorf("type syntax error at offset %d: %w", p.peek().start, errors.New(msg))})
}

// peek returns the current token; peekAt looks further ahead.
func (p *parser) peek() token { return p.peekAt(0) }

// peekAt returns the token n places ahead, or EOF past the end.
func (p *parser) peekAt(n int) token {
	if i := p.pos + n; i < len(p.toks) {
		return p.toks[i]
	}
	return p.toks[len(p.toks)-1]
}

// advance consumes and returns the current token (never past EOF).
func (p *parser) advance() token {
	t := p.peek()
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

// accept consumes the token when it is s.
func (p *parser) accept(s string) bool {
	if p.peek().is(s) {
		p.pos++
		return true
	}
	return false
}

// expect consumes s or fails.
func (p *parser) expect(s string) {
	if !p.accept(s) {
		p.fail("expected %q, found %q", s, p.peek().text)
	}
}

// expectIdent consumes an identifier and returns its text.
func (p *parser) expectIdent() string {
	if p.peek().kind != tokIdent {
		p.fail("expected a name, found %q", p.peek().text)
	}
	return p.advance().text
}

// enter guards recursion depth; the returned func restores it.
func (p *parser) enter() func() {
	if p.depth++; p.depth > maxDepth {
		p.fail("type nested too deeply")
	}
	return func() { p.depth-- }
}

// verbatim builds a TypeVerbatim node from the tokens consumed since start,
// joined with single spaces where the source had whitespace.
func (p *parser) verbatim(start int) *apimodel.TypeRef {
	return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: p.text(start, p.pos)}
}

// text renders tokens [from, to) with normalized spacing.
func (p *parser) text(from, to int) string {
	var b strings.Builder
	for i := from; i < to; i++ {
		if i > from && p.toks[i].start > p.toks[i-1].end {
			b.WriteByte(' ')
		}
		b.WriteString(p.toks[i].text)
	}
	return b.String()
}

// skipBalanced consumes a bracketed group starting at the current opener.
func (p *parser) skipBalanced() {
	depth := 0
	for {
		t := p.advance()
		switch {
		case t.kind == tokEOF:
			p.fail("unbalanced brackets")
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			if depth--; depth == 0 {
				return
			}
		}
	}
}
