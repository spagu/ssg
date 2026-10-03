# API documentation from code

ssg reads JavaScript and TypeScript, together with their documentation
comments, and publishes the API as ordinary pages of your site. Those pages use
the same theme, search, sitemap, `llms.txt`, MCP tools and link checking as the
guides you write by hand. A guide can link to a symbol and a symbol to a guide;
one build checks both.

A REST API described in OpenAPI gets the same treatment, with a "Try it"
console on every operation: see [REST_API_DOCS.md](REST_API_DOCS.md).

## Quick start

```yaml
api_docs:
  - root: packages/core          # holds package.json and the code
check_api: warn                  # report undocumented exports and dead links
```

`ssg` reads `package.json` to find the entry points (`exports`, then `module`,
then `main`) and publishes:

| URL | What |
|---|---|
| `/api/core/` | the package: its README and its modules |
| `/api/core/src/lex/` | a module: functions, types, enums and variables, each with an anchor |
| `/api/core/src/lex/Lexer/` | a class or interface: its constructor, properties and methods |
| `/api.json` | the whole model, for tools and agents (below) |

No Node.js is needed. JavaScript is read with its doc comments. A package
that ships TypeScript declarations (`types` in `package.json`, a `types`
condition in `exports`, or an `index.d.ts`) is read from those instead, since
they are exactly what its users import. For a TypeScript project, build its
declarations first, as `npm publish` would (`tsc --declaration`), or point
`entry` at the `.d.ts` files. A `types` path that does not exist yet does not
count: ssg then reads the JavaScript.

## Configuration

```yaml
api_docs:
  - name: core                   # default: "name" in package.json
    root: packages/core
    entry: [src/index.js]        # default: from package.json
    include: ["src/**"]          # which files may be read (globs, relative to root)
    exclude: ["**/*.test.*"]
    url: /api/core/              # default: /api/<name>/
    visibility: public           # public (default) | internal | all
    stability: [beta]            # which non-stable levels to show; empty = all
    readme: true                 # the package README on its front page
    source_url: https://example.com/repo/blob/{ref}/{path}#L{line}
    source_ref: auto             # a tag or branch; auto = the commit git reports
    playground: https://cdn.example.com/npm/core@1/+esm   # live examples (below)
check_api: warn                  # off (default) | warn | strict
```

Several entries document several packages, for example in a monorepo. Every
package gets its own URL prefix, and they share search and cross-links.

## Writing documentation comments

A comment directly above a declaration documents it:

```js
/**
 * Splits source text into tokens.
 *
 * The lexer is incremental: call {@link Lexer.next} until it returns `null`.
 *
 * @param {string} source - the text to read
 * @param {{strict?: boolean}} [options] - reading options
 * @returns {Lexer} a lexer positioned at the start
 * @throws {SyntaxError} on an unterminated string
 * @example
 * const lex = createLexer("a b")
 * lex.next() // → { kind: "word", text: "a" }
 * @since 1.2
 */
export function createLexer(source, options) { … }
```

- The first paragraph is the summary. It is shown in lists, in search results
  and in the page description.
- `@param` and `@returns` take a type in braces, written in TypeScript syntax
  (`{Array<string>}`, `{string | null}`, `{(x: number) => void}`).
  `[name]` marks a parameter as optional and `[name=default]` gives its
  default.
- `@typedef {Object} Options` with `@property` lines, and `@callback`, define
  types that exist only in comments. `@template T` makes a symbol generic.
- `@deprecated` says what to use instead. `@see`, `@since` and `@example` are
  shown as written. `@example` text that is not already a code block is
  shown as one.
- `@internal` hides a symbol unless `visibility` asks for it. `@hidden` or
  `@ignore` hides it always. `@beta`, `@alpha` and `@experimental` show a badge,
  and `stability` can leave those levels out.
- Any other tag is kept in `api.json` for a theme to show.

### Links between symbols

`{@link Name}` links to a symbol. `{@link Name | text}` uses your own link
text, and `{@linkcode Name}` shows the text as code. A name is looked up in
this order:

1. as a full ID;
2. in the comment's own module, so a module's own `Token` wins over another's;
3. as the only symbol with that name in the whole API.

`Lexer.next` reaches a member, and `src/lex#Lexer` a symbol in another module.
A name nobody has, or that two modules both have, is not guessed. It is shown
as text and reported by `check_api`.

## Live examples

