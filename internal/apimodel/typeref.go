package apimodel

import "strings"

// TypeKind is the shape of a TypeRef.
type TypeKind string

// The shapes a type can take.
const (
	TypeName         TypeKind = "name"         // string, Promise<T>, a symbol (Ref set)
	TypeUnion        TypeKind = "union"        // A | B
	TypeIntersection TypeKind = "intersection" // A & B
	TypeArray        TypeKind = "array"        // T[]
	TypeTuple        TypeKind = "tuple"        // [A, B]
	TypeLiteral      TypeKind = "literal"      // "on", 42, true
	TypeFunction     TypeKind = "function"     // (a: A) => R
	TypeObject       TypeKind = "object"       // { a: A }
	TypeVerbatim     TypeKind = "verbatim"     // conditional, mapped, template literal: shown as written
)

// TypeRef is a type as a tree, so a template can link each named part to its
// symbol instead of re-parsing a string.
type TypeRef struct {
	Kind TypeKind `json:"kind"`
	// Name is the type's name for TypeName, the literal's text for
	// TypeLiteral, the source text for TypeVerbatim.
	Name string `json:"name,omitempty"`
	// Ref is the ID of the symbol a TypeName points at, when it is one of the
	// documented symbols; empty for built-ins and outside types.
	Ref string `json:"ref,omitempty"`
	// Args are type arguments (Promise<T>), the members of a union,
	// intersection or tuple, or the element of an array.
	Args []*TypeRef `json:"args,omitempty"`
	// Signature is the shape of a TypeFunction.
	Signature *Signature `json:"signature,omitempty"`
	// Fields are the properties of a TypeObject.
	Fields []*Param `json:"fields,omitempty"`
}

// Named returns a TypeName reference, optionally pointing at a symbol.
func Named(name, ref string, args ...*TypeRef) *TypeRef {
	return &TypeRef{Kind: TypeName, Name: name, Ref: ref, Args: args}
}

// String renders the type as TypeScript-like text, for plain output such as
// search records, Markdown pages and tests. Templates render the tree.
func (t *TypeRef) String() string {
	if t == nil {
		return "unknown"
	}
	switch t.Kind {
	case TypeUnion:
		return joinTypes(t.Args, " | ")
	case TypeIntersection:
		return joinTypes(t.Args, " & ")
	case TypeArray:
		if len(t.Args) == 0 {
			return "unknown[]"
		}
		el := t.Args[0].String()
		if k := t.Args[0].Kind; k == TypeUnion || k == TypeIntersection || k == TypeFunction {
			el = "(" + el + ")"
		}
		return el + "[]"
	case TypeTuple:
		return "[" + joinTypes(t.Args, ", ") + "]"
	case TypeFunction:
		return t.Signature.arrow()
	case TypeObject:
		parts := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			parts[i] = f.String()
		}
		return "{ " + strings.Join(parts, "; ") + " }"
	case TypeName:
		if len(t.Args) > 0 {
			return t.Name + "<" + joinTypes(t.Args, ", ") + ">"
		}
	}
	return t.Name
}

// joinTypes renders a list of types with a separator.
func joinTypes(ts []*TypeRef, sep string) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.String()
	}
	return strings.Join(parts, sep)
}

// String renders a parameter: "...name?: Type".
func (p *Param) String() string {
	s := p.Name
	if p.Rest {
		s = "..." + s
	}
	if p.Optional {
		s += "?"
	}
	return s + ": " + p.Type.String()
}

// arrow renders a signature as a function type: (a: A) => R.
func (s *Signature) arrow() string {
	if s == nil {
		return "() => unknown"
	}
	return "(" + s.paramList() + ") => " + s.Returns.String()
}

// paramList renders the parameters, comma-separated.
func (s *Signature) paramList() string {
	parts := make([]string, len(s.Params))
	for i, p := range s.Params {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

// Declaration renders a signature for a named callable: name<T>(a: A): R.
func (s *Signature) Declaration(name string) string {
	tp := ""
	if len(s.TypeParams) > 0 {
		names := make([]string, len(s.TypeParams))
		for i, p := range s.TypeParams {
			names[i] = p.Name
			if p.Constraint != nil {
				names[i] += " extends " + p.Constraint.String()
			}
		}
		tp = "<" + strings.Join(names, ", ") + ">"
	}
	return name + tp + "(" + s.paramList() + "): " + s.Returns.String()
}
