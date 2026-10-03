package dts

import "github.com/spagu/ssg/internal/apimodel"

// parseClass reads `class C<T> extends B<T> implements I, J { members }`.
// A default export may leave the name out.
func (p *parser) parseClass(e *env, m modSet, first token) {
	p.expect("class")
	name := p.optionalName(m)
	n := &node{kind: apimodel.KindClass, name: name.text, line: name.line, doc: first.doc, mods: m}
	n.tparams = p.parseTypeParams()
	if p.accept("extends") {
		n.extends = append(n.extends, p.typeSpan(stopHerit))
	}
	if p.accept("implements") {
		n.implements = p.heritageList()
	}
	n.members = p.parseMembers(true)
	p.accept(";")
	declare(e, n)
}

// heritageList reads `A, B<C>` up to the "{" of a body.
func (p *parser) heritageList() []string {
	list := []string{p.typeSpan(stopHerit)}
	for p.accept(",") {
		list = append(list, p.typeSpan(stopHerit))
	}
	return list
}

// parseMembers reads `{ member; … }` of a class (inClass) or an interface.
// Overloads of a method join one member; a getter and a setter of the same
// name join one accessor.
func (p *parser) parseMembers(inClass bool) []*node {
	p.expect("{")
	var out []*node
	byKey := map[string]*node{}
	for !p.accept("}") {
		if p.accept(";") || p.accept(",") {
			continue
		}
		m := p.parseMember(inClass)
		if m == nil {
			continue
		}
		key := memberKey(m)
		if old, ok := byKey[key]; ok {
			mergeMember(old, m)
			continue
		}
		byKey[key] = m
		out = append(out, m)
	}
	return out
}

// memberKey identifies a member for merging: kind, static-ness and name.
func memberKey(m *node) string {
	k := string(m.kind) + " " + m.name
	if m.mods.static {
		k = "static " + k
	}
	return k
}

// mergeMember joins a later declaration of the same member into the first.
func mergeMember(old, m *node) {
	old.sigs = append(old.sigs, m.sigs...)
	old.mods.getter = old.mods.getter || m.mods.getter
	old.mods.setter = old.mods.setter || m.mods.setter
	if old.typ == "" || m.mods.getter {
		old.typ = m.typ
	}
	if old.doc == "" {
		old.doc = m.doc
	}
}

// memberEnd consumes the separator after a member: ";" or ",", or nothing
// before a "}" or a line break.
func (p *parser) memberEnd() {
	if p.accept(";") || p.accept(",") || p.peek().is("}") || p.peek().nl {
		return
	}
	p.fail("expected \";\" after a member, found %s", describe(p.peek()))
}
