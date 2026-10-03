package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
	"github.com/tdewolff/parse/v2/js"
)

// decl is one top-level declaration: what it is (the syntax node) and where
// it is (its file and the token its statement starts at).
type decl struct {
	name    string   // local name; "default" for an anonymous default export
	node    js.IExpr // function, arrow, method, class, or a value; nil when none
	isConst bool
	file    *file
	start   int // token index of the statement, -1 when not found
}

// comment returns the declaration's doc comment and whether it has one.
func (d *decl) comment() (*apidoc.Comment, bool) {
	return d.file.commentAt(d.start)
}

// source returns where the declaration is, or nil when it was not found.
func (d *decl) source() *apimodel.Source {
	return d.file.sourceAt(d.start)
}

// commentAt returns the doc comment before token i and whether there is
// one; without one it returns an empty comment, never nil.
func (f *file) commentAt(i int) (*apidoc.Comment, bool) {
	if i < 0 || i >= len(f.scan.toks) || f.scan.toks[i].doc < 0 {
		return apidoc.Parse(""), false
	}
	return f.docComment(f.scan.toks[i].doc), true
}

// docComment parses doc comment i of the file once.
func (f *file) docComment(i int) *apidoc.Comment {
	if c, ok := f.parsed[i]; ok {
		return c
	}
	c := apidoc.Parse(f.scan.docs[i].raw)
	f.parsed[i] = c
	return c
}

// sourceAt returns the location of token i, or nil when there is none.
func (f *file) sourceAt(i int) *apimodel.Source {
	if line := f.scan.tokenLine(i); line > 0 {
		return &apimodel.Source{File: f.path, Line: line}
	}
	return nil
}

// nonEmpty returns the doc, or nil when it says nothing — a comment of
// tags the model stores elsewhere (@type, @param) has no prose of its own.
func nonEmpty(d *apimodel.Doc) *apimodel.Doc {
	if d == nil || (d.Summary == "" && d.Body == "" && d.Returns == "" && d.Deprecated == nil &&
		d.Since == "" && d.Default == "" && len(d.Throws)+len(d.Examples)+len(d.See)+len(d.Tags) == 0) {
		return nil
	}
	return d
}

// skipped reports whether a comment hides its symbol.
func skipped(c *apidoc.Comment) bool {
	return c.Mods.Hidden || c.Mods.Private
}

// flagsOf maps comment modifiers to symbol flags.
func flagsOf(m apidoc.Modifiers) apimodel.Flags {
	return apimodel.Flags{Internal: m.Internal, Stability: m.Stability, Readonly: m.Readonly, Abstract: m.Abstract}
}

// declSymbol builds the symbol of a declaration under a public name and ID.
func (b *builder) declSymbol(d *decl, id, name string) *apimodel.Symbol {
	c, has := d.comment()
	sym := &apimodel.Symbol{ID: id, Name: name, Source: d.source(), Flags: flagsOf(c.Mods)}
	if has {
		sym.Doc = nonEmpty(c.Doc)
	}
	at := where{file: d.file.path, line: d.file.scan.tokenLine(d.start)}
	switch n := d.node.(type) {
	case *js.FuncDecl:
		b.function(sym, c, n.Params, n.Async, n.Generator, at)
	case *js.ArrowFunc:
		b.function(sym, c, n.Params, n.Async, false, at)
	case *js.MethodDecl:
		b.function(sym, c, n.Params, n.Async, n.Generator, at)
	case *js.ClassDecl:
		b.class(sym, c, n, d, at)
	default:
		b.variable(sym, c, d.node, d.isConst, at)
	}
	return sym
}

// function fills a function symbol.
func (b *builder) function(sym *apimodel.Symbol, c *apidoc.Comment, params js.Params, async, generator bool, at where) {
	sym.Kind = apimodel.KindFunction
	sym.Flags.Async = async
	sym.Signatures = []*apimodel.Signature{b.signature(params, c, at)}
	if generator {
		markGenerator(sym)
	}
}

// markGenerator notes a generator in the symbol's tags, copying the Doc so
// the shared parsed comment stays as written.
func markGenerator(sym *apimodel.Symbol) {
	doc := apimodel.Doc{}
	if sym.Doc != nil {
		doc = *sym.Doc
	}
	doc.Tags = append(append([]apimodel.Tag{}, doc.Tags...), apimodel.Tag{Name: "generator"})
	sym.Doc = &doc
}

// variable fills a variable symbol: its type from @type, else from a
// literal initialiser.
func (b *builder) variable(sym *apimodel.Symbol, c *apidoc.Comment, value js.IExpr, isConst bool, at where) {
	sym.Kind = apimodel.KindVariable
	sym.Flags.Readonly = sym.Flags.Readonly || isConst
	sym.Type = b.typeOf(c.Type, at)
	if sym.Type == nil {
		sym.Type = literalType(value)
	}
}

// literalType infers the type of a literal value, or nil.
func literalType(value js.IExpr) *apimodel.TypeRef {
	lit, ok := value.(*js.LiteralExpr)
	if !ok {
		return nil
	}
	switch {
	case lit.TokenType == js.StringToken:
		return apimodel.Named("string", "")
	case js.IsNumeric(lit.TokenType):
		return apimodel.Named("number", "")
	case lit.TokenType == js.TrueToken || lit.TokenType == js.FalseToken:
		return apimodel.Named("boolean", "")
	}
	return nil
}

// nodeText renders a syntax node back to source text.
func nodeText(n js.INode) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	n.JS(&sb)
	return sb.String()
}
