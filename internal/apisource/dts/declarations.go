package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// parseFunction reads `function name<T>(params): R;`. A default export may
// leave the name out; it is then called "default".
func (p *parser) parseFunction(e *env, m modSet, first token) {
	p.accept("async")
	p.expect("function")
	name := p.optionalName(m)
	sig := p.parseSignature(first.doc, name.line)
	p.skipBody()
	p.endOfStatement()
	declare(e, &node{kind: apimodel.KindFunction, name: name.text, line: name.line, doc: first.doc, sigs: []*sigSpan{sig}, mods: m})
}

// optionalName reads a declaration's name, which only a default export may
// omit.
func (p *parser) optionalName(m modSet) token {
	if t := p.peek(); m.isDefault && t.kind != tokIdent {
		return token{kind: tokIdent, text: "default", line: t.line}
	}
	t := p.expectName()
	t.text = unquote(t.text)
	return t
}

// skipBody skips a { … } block after a signature; declaration files have
// none, but hand-written ones sometimes do.
func (p *parser) skipBody() {
	if p.peek().is("{") {
		p.balanced()
	}
}

// balanced consumes a bracketed group starting at the current token and
// returns its text.
func (p *parser) balanced() string {
	from, depth := p.pos, 0
	for t := p.advance(); t.kind != tokEOF; t = p.advance() {
		if t.kind == tokPunct && strings.Contains("([{", t.text) {
			depth++
		}
		if t.kind == tokPunct && strings.Contains(")]}", t.text) {
			depth--
		}
		if depth == 0 {
			return p.tokenText(from, p.pos)
		}
	}
	p.fail("unbalanced %q", p.toks[from].text)
	return ""
}

// parseAlias reads `type Name<T> = Type;`.
func (p *parser) parseAlias(e *env, m modSet, first token) {
	p.expect("type")
	name := p.expectName()
	n := &node{kind: apimodel.KindType, name: name.text, line: name.line, doc: first.doc, mods: m}
	n.tparams = p.parseTypeParams()
	p.expect("=")
	n.typ = p.typeSpan(stopAlias)
	p.endOfStatement()
	declare(e, n)
}

// parseEnum reads `enum Name { A = 1, B }` (also `const enum`).
func (p *parser) parseEnum(e *env, m modSet, first token) {
	p.accept("const")
	p.expect("enum")
	name := p.expectName()
	n := &node{kind: apimodel.KindEnum, name: name.text, line: name.line, doc: first.doc, mods: m}
	p.expect("{")
	for !p.accept("}") {
		t := p.expectName()
		member := &node{kind: apimodel.KindEnumMember, name: unquote(t.text), line: t.line, doc: t.doc}
		if p.accept("=") {
			member.typ = p.valueSpan()
		}
		n.members = append(n.members, member)
		if !p.accept(",") {
			p.expect("}")
			break
		}
	}
	declare(e, n)
}

// valueSpan consumes an initializer up to a "," or ";" at depth 0, a line
// break after a complete value, or a bracket it did not open, and returns
// its text.
func (p *parser) valueSpan() string {
	from, depth := p.pos, 0
	for t := p.peek(); t.kind != tokEOF; t = p.peek() {
		if depth == 0 && (t.is(",") || t.is(";") || p.pos > from && lineEndsType(p.toks[p.pos-1], t)) {
			break
		}
		if t.kind == tokPunct && strings.Contains("([{", t.text) {
			depth++
		}
		if t.kind == tokPunct && strings.Contains(")]}", t.text) {
			if depth == 0 {
				break
			}
			depth--
		}
		p.advance()
	}
	if p.pos == from {
		p.fail("expected a value, found %s", describe(p.peek()))
	}
	return p.tokenText(from, p.pos)
}

// parseVariables reads `const a: A, b = 1;` (also let and var).
func (p *parser) parseVariables(e *env, m modSet, first token) {
	m.readonly = p.advance().is("const")
	for {
		name := p.expectName()
		n := &node{kind: apimodel.KindVariable, name: name.text, line: name.line, doc: first.doc, mods: m}
		if p.accept(":") {
			n.typ = p.typeSpan(stopMember)
		}
		if p.accept("=") {
			if value := p.valueSpan(); n.typ == "" {
				n.typ = value
			}
		}
		declare(e, n)
		if !p.accept(",") {
			break
		}
	}
	p.endOfStatement()
}
