package python

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// resolver maps a name used in a type to the ID of the documented symbol
// it means, or "".
type resolver func(name string) string

// maxTypeDepth bounds string annotations nested in string annotations.
const maxTypeDepth = 8

// parseType reads an annotation into a TypeRef: unions ("A | B",
// Union, Optional), generics ("list[T]", "Callable[[A], R]"), Literal
// and string forward references. What it cannot read stays verbatim.
func parseType(toks []token, resolve resolver) *apimodel.TypeRef {
	return parseTypeDepth(toks, resolve, 0)
}

// parseTypeText reads an annotation from text, e.g. a docstring type.
func parseTypeText(text string, resolve resolver) *apimodel.TypeRef {
	toks, _ := tokenize(text)
	return parseType(exprTokens(toks), resolve)
}

// exprTokens drops the layout tokens tokenize adds around an expression.
func exprTokens(toks []token) []token {
	out := toks[:0:0]
	for _, t := range toks {
		if t.kind != tNewline && t.kind != tIndent && t.kind != tDedent && t.kind != tEOF {
			out = append(out, t)
		}
	}
	return out
}

// parseTypeDepth is parseType at a nesting depth of string annotations.
func parseTypeDepth(toks []token, resolve resolver, depth int) *apimodel.TypeRef {
	if len(toks) == 0 {
		return nil
	}
	parts := splitTop(toks, "|")
	var args []*apimodel.TypeRef
	for _, part := range parts {
		t := primaryType(part, resolve, depth)
		if t == nil {
			return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: joinToks(toks)}
		}
		args = appendUnion(args, t)
	}
	if len(args) == 1 {
		return args[0]
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeUnion, Args: args}
}

// appendUnion appends a union member, flattening nested unions.
func appendUnion(args []*apimodel.TypeRef, t *apimodel.TypeRef) []*apimodel.TypeRef {
	if t.Kind == apimodel.TypeUnion {
		return append(args, t.Args...)
	}
	return append(args, t)
}

// primaryType reads one member of a union, or returns nil.
func primaryType(toks []token, resolve resolver, depth int) *apimodel.TypeRef {
	first := toks[0]
	switch {
	case len(toks) == 1 && first.kind == tString:
		inner := decodeString(first.text)
		if depth >= maxTypeDepth || strings.TrimSpace(inner) == "" {
			return nil
		}
		itoks, diags := tokenize(inner)
		if len(diags) > 0 {
			return nil
		}
		return parseTypeDepth(exprTokens(itoks), resolve, depth+1)
	case len(toks) == 1 && (first.kind == tNumber || first.is("...")):
		return apimodel.Named(first.text, "")
	case first.is("[") && toks[len(toks)-1].is("]"):
		return &apimodel.TypeRef{Kind: apimodel.TypeTuple, Args: typeArgs(toks[1:len(toks)-1], resolve, depth)}
	case first.kind == tName:
		return namedType(toks, resolve, depth)
	}
	return nil
}

// namedType reads "a.b.Name" with optional "[args]".
func namedType(toks []token, resolve resolver, depth int) *apimodel.TypeRef {
	i := 1
	for i+1 < len(toks) && toks[i].is(".") && toks[i+1].kind == tName {
		i += 2
	}
	name := joinToks(toks[:i])
	if i == len(toks) {
		if name == "None" {
			return apimodel.Named(name, "")
		}
		return apimodel.Named(name, resolve(name))
	}
	if !toks[i].is("[") || !toks[len(toks)-1].is("]") || !balanced(toks[i+1:len(toks)-1]) {
		return nil
	}
	inner := toks[i+1 : len(toks)-1]
	switch name[strings.LastIndex(name, ".")+1:] {
	case "Optional", "Union":
		var args []*apimodel.TypeRef
		for _, a := range typeArgs(inner, resolve, depth) {
			args = appendUnion(args, a)
		}
		if strings.HasSuffix(name, "Optional") {
			args = append(args, apimodel.Named("None", ""))
		}
		return &apimodel.TypeRef{Kind: apimodel.TypeUnion, Args: args}
	case "Literal":
		var args []*apimodel.TypeRef
		for _, part := range splitTop(inner, ",") {
			args = append(args, &apimodel.TypeRef{Kind: apimodel.TypeLiteral, Name: joinToks(part)})
		}
		return apimodel.Named(name, "", args...)
	}
	return apimodel.Named(name, resolve(name), typeArgs(inner, resolve, depth)...)
}

// typeArgs reads comma-separated type arguments.
func typeArgs(toks []token, resolve resolver, depth int) []*apimodel.TypeRef {
	var out []*apimodel.TypeRef
	for _, part := range splitTop(toks, ",") {
		out = append(out, parseTypeDepth(part, resolve, depth))
	}
	return out
}

// balanced reports whether brackets in tokens never close more than they
// opened, so "[" ... "]" around them is one pair.
func balanced(toks []token) bool {
	depth := 0
	for _, t := range toks {
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
		}
		if depth < 0 {
			return false
		}
	}
	return true
}
