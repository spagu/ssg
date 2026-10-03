package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// span is a half-open range of token indexes.
type span struct{ from, to int }

// declarators reads the comma-separated declarators of a constant or
// property statement up to its ";". A property hook block ({ get => … })
// ends the statement without one.
func (p *parser) declarators() []span {
	var out []span
	from := p.i
	for {
		t := p.next()
		switch {
		case t.kind == tokEOF:
			return append(out, span{from, p.i})
		case t.is(",") || t.is(";"):
			out = append(out, span{from, p.i - 1})
			if t.is(";") {
				return out
			}
			from = p.i
		case t.is("{"):
			out = append(out, span{from, p.i - 1})
			p.skipBlock()
			return out
		case t.is("(") || t.is("["):
			p.skipBlock()
		}
	}
}

// constants reads "const [Type] A = 1, B = 2;" whose first modifier or
// keyword is at start: top-level constants when d is nil, class constants
// otherwise.
func (p *parser) constants(start int, d *decl, mods *memberMods) {
	doc, attrs := p.doc, p.attrs
	p.clear()
	kwEnd := p.i
	spans := p.declarators()
	if d != nil && !mods.public(d) {
		return
	}
	prefix, typ := "", ""
	for k, s := range spans {
		eq := s.from
		for eq < s.to && !p.toks[eq].is("=") {
			eq++
		}
		name := eq - 1
		if eq == s.to || name < s.from {
			continue
		}
		if k == 0 {
			prefix, typ = p.code(start, name), p.typeText(kwEnd, name)
		}
		code := prefix + " " + p.code(name, s.to)
		line := p.toks[name].line
		if k == 0 {
			line = p.toks[start].line
		}
		if d == nil {
			p.decls = append(p.decls, &decl{kind: apimodel.KindVariable, name: p.toks[name].text, scope: p.scope,
				file: p.file, line: line, doc: doc, attrs: attrs, code: code, typ: typ})
			continue
		}
		d.members = append(d.members, &member{kind: apimodel.KindVariable, name: p.toks[name].text, line: line,
			doc: doc, attrs: attrs, code: code, static: true, readonly: true, typ: typ})
	}
}

// property reads a property statement whose first modifier is at begin;
// the token just read is its type or its first variable.
func (p *parser) property(d *decl, begin int, mods *memberMods) {
	doc, attrs := p.doc, p.attrs
	p.i--
	typeStart := p.i
	for k := p.peek(0); k.kind != tokVar && k.kind != tokEOF && !k.is(";") && !k.is("}"); k = p.peek(0) {
		p.next()
	}
	if p.peek(0).kind != tokVar {
		return
	}
	typ, prefix := p.typeText(typeStart, p.i), p.code(begin, p.i)
	spans := p.declarators()
	if !mods.public(d) {
		return
	}
	for k, s := range spans {
		if s.from >= s.to || p.toks[s.from].kind != tokVar {
			continue
		}
		line := p.toks[s.from].line
		if k == 0 {
			line = p.toks[begin].line
		}
		d.members = append(d.members, &member{kind: apimodel.KindProperty, name: p.toks[s.from].text[1:], line: line,
			doc: doc, attrs: attrs, code: strings.TrimSpace(prefix + " " + p.code(s.from, s.to)), static: mods.static,
			readonly: mods.readonly || d.readonly, typ: typ})
	}
}

// enumCase reads "case Name [= value];" of an enum.
func (p *parser) enumCase(d *decl, begin int) {
	name := p.next()
	for k := p.peek(0); k.kind != tokEOF && !k.is(";") && !k.is("}"); k = p.peek(0) {
		p.next()
	}
	d.members = append(d.members, &member{kind: apimodel.KindEnumMember, name: name.text, line: p.toks[begin].line,
		doc: p.doc, attrs: p.attrs, code: p.code(begin, p.i)})
	if p.peek(0).is(";") {
		p.next()
	}
}

// method reads a method or constructor whose first modifier is at begin;
// "function" has just been read.
func (p *parser) method(d *decl, begin int, mods *memberMods) {
	doc, attrs := p.doc, p.attrs
	if p.peek(0).is("&") {
		p.next()
	}
	name := p.next().text
	sig := p.signature(begin)
	kind := apimodel.KindMethod
	if strings.EqualFold(name, "__construct") {
		kind = apimodel.KindConstructor
		p.promoted(d, sig, doc)
	}
	if !mods.public(d) {
		return
	}
	d.members = append(d.members, &member{kind: kind, name: name, line: p.toks[begin].line, doc: doc, attrs: attrs,
		code: sig.code, static: mods.static, abstract: mods.abstract, sig: sig})
}

// promoted adds the public properties a constructor declares through its
// parameters, even when the constructor itself is not public.
func (p *parser) promoted(d *decl, sig *sigDecl, ctorDoc string) {
	for _, pd := range sig.params {
		if pd.public {
			d.members = append(d.members, &member{kind: apimodel.KindProperty, name: pd.name, line: pd.line,
				doc: ctorDoc, code: pd.code, readonly: pd.readonly || d.readonly, typ: pd.typ, param: true})
		}
	}
}

// function reads a top-level function; "function" is the token at start.
func (p *parser) function(start int) {
	doc, attrs := p.doc, p.attrs
	p.clear()
	if p.peek(0).is("&") {
		p.next()
	}
	name := p.next().text
	sig := p.signature(start)
	p.decls = append(p.decls, &decl{kind: apimodel.KindFunction, name: name, scope: p.scope, file: p.file,
		line: p.toks[start].line, doc: doc, attrs: attrs, code: sig.code, sig: sig})
}
