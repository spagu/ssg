package golang

import (
	"go/ast"

	"github.com/spagu/ssg/internal/apimodel"
)

// typeParamSet is the type parameters in scope, which shadow package types.
type typeParamSet map[string]bool

// with returns the set plus the names a type parameter list declares.
func (s typeParamSet) with(list *ast.FieldList) typeParamSet {
	out := typeParamSet{}
	for k := range s {
		out[k] = true
	}
	if list != nil {
		for _, f := range list.List {
			for _, n := range f.Names {
				out[n.Name] = true
			}
		}
	}
	return out
}

// typeRef turns a type expression into a TypeName with its text as written,
// pointing at the documented type it is built on, if any.
func (b *modBuilder) typeRef(e ast.Expr, tps typeParamSet) *apimodel.TypeRef {
	return apimodel.Named(b.ix.print(e), b.ref(e, tps))
}

// ref is the symbol ID of the named type an expression is built on: the
// element of a pointer, slice, array, map value or channel, the
// base of a generic instantiation. "" for built-ins, type parameters and
// types of packages outside the extraction.
func (b *modBuilder) ref(e ast.Expr, tps typeParamSet) string {
	switch t := e.(type) {
	case *ast.Ident:
		if tps[t.Name] {
			return ""
		}
		return b.ix.types[b.d.importPath][t.Name]
	case *ast.SelectorExpr:
		x, ok := t.X.(*ast.Ident)
		if !ok {
			return ""
		}
		file := b.ix.fset.Position(t.Pos()).Filename
		return b.ix.types[b.d.imports[file][x.Name]][t.Sel.Name]
	case *ast.StarExpr:
		return b.ref(t.X, tps)
	case *ast.ArrayType:
		return b.ref(t.Elt, tps)
	case *ast.MapType:
		return b.ref(t.Value, tps)
	case *ast.ChanType:
		return b.ref(t.Value, tps)
	case *ast.ParenExpr:
		return b.ref(t.X, tps)
	case *ast.IndexExpr:
		return b.ref(t.X, tps)
	case *ast.IndexListExpr:
		return b.ref(t.X, tps)
	}
	return ""
}

// typeParams lists the type parameters of a declaration.
func (b *modBuilder) typeParams(list *ast.FieldList, tps typeParamSet) []*apimodel.TypeParam {
	if list == nil {
		return nil
	}
	var out []*apimodel.TypeParam
	for _, f := range list.List {
		for _, n := range f.Names {
			out = append(out, &apimodel.TypeParam{Name: n.Name, Constraint: b.typeRef(f.Type, tps)})
		}
	}
	return out
}

// signature builds the callable shape of a function type, code being its
// declaration as written. tps holds the receiver's type parameters.
func (b *modBuilder) signature(ft *ast.FuncType, code string, tps typeParamSet) *apimodel.Signature {
	tps = tps.with(ft.TypeParams)
	sig := &apimodel.Signature{TypeParams: b.typeParams(ft.TypeParams, tps), Code: code}
	for _, f := range ft.Params.List {
		typ, rest := f.Type, false
		if el, ok := typ.(*ast.Ellipsis); ok {
			typ, rest = el.Elt, true
		}
		names := []string{"_"}
		if len(f.Names) > 0 {
			names = names[:0]
			for _, n := range f.Names {
				names = append(names, n.Name)
			}
		}
		for _, n := range names {
			sig.Params = append(sig.Params, &apimodel.Param{Name: n, Type: b.typeRef(typ, tps), Rest: rest})
		}
	}
	sig.Returns = b.results(ft.Results, tps)
	return sig
}

// results is the return type: nil for none, the type for one, a tuple for
// several.
func (b *modBuilder) results(list *ast.FieldList, tps typeParamSet) *apimodel.TypeRef {
	if list == nil || len(list.List) == 0 {
		return nil
	}
	var out []*apimodel.TypeRef
	for _, f := range list.List {
		for range max(len(f.Names), 1) {
			out = append(out, b.typeRef(f.Type, tps))
		}
	}
	if len(out) == 1 {
		return out[0]
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeTuple, Args: out}
}

// receiverTypeParams is the type parameters a method's receiver names:
// T in "func (s *Set[T]) Add(v T)".
func receiverTypeParams(recv *ast.FieldList) typeParamSet {
	out := typeParamSet{}
	if recv == nil || len(recv.List) == 0 {
		return out
	}
	e := recv.List[0].Type
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	var idx []ast.Expr
	switch t := e.(type) {
	case *ast.IndexExpr:
		idx = []ast.Expr{t.Index}
	case *ast.IndexListExpr:
		idx = t.Indices
	}
	for _, x := range idx {
		if id, ok := x.(*ast.Ident); ok {
			out[id.Name] = true
		}
	}
	return out
}
