package python

// decorators reads the "@..." lines before a def or class.
func (p *parser) decorators() []decorator {
	var out []decorator
	for p.peek(0).is("@") {
		p.i++
		d := decorator{name: p.dottedName()}
		if p.peek(0).is("(") {
			p.i++
			d.args = p.until(")")
			if d.args == nil {
				d.args = []token{}
			}
		}
		out = append(out, d)
		p.skipStatement()
	}
	return out
}

// dottedName reads NAME ("." NAME)*.
func (p *parser) dottedName() string {
	var name string
	for p.peek(0).kind == tName {
		name += p.take().text
		if !p.peek(0).is(".") || p.peek(1).kind != tName {
			break
		}
		name += p.take().text
	}
	return name
}

// until reads the tokens up to the closer matching an opener already
// consumed, and consumes the closer.
func (p *parser) until(closer string) []token {
	start, depth := p.i, 0
	for t := p.peek(0); t.kind != tEOF; t = p.peek(0) {
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case depth == 0 && t.is(closer):
			p.i++
			return p.toks[start : p.i-1]
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
		}
		p.i++
	}
	return p.toks[start:p.i]
}

// def reads "def name[T](params) -> R:" and its body's docstring.
func (p *parser) def(decos []decorator, async bool) *stmt {
	kw := p.take()
	s := &stmt{kind: sDef, line: kw.line, decorators: decos, async: async}
	if p.peek(0).kind != tName {
		p.skipStatement()
		return nil
	}
	s.name = p.take().text
	s.typeParams = p.typeParams()
	if p.peek(0).is("(") {
		p.i++
		s.params = splitTop(p.until(")"), ",")
	}
	if p.peek(0).is("->") {
		p.i++
	}
	s.returns = p.upTo(":")
	s.doc, _ = p.body()
	return s
}

// class reads "class Name[T](bases):" and its body.
func (p *parser) class(decos []decorator) *stmt {
	kw := p.take()
	s := &stmt{kind: sClass, line: kw.line, decorators: decos}
	if p.peek(0).kind != tName {
		p.skipStatement()
		return nil
	}
	s.name = p.take().text
	s.typeParams = p.typeParams()
	if p.peek(0).is("(") {
		p.i++
		s.bases = splitTop(p.until(")"), ",")
	}
	p.upTo(":")
	s.doc, s.body = p.body()
	return s
}

// typeParams reads a PEP 695 "[T, U: Bound]" list, if there is one.
func (p *parser) typeParams() [][]token {
	if !p.peek(0).is("[") {
		return nil
	}
	p.i++
	return splitTop(p.until("]"), ",")
}

// upTo reads tokens up to an operator at bracket depth zero (consumed) or
// the end of the logical line (not consumed).
func (p *parser) upTo(op string) []token {
	start, depth := p.i, 0
	for t := p.peek(0); t.kind != tEOF && t.kind != tNewline; t = p.peek(0) {
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
		case depth == 0 && t.is(op):
			p.i++
			return p.toks[start : p.i-1]
		}
		p.i++
	}
	return p.toks[start:p.i]
}

// body reads what follows a compound statement's ":" — an indented block
// or a one-line body — and returns its docstring and declarations. The
// ":" is already consumed.
func (p *parser) body() (string, []*stmt) {
	if p.peek(0).kind == tNewline {
		p.i++
		if p.peek(0).kind != tIndent {
			return "", nil
		}
		p.i++
		return p.block(true)
	}
	doc := ""
	if t := p.peek(0); t.kind == tString && (p.peek(1).kind == tNewline || p.peek(1).is(";")) {
		doc = decodeString(t.text)
	}
	p.skipStatement()
	return doc, nil
}

// typeStatement reads "type Name[T] = value" (PEP 695).
func (p *parser) typeStatement() *stmt {
	kw := p.take()
	s := &stmt{kind: sTypeAlias, line: kw.line, name: p.take().text}
	s.typeParams = p.typeParams()
	p.upTo("=")
	s.value = p.upTo(";")
	p.endSimple()
	return s
}

// skipStatement moves past the current logical line and, when it opened
// a block, the whole block.
func (p *parser) skipStatement() {
	for t := p.peek(0); t.kind != tEOF && t.kind != tNewline; t = p.peek(0) {
		p.i++
	}
	p.take()
	if p.peek(0).kind == tIndent {
		p.skipBlock()
	}
}

// skipBlock moves past an indented block, from its INDENT to the matching
// DEDENT.
func (p *parser) skipBlock() {
	depth := 0
	for t := p.take(); t.kind != tEOF; t = p.take() {
		switch t.kind {
		case tIndent:
			depth++
		case tDedent:
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

// endSimple ends a simple statement: after a ";" (consumed by upTo) the next statement on the
// line is read as its own; otherwise the rest of the line is skipped.
func (p *parser) endSimple() {
	if p.i > 0 && p.toks[p.i-1].is(";") {
		return
	}
	p.skipStatement()
}
