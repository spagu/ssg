package php

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// builder turns the declarations of every file into modules.
type builder struct {
	pkg   string
	res   *resolver
	diags []apisource.Diagnostic
}

// symbolID is the ID a declaration documents under.
func (b *builder) symbolID(d *decl) string {
	return apimodel.SymbolID(apimodel.ModuleID(b.pkg, modulePath(d.scope.ns)), d.name)
}

// buildModules groups declarations by namespace into modules ordered by ID,
// their symbols ordered by name. A second declaration of the same name in
// one namespace (a polyfill, a conditional definition) is reported and
// left out.
func buildModules(pkg string, decls []*decl) ([]*apimodel.Module, []apisource.Diagnostic) {
	b := &builder{pkg: pkg, res: &resolver{byName: map[string]string{}}}
	for _, d := range decls {
		if d.kind != apimodel.KindFunction && d.kind != apimodel.KindVariable {
			if _, dup := b.res.byName[strings.ToLower(d.fqName())]; !dup {
				b.res.byName[strings.ToLower(d.fqName())] = b.symbolID(d)
			}
		}
	}
	mods := map[string]*apimodel.Module{}
	seen := map[string]bool{}
	for _, d := range decls {
		id := b.symbolID(d)
		if seen[id] {
			b.diags = append(b.diags, apisource.Diagnostic{Severity: apisource.Warning, File: d.file, Line: d.line,
				Message: d.fqName() + " is declared more than once; the first declaration is documented"})
			continue
		}
		seen[id] = true
		s := b.symbol(d, id)
		if s == nil {
			continue
		}
		path := modulePath(d.scope.ns)
		m, ok := mods[path]
		if !ok {
			m = &apimodel.Module{ID: apimodel.ModuleID(pkg, path), Path: path, Symbols: []*apimodel.Symbol{}}
			mods[path] = m
		}
		m.Symbols = append(m.Symbols, s)
	}
	out := make([]*apimodel.Module, 0, len(mods))
	for _, m := range mods {
		sort.SliceStable(m.Symbols, func(i, j int) bool { return m.Symbols[i].Name < m.Symbols[j].Name })
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, b.diags
}

// symbol builds the symbol for a declaration, or nil when its docblock
// hides it (@private, @hidden, @ignore).
func (b *builder) symbol(d *decl, id string) *apimodel.Symbol {
	info := parseDoc(d.doc)
	if info.Mods.Hidden || info.Mods.Private {
		return nil
	}
	s := &apimodel.Symbol{ID: id, Name: d.name, Kind: d.kind, Code: d.code,
		Source: &apimodel.Source{File: d.file, Line: d.line}, Flags: flagsOf(info)}
	ctx := typeCtx{scope: d.scope}
	switch d.kind {
	case apimodel.KindFunction:
		s.Code = ""
		s.Signatures = []*apimodel.Signature{b.signature(d.sig, info, ctx)}
	case apimodel.KindVariable:
		s.Flags.Readonly = true
		s.Type = b.res.typeRef(firstOf(d.typ, info.varType), ctx)
	default:
		ctx.self = id
		if len(d.extends) > 0 && d.kind == apimodel.KindClass {
			ctx.parent = d.extends[0]
		}
		s.Flags.Abstract = s.Flags.Abstract || d.abstract
		s.Extends, s.Implements = b.res.names(d.extends, ctx), b.res.names(d.implements, ctx)
		s.Members = b.members(d, id, ctx)
	}
	s.Doc = docOf(info, d.attrs)
	if d.trait {
		if s.Doc == nil {
			s.Doc = &apimodel.Doc{}
		}
		s.Doc.Tags = append(s.Doc.Tags, apimodel.Tag{Name: "trait"})
	}
	return s
}

// flagsOf returns the flags a docblock sets.
func flagsOf(info *docInfo) apimodel.Flags {
	return apimodel.Flags{Internal: info.Mods.Internal, Stability: info.Mods.Stability,
		Readonly: info.Mods.Readonly, Abstract: info.Mods.Abstract}
}

// docOf returns the docblock's Doc, marked deprecated by a #[\Deprecated]
// attribute, or nil when it says nothing.
func docOf(info *docInfo, attrs []string) *apimodel.Doc {
	d := info.Doc
	if d.Deprecated == nil {
		if text, ok := deprecatedAttr(attrs); ok {
			d.Deprecated = &text
		}
	}
	if d.Summary == "" && d.Body == "" && d.Returns == "" && d.Deprecated == nil && d.Since == "" &&
		d.Default == "" && len(d.Throws)+len(d.Examples)+len(d.See)+len(d.Tags) == 0 {
		return nil
	}
	return d
}

// firstOf returns a when it is not "", else b.
func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
