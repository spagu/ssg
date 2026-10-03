package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
	"github.com/tdewolff/parse/v2/js"
)

// classBuilder fills the members of one class symbol.
type classBuilder struct {
	b    *builder
	sym  *apimodel.Symbol
	file *file
	cur  *memberCursor
	byID map[string]*apimodel.Symbol
}

// class fills a class symbol: type parameters, the class it extends, and
// its public members. #private members and members whose comment says
// @private or @hidden are left out; static blocks are not members.
func (b *builder) class(sym *apimodel.Symbol, c *apidoc.Comment, n *js.ClassDecl, d *decl, at where) {
	sym.Kind = apimodel.KindClass
	b = b.withTemplates(c.Templates)
	sym.TypeParams = b.typeParams(c.Templates, at)
	if n.Extends != nil {
		sym.Extends = []*apimodel.TypeRef{b.heritage(nodeText(n.Extends), at)}
	}
	cb := &classBuilder{b: b, sym: sym, file: d.file, cur: d.file.scan.membersAfter(d.start, "class"),
		byID: map[string]*apimodel.Symbol{}}
	for _, el := range n.List {
		switch {
		case el.StaticBlock != nil:
		case el.Method != nil:
			cb.method(el.Method)
		default:
			cb.field(el.Field)
		}
	}
}

// heritage reads the expression after "extends": a (dotted) name becomes a
// type that may resolve; anything else (a mixin call) is kept as written.
func (b *builder) heritage(text string, at where) *apimodel.TypeRef {
	for _, part := range strings.Split(text, ".") {
		if !isIdent(part) {
			return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: text}
		}
	}
	return b.typeOf(text, at)
}

// member starts a member symbol, or returns nil when it is not public.
func (cb *classBuilder) member(name js.ClassElementName, kind apimodel.Kind, static bool) (*apimodel.Symbol, *apidoc.Comment) {
	text := nodeText(name)
	at := cb.cur.find(text)
	c, has := cb.file.commentAt(at)
	if name.Private != nil || skipped(c) {
		return nil, nil
	}
	m := &apimodel.Symbol{ID: apimodel.MemberID(cb.sym.ID, unquote(text)), Name: unquote(text), Kind: kind,
		Flags: flagsOf(c.Mods), Source: cb.file.sourceAt(at)}
	m.Flags.Static = static
	if has {
		m.Doc = nonEmpty(c.Doc)
	}
	return m, c
}

// add appends a member unless one with its ID is already there; a getter
// and setter pair merges into one accessor.
func (cb *classBuilder) add(m *apimodel.Symbol, setter bool) {
	prev, ok := cb.byID[m.ID]
	if !ok {
		cb.byID[m.ID] = m
		cb.sym.Members = append(cb.sym.Members, m)
		return
	}
	if prev.Kind != apimodel.KindAccessor || m.Kind != apimodel.KindAccessor {
		return
	}
	if prev.Type == nil {
		prev.Type = m.Type
	}
	if prev.Doc == nil {
		prev.Doc = m.Doc
	}
	if setter {
		prev.Flags.Readonly = m.Flags.Readonly
	}
}

// method adds a method, constructor or accessor.
func (cb *classBuilder) method(md *js.MethodDecl) {
	kind := apimodel.KindMethod
	switch {
	case md.Get || md.Set:
		kind = apimodel.KindAccessor
	case !md.Static && md.Name.IsIdent([]byte("constructor")):
		kind = apimodel.KindConstructor
	}
	m, c := cb.member(md.Name, kind, md.Static)
	if m == nil {
		return
	}
	at := where{file: cb.file.path}
	if m.Source != nil {
		at.line = m.Source.Line
	}
	if kind == apimodel.KindAccessor {
		m.Type = cb.b.accessorType(md, c, at)
		m.Flags.Readonly = m.Flags.Readonly || md.Get
		cb.add(m, md.Set)
		return
	}
	m.Flags.Async = md.Async
	m.Signatures = []*apimodel.Signature{cb.b.signature(md.Params, c, at)}
	if md.Generator {
		markGenerator(m)
	}
	cb.add(m, false)
}

// accessorType is the type of a getter (@type or @returns) or setter
// (@type or its one @param).
func (b *builder) accessorType(md *js.MethodDecl, c *apidoc.Comment, at where) *apimodel.TypeRef {
	expr := c.Type
	switch {
	case expr != "":
	case md.Get && c.Returns != nil:
		expr = c.Returns.Type
	case md.Set && len(c.Params) > 0:
		expr = c.Params[0].Type
	}
	return b.typeOf(expr, at)
}

// field adds a class field as a property.
func (cb *classBuilder) field(f js.Field) {
	m, c := cb.member(f.Name, apimodel.KindProperty, f.Static)
	if m == nil {
		return
	}
	at := where{file: cb.file.path}
	if m.Source != nil {
		at.line = m.Source.Line
	}
	m.Type = cb.b.typeOf(c.Type, at)
	if m.Type == nil {
		m.Type = literalType(f.Init)
	}
	cb.add(m, false)
}
