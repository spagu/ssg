package dts

// parseSignature reads `<T>(a: A, b?: B, ...c: C[]): R`; the return type
// may be absent (a setter, a constructor).
func (p *parser) parseSignature(doc string, line int) *sigSpan {
	s := &sigSpan{doc: doc, line: line, tparams: p.parseTypeParams()}
	p.expect("(")
	for !p.accept(")") {
		s.params = append(s.params, p.parseParam())
		if !p.accept(",") {
			p.expect(")")
			break
		}
	}
	if p.accept(":") {
		s.returns = p.typeSpan(stopMember)
	}
	return s
}

// paramModifiers are the words a constructor parameter may start with.
var paramModifiers = map[string]bool{
	"public": true, "private": true, "protected": true, "readonly": true, "override": true,
}

// parseParam reads one parameter: `...name?: Type = default`, the name
// possibly a destructuring pattern.
func (p *parser) parseParam() paramSpan {
	for t := p.peek(); t.kind == tokIdent && paramModifiers[t.text] && startsParam(p.peekAt(1)); t = p.peek() {
		p.advance()
	}
	var ps paramSpan
	ps.rest = p.accept("...")
	if t := p.peek(); t.is("{") || t.is("[") {
		ps.name = p.balanced()
	} else {
		ps.name = p.expectName().text
	}
	ps.optional = p.accept("?")
	if p.accept(":") {
		ps.typ = p.typeSpan(stopParam)
	}
	if p.accept("=") {
		ps.def = p.valueSpan()
		ps.optional = true
	}
	return ps
}

// startsParam reports whether t can begin a parameter after a modifier, so
// that a parameter named "readonly" is still read as a name.
func startsParam(t token) bool {
	return t.kind == tokIdent || t.is("{") || t.is("[") || t.is("...")
}

// parseTypeParams reads `<const T extends C = D, …>`, or nothing when the
// next token is not "<".
func (p *parser) parseTypeParams() []tparamSpan {
	if !p.accept("<") {
		return nil
	}
	var out []tparamSpan
	for !p.accept(">") {
		for t := p.peek(); (t.is("const") || t.is("in") || t.is("out")) && p.peekAt(1).kind == tokIdent; t = p.peek() {
			p.advance()
		}
		tp := tparamSpan{name: p.expectName().text}
		if p.accept("extends") {
			tp.constraint = p.typeSpan(stopTParam)
		}
		if p.accept("=") {
			tp.def = p.typeSpan(stopTParam)
		}
		out = append(out, tp)
		if !p.accept(",") {
			p.expect(">")
			break
		}
	}
	return out
}
