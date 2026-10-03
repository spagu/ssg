package golang

import (
	"go/ast"
	"go/doc"
	"sort"

	"github.com/spagu/ssg/internal/apimodel"
)

// modBuilder builds the module of one package: the second pass, with the
// index of every package's types at hand.
type modBuilder struct {
	ix *index
	d  *pkgDir
	cm *commenter
}

// buildModule builds the module page of a package.
func (ix *index) buildModule(d *pkgDir) *apimodel.Module {
	b := &modBuilder{ix: ix, d: d}
	b.cm = b.newCommenter()
	syms := b.symbols()
	if d.internal {
		apimodel.Walk(syms, func(s, _ *apimodel.Symbol) { s.Flags.Internal = true })
	}
	return &apimodel.Module{ID: d.modID, Path: d.modPath, Doc: b.cm.doc(d.docPkg.Doc, d.docPkg.Examples), Symbols: syms}
}

// symbols lists what a package exports, by name, each name once (a name
// declared in files for different platforms is documented from the first).
func (b *modBuilder) symbols() []*apimodel.Symbol {
	p := b.d.docPkg
	var all []*apimodel.Symbol
	for _, f := range p.Funcs {
		all = append(all, b.funcSymbol(f))
	}
	for _, t := range p.Types {
		all = append(all, b.typeSymbol(t))
		for _, f := range t.Funcs {
			all = append(all, b.funcSymbol(f))
		}
		all = append(all, b.values(t.Vars)...)
	}
	all = append(all, b.values(p.Consts)...)
	all = append(all, b.values(p.Vars)...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	out := []*apimodel.Symbol{}
	for _, s := range all {
		if len(out) == 0 || out[len(out)-1].Name != s.Name {
			out = append(out, s)
		}
	}
	return out
}

// funcSymbol builds a top-level function, constructors included.
func (b *modBuilder) funcSymbol(f *doc.Func) *apimodel.Symbol {
	return b.callable(f, apimodel.SymbolID(b.d.modID, f.Name), apimodel.KindFunction)
}

// callable builds a function or method from its declaration; the signature
// shows the declaration without its body.
func (b *modBuilder) callable(f *doc.Func, id string, kind apimodel.Kind) *apimodel.Symbol {
	decl := *f.Decl
	decl.Body, decl.Doc = nil, nil
	sig := b.signature(f.Decl.Type, b.ix.print(&decl), receiverTypeParams(f.Decl.Recv))
	return &apimodel.Symbol{ID: id, Name: f.Name, Kind: kind, Signatures: []*apimodel.Signature{sig},
		Source: b.ix.source(f.Decl.Pos()), Doc: b.cm.doc(f.Doc, f.Examples)}
}

// methods builds the exported methods of a type as members, by name.
func (b *modBuilder) methods(t *doc.Type, typeID string) []*apimodel.Symbol {
	fs := append([]*doc.Func(nil), t.Methods...)
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
	out := make([]*apimodel.Symbol, 0, len(fs))
	for _, f := range fs {
		out = append(out, b.callable(f, apimodel.MemberID(typeID, f.Name), apimodel.KindMethod))
	}
	return out
}

// typeSymbol builds a type: a struct is a class, an interface an
// interface, a type with constants of its own an enum, anything else (an
// alias included) a type.
func (b *modBuilder) typeSymbol(t *doc.Type) *apimodel.Symbol {
	spec := typeSpec(t)
	id := apimodel.SymbolID(b.d.modID, t.Name)
	tps := typeParamSet{}.with(spec.TypeParams)
	s := &apimodel.Symbol{ID: id, Name: t.Name, TypeParams: b.typeParams(spec.TypeParams, tps),
		Source: b.ix.source(spec.Pos()), Doc: b.cm.doc(t.Doc, t.Examples)}
	st, isStruct := spec.Type.(*ast.StructType)
	it, isInterface := spec.Type.(*ast.InterfaceType)
	alias := spec.Assign.IsValid()
	switch {
	case isStruct && !alias:
		s.Kind, s.Code = apimodel.KindClass, b.header(spec, "struct")
		b.fields(s, st, tps)
	case isInterface && !alias:
		s.Kind, s.Code = apimodel.KindInterface, b.header(spec, "interface")
		b.interfaceMembers(s, it, tps)
	default:
		b.namedType(s, spec, tps)
		if len(t.Consts) > 0 {
			s.Kind = apimodel.KindEnum
			s.Members = b.enumMembers(t.Consts, id)
		}
	}
	s.Members = append(s.Members, b.methods(t, id)...)
	return s
}

// namedType fills a type defined from another, or an alias: its whole
// declaration is the code, its underlying type the type.
func (b *modBuilder) namedType(s *apimodel.Symbol, spec *ast.TypeSpec, tps typeParamSet) {
	decl := *spec
	decl.Doc, decl.Comment = nil, nil
	s.Kind, s.Code, s.Type = apimodel.KindType, "type "+b.ix.print(&decl), b.typeRef(spec.Type, tps)
}

// header is the first line of a struct or interface declaration:
// "type Set[T comparable] struct".
func (b *modBuilder) header(spec *ast.TypeSpec, keyword string) string {
	return "type " + b.ix.print(&ast.TypeSpec{Name: spec.Name, TypeParams: spec.TypeParams, Type: ast.NewIdent(keyword)})
}

// typeSpec is the declaration of a documented type: go/doc gives each type
// a GenDecl of its own, holding its one spec.
func typeSpec(t *doc.Type) *ast.TypeSpec {
	return t.Decl.Specs[0].(*ast.TypeSpec)
}
