package php

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// typeCtx is where a type is written: the scope that resolves its class
// names, and the class "self"/"static" and "parent" point at.
type typeCtx struct {
	scope  *scope
	self   string // symbol ID of the enclosing class-like, "" outside one
	parent string // the enclosing class's first parent as written
}

// typeRef parses a PHP or PHPDoc type: unions (A|B), intersections (A&B),
// nullables (?T), DNF groups ((A&B)|null), arrays (T[]), generics
// (array<K, V>), literals and names. A shape it cannot read (array{…},
// callable(…): R) is kept verbatim. "" is no type.
func (r *resolver) typeRef(s string, ctx typeCtx) *apimodel.TypeRef {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if parts := splitTop(s, '|'); len(parts) > 1 {
		return r.compound(apimodel.TypeUnion, parts, ctx)
	}
	if parts := splitTop(s, '&'); len(parts) > 1 {
		return r.compound(apimodel.TypeIntersection, parts, ctx)
	}
	switch {
	case s[0] == '?':
		return &apimodel.TypeRef{Kind: apimodel.TypeUnion, Args: []*apimodel.TypeRef{r.typeRef(s[1:], ctx), apimodel.Named("null", "")}}
	case s[0] == '(' && closingAt(s, 0) == len(s)-1:
		return r.typeRef(s[1:len(s)-1], ctx)
	case strings.HasSuffix(s, "[]") && len(s) > 2:
		return &apimodel.TypeRef{Kind: apimodel.TypeArray, Args: []*apimodel.TypeRef{r.typeRef(s[:len(s)-2], ctx)}}
	case s[0] == '\'' || s[0] == '"' || isDigit(s[0]) || s[0] == '-':
		return &apimodel.TypeRef{Kind: apimodel.TypeLiteral, Name: s}
	case isTypeName(s):
		return r.named(s, ctx)
	}
	if i := strings.IndexByte(s, '<'); i > 0 && isTypeName(s[:i]) && closingAt(s, i) == len(s)-1 {
		t := r.named(s[:i], ctx)
		for _, a := range splitTop(s[i+1:len(s)-1], ',') {
			t.Args = append(t.Args, r.typeRef(a, ctx))
		}
		return t
	}
	return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: s}
}

// compound builds a union or intersection of the parsed parts.
func (r *resolver) compound(kind apimodel.TypeKind, parts []string, ctx typeCtx) *apimodel.TypeRef {
	t := &apimodel.TypeRef{Kind: kind}
	for _, p := range parts {
		t.Args = append(t.Args, r.typeRef(p, ctx))
	}
	return t
}

// splitTop splits s at sep where no bracket or quote is open.
func splitTop(s string, sep byte) []string {
	var parts []string
	depth, from := 0, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '<' || c == '(' || c == '[' || c == '{':
			depth++
		case c == '>' || c == ')' || c == ']' || c == '}':
			depth--
		case c == '\'' || c == '"':
			if end, ok := skipQuoted(s, i); ok {
				i = end - 1
			}
		case c == sep && depth == 0:
			parts = append(parts, s[from:i])
			from = i + 1
		}
	}
	return append(parts, s[from:])
}

// closingAt returns the index of the bracket closing the one at s[i], or -1.
func closingAt(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '<', '(', '[', '{':
			depth++
		case '>', ')', ']', '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// isTypeName reports whether s is a plain, possibly qualified, name such
// as "int", "\Acme\Doc", "class-string" or "$this".
func isTypeName(s string) bool {
	s = strings.TrimPrefix(s, "$")
	if s == "" || (!isNameStart(s[0]) && s[0] != '\\') {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isNameChar(s[i]) && s[i] != '\\' && s[i] != '-' {
			return false
		}
	}
	return true
}
