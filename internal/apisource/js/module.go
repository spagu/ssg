package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// publicSymbol is one name a module (or namespace) makes public, resolved
// to its definition, with the ID it will have.
type publicSymbol struct {
	name      string // symbol name: the public name, or a default export's own name
	id        string
	isDefault bool
	t         target
	members   []*publicSymbol // the contents of a namespace
}

// entryModule is the plan of one module page.
type entryModule struct {
	file    *file
	id      string
	path    string
	exports []*publicSymbol
	types   []*typeDecl
	typeIDs []string
}

// planModule resolves what an entry module exports and which typedefs and
// callbacks its files declare. Typedef names an export already uses are
// left out.
func (ld *loader) planModule(pkg string, f *file) *entryModule {
	m := &entryModule{file: f, id: apimodel.ModuleID(pkg, f.path)}
	m.path = strings.TrimPrefix(m.id, pkg+"/")
	idOf := func(name string) string { return apimodel.SymbolID(m.id, name) }
	m.exports = ld.publicSymbols(f, idOf, map[string]bool{f.path: true})
	taken := map[string]bool{}
	for _, e := range m.exports {
		taken[e.name] = true
	}
	for _, g := range ld.reachable(f) {
		for _, td := range g.typeDecls() {
			if !taken[td.name] {
				taken[td.name] = true
				m.types = append(m.types, td)
				m.typeIDs = append(m.typeIDs, idOf(td.name))
			}
		}
	}
	return m
}

// publicSymbols lists the visible public names of f. A default export that
// is a named function or class keeps its name unless f also exports that
// name. Stack holds the files of the enclosing namespaces, so a namespace
// that contains itself stops there.
func (ld *loader) publicSymbols(f *file, idOf func(string) string, stack map[string]bool) []*publicSymbol {
	names := ld.exportNames(f)
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}
	var out []*publicSymbol
	for _, public := range names {
		t, ok := ld.resolve(f, public)
		if !ok || !visible(t) {
			continue
		}
		ps := &publicSymbol{name: public, isDefault: public == "default", t: t}
		if ps.isDefault && t.decl != nil && t.decl.name != "default" && !taken[t.decl.name] {
			ps.name = t.decl.name
			taken[ps.name] = true
		}
		ps.id = idOf(ps.name)
		if t.ns != nil && !stack[t.ns.path] {
			stack[t.ns.path] = true
			ps.members = ld.publicSymbols(t.ns, func(n string) string { return apimodel.MemberID(ps.id, n) }, stack)
			delete(stack, t.ns.path)
		}
		out = append(out, ps)
	}
	return out
}

// visible reports whether a target is documented: a namespace always, a
// declaration unless its comment hides it.
func visible(t target) bool {
	if t.decl == nil {
		return true
	}
	c, _ := t.decl.comment()
	return !skipped(c)
}

// typeIndex maps the names a type expression may use to symbol IDs: public
// names, the local names of their definitions, "ns.Name" for namespace
// members, typedefs and callbacks. Modules are visited in ID order and the
// first claim wins, so the map is the same on every run.
func typeIndex(mods []*entryModule) map[string]string {
	ix := map[string]string{}
	claim := func(name, id string) {
		if _, ok := ix[name]; !ok {
			ix[name] = id
		}
	}
	for _, m := range mods {
		for _, e := range m.exports {
			claim(e.name, e.id)
			if e.t.decl != nil {
				claim(e.t.decl.name, e.id)
			}
			for _, c := range e.members {
				claim(e.name+"."+c.name, c.id)
			}
		}
		for i, td := range m.types {
			claim(td.name, m.typeIDs[i])
		}
	}
	return ix
}

// buildModule builds the module page from its plan.
func (b *builder) buildModule(m *entryModule) *apimodel.Module {
	mod := &apimodel.Module{ID: m.id, Path: m.path, Doc: moduleDoc(m.file), Symbols: []*apimodel.Symbol{}}
	for _, e := range m.exports {
		mod.Symbols = append(mod.Symbols, b.publicSymbol(e))
	}
	for i, td := range m.types {
		if td.typedef != nil {
			mod.Symbols = append(mod.Symbols, b.typedefSymbol(td, m.typeIDs[i]))
		} else {
			mod.Symbols = append(mod.Symbols, b.callbackSymbol(td, m.typeIDs[i]))
		}
	}
	return mod
}

// publicSymbol builds the symbol of one public name.
func (b *builder) publicSymbol(ps *publicSymbol) *apimodel.Symbol {
	var sym *apimodel.Symbol
	if ps.t.ns != nil {
		sym = &apimodel.Symbol{ID: ps.id, Name: ps.name, Kind: apimodel.KindNamespace,
			Doc: moduleDoc(ps.t.ns), Source: &apimodel.Source{File: ps.t.ns.path, Line: 1}}
		for _, c := range ps.members {
			sym.Members = append(sym.Members, b.publicSymbol(c))
		}
	} else {
		sym = b.declSymbol(ps.t.decl, ps.id, ps.name)
	}
	sym.Flags.Default = ps.isDefault
	return sym
}