With `playground:` set to the package as an ES module, every `@example` that is
a single JavaScript code block (` ```js `) gets an editor with **Run** and
**Reset**, and Ctrl+Enter runs it:

```js
/**
 * @example
 * ```js
 * import { tokenize } from "textkit";
 * console.log(tokenize("Hello, world"));
 * ```
 */
```

The code runs in a sandboxed frame. It can run scripts, but it has its own
opaque origin, so it cannot read the page, its cookies or its storage. An
import map points the package name at the `playground` URL, so `import … from
"textkit"` works, and the package's exports are also available as plain names.
`console` output and uncaught errors appear under the editor. A run that takes
longer than 5 seconds is stopped. Without JavaScript the example stays an
ordinary code block.

The URL has to answer cross-origin requests. A CDN build of an npm package
(jsDelivr's `+esm`, esm.sh, unpkg with `?module`) does. A file the site
publishes itself needs `Access-Control-Allow-Origin: *`. The script and its
styles are added only to pages that have an example, in every theme.

## Checking the documentation

`check_api: warn` lists, and `strict` (or a strict build) fails on:

- an exported symbol with no description;
- a parameter with no description, when its function has one;
- `@deprecated` that does not say what to use instead;
- an `@example` that is not code;
- a `{@link}` that leads nowhere;
- code the extractor could not read: a syntax error, or a missing entry point.

## Themes

**`apidoc`** is a theme made for documentation only. It has a guides and
reference sidebar that opens the current package's tree, a search box over
`search-index.json` (press `/`), an "On this page" column, breadcrumbs, and
light and dark schemes. It ships inside the binary:

```yaml
template: apidoc
search_index: true
variables:
  title: textkit                       # name in the header and titles
  tagline: Tokenize and format text.   # the front page's lead
  start: {url: /getting-started/, label: Get started}
  nav:                                 # your header links; icon shows a mark
    - {url: /changelog/, label: Changelog}
    - {url: "https://npm.example.com/package/core", label: npm, icon: npm}
  repository_url: https://github.com/example/core   # repo icon in the header
  docs_nav_order: [getting-started, configuration]   # guide slugs, in order
  gtm_id: GTM-XXXXXXX                  # optional Google Tag Manager
  ssg_credit: false                    # hide "Built with SSG" in the footer
```

The logo, favicon and social profiles come from `marketing:` (`logo`,
`favicon`, `social_profiles`), and brand colours from `colors:` (`primary`,
`primary_dark`). Its colours, type, contrast ratios and every branding setting
are in
[templates/apidoc/README.md](../templates/apidoc/README.md).

Every API page is a normal page whose body is the reference written as
Markdown, so any theme shows it through `page.html`. A theme can draw API pages
its own way with three layouts:

- `api-index.html` for the package page;
- `api-module.html` for module pages;
- `api-symbol.html` for class and interface pages.

apidoc and ssgtheme ship all three. They put the package tree beside the page and a
breadcrumb trail above it. They read:

| Data | What |
|---|---|
| `.Page.Content` | the reference body |
| `.Page.Extra.api.package`, `.module`, `.symbol` | the model of this page (a REST API: `package.Name` and `rest: true`) |
| `.Page.Extra.api.nav` | the package tree: `Title`, `URL`, `Kind`, `Children` |
| `.Page.Extra.api.crumbs` | the trail: package › module › symbol |

`{{ apiHref "core/src/lex#Lexer" }}` and `{{ apiType .Type }}` (a type with every
documented name linked) are there for a theme that draws signatures itself.
Use them in your own theme only: a theme in the ssg repository is also built by
older releases, which do not have them.

The module and class pages carry `hide_from_lists: true`, so a theme's guide
list shows each package once, by its front page.

## For agents

- `api.json` is the model, described below.
- Every API page is also published as Markdown when `markdown_publish` is on,
  and is listed in `llms.txt`.
- `ssg mcp` adds `api_search`, `api_symbol` and `api_module`. A class answers
  with its members as a list of IDs; an agent asks for one member by its ID.
- With `webmcp: true`, the site's pages register `findSymbol` and `getSymbol`
  in the browser, reading `api.json` the first time they are used.

## In `--watch`

Every source file of a documented package (`.js`, `.ts`, `.json`, `.md` and so
on, outside `node_modules`) is an input of the build. A change makes the next
build full, so the reference never shows code that is no longer there.

## The API model (`api.json`)

A build that documents code also publishes `api.json`. It is the whole API as
data, for tools and agents that would rather not read HTML. Its shape is
described by [`docs/api-model.schema.json`](https://github.com/spagu/ssg/blob/main/docs/api-model.schema.json) (JSON Schema
2020-12) and versioned by its `schema` field. A change a reader could notice
raises the number, and ssg refuses a document with a number it does not know.

```json
{
  "schema": 1,
  "packages": [{
    "name": "core",
    "modules": [{
      "id": "core/src/lex",
      "path": "src/lex",
      "symbols": [{
        "id": "core/src/lex#Lexer",
        "name": "Lexer",
        "kind": "class",
        "members": [{
          "id": "core/src/lex#Lexer.next",
          "name": "next",
          "kind": "method",
          "signatures": [{ "returns": { "kind": "name", "name": "Token", "ref": "core/src/lex#Token" } }]
        }]
      }]
    }]
  }]
}
```

### IDs

IDs come from where a symbol lives and what it is called, never from its
position in a file. Moving code around inside a file changes no URL and no link.

| What | ID | Anchor on the module page |
|---|---|---|
| module | `core/src/lex` | — |
| symbol | `core/src/lex#Lexer` | `#Lexer` |
| member | `core/src/lex#Lexer.next` | `#Lexer.next` |

A module path loses its extension (`.js`, `.mjs`, `.d.ts` and so on) and any
leading `./`. A JavaScript file and its declaration file therefore describe the
same module.

### Kinds

`namespace`, `class`, `interface`, `function`, `method`, `constructor`,
`property`, `accessor`, `type`, `enum`, `enumMember`, `variable`.

### Types are trees

A type is never a string to be parsed again. `Promise<Token[]>` is a `name`
node with one `array` argument, whose element is a `name` node carrying `ref`:
the ID of the documented `Token`. A theme can link every part, and an agent can
follow `ref` without guessing. The shapes are `name`, `union`, `intersection`,
`array`, `tuple`, `literal`, `function` and `object`. A type the model does not
take apart, such as a conditional or mapped type, is `verbatim`, with its source
text in `name`.

### Order

`api.json` is written in a fixed order: packages by name, modules and symbols by
ID. Two builds of the same sources produce identical bytes, so a diff of the
file is a diff of the API. Overloads keep the order they were declared in.
