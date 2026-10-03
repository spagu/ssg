// Package golang reads a Go module or package into the language-neutral API
// model. It uses the standard library only: go/parser for the syntax,
// go/doc for what a package exports and which examples belong to which
// symbol, go/doc/comment for doc comments and go/printer for declarations.
//
// # What is read
//
// Every importable package under the root becomes one module: a directory
// with non-test .go files that is not package main. Directories named
// testdata or vendor, dot- and underscore-directories and nested modules
// (their own go.mod) are skipped. Packages under an internal directory are
// documented with Flags.Internal set, so a site shows them only when its
// visibility asks for it.
//
// The root package's module path is its package name ("textkit"), every
// other package's is its directory relative to the root ("lexer",
// "internal/fmt").
//
// # Mapping
//
// Functions become functions, structs classes (exported fields are
// properties, embedded types Extends), interfaces interfaces, and other
// named types and aliases types. A type with constants of its own (the
// "type Level int; const ( Debug Level = iota ... )" pattern) becomes an
// enum whose members are those constants. Methods are members of their
// type. Constructors (NewT) stay top-level functions. Exported constants
// and variables become variables, constants read-only.
//
// Doc comments are rendered to Markdown; Go doc links ([Name],
// [Type.Method], [pkg.Name]) become {@link ...} targets that ssg resolves,
// links to packages outside the extraction point at pkg.go.dev, and a
// "Deprecated:" paragraph becomes Doc.Deprecated. Example functions in
// _test.go files become the examples of the symbol they name.
//
// Build constraints are not evaluated: every file of a directory is read,
// except those marked //go:build ignore.
package golang
