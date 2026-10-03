package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// memberOrder is the order members are listed in, each group in
// declaration order: enum cases, constants, properties, the constructor,
// methods.
var memberOrder = map[apimodel.Kind]int{
	apimodel.KindEnumMember: 0, apimodel.KindVariable: 1, apimodel.KindProperty: 2,
	apimodel.KindConstructor: 3, apimodel.KindMethod: 4,
}

// members builds the public members of a class-like, ordered by memberOrder.
// A property whose name a method or constant already uses gets the ID
// "Owner.$name", since PHP keeps the two apart.
func (b *builder) members(d *decl, parentID string, ctx typeCtx) []*apimodel.Symbol {
	var groups [5][]*apimodel.Symbol
	for _, m := range d.members {
		if s := b.member(d, m, ctx); s != nil {
			groups[memberOrder[m.kind]] = append(groups[memberOrder[m.kind]], s)
		}
	}
	used := map[string]bool{}
	for _, g := range []int{0, 1, 3, 4, 2} {
		kept := groups[g][:0]
		for _, s := range groups[g] {
			id := apimodel.MemberID(parentID, s.Name)
			if used[id] {
				id = apimodel.MemberID(parentID, "$"+s.Name)
			}
			if used[id] {
				continue
			}
			used[id], s.ID = true, id
			kept = append(kept, s)
		}
		groups[g] = kept
	}
	var out []*apimodel.Symbol
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// member builds one member, or nil when its docblock hides it.
func (b *builder) member(d *decl, m *member, ctx typeCtx) *apimodel.Symbol {
	info := parseDoc(m.doc)
	s := &apimodel.Symbol{Name: m.name, Kind: m.kind, Source: &apimodel.Source{File: d.file, Line: m.line}}
	if m.param {
		text, typ := info.param(m.name)
		s.Code, s.Type = m.code, b.res.typeRef(firstOf(m.typ, typ), ctx)
		s.Flags.Readonly = m.readonly
		if text != "" {
			s.Doc = &apimodel.Doc{Summary: text}
		}
		return s
	}
	if info.Mods.Hidden || info.Mods.Private {
		return nil
	}
	s.Flags = flagsOf(info)
	s.Flags.Static = m.static
	s.Flags.Readonly = s.Flags.Readonly || m.readonly
	s.Flags.Abstract = s.Flags.Abstract || m.abstract
	switch m.kind {
	case apimodel.KindConstructor, apimodel.KindMethod:
		s.Signatures = []*apimodel.Signature{b.signature(m.sig, info, ctx)}
	case apimodel.KindProperty, apimodel.KindVariable:
		s.Code, s.Type = m.code, b.res.typeRef(firstOf(m.typ, info.varType), ctx)
	default:
		s.Code = m.code
	}
	s.Doc = docOf(info, m.attrs)
	return s
}

// signature builds a callable's signature. A parameter or return without a
// declared type takes the one its @param or @return gives.
func (b *builder) signature(sd *sigDecl, info *docInfo, ctx typeCtx) *apimodel.Signature {
	sig := &apimodel.Signature{Code: sd.code}
	for _, pd := range sd.params {
		text, typ := info.param(pd.name)
		sig.Params = append(sig.Params, &apimodel.Param{Name: pd.name, Type: b.res.typeRef(firstOf(pd.typ, typ), ctx),
			Optional: pd.def != "", Rest: pd.rest, Default: pd.def, Doc: text})
	}
	sig.Returns = b.res.typeRef(firstOf(sd.returns, info.returnType()), ctx)
	return sig
}

// deprecatedAttr reports whether the attributes include #[\Deprecated],
// with the first string written in it as the text.
func deprecatedAttr(attrs []string) (string, bool) {
	for _, a := range attrs {
		inner := strings.TrimSuffix(strings.TrimPrefix(a, "#["), "]")
		for _, part := range splitTop(inner, ',') {
			name, args, _ := strings.Cut(strings.TrimSpace(part), "(")
			if !strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(name), "\\"), "Deprecated") {
				continue
			}
			return firstString(args), true
		}
	}
	return "", false
}

// firstString returns the contents of the first quoted string in s, or "".
func firstString(s string) string {
	i := strings.IndexAny(s, `'"`)
	if i < 0 {
		return ""
	}
	end, ok := skipQuoted(s, i)
	if !ok {
		return ""
	}
	return s[i+1 : end-1]
}
