package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// classKinds maps the class-like keywords to the kind they document as.
var classKinds = map[string]apimodel.Kind{
	"class": apimodel.KindClass, "interface": apimodel.KindInterface,
	"trait": apimodel.KindClass, "enum": apimodel.KindEnum,
}

// classModifiers are the modifiers a class-like may carry.
var classModifiers = map[string]bool{"abstract": true, "final": true, "readonly": true}

// classAhead reports whether the token at start opens a named class-like
// declaration: modifiers, a class keyword and a name. "new class" and
// "Foo::class" are not declarations.
func (p *parser) classAhead(start int) bool {
	if start > 0 {
		if prev := p.toks[start-1]; prev.keyword("new") || prev.is("::") || prev.is("->") || prev.is("?->") {
			return false
		}
	}
	i := start
	for i < len(p.toks) && p.toks[i].kind == tokIdent && classModifiers[strings.ToLower(p.toks[i].text)] {
		i++
	}
	if i+1 >= len(p.toks) || p.toks[i].kind != tokIdent {
		return false
	}
	_, ok := classKinds[strings.ToLower(p.toks[i].text)]
	return ok && p.toks[i+1].kind == tokIdent
}

// classLike reads a class, interface, trait or enum whose first modifier
// or keyword is the token at start, body included.
func (p *parser) classLike(start int) {
	d := &decl{scope: p.scope, file: p.file, line: p.toks[start].line, doc: p.doc, attrs: p.attrs}
	p.clear()
	p.i = start
	for {
		kw := strings.ToLower(p.next().text)
		if kind, ok := classKinds[kw]; ok {
			d.kind, d.trait = kind, kw == "trait"
			break
		}
		d.abstract = d.abstract || kw == "abstract"
		d.readonly = d.readonly || kw == "readonly"
	}
	d.name = p.next().text
	for !p.peek(0).is("{") && p.peek(0).kind != tokEOF {
		t := p.next()
		switch {
		case t.keyword("extends"):
			d.extends = p.nameList()
		case t.keyword("implements"):
			d.implements = p.nameList()
		}
	}
	d.code = p.code(start, p.i)
	p.next()
	p.classBody(d)
	p.decls = append(p.decls, d)
}

// nameList reads "A, B\C, D" and returns the names as written.
func (p *parser) nameList() []string {
	var names []string
	for p.peek(0).kind == tokIdent {
		names = append(names, p.next().text)
		if !p.peek(0).is(",") {
			break
		}
		p.next()
	}
	return names
}

// memberMods are the modifiers in front of a class member.
type memberMods struct {
	start    int // index of the first modifier, -1 when none
	vis      string
	static   bool
	abstract bool
	readonly bool
}

// memberModifiers are the words memberMods records.
var memberModifiers = map[string]bool{
	"public": true, "protected": true, "private": true, "var": true,
	"static": true, "abstract": true, "final": true, "readonly": true,
}

// add records one modifier; a set-visibility such as "private(set)" is
// read and ignored, since it does not hide the member.
func (m *memberMods) add(p *parser, t token) {
	if m.start < 0 {
		m.start = p.i - 1
	}
	switch w := strings.ToLower(t.text); w {
	case "public", "protected", "private":
		if p.peek(0).is("(") && p.peek(1).keyword("set") {
			p.i += 3
			return
		}
		m.vis = w
	case "static":
		m.static = true
	case "abstract":
		m.abstract = true
	case "readonly":
		m.readonly = true
	}
}

// public reports whether the member is part of the public API: declared
// public, or with no visibility at all, or inside an interface.
func (m *memberMods) public(d *decl) bool {
	return d.kind == apimodel.KindInterface || (m.vis != "protected" && m.vis != "private")
}

// classBody reads the members of a class-like up to its closing brace.
func (p *parser) classBody(d *decl) {
	mods := memberMods{start: -1}
	for {
		t := p.next()
		begin := mods.start
		if begin < 0 {
			begin = p.i - 1
		}
		switch {
		case t.kind == tokEOF, t.is("}"):
			return
		case t.kind == tokDoc:
			p.doc = t.text
			continue
		case t.kind == tokAttr:
			p.attrs = append(p.attrs, t.text)
			continue
		case t.kind == tokIdent && memberModifiers[strings.ToLower(t.text)]:
			mods.add(p, t)
			continue
		case t.keyword("case") && d.kind == apimodel.KindEnum:
			p.enumCase(d, begin)
		case t.keyword("const"):
			p.constants(begin, d, &mods)
		case t.keyword("function"):
			p.method(d, begin, &mods)
		case t.kind == tokVar || (mods.start >= 0 && (t.kind == tokIdent || t.is("?") || t.is("("))):
			p.property(d, begin, &mods)
		case t.keyword("use"):
			p.skipStatement() // trait use
		case t.is("{"):
			p.skipBlock()
		}
		mods = memberMods{start: -1}
		p.clear()
	}
}
