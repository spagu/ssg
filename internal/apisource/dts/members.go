package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// memberModifiers are the words that may precede a member name.
var memberModifiers = map[string]func(*modSet){
	"public":    func(*modSet) {},
	"declare":   func(*modSet) {},
	"override":  func(*modSet) {},
	"accessor":  func(*modSet) {},
	"async":     func(*modSet) {},
	"private":   func(m *modSet) { m.private = true },
	"protected": func(m *modSet) { m.protected = true },
	"static":    func(m *modSet) { m.static = true },
	"readonly":  func(m *modSet) { m.readonly = true },
	"abstract":  func(m *modSet) { m.abstract = true },
}

// parseMember reads one member of a class or interface. It returns nil for
// a member that has no name a reader could use (a #private field).
func (p *parser) parseMember(inClass bool) *node {
	first := p.peek()
	n := &node{doc: first.doc, line: first.line}
	for t := p.peek(); t.kind == tokIdent && memberModifiers[t.text] != nil && startsMemberName(p.peekAt(1)) && !p.peekAt(1).nl; t = p.peek() {
		memberModifiers[t.text](&n.mods)
		p.advance()
	}
	t := p.peek()
	n.line = t.line
	switch {
	case t.kind == tokIdent && strings.HasPrefix(t.text, "#"):
		p.advance()
		p.skipMemberRest()
		return nil
	case t.is("constructor") && startsSignature(p.peekAt(1)), !inClass && t.is("new") && startsSignature(p.peekAt(1)):
		p.advance()
		n.kind, n.name = apimodel.KindConstructor, "constructor"
		n.sigs = []*sigSpan{p.parseSignature(first.doc, t.line)}
	case startsSignature(t):
		n.kind, n.name = apimodel.KindMethod, "()"
		n.sigs = []*sigSpan{p.parseSignature(first.doc, t.line)}
	case (t.is("get") || t.is("set")) && startsMemberName(p.peekAt(1)):
		p.parseAccessor(n)
	case t.is("[") && p.peekAt(1).kind == tokIdent && p.peekAt(2).is(":"):
		n.kind, n.name = apimodel.KindProperty, p.balanced()
		p.expect(":")
		n.typ = p.typeSpan(stopMember)
	default:
		p.parseNamedMember(n)
	}
	p.skipBody()
	p.memberEnd()
	return n
}

// parseAccessor reads `get name(): T` or `set name(v: T)`.
func (p *parser) parseAccessor(n *node) {
	getter := p.advance().is("get")
	n.kind, n.name = apimodel.KindAccessor, p.memberName()
	sig := p.parseSignature("", n.line)
	n.mods.getter, n.mods.setter = getter, !getter
	n.typ = sig.returns
	if !getter && len(sig.params) > 0 {
		n.typ = sig.params[0].typ
	}
}

// parseNamedMember reads a property `name?: T` or a method `name?<T>(…): R`.
func (p *parser) parseNamedMember(n *node) {
	n.name = p.memberName()
	n.mods.optional = p.accept("?")
	p.accept("!")
	switch {
	case startsSignature(p.peek()):
		n.kind = apimodel.KindMethod
		n.sigs = []*sigSpan{p.parseSignature(n.doc, n.line)}
	case p.accept(":"):
		n.kind = apimodel.KindProperty
		n.typ = p.typeSpan(stopMember)
	default:
		n.kind = apimodel.KindProperty
	}
	if p.accept("=") {
		p.valueSpan()
	}
}

// memberName reads a member name: an identifier, a quoted or numeric name,
// or a computed one such as [Symbol.iterator].
func (p *parser) memberName() string {
	if p.peek().is("[") {
		return p.balanced()
	}
	return unquote(p.expectName().text)
}

// startsMemberName reports whether t can begin a member name, so that a
// member called "static" or "get" is not taken for a modifier.
func startsMemberName(t token) bool {
	return t.kind == tokIdent || t.kind == tokString || t.kind == tokNumber || t.is("[")
}

// startsSignature reports whether t opens a call signature.
func startsSignature(t token) bool { return t.is("(") || t.is("<") }

// skipMemberRest skips what is left of a member up to its separator.
func (p *parser) skipMemberRest() {
	for t := p.peek(); t.kind != tokEOF && !t.is(";") && !t.is("}") && !t.nl; t = p.peek() {
		if t.is("{") || t.is("(") || t.is("[") {
			p.balanced()
			continue
		}
		p.advance()
	}
}
