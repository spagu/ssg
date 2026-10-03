// Package apidoc parses documentation comments — the text between /** and */
// — into the API model's Doc plus the tags an extractor needs: parameter and
// return types written in the comment, typedefs, callbacks, template
// parameters and modifiers (GO-106). It also finds and rewrites inline links
// between symbols ({@link Name}).
//
// This file is the contract the extractors (GO-104, GO-105) and the generator
// (GO-108) build against; the implementation fills it.
package apidoc

import "github.com/spagu/ssg/internal/apimodel"

// Comment is one parsed documentation comment.
type Comment struct {
	// Doc is the description and the block tags the model stores. Doc.Summary
	// is the first paragraph; Doc.Body the rest. Never nil.
	Doc *apimodel.Doc
	// Params are @param tags in the order written. Nested object params
	// ("@param opts.strict") keep their dotted name.
	Params []ParamTag
	// Returns is @returns/@return, with the type in braces if one was given.
	Returns *TypeTag
	// Type is the expression of @type {Expr}, "" when absent.
	Type string
	// Throws are @throws tags; the type part (if any) is kept in Type.
	Throws []TypeTag
	// Typedefs are @typedef {Type} Name blocks with their @property lines.
	Typedefs []Typedef
	// Callbacks are @callback Name blocks with their @param/@returns.
	Callbacks []Callback
	// Templates are @template T (and "@template {Constraint} T").
	Templates []ParamTag
	// Mods are the modifiers that change visibility or stability.
	Mods Modifiers
}

// ParamTag is "@param {Type} [name=default] text" and its variants.
type ParamTag struct {
	Name     string
	Type     string // expression inside braces, "" when absent
	Text     string // Markdown
	Optional bool   // [name] or {Type=}
	Default  string // from [name=default]
	Rest     bool   // {...Type}
}

// TypeTag is a tag that carries an optional type and text.
type TypeTag struct {
	Type string
	Text string
}

// Typedef is "@typedef {Type} Name" with "@property {Type} name text" lines.
type Typedef struct {
	Name       string
	Type       string
	Doc        *apimodel.Doc
	Properties []ParamTag
}

// Callback is "@callback Name" with its own @param and @returns.
type Callback struct {
	Name    string
	Doc     *apimodel.Doc
	Params  []ParamTag
	Returns *TypeTag
}

// Modifiers are tags that change how a symbol is shown.
type Modifiers struct {
	Internal  bool // @internal
	Private   bool // @private
	Hidden    bool // @hidden or @ignore
	Stability apimodel.Stability
	Readonly  bool // @readonly
	Abstract  bool // @abstract
	Override  bool // @override
}

// Link is one inline link found in Markdown: {@link Target} or
// {@link Target | text} (also {@link Target text}, {@linkcode …},
// {@linkplain …}). Start/End are byte offsets of the whole {...} span.
type Link struct {
	Target string
	Text   string
	Code   bool // {@linkcode}: render the text as code
	Start  int
	End    int
}
