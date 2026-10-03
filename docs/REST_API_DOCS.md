# REST API documentation from OpenAPI

ssg turns an OpenAPI 3.0 or 3.1 file into reference pages of your site: one
for the API, one per tag with its operations, and one for the schemas. The
pages share the theme, search, sitemap, `llms.txt` and link checking with your
guides, and every operation has a **Try it** console that sends a real request
from the browser.

For JavaScript and TypeScript packages documented from their code, see
[API_DOCS.md](API_DOCS.md). Both kinds can sit in one `api_docs` list.

## Quick start

```yaml
api_docs:
  - openapi: api/openapi.yaml    # YAML or JSON
check_api: warn                  # report problems in the file
```

| URL | What |
|---|---|
| `/api/<title>/` | the API: description, servers, authentication, every endpoint by tag |
| `/api/<title>/<tag>/` | a tag: each operation with parameters, body, responses and Try it |
| `/api/<title>/schemas/` | every schema in `components.schemas`, with an anchor each |

`<title>` is `info.title` as a URL segment ("Orders API" becomes `orders-api`).

## Configuration

```yaml
api_docs:
  - openapi: api/openapi.yaml
    name: Orders                 # default: info.title
    url: /reference/orders/      # default: /api/<name or title or file name>/
    try_it: true                 # the console on every operation (default true)
```

`root`, `entry`, `include`, `exclude` and `playground` are for code packages and
are not used here.

## What the pages show

- **Operations** are listed in the order the file has them. Methods are
  ordered GET, PUT, POST, DELETE, OPTIONS, HEAD, PATCH, TRACE. Each one gets an
  anchor from its `operationId`, or from the method and path when it has none.
  An operation with several tags appears on each tag's page, and one with no
  tag goes under `default`.
- **Parameters** combine the path-level and operation-level lists. Each one
  shows its type, enum values, default, limits and pattern.
- **Request bodies and responses** show the schema, with named schemas linked
  to the schemas page, and an example. The example is the one the file gives,
  or one built from the schema: `"string"`, `0`, a date for `format: date`,
  and so on.
- **Schemas** list their properties as a table. Nested objects continue as
  dotted names (`customer.address`), down to three levels. A recursive
  schema links to itself instead of being expanded.
- **Authorization** says which security schemes an operation accepts. "or"
  separates alternatives, "and" joins schemes needed together, and the
  front page describes each scheme.

Local `$ref`s are followed (`#/components/...`). A reference to another file or
URL is reported and skipped.

## Try it

The console opens under each operation. It has:

- the server, chosen from `servers` with their variables at their defaults;
- the parameters, with the file's examples filled in and enums as a list;
- the request body, starting from the example;
- the credentials for the operation's security schemes: an API key in a
  header or the query, a bearer token (also for OAuth 2.0 and OpenID Connect),
  or a Basic user name and password.

**Send request** shows the status, the time, the response headers the server
exposes and the body (JSON is indented), and the same request as a `curl`
command.

Credentials are typed once per page and shared by every console on it. They
stay in the page's memory only: they are never saved, never shown in the
`curl` command (which uses placeholders such as `<X-API-Key>`), and gone when
the page closes.

The request goes from the reader's browser to your API, so the API must allow
the documentation site's origin in CORS (`Access-Control-Allow-Origin`). When
it does not, the console says so and still shows the `curl` command, which
works from a terminal. Browsers do not let a page set cookies, so cookie
parameters and cookie API keys are noted but not sent.

`try_it: false` removes the console. The rest of the page does not need
JavaScript.

## Checking the file

`check_api: warn` lists, and `strict` (or a strict build) fails on:

- a `$ref` that points nowhere, to another file, or round in a circle;
- a path parameter (`{id}`) with no `in: path` parameter;
- an operation with no responses;
- a security requirement naming a scheme that does not exist;
- two operations with the same `operationId`;
- a server with no `url`.

Each problem is reported with its location in the file, for example
`#/paths/~1orders~1{id}/get/responses`. A file that cannot be read, or that is
not OpenAPI 3, stops the build.

## Themes

The pages use the same layouts as code references: `api-index.html` for the
API's front page and `api-module.html` for tags and schemas. A theme without
them renders the pages through `page.html`. `.Page.Extra.api` holds
`package.Name` (the API's name), `rest: true`, `nav` (overview, tags and
schemas) and `crumbs`.

The method labels, the console and their styles are added by ssg to the pages
that need them, in every theme. A theme can set the colours with CSS custom
properties:

| Property | Used for |
|---|---|
| `--ssg-api-accent`, `--ssg-api-on-accent` | the Send and Run buttons |
| `--ssg-api-border`, `--ssg-api-focus` | field borders, the focus ring |
| `--ssg-api-code`, `--ssg-api-mono` | response and output blocks |
| `--ssg-api-get`, `--ssg-api-post`, `--ssg-api-put`, `--ssg-api-delete` | method labels |
| `--ssg-api-error`, `--ssg-api-warn` | errors and warnings in live examples |

The `apidoc` theme sets all of them.

## In `--watch`

The OpenAPI file is an input of the build. Changing it makes the next build
full.
