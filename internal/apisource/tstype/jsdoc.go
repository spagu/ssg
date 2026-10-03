package tstype

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// StripOptional removes the JSDoc optional marker, a trailing `=`
// ("string=" means an optional string), and reports whether it was there.
// Parse itself also accepts and ignores the marker; callers use this to
// learn that the parameter is optional.
func StripOptional(expr string) (string, bool) {
	s := strings.TrimSpace(expr)
	if strings.HasSuffix(s, "=") && !strings.HasSuffix(s, "==") {
		return strings.TrimSpace(strings.TrimSuffix(s, "=")), true
	}
	return s, false
}

// parseJSDocPrefix parses the JSDoc prefixes: `?T` is nullable (T | null),
// `!T` is non-null (T), and a lone `?` is unknown.
func (p *parser) parseJSDocPrefix() *apimodel.TypeRef {
	nullable := p.advance().is("?")
	if nullable && !startsOperand(p.peek()) {
		return builtin("unknown")
	}
	t := p.parseOperator()
	if !nullable {
		return t
	}
	null := builtin("null")
	if t.Kind == apimodel.TypeUnion {
		t.Args = append(t.Args, null)
		return t
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeUnion, Args: []*apimodel.TypeRef{t, null}}
}

// parseJSDocFunction parses `function(string, number=, ...boolean): R`.
// Parameters are unnamed, so they are called p0, p1, ...; `this:T` and
// `new:T` keep those names. Without `: R` the result is unknown.
func (p *parser) parseJSDocFunction() *apimodel.TypeRef {
	p.expect("function")
	p.expect("(")
	sig := &apimodel.Signature{}
	for !p.peek().is(")") {
		sig.Params = append(sig.Params, p.parseJSDocParam(len(sig.Params)))
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	sig.Returns = builtin("unknown")
	if p.accept(":") {
		sig.Returns = p.parseType()
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeFunction, Signature: sig}
}

// parseJSDocParam parses one parameter of a JSDoc function type.
func (p *parser) parseJSDocParam(i int) *apimodel.Param {
	param := &apimodel.Param{Name: positional(i), Rest: p.accept("...")}
	if (p.peek().is("this") || p.peek().is("new")) && p.peekAt(1).is(":") {
		param.Name = p.advance().text
		p.advance()
	}
	param.Type = p.parseType()
	param.Optional = p.accept("=")
	return param
}
