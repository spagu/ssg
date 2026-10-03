package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/apisource/tstype"
)

// symbol builds the model of declaration n listed as name with ID id.
// outer are the type parameters of the enclosing class or interface, which
// a member's types may use.
func (b *builder) symbol(n *node, name, id string, outer []string) *apimodel.Symbol {
	c := b.comment(n)
	s := &apimodel.Symbol{
		ID: id, Name: name, Kind: n.kind, Flags: b.flags(n, c),
		Source: b.source(n.env.file, n.line), Doc: docOf(c),
	}
	tps := append(append([]string{}, outer...), tparamNames(n.tparams)...)
	res := b.resolver(n.env, tps)
	s.TypeParams = b.typeParams(n, n.tparams, c, res)
	switch n.kind {
	case apimodel.KindFunction, apimodel.KindMethod, apimodel.KindConstructor:
		s.Signatures = b.signatures(n, tps)
	case apimodel.KindEnumMember:
		s.Type, _ = tstype.Parse(n.typ, nil)
		if n.typ == "" {
			s.Type = nil
		}
	default:
		s.Type = b.typeRef(n, n.typ, res)
	}
	for _, t := range n.extends {
		s.Extends = append(s.Extends, b.typeRef(n, t, res))
	}
	for _, t := range n.implements {
		s.Implements = append(s.Implements, b.typeRef(n, t, res))
	}
	if n.mods.protected {
		s.Doc = withTag(s.Doc, "protected")
	}
	if !b.building[n] {
		b.building[n] = true
		for _, ch := range b.children(n) {
			s.Members = append(s.Members, b.symbol(ch.n, ch.name, apimodel.MemberID(id, ch.name), tps))
		}
		delete(b.building, n)
	}
	return s
}

// flags collects the modifiers written in the code and in the comment.
func (b *builder) flags(n *node, c *commentInfo) apimodel.Flags {
	m := n.mods
	return apimodel.Flags{
		Static:    m.static,
		Readonly:  m.readonly || c.Mods.Readonly || m.getter && !m.setter,
		Optional:  m.optional,
		Abstract:  m.abstract || c.Mods.Abstract,
		Stability: c.Mods.Stability,
		Internal:  c.Mods.Internal,
	}
}

// signatures builds a callable's overloads. An overload with its own
// comment documents its parameters and, when there are several, itself.
func (b *builder) signatures(n *node, outer []string) []*apimodel.Signature {
	base := b.comment(n)
	var out []*apimodel.Signature
	for _, sg := range n.sigs {
		c := base
		if sg.doc != "" {
			c = parseComment(sg.doc)
		}
		sig := b.signature(n, sg, outer, c)
		if len(n.sigs) > 1 && sg.doc != "" {
			sig.Doc = docOf(c)
		}
		out = append(out, sig)
	}
	return out
}

// signature builds one call signature; a `this` parameter only types the
// receiver and is left out.
func (b *builder) signature(n *node, sg *sigSpan, outer []string, c *commentInfo) *apimodel.Signature {
	tps := append(append([]string{}, outer...), tparamNames(sg.tparams)...)
	res := b.resolver(n.env, tps)
	sig := &apimodel.Signature{TypeParams: b.typeParams(n, sg.tparams, c, res)}
	for _, p := range sg.params {
		if p.name == "this" {
			continue
		}
		sig.Params = append(sig.Params, &apimodel.Param{
			Name: p.name, Type: b.typeRef(n, p.typ, res), Optional: p.optional,
			Rest: p.rest, Default: p.def, Doc: c.paramDoc(p.name),
		})
	}
	sig.Returns = b.typeRef(n, sg.returns, res)
	return sig
}

// typeParams builds generic parameters with their @template documentation.
func (b *builder) typeParams(n *node, tps []tparamSpan, c *commentInfo, res tstype.Resolver) []*apimodel.TypeParam {
	var out []*apimodel.TypeParam
	for _, tp := range tps {
		out = append(out, &apimodel.TypeParam{
			Name: tp.name, Constraint: b.typeRef(n, tp.constraint, res),
			Default: b.typeRef(n, tp.def, res), Doc: c.templateDoc(tp.name),
		})
	}
	return out
}

// typeRef parses a type written in n; a type that does not parse is kept
// as written and reported.
func (b *builder) typeRef(n *node, text string, res tstype.Resolver) *apimodel.TypeRef {
	if text == "" {
		return nil
	}
	t, err := tstype.Parse(text, res)
	if err != nil {
		b.diags = append(b.diags, apisource.Diagnostic{
			Severity: apisource.Warning, File: n.env.file.path, Line: n.line,
			Message: "type " + text + ": " + err.Error(),
		})
	}
	return t
}

// resolver maps a type name used in e to the ID of the declaration it
// denotes; type parameters in scope, built-ins and outside names map to "".
func (b *builder) resolver(e *env, tps []string) tstype.Resolver {
	return func(name string) string {
		head, _, _ := strings.Cut(name, ".")
		for _, tp := range tps {
			if tp == head {
				return ""
			}
		}
		if n := b.resolveName(e, name); n != nil {
			return b.ids[n]
		}
		return ""
	}
}

// source is where a line of a declaration file came from: the original
// source when a source map says so, else the declaration file itself.
func (b *builder) source(f *file, line int) *apimodel.Source {
	if file, orig, ok := f.srcMap.lookup(line); ok {
		return &apimodel.Source{File: file, Line: orig}
	}
	return &apimodel.Source{File: f.path, Line: line}
}

// tparamNames lists type parameter names.
func tparamNames(tps []tparamSpan) []string {
	out := make([]string, len(tps))
	for i, tp := range tps {
		out[i] = tp.name
	}
	return out
}
