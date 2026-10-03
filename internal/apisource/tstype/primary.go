package tstype

import "github.com/spagu/ssg/internal/apimodel"

// parsePrimary parses an operand: a name, literal, tuple, object, function
// type, parenthesized type, type query or type predicate.
func (p *parser) parsePrimary() *apimodel.TypeRef {
	t := p.peek()
	switch t.kind {
	case tokString, tokNumber:
		return literal(p.advance().text)
	case tokTemplate:
		if t.hasSubst {
			p.advance()
			return p.verbatim(p.pos - 1)
		}
		return literal(p.advance().text)
	case tokIdent:
		return p.parseIdentStart()
	}
	switch t.text {
	case "(":
		if p.arrowAhead() {
			return p.parseFunctionType()
		}
		p.advance()
		inner := p.parseType()
		p.expect(")")
		return inner
	case "<":
		return p.parseFunctionType()
	case "[":
		return p.parseTuple()
	case "{":
		return p.parseObject()
	case "-":
		if p.peekAt(1).kind == tokNumber {
			p.advance()
			return literal("-" + p.advance().text)
		}
	case "*":
		p.advance()
		return builtin("any")
	}
	p.fail("unexpected %q", t.text)
	return nil
}

// parseIdentStart parses an operand that begins with an identifier.
func (p *parser) parseIdentStart() *apimodel.TypeRef {
	t, next := p.peek(), p.peekAt(1)
	switch {
	case t.text == "true" || t.text == "false":
		return literal(p.advance().text)
	case t.text == "function" && next.is("("):
		return p.parseJSDocFunction()
	case (t.text == "new" || t.text == "abstract") && (next.is("(") || next.is("<") || next.is("new")):
		return p.parseConstructorType()
	case t.text == "typeof" && next.kind == tokIdent:
		start := p.pos
		p.advance()
		p.parseEntityName()
		if p.peek().is("<") {
			p.parseTypeArgs()
		}
		return p.verbatim(start)
	case next.is("is") || (t.text == "asserts" && next.kind == tokIdent):
		return p.parsePredicate()
	}
	return p.parseTypeReference()
}

// parsePredicate parses `x is T`, `asserts x` and `asserts x is T`, kept verbatim.
func (p *parser) parsePredicate() *apimodel.TypeRef {
	start := p.pos
	if p.peek().text == "asserts" && p.peekAt(1).kind == tokIdent && !p.peekAt(1).is("is") {
		p.advance()
	}
	p.advance()
	if p.accept("is") {
		p.parseType()
	}
	return p.verbatim(start)
}

// parseTypeReference parses Name, ns.Name, Name<A, B> and JSDoc Name.<A>.
func (p *parser) parseTypeReference() *apimodel.TypeRef {
	name := p.parseEntityName()
	var args []*apimodel.TypeRef
	if p.peek().is("<") {
		args = p.parseTypeArgs()
	}
	return p.named(name, args)
}

// parseEntityName parses a dotted name; a dot before `<` (JSDoc Array.<T>)
// is consumed and dropped.
func (p *parser) parseEntityName() string {
	name := p.expectIdent()
	for p.peek().is(".") {
		if p.peekAt(1).is("<") {
			p.advance()
			break
		}
		p.advance()
		name += "." + p.expectIdent()
	}
	return name
}

// parseTypeArgs parses `<A, B>`; `>>` closes two lists because the lexer
// emits every `>` on its own.
func (p *parser) parseTypeArgs() []*apimodel.TypeRef {
	p.expect("<")
	var args []*apimodel.TypeRef
	for {
		args = append(args, p.parseType())
		if !p.accept(",") || p.peek().is(">") {
			break
		}
	}
	p.expect(">")
	return args
}

// parseTuple parses `[A, B]`. Labels are dropped; a tuple with an optional
// (B?) or rest (...B[]) member is kept verbatim, as TypeRef has no
// per-member flags.
func (p *parser) parseTuple() *apimodel.TypeRef {
	start := p.pos
	p.expect("[")
	var members []*apimodel.TypeRef
	plain := true
	for !p.peek().is("]") {
		if p.accept("...") {
			plain = false
		}
		if p.peek().kind == tokIdent && (p.peekAt(1).is(":") || p.peekAt(1).is("?") && p.peekAt(2).is(":")) {
			p.advance() // label
			plain = !p.accept("?") && plain
			p.expect(":")
		}
		members = append(members, p.parseType())
		plain = !p.accept("?") && plain
		if !p.accept(",") {
			break
		}
	}
	p.expect("]")
	if !plain {
		return p.verbatim(start)
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeTuple, Args: members}
}

// literal builds a TypeLiteral holding the text as written.
func literal(text string) *apimodel.TypeRef {
	return &apimodel.TypeRef{Kind: apimodel.TypeLiteral, Name: text}
}
