package dts

import (
	"github.com/spagu/ssg/internal/apisource"
)

// parseBody reads statements into e.scope until the end of the file or a
// "}" closing the enclosing block (left for the caller).
func (p *parser) parseBody(e *env) {
	for t := p.peek(); t.kind != tokEOF && !t.is("}"); t = p.peek() {
		p.statement(e)
	}
}

// statement reads one statement; a syntax error becomes a diagnostic and the
// statement is skipped.
func (p *parser) statement(e *env) {
	start := p.pos
	defer func() {
		if r := recover(); r != nil {
			se, ok := r.(syntaxError)
			if !ok {
				panic(r)
			}
			p.diags = append(p.diags, apisource.Diagnostic{
				Severity: apisource.Warning, File: p.file.path, Line: se.line, Message: se.msg,
			})
			p.pos = start
			p.resync()
		}
	}()
	switch t := p.peek(); {
	case t.is(";"):
		p.advance()
	case t.is("import"):
		p.parseImport()
	case t.is("export"):
		p.parseExport(e)
	default:
		p.parseDeclaration(e, modSet{}, t)
	}
}

// skipStatement skips to the end of the statement at p.pos: past a ";" or a
// block at bracket depth 0, or up to a "}" closing the enclosing block.
func (p *parser) skipStatement() {
	depth := 0
	for t := p.advance(); t.kind != tokEOF; t = p.advance() {
		switch {
		case t.is("{") || t.is("(") || t.is("["):
			depth++
		case t.is("}") || t.is(")") || t.is("]"):
			depth--
			if depth < 0 {
				p.pos--
				return
			}
			if depth == 0 && t.is("}") {
				p.accept(";")
				return
			}
		case t.is(";") && depth == 0:
			return
		}
	}
}

// resync skips a statement that did not parse: up to the next line that
// starts a statement, or a "}" closing the enclosing block. An unbalanced
// bracket cannot hide the rest of the file: a line starting with export or
// import ends the skip at any depth, other declaration keywords at depth 0.
func (p *parser) resync() {
	depth := 0
	for t := p.advance(); t.kind != tokEOF; t = p.advance() {
		depth += bracketDelta(t)
		next := p.peek()
		if next.is("}") && depth == 0 {
			return
		}
		if next.nl && (next.is("export") || next.is("import") || depth <= 0 && declarationWords[next.text] && next.kind == tokIdent) {
			return
		}
	}
}

// declarationWords start a declaration statement.
var declarationWords = map[string]bool{
	"declare": true, "interface": true, "type": true, "function": true, "class": true,
	"namespace": true, "module": true, "enum": true, "const": true, "let": true, "var": true, "abstract": true,
}

// bracketDelta is +1 for an opening bracket, -1 for a closing one.
func bracketDelta(t token) int {
	switch {
	case t.is("{") || t.is("(") || t.is("["):
		return 1
	case t.is("}") || t.is(")") || t.is("]"):
		return -1
	}
	return 0
}

// parseDeclaration reads a declaration after any "export"/"default": the
// first token carries the documentation comment.
func (p *parser) parseDeclaration(e *env, m modSet, first token) {
	p.accept("declare")
	t := p.peek()
	switch {
	case t.is("abstract") && p.peekAt(1).is("class"):
		p.advance()
		m.abstract = true
		p.parseClass(e, m, first)
	case t.is("class"):
		p.parseClass(e, m, first)
	case t.is("function") || t.is("async") && p.peekAt(1).is("function"):
		p.parseFunction(e, m, first)
	case t.is("interface"):
		p.parseInterface(e, m, first)
	case t.is("type") && p.peekAt(1).kind == tokIdent:
		p.parseAlias(e, m, first)
	case t.is("enum") || t.is("const") && p.peekAt(1).is("enum"):
		p.parseEnum(e, m, first)
	case t.is("const") || t.is("let") || t.is("var"):
		p.parseVariables(e, m, first)
	case t.is("namespace") || t.is("module") && p.peekAt(1).kind == tokIdent:
		p.parseNamespace(e, m, first)
	case t.is("module") && p.peekAt(1).kind == tokString:
		p.parseAmbientModule(e)
	case t.is("global") && p.peekAt(1).is("{"):
		p.skipStatement()
	default:
		p.fail("unexpected %s", describe(t))
	}
}

// declare adds a declaration to e's scope. A second declaration of the same
// name merges into the first, as TypeScript merges overloads, interfaces and
// namespaces: signatures, members and namespace contents are joined.
func declare(e *env, n *node) {
	n.env = e
	for _, m := range n.members {
		m.env = e
	}
	if n.mods.exported {
		e.scope.explicit = true
		e.scope.exports[n.name] = exportRef{local: n.name}
	}
	if n.mods.isDefault {
		e.scope.explicit = true
		e.scope.exports["default"] = exportRef{local: n.name}
	}
	old, ok := e.scope.decls[n.name]
	if !ok {
		e.scope.decls[n.name] = n
		e.scope.order = append(e.scope.order, n)
		return
	}
	old.sigs = append(old.sigs, n.sigs...)
	old.members = append(old.members, n.members...)
	old.extends = append(old.extends, n.extends...)
	if n.body != nil {
		mergeBody(old, n)
	}
	if old.doc == "" {
		old.doc = n.doc
	}
}

// mergeBody joins a namespace's contents into a declaration of the same name.
func mergeBody(old, n *node) {
	if old.body == nil {
		old.body = n.body
		return
	}
	for _, d := range n.body.order {
		declare(&env{scope: old.body, file: d.env.file, parent: old.env}, d)
	}
	for k, v := range n.body.exports {
		old.body.exports[k] = v
	}
	old.body.explicit = old.body.explicit || n.body.explicit
}
