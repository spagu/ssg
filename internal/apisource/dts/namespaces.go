package dts

import "github.com/spagu/ssg/internal/apimodel"

// parseNamespace reads `namespace A.B { … }` (also `module A`). A dotted name
// is a namespace nested in another, the inner one exported.
func (p *parser) parseNamespace(e *env, m modSet, first token) {
	p.advance()
	name := p.expectName()
	outer := &node{kind: apimodel.KindNamespace, name: name.text, line: name.line, doc: first.doc, mods: m, body: newScope()}
	innerEnv := &env{scope: outer.body, file: e.file, parent: e}
	for p.accept(".") {
		name = p.expectName()
		n := &node{kind: apimodel.KindNamespace, name: name.text, line: name.line, doc: first.doc,
			mods: modSet{exported: true}, body: newScope()}
		declare(innerEnv, n)
		innerEnv = &env{scope: n.body, file: e.file, parent: innerEnv}
	}
	p.block(innerEnv)
	declare(e, outer) // after the body, so a merge sees the contents
}

// parseAmbientModule reads `declare module "name" { … }`: a module named by
// the string, whose declarations join every other block of the same name.
func (p *parser) parseAmbientModule(e *env) {
	p.expect("module")
	name := unquote(p.advance().text)
	sc, ok := p.file.ambients[name]
	if !ok {
		sc = newScope()
		p.file.ambients[name] = sc
	}
	if p.accept(";") || !p.peek().is("{") {
		return // shorthand ambient module: `declare module "x";`
	}
	p.block(&env{scope: sc, file: e.file, parent: e})
}

// block reads `{ statements }` into e.
func (p *parser) block(e *env) {
	p.expect("{")
	p.parseBody(e)
	p.expect("}")
}
