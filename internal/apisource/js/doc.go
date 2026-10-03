// Package js reads a JavaScript package — ES modules, with a best-effort pass
// over CommonJS — into the language-neutral API model (GO-104). Types come
// from documentation comments (/** ... */) parsed by apidoc and tstype.
//
// # Parser
//
// The syntax tree comes from github.com/tdewolff/parse/v2/js (MIT licence,
// pure Go, no cgo). It reads ES2022 and later: classes with fields, static
// members and #private names, async functions and generators, optional
// chaining and every import/export form, "export * as ns" included. It is
// small, fast and has no dependencies of its own.
//
// Its tree carries neither positions nor ordinary comments, so the package
// also runs the same library's lexer over the source (scan.go): the token
// stream gives the byte offset and line of every declaration and the
// documentation comment that precedes it, separated only by whitespace. The
// tree says what a declaration is; the tokens say where it is.
//
// # What is read
//
// Each entry module (from the configuration or package.json) becomes a
// module page listing what it exports, re-exports included: a symbol is
// documented from where it is defined but listed under its public name.
// Relative imports are followed within the package root; anything else is
// external and left out.
package js
