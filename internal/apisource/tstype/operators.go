package tstype

import "github.com/spagu/ssg/internal/apimodel"

// parseType parses a full type: a union, optionally the check type of a
// conditional `A extends B ? C : D`, which is kept verbatim.
func (p *parser) parseType() *apimodel.TypeRef {
	defer p.enter()()
	start := p.pos
	t := p.parseUnion()
	if !p.peek().is("extends") {
		return t
	}
	p.advance()
	p.parseUnion() // the extends type may not itself be a conditional
	p.expect("?")
	p.parseType()
	p.expect(":")
	p.parseType()
	return p.verbatim(start)
}

// parseUnion parses `A | B | C`, with an optional leading bar.
func (p *parser) parseUnion() *apimodel.TypeRef {
	return p.parseList("|", apimodel.TypeUnion, p.parseIntersection)
}

// parseIntersection parses `A & B & C`, with an optional leading ampersand.
func (p *parser) parseIntersection() *apimodel.TypeRef {
	return p.parseList("&", apimodel.TypeIntersection, p.parseOperator)
}

// parseList parses operands separated by op; one operand is returned as is.
func (p *parser) parseList(op string, kind apimodel.TypeKind, operand func() *apimodel.TypeRef) *apimodel.TypeRef {
	p.accept(op)
	members := []*apimodel.TypeRef{operand()}
	for p.accept(op) {
		members = append(members, operand())
	}
	if len(members) == 1 {
		return members[0]
	}
	return &apimodel.TypeRef{Kind: kind, Args: members}
}

// parseOperator parses the prefix type operators. keyof, unique and infer
// are kept verbatim; readonly is dropped (readonly T[] stays an array); the
// JSDoc prefixes ?T (nullable) and !T (non-null) are handled in jsdoc.go.
func (p *parser) parseOperator() *apimodel.TypeRef {
	defer p.enter()()
	start := p.pos
	t := p.peek()
	switch {
	case t.is("?") || t.is("!"):
		return p.parseJSDocPrefix()
	case t.is("readonly") && startsOperand(p.peekAt(1)):
		p.advance()
		return p.parseOperator()
	case (t.is("keyof") || t.is("unique")) && startsOperand(p.peekAt(1)):
		p.advance()
		p.parseOperator()
		return p.verbatim(start)
	case t.is("infer") && p.peekAt(1).kind == tokIdent:
		p.advance()
		p.advance()
		if p.accept("extends") {
			p.parseUnion()
		}
		return p.verbatim(start)
	}
	return p.parsePostfix()
}

// startsOperand reports whether t can begin a type, so that a keyword used
// as a plain name (a type called "readonly") still parses as a name.
func startsOperand(t token) bool {
	switch t.kind {
	case tokIdent, tokNumber, tokString, tokTemplate:
		return true
	case tokPunct:
		for _, s := range []string{"(", "[", "{", "<", "-", "*", "?", "!"} {
			if t.text == s {
				return true
			}
		}
	}
	return false
}

// parsePostfix parses array suffixes T[] and indexed access T[K]; an
// indexed access is kept verbatim, an array wraps its element.
func (p *parser) parsePostfix() *apimodel.TypeRef {
	start := p.pos
	t := p.parsePrimary()
	for p.peek().is("[") {
		p.advance()
		if p.accept("]") {
			t = &apimodel.TypeRef{Kind: apimodel.TypeArray, Args: []*apimodel.TypeRef{t}}
			continue
		}
		p.parseType()
		p.expect("]")
		t = p.verbatim(start)
	}
	return t
}
