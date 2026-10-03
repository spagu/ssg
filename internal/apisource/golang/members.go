package golang

import (
	"go/ast"
	"go/doc"
	"go/token"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// fields adds a struct's exported fields as properties, in declaration
// order, and its embedded types to Extends.
func (b *modBuilder) fields(s *apimodel.Symbol, st *ast.StructType, tps typeParamSet) {
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			s.Extends = append(s.Extends, b.typeRef(f.Type, tps))
			continue
		}
		text := fieldDoc(f)
		for _, n := range f.Names { // go/doc has removed the unexported ones
			s.Members = append(s.Members, &apimodel.Symbol{ID: apimodel.MemberID(s.ID, n.Name), Name: n.Name,
				Kind: apimodel.KindProperty, Type: b.typeRef(f.Type, tps), Code: n.Name + " " + b.ix.print(f.Type),
				Source: b.ix.source(n.Pos()), Doc: b.cm.doc(text, nil)})
		}
	}
}

// interfaceMembers adds an interface's methods as members, in declaration
// order, and its embedded interfaces and type terms to Extends.
func (b *modBuilder) interfaceMembers(s *apimodel.Symbol, it *ast.InterfaceType, tps typeParamSet) {
	for _, f := range it.Methods.List {
		ft, ok := f.Type.(*ast.FuncType)
		if len(f.Names) == 0 || !ok {
			s.Extends = append(s.Extends, b.typeRef(f.Type, tps))
			continue
		}
		name := f.Names[0].Name
		code := name + strings.TrimPrefix(b.ix.print(ft), "func")
		s.Members = append(s.Members, &apimodel.Symbol{ID: apimodel.MemberID(s.ID, name), Name: name,
			Kind: apimodel.KindMethod, Signatures: []*apimodel.Signature{b.signature(ft, code, tps)},
			Source: b.ix.source(f.Pos()), Doc: b.cm.doc(fieldDoc(f), nil)})
	}
}

// fieldDoc is the comment above a field, else the one after it.
func fieldDoc(f *ast.Field) string {
	if text := f.Doc.Text(); text != "" {
		return text
	}
	return f.Comment.Text()
}

// enumMembers builds the constants of an enum type as its members.
func (b *modBuilder) enumMembers(vs []*doc.Value, typeID string) []*apimodel.Symbol {
	var out []*apimodel.Symbol
	for _, v := range vs {
		b.eachValue(v, func(n *ast.Ident, spec *ast.ValueSpec, i int, typ ast.Expr) {
			s := b.value(v.Decl.Tok, "", n, spec, i, typ)
			s.ID, s.Kind, s.Type = apimodel.MemberID(typeID, n.Name), apimodel.KindEnumMember, nil
			out = append(out, s)
		})
	}
	return out
}

// values builds exported constants and variables as top-level symbols.
func (b *modBuilder) values(vs []*doc.Value) []*apimodel.Symbol {
	var out []*apimodel.Symbol
	for _, v := range vs {
		b.eachValue(v, func(n *ast.Ident, spec *ast.ValueSpec, i int, typ ast.Expr) {
			s := b.value(v.Decl.Tok, v.Doc, n, spec, i, typ)
			s.ID = apimodel.SymbolID(b.d.modID, n.Name)
			out = append(out, s)
		})
	}
	return out
}

// eachValue calls fn for every exported name of a const or var group, with
// its type: in a const group a spec without type or values repeats the
// type of the one before it (the iota pattern).
func (b *modBuilder) eachValue(v *doc.Value, fn func(n *ast.Ident, spec *ast.ValueSpec, i int, typ ast.Expr)) {
	var prev ast.Expr
	for _, sp := range v.Decl.Specs {
		spec := sp.(*ast.ValueSpec)
		typ := spec.Type
		if typ == nil && len(spec.Values) == 0 {
			typ = prev
		}
		prev = typ
		for i, n := range spec.Names {
			if ast.IsExported(n.Name) {
				fn(n, spec, i, typ)
			}
		}
	}
}

// value builds one constant or variable. Code shows a constant's value and
// a variable's type, or its value when the type is not written. groupDoc is
// the doc of the whole group, used when the spec has none of its own; enum
// members pass "", the group's doc being the enum's.
func (b *modBuilder) value(tok token.Token, groupDoc string, n *ast.Ident, spec *ast.ValueSpec, i int, typ ast.Expr) *apimodel.Symbol {
	isConst := tok == token.CONST
	code := tok.String() + " " + n.Name
	if typ != nil {
		code += " " + b.ix.print(typ)
	}
	if i < len(spec.Values) && (isConst || typ == nil) {
		code += " = " + truncate(b.ix.print(spec.Values[i]))
	}
	text := fieldDoc(&ast.Field{Doc: spec.Doc, Comment: spec.Comment})
	if text == "" {
		text = groupDoc
	}
	s := &apimodel.Symbol{Name: n.Name, Kind: apimodel.KindVariable, Code: code,
		Flags: apimodel.Flags{Readonly: isConst}, Source: b.ix.source(n.Pos()), Doc: b.cm.doc(text, nil)}
	if typ != nil {
		s.Type = b.typeRef(typ, nil)
	}
	return s
}
