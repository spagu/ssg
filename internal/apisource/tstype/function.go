package tstype

import (
	"strconv"

	"github.com/spagu/ssg/internal/apimodel"
)

// arrowAhead reports whether the `(` at the current token opens the
// parameter list of a function type: its matching `)` is followed by `=>`.
func (p *parser) arrowAhead() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch t := p.toks[i]; {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			if depth--; depth == 0 {
				return i+1 < len(p.toks) && p.toks[i+1].is("=>")
			}
		}
	}
	return false
}

// parseFunctionType parses `<T>(a: A, b?: B, ...c: C[]) => R`. Type
// parameters shadow outer names for the whole signature, so they are never
// resolved.
func (p *parser) parseFunctionType() *apimodel.TypeRef {
	sig := &apimodel.Signature{}
	defer p.pushScope()()
	if p.peek().is("<") {
		sig.TypeParams = p.parseTypeParams()
	}
	sig.Params = p.parseParams()
	p.expect("=>")
	sig.Returns = p.parseType()
	return &apimodel.TypeRef{Kind: apimodel.TypeFunction, Signature: sig}
}

// parseConstructorType parses `new (a: A) => B` (optionally `abstract new`)
// as a TypeFunction whose Name is "new".
func (p *parser) parseConstructorType() *apimodel.TypeRef {
	p.accept("abstract")
	p.expect("new")
	t := p.parseFunctionType()
	t.Name = "new"
	return t
}

// parseTypeParams parses `<T extends C = D, U>`, bringing each name into
// scope before its constraint is read (T extends Foo<T>). Variance and
// const modifiers (in, out, const) are skipped.
func (p *parser) parseTypeParams() []*apimodel.TypeParam {
	p.expect("<")
	var params []*apimodel.TypeParam
	for !p.peek().is(">") {
		for p.peek().kind == tokIdent && p.peekAt(1).kind == tokIdent && !p.peekAt(1).is("extends") {
			p.advance() // modifier
		}
		tp := &apimodel.TypeParam{Name: p.expectIdent()}
		p.scope = append(p.scope, tp.Name)
		if p.accept("extends") {
			tp.Constraint = p.parseType()
		}
		if p.accept("=") {
			tp.Default = p.parseType()
		}
		params = append(params, tp)
		if !p.accept(",") {
			break
		}
	}
	p.expect(">")
	return params
}

// parseParams parses a parenthesized parameter list.
func (p *parser) parseParams() []*apimodel.Param {
	p.expect("(")
	var params []*apimodel.Param
	for !p.peek().is(")") {
		params = append(params, p.parseParam())
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	return params
}

// parseParam parses `...name?: Type`. A destructuring pattern keeps its
// normalized text as the name; a missing type is `any`, as in TypeScript.
func (p *parser) parseParam() *apimodel.Param {
	param := &apimodel.Param{Rest: p.accept("...")}
	if p.peek().is("{") || p.peek().is("[") {
		start := p.pos
		p.skipBalanced()
		param.Name = p.text(start, p.pos)
	} else {
		param.Name = p.expectIdent()
	}
	param.Optional = p.accept("?")
	param.Type = builtin("any")
	if p.accept(":") {
		param.Type = p.parseType()
	}
	return param
}

// positional names an unnamed parameter: p0, p1, ...
func positional(i int) string {
	return "p" + strconv.Itoa(i)
}
