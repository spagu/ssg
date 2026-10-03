// Package php reads a PHP package — a Composer project, or a directory of
// .php files — into the language-neutral API model, so ssg can document it
// like a JavaScript or TypeScript package.
//
// # Scanner
//
// There is no PHP parser in the standard library and the package takes no
// dependency, so it carries its own two-pass reader:
//
//   - lexer.go and lexstr.go turn a file into tokens. They know inline HTML
//     outside <?php ... ?>, quoted strings with escapes, heredoc and nowdoc,
//     every comment form, docblocks (kept as tokens) and #[...] attributes
//     (kept whole, so #[\Deprecated] can be noticed). balance.go then checks
//     that braces, parentheses and brackets pair up.
//   - parser.go, classes.go, members.go and params.go walk the tokens for
//     declarations only: namespaces, use imports, classes, interfaces,
//     traits, enums, functions and constants, and the public members of
//     class-likes. Function and method bodies are skipped by brace matching;
//     nothing inside them is read.
//
// # Model
//
// Every namespace becomes a module ("Acme\Textkit\Lexer" → path
// "Acme/Textkit/Lexer", the global namespace → "global"). Only public API is
// documented: protected and private members are left out. Docblocks are
// PHPDoc; phpdoc.go rewrites their PHP-style tags ("@param Type $name") into
// the JSDoc form apidoc.Parse reads ("@param {Type} name"). Type names are
// resolved through the namespace and its use imports, and point at the
// documented symbol they name.
package php
