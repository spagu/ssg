// Package python reads a Python package into the language-neutral API model
// (apimodel). It does not run Python: a small scanner written here tokenizes
// each source file and a declaration pass reads what a page needs.
//
// # Scanner
//
// The tokenizer (token.go, lexstring.go, lexline.go) knows comments,
// every string form (prefixes r, b, u, f, t and their pairs, triple quotes,
// escapes, f-string replacement fields kept opaque), line continuations,
// implicit line joining inside brackets, and indentation as INDENT and
// DEDENT tokens. Code inside strings is never mistaken for a declaration.
//
// # Declarations
//
// The parser (parse*.go) reads, at module and class level, def and async
// def with their parameters, class headers, assignments with or without
// annotations, PEP 695 type statements and generics, decorators, from-imports
// and __all__. Bodies of functions and of compound statements (if, try, for,
// with) are skipped; only their docstrings are kept.
//
// # What is public
//
// A module with a literal __all__ exports exactly those names, re-exports
// included: such a symbol is documented from where it is defined but listed
// under the module that exports it. Without __all__, every top-level name
// not starting with "_" is public. Docstrings in Google, NumPy and reST
// styles become apimodel.Doc (docstring*.go).
package python
