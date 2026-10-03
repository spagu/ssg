package dts

import "github.com/spagu/ssg/internal/apimodel"

// node is one declaration or member as written: names and the source text
// of every type position, before anything is resolved. Building the model
// from nodes is a second pass, once every module's names are known.
type node struct {
	kind       apimodel.Kind
	name       string
	line       int
	doc        string // raw documentation comment, "" when absent
	sigs       []*sigSpan
	typ        string // property, variable and alias type; enum member value
	tparams    []tparamSpan
	extends    []string
	implements []string
	members    []*node // class, interface and enum members
	body       *scope  // namespace contents
	mods       modSet
	env        *env  // where the declaration was written, for name lookup
	fileNS     *file // a namespace standing for a whole module (export * as ns)
}

// env is a lexical environment: a scope, the file it is in and the
// enclosing environment (nil at the file's top level).
type env struct {
	scope  *scope
	file   *file
	parent *env
}

// modSet holds the modifiers a declaration or member was written with.
type modSet struct {
	exported, isDefault          bool
	optional, readonly, static   bool
	abstract, protected, private bool
	getter, setter               bool
}

// sigSpan is one call signature: function, method, constructor or overload.
type sigSpan struct {
	tparams []tparamSpan
	params  []paramSpan
	returns string
	doc     string
	line    int
}

// paramSpan is one parameter as written.
type paramSpan struct {
	name, typ, def string
	optional, rest bool
}

// tparamSpan is one type parameter: <name extends constraint = def>.
type tparamSpan struct {
	name, constraint, def string
}

// exportRef is one exported name: a local declaration, or a name taken from
// another module (from is the module specifier, name "*" for the module
// itself as a namespace).
type exportRef struct {
	local string
	from  string
	name  string
}

// importRef is one imported binding: name from the module specifier, with
// "*" for a namespace import and "default" for the default export.
type importRef struct {
	from string
	name string
}

// scope is a list of declarations with the names it exports: a file, a
// namespace body or a `declare module "name"` block.
type scope struct {
	decls   map[string]*node
	order   []*node
	exports map[string]exportRef
	stars   []string // specifiers of `export * from`
	// explicit reports that the scope says what it exports; without any
	// export statement (a global script, an ambient block) every declaration
	// is public.
	explicit bool
}

// newScope returns an empty scope.
func newScope() *scope {
	return &scope{decls: map[string]*node{}, exports: map[string]exportRef{}}
}

// file is one parsed declaration file.
type file struct {
	path     string // relative to the package root, "/" separated
	top      *scope
	imports  map[string]importRef
	ambients map[string]*scope // declare module "name" { … }
	doc      string            // the file's leading documentation comment
	srcMap   *sourceMap
}
