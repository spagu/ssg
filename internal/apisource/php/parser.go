package php

import "strings"

// parser walks the tokens of one file for declarations.
type parser struct {
	src   string
	file  string
	toks  []token
	i     int // index of the next token
	scope *scope
	decls []*decl
	doc   string   // the docblock waiting for a declaration
	attrs []string // the attributes waiting for a declaration
}

// parseFile reads the declarations of one file. A file the scanner cannot
// read, or whose brackets do not pair up, is an error with its line.
func parseFile(file, src string) ([]*decl, error) {
	toks, err := tokenize(src)
	if err == nil {
		err = checkBalance(toks)
	}
	if err != nil {
		return nil, err
	}
	p := &parser{src: src, file: file, toks: toks, scope: newScope("")}
	p.statements(false)
	return p.decls, nil
}

// peek returns the token n places ahead, or an EOF token past the end.
func (p *parser) peek(n int) token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return token{kind: tokEOF, start: len(p.src), end: len(p.src)}
}

// next consumes and returns the next token.
func (p *parser) next() token {
	t := p.peek(0)
	if p.i < len(p.toks) {
		p.i++
	}
	return t
}

// clear forgets the pending docblock and attributes.
func (p *parser) clear() { p.doc, p.attrs = "", nil }

// skipBlock skips to just past the bracket closing the one just consumed.
func (p *parser) skipBlock() {
	for depth := 1; depth > 0; {
		t := p.next()
		switch {
		case t.kind == tokEOF:
			return
		case t.is("{") || t.is("(") || t.is("["):
			depth++
		case t.is("}") || t.is(")") || t.is("]"):
			depth--
		}
	}
}

// skipStatement skips to just past the next ";" or block at this level.
func (p *parser) skipStatement() {
	for {
		t := p.next()
		switch {
		case t.kind == tokEOF || t.is(";"):
			return
		case t.is("{"):
			p.skipBlock()
			return
		case t.is("(") || t.is("["):
			p.skipBlock()
		}
	}
}

// statements reads top-level statements; inside a braced namespace it
// stops after the closing brace.
func (p *parser) statements(braced bool) {
	for {
		start := p.i
		t := p.next()
		switch {
		case t.kind == tokEOF, braced && t.is("}"):
			return
		case t.kind == tokDoc:
			p.doc = t.text
		case t.kind == tokAttr:
			p.attrs = append(p.attrs, t.text)
		case t.keyword("namespace") && (p.peek(0).kind == tokIdent || p.peek(0).is("{")):
			p.namespace()
		case t.keyword("use") && p.peek(0).kind == tokIdent:
			p.useStatement()
		case t.keyword("function") && p.functionNameAhead():
			p.function(start)
		case t.keyword("const"):
			p.constants(start, nil, nil)
		case p.classAhead(start):
			p.classLike(start)
		default:
			p.clear()
			if t.is("{") || t.is("(") || t.is("[") {
				p.skipBlock()
			}
		}
	}
}

// functionNameAhead reports whether "function" just read declares a named
// function (function name( or function &name() rather than a closure.
func (p *parser) functionNameAhead() bool {
	if p.peek(0).is("&") {
		return p.peek(1).kind == tokIdent
	}
	return p.peek(0).kind == tokIdent
}

// namespace reads "namespace X;" (the rest of the file, or up to the next
// namespace) and "namespace X { ... }".
func (p *parser) namespace() {
	name := ""
	if p.peek(0).kind == tokIdent {
		name = p.next().text
	}
	p.scope = newScope(name)
	p.clear()
	if p.next().is("{") {
		p.statements(true)
		p.scope = newScope("")
	}
}

// useStatement reads class imports: "use A\B;", "use A\B as C, D;" and the
// grouped "use A\{B, C as D};". Function and constant imports are skipped.
func (p *parser) useStatement() {
	p.clear()
	if p.peek(0).keyword("function") || p.peek(0).keyword("const") {
		p.skipStatement()
		return
	}
	prefix := ""
	for {
		t := p.next()
		switch {
		case t.kind == tokEOF || t.is(";"):
			return
		case t.is("}"):
			prefix = ""
		case t.keyword("function") || t.keyword("const"):
			p.next() // a function or constant in a group
		case t.kind == tokIdent && p.peek(0).is("{"):
			prefix = t.text // "A\" of "A\{B, C}"
		case t.kind == tokIdent:
			p.addUse(prefix+t.text, p.aliasAhead())
		}
	}
}

// aliasAhead consumes "as Alias" when it follows, returning the alias.
func (p *parser) aliasAhead() string {
	if p.peek(0).keyword("as") && p.peek(1).kind == tokIdent {
		p.next()
		return p.next().text
	}
	return ""
}

// addUse records an import of a fully qualified class name under alias,
// by default the name's last segment.
func (p *parser) addUse(fq, alias string) {
	fq = strings.Trim(fq, "\\")
	if alias == "" {
		alias = fq[strings.LastIndex(fq, "\\")+1:]
	}
	p.scope.uses[strings.ToLower(alias)] = fq
}
