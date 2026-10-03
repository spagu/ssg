package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// builtinTypes are the type names that name no class: PHP's own and the
// common PHPDoc pseudo-types. Compared in lower case.
var builtinTypes = map[string]bool{
	"int": true, "integer": true, "float": true, "double": true, "string": true, "bool": true,
	"boolean": true, "array": true, "callable": true, "iterable": true, "object": true,
	"mixed": true, "void": true, "null": true, "never": true, "false": true, "true": true,
	"resource": true, "scalar": true, "numeric": true, "list": true, "non-empty-list": true,
	"non-empty-array": true, "array-key": true, "class-string": true, "non-empty-string": true,
	"numeric-string": true, "literal-string": true, "positive-int": true, "negative-int": true,
	"non-negative-int": true, "callable-string": true, "key-of": true, "value-of": true,
}

// resolver turns type names into references to documented symbols.
type resolver struct {
	byName map[string]string // lower-case fully qualified class name → symbol ID
}

// named returns a reference to a type name. A class name is resolved
// through ctx's namespace and imports and points at its symbol when it is
// documented; "self", "static" and "$this" point at the enclosing class,
// "parent" at its parent. The name is shown as written, minus a leading "\".
func (r *resolver) named(name string, ctx typeCtx) *apimodel.TypeRef {
	shown := strings.TrimPrefix(name, "\\")
	switch lower := strings.ToLower(name); {
	case lower == "self" || lower == "static" || lower == "$this":
		return apimodel.Named(shown, ctx.self)
	case lower == "parent":
		if ctx.parent == "" {
			return apimodel.Named(shown, "")
		}
		return apimodel.Named(shown, r.ref(ctx.parent, ctx.scope))
	case builtinTypes[lower]:
		return apimodel.Named(shown, "")
	}
	return apimodel.Named(shown, r.ref(name, ctx.scope))
}

// ref returns the symbol ID a class name refers to from scope s, or "".
func (r *resolver) ref(name string, s *scope) string {
	return r.byName[strings.ToLower(s.resolve(name))]
}

// names returns references to class names as written, for extends and
// implements.
func (r *resolver) names(list []string, ctx typeCtx) []*apimodel.TypeRef {
	var out []*apimodel.TypeRef
	for _, n := range list {
		out = append(out, r.named(n, ctx))
	}
	return out
}
