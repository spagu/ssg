# API docs: languages

`api_docs` reads JavaScript, TypeScript declarations, Go, PHP and Python. Every
reader is written in Go and built into ssg, so you install nothing: no Node,
no PHP, no Python. Every language produces the same model and the same pages,
search entries, `api.json` and `check_api` findings. Declarations are shown
the way the language writes them.

For the configuration, comments, links and themes, see
[API_DOCS.md](API_DOCS.md).

## Choosing the language

```yaml
api_docs:
  - root: packages/core          # language detected
  - root: services/billing
    language: go                 # javascript | typescript | go | php | python
```

Without `language:`, ssg looks in `root` for the first of `package.json`
(JavaScript or TypeScript), `go.mod`, `composer.json`, `pyproject.toml`,
`setup.py` and `setup.cfg`. If none is there, it looks at the code itself.
`package.json` wins in a mixed directory. Give each language its own entry
with its own `root`.

Links written as `{@link Name}` and each language's own link syntax resolve
the same way: first in the comment's module, then by a name only one symbol
has, then by a name only one symbol in the same package has. A site that
documents the same library in several languages therefore links each
`Lexer` to its own.

## Go

Read with the Go standard library's own parser and documentation reader.

| What | How |
|---|---|
| Packages | every importable package under `root`; `internal/` ones marked internal; `package main`, `testdata`, `vendor`, nested modules and `_`/`.` directories skipped |
| Symbols | exported functions, types (structs with fields and methods, interfaces, named types, aliases), constants and variables; generics |
| Enums | a type with constants of that type (`const ( Debug Level = iota … )`) becomes an enum with those members |
| Comments | Go doc comments: the first paragraph is the summary, `Deprecated:` the deprecation, `[Name]` and `[pkg.Name]` links, indented code as Go blocks |
| Examples | `ExampleParse`, `ExampleLexer_Next` from `_test.go` files, with their `// Output:` |

Go documents parameters in a function's own sentences, so `check_api` does
not ask for a description of each one.

Limits: build constraints are not evaluated (every file except
`//go:build ignore` is read), and methods promoted from embedded exported
types are not listed.

## PHP

Read by ssg's own scanner of PHP declarations; function bodies are skipped.

| What | How |
|---|---|
| Files | `composer.json` autoload paths (psr-4, psr-0, classmap, files), else `src/`, else the root; `vendor/` and `tests/` skipped |
| Modules | one per namespace (`Acme/Textkit/Lexer`); the global namespace is `global` |
| Symbols | classes (abstract, final, readonly), interfaces, traits, enums with cases, functions, public methods, properties (also promoted constructor properties) and constants |
| Types | declared types (`?T`, `A\|B`, `A&B`), else PHPDoc types; class names resolved through `namespace` and `use` |
| Comments | PHPDoc: `@param Type $name`, `@return`, `@throws`, `@deprecated` (also `#[\Deprecated]`), `@internal`, `@example`, `{@link}` and `{@see}` |

Limits: only public members are documented. Declarations inside blocks (a
function defined in an `if`), `define()` constants and the magic members
`@property` and `@method` are not read.

## Python

Read by ssg's own scanner of Python declarations; Python is never run.

| What | How |
|---|---|
| Files | the package in `src/<name>/` or `<name>/` (directories with `__init__.py`); tests, `docs/`, `build/`, virtual environments and `setup.py` skipped |
| Modules | one per file (`textkit/lexer`); private modules (`_impl.py`) are shown only through the names public modules re-export |
| Public names | `__all__` when it is a literal list, else every name not starting with `_` |
| Symbols | classes, functions (`async` too), methods, `@property`, `@staticmethod`, `@classmethod`, `@abstractmethod`, `@overload`, dataclasses (with a constructor built from their fields), `Enum` subclasses, `Protocol` as interfaces, type aliases, module constants; PEP 695 generics |
| Docstrings | Google (`Args:`, `Returns:`, `Raises:`, `Examples:`), NumPy (underlined sections) and reST (`:param x:`, `:rtype:`) styles; doctests become examples; `:class:` and `:func:` roles become links |
| Deprecation | `@deprecated("…")` (PEP 702) and `.. deprecated::` |

A name defined in one public module and re-exported by another is documented
once, where it is defined. A renamed re-export (`Thing as Alias`) is a name of
its own and appears in both.

Limits: declarations inside `if` and `try` blocks (including imports under
`if TYPE_CHECKING:`) and attributes set in `__init__` (`self.x = …`) are not
read, and `.pyi` stubs are not used.

## JavaScript and TypeScript

See [API_DOCS.md](API_DOCS.md): JavaScript with its doc comments, or
TypeScript through the `.d.ts` files a package ships.
