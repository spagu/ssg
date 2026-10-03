package tstype

import "github.com/spagu/ssg/internal/apimodel"

// parseObject parses an object type literal `{ a: A; b?: B }` into a
// TypeObject. Members may be separated by `;`, `,` or a line break. A
// mapped type `{ [K in keyof T]: V }` is kept verbatim.
func (p *parser) parseObject() *apimodel.TypeRef {
	if p.mappedAhead() {
		return p.parseMapped()
	}
	p.expect("{")
	obj := &apimodel.TypeRef{Kind: apimodel.TypeObject}
	for !p.peek().is("}") {
		obj.Fields = append(obj.Fields, p.parseMember())
		if !p.accept(";") && !p.accept(",") && !p.newlineBefore() {
			break
		}
	}
	p.expect("}")
	return obj
}

// newlineBefore reports whether a line break separates the previous token
// from the current one, which ends a member like a semicolon would.
func (p *parser) newlineBefore() bool {
	prev, cur := p.toks[p.pos-1], p.peek()
	for _, c := range p.source[prev.end:cur.start] {
		if c == '\n' || c == '\r' {
			return true
		}
	}
	return false
}

// mappedAhead reports whether the `{` opens a mapped type:
// `{ [+-]readonly? [K in ...`.
func (p *parser) mappedAhead() bool {
	i := 1
	if p.peekAt(i).is("+") || p.peekAt(i).is("-") {
		i++
	}
	if p.peekAt(i).is("readonly") {
		i++
	}
	return p.peekAt(i).is("[") && p.peekAt(i+1).kind == tokIdent && p.peekAt(i+2).is("in")
}

// parseMapped parses `{ readonly [K in T as N]-?: V }` and returns it verbatim.
func (p *parser) parseMapped() *apimodel.TypeRef {
	start := p.pos
	p.expect("{")
	p.acceptModifier("readonly")
	p.expect("[")
	p.expectIdent()
	p.expect("in")
	p.parseType()
	if p.accept("as") {
		p.parseType()
	}
	p.expect("]")
	p.acceptModifier("?")
	if p.accept(":") {
		p.parseType()
	}
	p.accept(";")
	p.expect("}")
	return p.verbatim(start)
}

// acceptModifier consumes an optional +/- followed by the modifier.
func (p *parser) acceptModifier(mod string) {
	if p.accept("+") || p.accept("-") {
		p.expect(mod)
		return
	}
	p.accept(mod)
}

// parseMember parses one member: property, method, index signature, call or
// construct signature, or accessor. A property without a type (JSDoc
// `{a, b}`) is unknown.
func (p *parser) parseMember() *apimodel.Param {
	t, next := p.peek(), p.peekAt(1)
	switch {
	case t.is("(") || t.is("<"):
		return &apimodel.Param{Name: "()", Type: p.parseMethod()}
	case t.is("new") && (next.is("(") || next.is("<")):
		p.advance()
		return &apimodel.Param{Name: "new", Type: p.parseMethod()}
	case t.is("readonly") && startsName(next):
		p.advance()
		return p.parseMember()
	case (t.is("get") || t.is("set")) && startsName(next):
		return p.parseAccessor()
	}
	field := &apimodel.Param{Name: p.parseMemberName()}
	field.Optional = p.accept("?")
	switch {
	case p.peek().is("(") || p.peek().is("<"):
		field.Type = p.parseMethod()
	case p.accept(":"):
		field.Type = p.parseType()
	default:
		field.Type = builtin("unknown")
	}
	return field
}

// startsName reports whether t can begin a member name.
func startsName(t token) bool {
	return t.kind == tokIdent || t.kind == tokString || t.kind == tokNumber || t.is("[")
}

// parseMemberName parses an identifier, string or number key, an index
// signature `[key: string]` or a computed key `[Symbol.iterator]`; bracketed
// names keep their normalized text.
func (p *parser) parseMemberName() string {
	if !p.peek().is("[") {
		if !startsName(p.peek()) {
			p.fail("expected a member name, found %q", p.peek().text)
		}
		return p.advance().text
	}
	start := p.pos
	if p.peekAt(1).kind == tokIdent && p.peekAt(2).is(":") {
		p.advance()
		p.advance()
		p.advance()
		p.parseType()
		p.expect("]")
	} else {
		p.skipBalanced()
	}
	return p.text(start, p.pos)
}

// parseMethod parses `<T>(a: A): R` after a member name; a missing return
// type is `any`, as in TypeScript.
func (p *parser) parseMethod() *apimodel.TypeRef {
	sig := &apimodel.Signature{}
	defer p.pushScope()()
	if p.peek().is("<") {
		sig.TypeParams = p.parseTypeParams()
	}
	sig.Params = p.parseParams()
	sig.Returns = builtin("any")
	if p.accept(":") {
		sig.Returns = p.parseType()
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeFunction, Signature: sig}
}

// parseAccessor parses `get x(): T` and `set x(v: T)` as a property x of
// type T.
func (p *parser) parseAccessor() *apimodel.Param {
	getter := p.advance().text == "get"
	field := &apimodel.Param{Name: p.parseMemberName()}
	sig := p.parseMethod().Signature
	field.Type = sig.Returns
	if !getter && len(sig.Params) > 0 {
		field.Type = sig.Params[0].Type
	}
	return field
}
