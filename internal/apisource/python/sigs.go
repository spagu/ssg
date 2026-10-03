package python

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// rawParam is one parameter of a def, split into its parts.
type rawParam struct {
	name   string
	stars  string // "", "*" or "**"
	marker bool   // a bare "/" or "*"
	annot  []token
	def    []token
	code   string // as written, whitespace normalised
}

// parseParam splits "*name: T = default", or reads a "/" or "*" marker.
func parseParam(toks []token) rawParam {
	if len(toks) == 1 && (toks[0].is("/") || toks[0].is("*")) {
		return rawParam{marker: true, code: toks[0].text}
	}
	var p rawParam
	i := 0
	if toks[0].is("*") || toks[0].is("**") {
		p.stars, i = toks[0].text, 1
	}
	if i < len(toks) && toks[i].kind == tName {
		p.name = toks[i].text
		i++
	}
	rest := toks[i:]
	if eq := indexTop(rest, "="); eq >= 0 {
		p.def, rest = rest[eq+1:], rest[:eq]
	}
	if len(rest) > 0 && rest[0].is(":") {
		p.annot = rest[1:]
	}
	p.code = assignCode(p.stars+p.name, p.annot, p.def, "=")
	return p
}

// assignCode renders "name: T = value", "name: T" or "name<bare>value";
// bare is the separator without an annotation ("=" for parameters, " = "
// for assignments).
func assignCode(name string, annot, value []token, bare string) string {
	s := name
	if len(annot) > 0 {
		s += ": " + joinToks(annot)
		bare = " = "
	}
	if len(value) > 0 {
		s += bare + shortValue(value)
	}
	return s
}

// maxValue is how long a value may be in Code before it is elided.
const maxValue = 80

// shortValue renders a value, or "..." when it is too long to show.
func shortValue(value []token) string {
	if v := joinToks(value); len(v) <= maxValue {
		return v
	}
	return "..."
}

// signature builds one callable shape of a def. dropFirst leaves out the
// self or cls parameter of a method (it stays in Code). Parameter docs and
// fallback types come from the docstring.
func signature(d *stmt, dropFirst bool, info *docInfo, res resolver) *apimodel.Signature {
	sig := &apimodel.Signature{TypeParams: typeParams(d.typeParams, res)}
	var codes []string
	for _, raw := range d.params {
		p := parseParam(raw)
		codes = append(codes, p.code)
		if p.marker {
			continue
		}
		if dropFirst {
			dropFirst = false
			if p.stars == "" {
				continue
			}
		}
		mp := &apimodel.Param{Name: p.name, Rest: p.stars != "", Optional: len(p.def) > 0,
			Default: joinToks(p.def), Type: parseType(p.annot, res)}
		if dp := lookup(info.params, p.name); dp != nil {
			mp.Doc = dp.text
			if mp.Type == nil && dp.typ != "" {
				mp.Type = parseTypeText(dp.typ, res)
			}
		}
		sig.Params = append(sig.Params, mp)
	}
	sig.Returns = parseType(d.returns, res)
	if sig.Returns == nil && info.returnType != "" {
		sig.Returns = parseTypeText(info.returnType, res)
	}
	sig.Code = "def " + d.name + typeParamCode(d.typeParams) + "(" + strings.Join(codes, ", ") + ")"
	if len(d.returns) > 0 {
		sig.Code += " -> " + joinToks(d.returns)
	}
	if d.async {
		sig.Code = "async " + sig.Code
	}
	return sig
}

// signatures builds a def's signatures: one per @overload variant when
// there are any, else its own.
func signatures(d *stmt, dropFirst bool, info *docInfo, res resolver) []*apimodel.Signature {
	defs := d.overloads
	if len(defs) == 0 {
		defs = []*stmt{d}
	}
	out := make([]*apimodel.Signature, len(defs))
	for i, v := range defs {
		out[i] = signature(v, dropFirst, info, res)
	}
	return out
}

// typeParams reads PEP 695 type parameters: "T", "T: Bound", "*Ts",
// "**P", "T = Default".
func typeParams(raw [][]token, res resolver) []*apimodel.TypeParam {
	var out []*apimodel.TypeParam
	for _, toks := range raw {
		p := parseParam(toks)
		tp := &apimodel.TypeParam{Name: p.stars + p.name, Constraint: parseType(p.annot, res), Default: parseType(p.def, res)}
		out = append(out, tp)
	}
	return out
}

// typeParamCode renders "[T, U: int]", or "" without type parameters.
func typeParamCode(raw [][]token) string {
	if len(raw) == 0 {
		return ""
	}
	parts := make([]string, len(raw))
	for i, toks := range raw {
		parts[i] = joinToks(toks)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// docSource is the docstring of a def, or of its first documented overload.
func docSource(d *stmt) string {
	if d.doc != "" {
		return d.doc
	}
	for _, o := range d.overloads {
		if o.doc != "" {
			return o.doc
		}
	}
	return ""
}
