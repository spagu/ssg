package dts

// parseImport reads the import forms a declaration file uses:
// `import d, { a, b as c } from "m"`, `import * as ns from "m"`,
// `import x = require("m")` and `import "m"`. Bindings are file-wide.
func (p *parser) parseImport() {
	p.expect("import")
	if p.peek().kind == tokString {
		p.advance()
		p.endOfStatement()
		return
	}
	if p.peek().is("type") && !p.peekAt(1).is("from") && !p.peekAt(1).is(",") && !p.peekAt(1).is("=") {
		p.advance()
	}
	if p.peekAt(1).is("=") {
		p.importEquals()
		return
	}
	bindings := map[string]string{} // local name → imported name
	if t := p.peek(); t.kind == tokIdent {
		bindings[p.advance().text] = "default"
		p.accept(",")
	}
	if p.accept("*") {
		p.expect("as")
		bindings[p.expectName().text] = "*"
	} else if p.peek().is("{") {
		p.namedList(func(name, alias string) { bindings[alias] = name })
	}
	p.expect("from")
	spec := p.specifier()
	for local, name := range bindings {
		p.file.imports[local] = importRef{from: spec, name: name}
	}
	p.endOfStatement()
}

// importEquals reads `import x = require("m")`, bound to the module's
// `export =` target, or skips `import x = A.B` (an alias of a name).
func (p *parser) importEquals() {
	local := p.expectName().text
	p.expect("=")
	if !p.accept("require") {
		p.skipStatement()
		return
	}
	p.expect("(")
	p.file.imports[local] = importRef{from: p.specifier(), name: "default"}
	p.expect(")")
	p.endOfStatement()
}

// namedList reads `{ a, b as c, type d }`, calling add(name, alias) for each.
func (p *parser) namedList(add func(name, alias string)) {
	p.expect("{")
	for !p.accept("}") {
		if p.peek().is("type") && p.peekAt(1).kind == tokIdent && !p.peekAt(1).is("as") {
			p.advance()
		}
		name := unquote(p.expectName().text)
		alias := name
		if p.accept("as") {
			alias = unquote(p.expectName().text)
		}
		add(name, alias)
		if !p.accept(",") {
			p.expect("}")
			break
		}
	}
}

// specifier reads a module specifier string.
func (p *parser) specifier() string {
	if p.peek().kind != tokString {
		p.fail("expected a module specifier, found %s", describe(p.peek()))
	}
	spec := unquote(p.advance().text)
	if p.peek().is("with") || p.peek().is("assert") {
		p.advance()
		p.balanced()
	}
	return spec
}

// parseExport reads every form of export statement.
func (p *parser) parseExport(e *env) {
	first := p.advance()
	sc := e.scope
	switch t := p.peek(); {
	case t.is("="):
		p.advance()
		sc.explicit = true
		sc.exports["default"] = exportRef{local: p.dottedName()}
		p.endOfStatement()
	case t.is("as") || t.is("import"):
		p.skipStatement() // export as namespace X; export import A = B.C;
	case t.is("*"):
		p.exportStar(sc)
	case t.is("{") || t.is("type") && p.peekAt(1).is("{"):
		p.accept("type")
		p.exportList(sc)
	case t.is("default"):
		p.advance()
		p.exportDefault(e, first)
	default:
		p.parseDeclaration(e, modSet{exported: true}, first)
	}
}

// exportStar reads `export * from "m"` and `export * as ns from "m"`.
func (p *parser) exportStar(sc *scope) {
	p.expect("*")
	sc.explicit = true
	if p.accept("as") {
		name := unquote(p.expectName().text)
		p.expect("from")
		sc.exports[name] = exportRef{from: p.specifier(), name: "*"}
	} else {
		p.expect("from")
		sc.stars = append(sc.stars, p.specifier())
	}
	p.endOfStatement()
}

// exportList reads `export { a, b as c } [from "m"]`.
func (p *parser) exportList(sc *scope) {
	sc.explicit = true
	var pairs [][2]string
	p.namedList(func(name, alias string) { pairs = append(pairs, [2]string{name, alias}) })
	from := ""
	if p.accept("from") {
		from = p.specifier()
	}
	for _, pr := range pairs {
		if from == "" {
			sc.exports[pr[1]] = exportRef{local: pr[0]}
		} else {
			sc.exports[pr[1]] = exportRef{from: from, name: pr[0]}
		}
	}
	p.endOfStatement()
}

// exportDefault reads `export default` followed by a declaration or a name;
// any other expression has nothing to document and is skipped.
func (p *parser) exportDefault(e *env, first token) {
	t := p.peek()
	declares := t.is("function") || t.is("class") || t.is("interface") ||
		t.is("async") && p.peekAt(1).is("function") || t.is("abstract") && p.peekAt(1).is("class")
	switch {
	case declares:
		p.parseDeclaration(e, modSet{isDefault: true}, first)
	case t.kind == tokIdent:
		e.scope.explicit = true
		e.scope.exports["default"] = exportRef{local: p.dottedName()}
		p.endOfStatement()
	default:
		e.scope.explicit = true
		p.skipStatement()
	}
}

// dottedName reads `A` or `A.B.C`.
func (p *parser) dottedName() string {
	name := p.expectName().text
	for p.accept(".") {
		name += "." + p.expectName().text
	}
	return name
}
