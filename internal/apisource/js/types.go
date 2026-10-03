package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/apisource/tstype"
)

// builder turns resolved declarations into symbols. resolve maps a type
// name to the ID of a symbol this package documents.
type builder struct {
	ld      *loader
	resolve tstype.Resolver
}

// where locates a declaration, for diagnostics.
type where struct {
	file string
	line int
}

// typeOf parses a type expression from a comment; "" gives nil. An
// expression that cannot be read is kept as written, with a warning.
func (b *builder) typeOf(expr string, at where) *apimodel.TypeRef {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	t, err := tstype.Parse(expr, b.resolve)
	if err != nil {
		b.ld.diag(apisource.Warning, at.file, at.line, "cannot read the type {"+expr+"}: "+err.Error())
	}
	return t
}

// typeParams builds type parameters from @template tags.
func (b *builder) typeParams(tags []apidoc.ParamTag, at where) []*apimodel.TypeParam {
	var out []*apimodel.TypeParam
	for _, t := range tags {
		out = append(out, &apimodel.TypeParam{Name: t.Name, Constraint: b.typeOf(t.Type, at), Doc: t.Text})
	}
	return out
}

// withTemplates returns a builder whose resolver leaves the template names
// unresolved: inside a generic declaration, T is the parameter, not a
// symbol that happens to share its name.
func (b *builder) withTemplates(tags []apidoc.ParamTag) *builder {
	if len(tags) == 0 || b.resolve == nil {
		return b
	}
	names := map[string]bool{}
	for _, t := range tags {
		names[t.Name] = true
	}
	outer := b.resolve
	return &builder{ld: b.ld, resolve: func(name string) string {
		head, _, _ := strings.Cut(name, ".")
		if names[head] {
			return ""
		}
		return outer(name)
	}}
}

// typedefSymbol builds a type symbol from @typedef: an object type with
// fields when it lists @property tags on Object, else an alias.
func (b *builder) typedefSymbol(t *typeDecl, id string) *apimodel.Symbol {
	td := t.typedef
	b = b.withTemplates(t.comment.Templates)
	sym := b.typeSymbol(t, id, td.Doc)
	switch base := strings.TrimSpace(td.Type); {
	case len(td.Properties) > 0 && (base == "" || strings.EqualFold(base, "object")):
		obj := &apimodel.TypeRef{Kind: apimodel.TypeObject}
		for _, p := range td.Properties {
			obj.Fields = append(obj.Fields, b.tagParam(p, t.at()))
		}
		sym.Type = obj
	default:
		sym.Type = b.typeOf(base, t.at())
	}
	return sym
}

// callbackSymbol builds a type symbol whose type is a function, from
// @callback with its @param and @returns.
func (b *builder) callbackSymbol(t *typeDecl, id string) *apimodel.Symbol {
	cb := t.callback
	b = b.withTemplates(t.comment.Templates)
	sym := b.typeSymbol(t, id, cb.Doc)
	sig := &apimodel.Signature{}
	for _, p := range cb.Params {
		sig.Params = append(sig.Params, b.tagParam(p, t.at()))
	}
	if cb.Returns != nil {
		sig.Returns = b.typeOf(cb.Returns.Type, t.at())
	}
	sym.Type = &apimodel.TypeRef{Kind: apimodel.TypeFunction, Signature: sig}
	return sym
}

// typeSymbol is what typedef and callback symbols share.
func (b *builder) typeSymbol(t *typeDecl, id string, doc *apimodel.Doc) *apimodel.Symbol {
	sym := &apimodel.Symbol{
		ID: id, Name: t.name, Kind: apimodel.KindType, Flags: flagsOf(t.comment.Mods),
		TypeParams: b.typeParams(t.comment.Templates, t.at()),
		Source:     &apimodel.Source{File: t.file.path, Line: t.line},
	}
	sym.Doc = typeDoc(t.comment, doc)
	return sym
}

// typeDoc is the documentation of a typedef or callback: the text after its
// name, or — when the comment declares just this one type — the comment's
// description, which is where it is usually written.
func typeDoc(c *apidoc.Comment, own *apimodel.Doc) *apimodel.Doc {
	if own == nil {
		own = &apimodel.Doc{}
	}
	if own.Summary != "" || len(c.Typedefs)+len(c.Callbacks) > 1 {
		trimmed := *own
		return nonEmpty(&trimmed)
	}
	doc := *c.Doc
	if own.Returns != "" {
		doc.Returns = own.Returns
	}
	return nonEmpty(&doc)
}
