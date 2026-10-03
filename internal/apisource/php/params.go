package php

import "strings"

// promotionModifiers mark a constructor parameter that is also a property.
var promotionModifiers = map[string]bool{"public": true, "protected": true, "private": true, "readonly": true}

// signature reads a parameter list and return type after a function's
// name, then skips its body (or the ";" of an abstract one). begin is the
// first token of the declaration, for its code.
func (p *parser) signature(begin int) *sigDecl {
	sd := &sigDecl{}
	if p.peek(0).is("(") {
		p.next()
		sd.params = p.params()
	}
	if p.peek(0).is(":") {
		p.next()
		from := p.i
		for k := p.peek(0); k.kind != tokEOF && !k.is("{") && !k.is(";"); k = p.peek(0) {
			p.next()
		}
		sd.returns = p.typeText(from, p.i)
	}
	sd.code = p.code(begin, p.i)
	switch {
	case p.peek(0).is("{"):
		p.next()
		p.skipBlock()
	case p.peek(0).is(";"):
		p.next()
	}
	return sd
}

// params reads the parameters up to the ")" closing the list.
func (p *parser) params() []*paramDecl {
	var out []*paramDecl
	add := func(from, to int) {
		if from < to {
			out = append(out, p.param(from, to))
		}
	}
	from := p.i
	for depth := 0; ; {
		t := p.next()
		switch {
		case t.kind == tokEOF:
			return out
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			if depth == 0 {
				add(from, p.i-1)
				return out
			}
			depth--
		case t.is(",") && depth == 0:
			add(from, p.i-1)
			from = p.i
		}
	}
}

// param reads one parameter from tokens [from, to):
// [#[Attr]] [modifiers] [Type] [&] [...]$name [= default].
func (p *parser) param(from, to int) *paramDecl {
	pd := &paramDecl{line: p.toks[from].line, code: p.code(from, to)}
	i := from
	for i < to && p.toks[i].kind == tokAttr {
		i++
	}
	vis := ""
	for i < to && p.toks[i].kind == tokIdent && promotionModifiers[strings.ToLower(p.toks[i].text)] {
		pd.promoted = true
		switch w := strings.ToLower(p.toks[i].text); {
		case w == "readonly":
			pd.readonly = true
		case i+1 < to && p.toks[i+1].is("("):
			i += 3 // private(set): who may write, not who may read
		default:
			vis = w
		}
		i++
	}
	pd.public = pd.promoted && (vis == "" || vis == "public")
	typeFrom := i
	for i < to && !p.paramNameAt(i, to) {
		i++
	}
	pd.typ = p.typeText(typeFrom, i)
	if i < to && p.toks[i].is("&") {
		i++
	}
	if i < to && p.toks[i].is("...") {
		pd.rest = true
		i++
	}
	if i < to && p.toks[i].kind == tokVar {
		pd.name, pd.line = p.toks[i].text[1:], p.toks[i].line
		i++
	}
	if i < to && p.toks[i].is("=") {
		pd.def = p.code(i+1, to)
	}
	return pd
}

// paramNameAt reports whether the parameter's name part starts at i: its
// variable, "...", or a by-reference "&" right before either. An "&"
// between two types is an intersection.
func (p *parser) paramNameAt(i, to int) bool {
	t := p.toks[i]
	if t.kind == tokVar || t.is("...") || t.is("=") {
		return true
	}
	return t.is("&") && i+1 < to && (p.toks[i+1].kind == tokVar || p.toks[i+1].is("..."))
}
