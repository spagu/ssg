// Package apimodel is the language-neutral model of a code API that ssg
// documents (GO-103): packages, modules and the symbols they export, with
// signatures, types and documentation comments.
//
// Extractors fill it (JavaScript and TypeScript declarations alike), the
// generator renders pages from it, and api.json publishes it as a contract
// for tools and agents. It does no I/O and knows no language: a TypeRef is a
// tree, not a string to be parsed again by every template.
package apimodel

// Schema versions api.json. Bump it on any change a reader could notice.
const Schema = 1

// API is the root of the model: every documented package of one build.
type API struct {
	Schema   int        `json:"schema"`
	Packages []*Package `json:"packages"`
}

// Package is one documented library — a package.json, or one entry of
// api_docs in the configuration.
type Package struct {
	Name    string    `json:"name"`
	Version string    `json:"version,omitempty"`
	Readme  string    `json:"readme,omitempty"` // Markdown, shown as the package's front page
	Modules []*Module `json:"modules"`
}

// Module is one importable entry point or file, e.g. "index" or "parser/lex".
type Module struct {
	ID      string    `json:"id"`   // "pkg/path", see ModuleID
	Path    string    `json:"path"` // module path within its package
	Doc     *Doc      `json:"doc,omitempty"`
	Symbols []*Symbol `json:"symbols"`
}

// Kind is what a symbol is.
type Kind string

// The kinds of symbol the model knows.
const (
	KindNamespace   Kind = "namespace"
	KindClass       Kind = "class"
	KindInterface   Kind = "interface"
	KindFunction    Kind = "function"
	KindMethod      Kind = "method"
	KindConstructor Kind = "constructor"
	KindProperty    Kind = "property"
	KindAccessor    Kind = "accessor"
	KindType        Kind = "type"
	KindEnum        Kind = "enum"
	KindEnumMember  Kind = "enumMember"
	KindVariable    Kind = "variable"
)

// Symbol is one exported name, or a member of one.
type Symbol struct {
	ID         string       `json:"id"` // "pkg/module#Name.member", see SymbolID
	Name       string       `json:"name"`
	Kind       Kind         `json:"kind"`
	Signatures []*Signature `json:"signatures,omitempty"` // functions, methods, constructors; several = overloads
	Type       *TypeRef     `json:"type,omitempty"`       // properties, variables, type aliases
	TypeParams []*TypeParam `json:"typeParams,omitempty"`
	Extends    []*TypeRef   `json:"extends,omitempty"`
	Implements []*TypeRef   `json:"implements,omitempty"`
	Members    []*Symbol    `json:"members,omitempty"` // class/interface/enum/namespace contents
	Flags      Flags        `json:"flags,omitzero"`
	Source     *Source      `json:"source,omitempty"`
	Doc        *Doc         `json:"doc,omitempty"`
}

// Flags are the modifiers a symbol carries.
type Flags struct {
	Static    bool      `json:"static,omitempty"`
	Readonly  bool      `json:"readonly,omitempty"`
	Optional  bool      `json:"optional,omitempty"`
	Abstract  bool      `json:"abstract,omitempty"`
	Async     bool      `json:"async,omitempty"`
	Default   bool      `json:"default,omitempty"` // the module's default export
	Stability Stability `json:"stability,omitempty"`
	Internal  bool      `json:"internal,omitempty"` // hidden unless visibility asks for it
}

// Stability marks how settled a symbol is; empty means stable.
type Stability string

// Stability levels, from documentation modifiers.
const (
	Stable       Stability = ""
	Beta         Stability = "beta"
	Alpha        Stability = "alpha"
	Experimental Stability = "experimental"
)

// Signature is one callable shape of a function, method or constructor.
type Signature struct {
	TypeParams []*TypeParam `json:"typeParams,omitempty"`
	Params     []*Param     `json:"params,omitempty"`
	Returns    *TypeRef     `json:"returns,omitempty"`
	Doc        *Doc         `json:"doc,omitempty"` // per-overload documentation
}

// Param is one parameter of a signature.
type Param struct {
	Name     string   `json:"name"`
	Type     *TypeRef `json:"type,omitempty"`
	Optional bool     `json:"optional,omitempty"`
	Rest     bool     `json:"rest,omitempty"`
	Default  string   `json:"default,omitempty"` // source text of the default value
	Doc      string   `json:"doc,omitempty"`     // Markdown from @param
}

// TypeParam is a generic parameter: <T extends Base = Default>.
type TypeParam struct {
	Name       string   `json:"name"`
	Constraint *TypeRef `json:"constraint,omitempty"`
	Default    *TypeRef `json:"default,omitempty"`
	Doc        string   `json:"doc,omitempty"`
}

// Source is where a symbol is defined, for links to the code.
type Source struct {
	File string `json:"file"` // path relative to the package root
	Line int    `json:"line"`
}

// Doc is a parsed documentation comment.
type Doc struct {
	Summary    string   `json:"summary,omitempty"` // first paragraph, Markdown
	Body       string   `json:"body,omitempty"`    // the rest, Markdown
	Returns    string   `json:"returns,omitempty"`
	Throws     []string `json:"throws,omitempty"`
	Examples   []string `json:"examples,omitempty"`   // Markdown, usually a fenced code block
	Deprecated *string  `json:"deprecated,omitempty"` // non-nil when deprecated; the text says what to use instead
	Since      string   `json:"since,omitempty"`
	See        []string `json:"see,omitempty"`
	Default    string   `json:"default,omitempty"`
	// Tags keeps block tags the model has no field for, so a theme can show them.
	Tags []Tag `json:"tags,omitempty"`
}

// Tag is a block tag kept verbatim: @name text.
type Tag struct {
	Name string `json:"name"`
	Text string `json:"text,omitempty"`
}
