package dts

import "github.com/spagu/ssg/internal/apimodel"

// parseInterface reads `interface I<T> extends A, B<T> { members }`: its
// members may be properties, methods, index signatures and call and
// construct signatures.
func (p *parser) parseInterface(e *env, m modSet, first token) {
	p.expect("interface")
	name := p.optionalName(m)
	n := &node{kind: apimodel.KindInterface, name: name.text, line: name.line, doc: first.doc, mods: m}
	n.tparams = p.parseTypeParams()
	if p.accept("extends") {
		n.extends = p.heritageList()
	}
	n.members = p.parseMembers(false)
	declare(e, n)
}
