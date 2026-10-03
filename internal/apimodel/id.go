package apimodel

import (
	"regexp"
	"strings"
)

// IDs are stable across builds (GO-103): they come from where a symbol lives
// and what it is called, never from its position in a file, so reordering a
// source file changes no URL and no anchor.
//
//	module:  "pkg/path"            e.g. "core/parser/lex"
//	symbol:  "pkg/path#Name"       e.g. "core/parser/lex#Lexer"
//	member:  "pkg/path#Name.member"

// ModuleID is the ID of module path in package pkg. The module path is
// normalised: no leading "./", no extension, "/" separators.
func ModuleID(pkg, path string) string {
	return pkg + "/" + normalizeModulePath(path)
}

// SymbolID is the ID of a top-level symbol of a module.
func SymbolID(moduleID, name string) string { return moduleID + "#" + name }

// MemberID is the ID of a member of a symbol.
func MemberID(parentID, name string) string { return parentID + "." + name }

// normalizeModulePath turns "./src/parser/lex.js" or "src\\parser\\lex.d.ts"
// into "src/parser/lex".
func normalizeModulePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	for _, ext := range []string{".d.ts", ".d.mts", ".d.cts", ".ts", ".mts", ".cts", ".tsx", ".js", ".mjs", ".cjs", ".jsx"} {
		if strings.HasSuffix(p, ext) {
			return strings.TrimSuffix(p, ext)
		}
	}
	return p
}

// anchorUnsafe is what an HTML id built from a symbol ID may not contain.
var anchorUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// Anchor is the fragment for a symbol or member on its module's page:
// "core/parser/lex#Lexer.next" → "Lexer.next". Characters an id cannot carry
// become "-"; a symbol named "$" or "#private" still gets a usable anchor.
func Anchor(id string) string {
	_, local, found := strings.Cut(id, "#")
	if !found {
		return ""
	}
	a := strings.Trim(anchorUnsafe.ReplaceAllString(local, "-"), "-")
	if a == "" {
		return "symbol"
	}
	return a
}

// ModuleOf returns the module ID part of a symbol or member ID.
func ModuleOf(id string) string {
	m, _, _ := strings.Cut(id, "#")
	return m
}
