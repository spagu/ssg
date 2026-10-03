package tstype

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// builtins are the primitive and special type names; they never resolve.
// null and undefined are names here, not literals.
var builtins = map[string]bool{
	"any": true, "unknown": true, "never": true, "void": true,
	"null": true, "undefined": true, "object": true, "this": true,
	"string": true, "number": true, "boolean": true, "bigint": true, "symbol": true,
}

// builtin returns an unresolved TypeName for a built-in type.
func builtin(name string) *apimodel.TypeRef {
	return apimodel.Named(name, "")
}

// named builds a TypeName and resolves it unless it is a built-in or a type
// parameter in scope. A qualified name is shadowed by its first segment
// (T.x where T is a type parameter); generics resolve by their bare name.
func (p *parser) named(name string, args []*apimodel.TypeRef) *apimodel.TypeRef {
	t := apimodel.Named(name, "", args...)
	if p.resolve == nil || builtins[name] || p.inScope(name) {
		return t
	}
	t.Ref = p.resolve(name)
	return t
}

// inScope reports whether the name (or its first segment) is a type
// parameter of an enclosing generic signature.
func (p *parser) inScope(name string) bool {
	head, _, _ := strings.Cut(name, ".")
	for _, s := range p.scope {
		if s == head {
			return true
		}
	}
	return false
}

// pushScope marks the scope depth; the returned func drops every type
// parameter added since, when the generic signature ends.
func (p *parser) pushScope() func() {
	n := len(p.scope)
	return func() { p.scope = p.scope[:n] }
}
