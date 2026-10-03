package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// decl is one top-level declaration as read from a file, before it becomes
// a model symbol: a class-like, a function or a constant.
type decl struct {
	kind       apimodel.Kind // KindClass, KindInterface, KindEnum, KindFunction or KindVariable
	trait      bool          // a trait, documented as a class
	name       string
	scope      *scope // namespace and imports in force where it is declared
	file       string
	line       int
	doc        string   // raw docblock text, "" when none
	attrs      []string // #[...] attributes written before it
	code       string   // the declaration as written, without body
	abstract   bool
	readonly   bool     // a readonly class: every property is readonly
	extends    []string // type names as written
	implements []string
	typ        string   // a constant's declared type
	sig        *sigDecl // a function's signature
	members    []*member
}

// fqName is the declaration's fully qualified name, without a leading "\".
func (d *decl) fqName() string { return d.scope.qualify(d.name) }

// member is one public member of a class-like.
type member struct {
	kind     apimodel.Kind // KindEnumMember, KindVariable, KindProperty, KindConstructor or KindMethod
	name     string
	line     int
	doc      string
	attrs    []string
	code     string
	static   bool
	abstract bool
	readonly bool
	typ      string   // a property's or constant's declared type
	sig      *sigDecl // a method's or constructor's signature
	param    bool     // a promoted constructor parameter; doc is the constructor's
}

// sigDecl is the signature of a function, method or constructor.
type sigDecl struct {
	params  []*paramDecl
	returns string // the return type as written, "" when none
	code    string
}

// paramDecl is one parameter as written.
type paramDecl struct {
	name     string // without "$"
	typ      string
	def      string // default value source text
	rest     bool   // ...$args
	promoted bool   // a constructor parameter that is also a property
	public   bool   // a promoted parameter declared public
	readonly bool
	line     int
	code     string // the parameter as written, for a promoted property
}

// scope is the namespace a declaration lives in and the class imports
// (use statements) in force there.
type scope struct {
	ns   string            // "Acme\Textkit", "" for the global namespace
	uses map[string]string // lower-case alias → fully qualified name
}

// newScope starts a namespace with no imports.
func newScope(ns string) *scope {
	return &scope{ns: strings.Trim(ns, "\\"), uses: map[string]string{}}
}

// qualify returns the fully qualified name of a declaration named name.
func (s *scope) qualify(name string) string {
	if s.ns == "" {
		return name
	}
	return s.ns + "\\" + name
}

// resolve returns the fully qualified name a class name refers to:
// "\X" is already qualified, an imported alias is replaced, and anything
// else is relative to the namespace.
func (s *scope) resolve(name string) string {
	if strings.HasPrefix(name, "\\") {
		return name[1:]
	}
	first, rest, qualified := strings.Cut(name, "\\")
	if strings.EqualFold(first, "namespace") && qualified {
		return s.qualify(rest)
	}
	if fq, ok := s.uses[strings.ToLower(first)]; ok {
		if qualified {
			return fq + "\\" + rest
		}
		return fq
	}
	return s.qualify(name)
}

// modulePath is the module a namespace becomes: "Acme\Textkit" →
// "Acme/Textkit", the global namespace → "global".
func modulePath(ns string) string {
	if ns == "" {
		return "global"
	}
	return strings.ReplaceAll(ns, "\\", "/")
}
