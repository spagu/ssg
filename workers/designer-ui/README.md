# designer-ui — the site's settings and theme, in the browser

![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)
![Auth required](https://img.shields.io/badge/sign--in-auth%20module%20only-00695c)
![License BUSL-1.1](https://img.shields.io/badge/license-BUSL--1.1-blue)

A standalone Cloudflare Worker. It shows **every setting of ssg** — grouped
and described by the configuration schema ssg exports (`ssg config schema`) —
with its value and where the value comes from, and lets a person change the
settings and the theme's files. Each publication is **one commit** to the
site's repository, signed with the name of the person who made it; the site's
own CI builds and deploys it as after any push.

- Settings are written to **`.ssg.designer.json`** beside `.ssg.yaml` — never
  to `.ssg.yaml`, whose comments and layout stay as written by hand. ssg
  merges the overlay over the config file when it loads and validates it the
  same way (unknown key, wrong type, literal secret: the build stops).
- No answer carries a secret's value: a value inside a secret key is
  `{"set": true, "reference": "$NAME" | null}`. A secret is accepted only as
  an environment reference (`$NAME`, `login:$NAME`, `Bearer $NAME`).
- No database. History is the repository's history; going back to a revision
  is an ordinary publication of the old values.

The panel (GO-179) is at `DESIGNER_BASE/`: **Settings** (every key), **Colours
and fonts** (the theme's tokens with live WCAG 2.2 contrast checks) and **Theme
files** (tree, editor, image replacement). Scripts and AI assistants use the
same operations through `POST {base}/mcp` with an `auth` service token
([MCP](#mcp)).

This README is the reference. The guide for site owners — setup, roles,
the overlay, colours, limits and what is not built yet — is
[docs/DESIGNER.md](../../docs/DESIGNER.md).

## Setup

Three things, all in `wrangler.toml` of this directory:

```toml
[vars]
DESIGNER_BASE      = "/designer"
DESIGNER_GIT_REPO  = "github.com/<owner>/<repo>"
DESIGNER_GIT_BRANCH = "main"            # empty = the repository's default branch
DESIGNER_AUTH_ISSUER = "https://example.com/api/auth"
DESIGNER_AUTH_LOGIN_URL = "/auth/login"
DESIGNER_AUTH_PUBLIC_KEY = '<AUTH_PUBLIC_KEY= line of workers/auth/scripts/auth-keys.mjs>'
```

```sh
npx wrangler secret put DESIGNER_GIT_TOKEN   # fine-grained, one repository, "Contents: read and write" only
npx wrangler deploy
```

A token of its own, separate from `editor-ui`'s, is recommended, not
required. Without workflow permission the host itself refuses changes to
`.github/workflows/` — a second, independent layer.

**Auth is required.** The `auth` worker must run on the same host. Without a
complete `DESIGNER_AUTH_*` set every path, the panel's files included,
answers `503 auth_not_configured`; there is no password mode and no
Access-only mode (every publication needs an author and a permission).

**Plan.** `DESIGNER_PLAN = "paid"` on a Workers Paid account raises one
publication from 10 files and 2 MiB to 50 files, 10 MiB per file and 20 MiB
in all, and a request body from 8 MiB to 28 MiB; unset or `"free"` keeps the
Free limits, any other value answers `503 bad_plan` everywhere. `api/health`
reports the active `plan` with its limits
([EDITOR-OPERATIONS.md](../../docs/EDITOR-OPERATIONS.md#limits-per-cloudflare-plan)).

Everything about the site is read from the branch: the config file in ssg's
own search order (`.ssg.yaml`, `.ssg.yml`, `.ssg.toml`, `.ssg.json`, …), the
overlay beside it, the theme folder `<templates_dir>/<template>`.

## Permissions

| Permission | Allows |
|---|---|
| `designer-ui:read` | theme files, every setting (secrets as set / not set), revisions, the panel |
| `designer-ui:settings` | publishing settings of the class `plain` |
| `designer-ui:advanced` | **sensitive** — settings of the class `executes` (hooks, workers, deployment, program paths, sources, template files the build reads — `shortcodes[].template`, `render_hooks`, `outputs_custom[].template` — third-party scripts: `analytics`, `analytics_ids` — and keys that send visitors elsewhere: `redirects`, `link_rewrites`, `domain`, `modules[].url`) and `secret`, and any value holding a `$NAME` reference |
| `designer-ui:theme` | **sensitive** — publishing theme files (templates are code the build runs and HTML every visitor gets) |

A settings publication needs `designer-ui:settings`; when it touches any key
that needs `designer-ui:advanced` and the caller lacks it, the **whole**
publication is refused with `403 {"error": "forbidden", "permission":
"designer-ui:advanced", "keys": [...], "refused": [{"key", "reason"}]}`
naming every such key. A `422 invalid_settings` carries `refused` too: each
key with its reason (`unknown configuration key`, a wrong type, a secret not
written as a reference).

How the trust a change needs is decided (src/settings-diff.ts):

- the changed keys are the leaves where the old and new overlay differ;
  mappings are descended into only where ssg merges them key by key;
- where the file holds another form at that key (`mddb: true`) or the reader
  could not follow it, an object replaces it whole and its own class counts;
- a map entry that appears or disappears (a new external source, a new model
  endpoint) is a whole thing: the entry's class counts;
- a leaf needs the strongest class any node its old or new value reaches
  (a `workers` entry holding `auth.token` reaches `secret`).

`designer-ui:advanced` and `designer-ui:theme` are declared `"sensitive":
true` in [module.json](module.json). The `auth` worker then treats them as it
treats `*` or an `…:erase`: a **service token carries them only when an owner
allowed it** at creation (`allowSensitive`,
[SERVICE-TOKENS.md](../auth/docs/SERVICE-TOKENS.md#sensitive-permissions)).
`auth` learns declarations from `AUTH_KNOWN_PERMISSIONS` — list this module
there (`node workers/auth/scripts/auth-permissions.mjs
workers/designer-ui/module.json …`) so the permissions are known and
described. Their sensitivity does not depend on it: `auth` treats the
sensitive permissions of the bundled templates as sensitive even when the
variable omits them or lists bare names (GO-183).

## API

All paths under `DESIGNER_BASE`; full contract in [openapi.yaml](openapi.yaml).

| Method | Path | Permission | |
|---|---|---|---|
| GET | `/api/module`, `/api/health`, `/api/module/summary`, `/api/me` | read | module manifest, configuration state, summary, who is signed in |
| GET | `/api/settings` | read | every named key: schema node, class, `source` (`overlay` / `file` / `default` / `unknown`), value, `overlayBlobId` |
| POST | `/api/settings/publish` | settings (+ advanced) | `{message, baseBlobId, changes: [{path: ["mddb","lang"], value}]}`; `value: null` removes the key from the overlay |
| GET | `/api/theme` | read | the theme's files, or `available: false` with the reason and the way out |
| GET | `/api/theme/files/{path}` | read | one file: UTF-8 text, or base64 for images and fonts |
| POST | `/api/publish` | settings and/or theme (+ advanced) | `{message, settings?: {baseBlobId, changes}, theme?: {changes}}` — one commit with both; a mixed publication needs every permission its parts need |
| POST | `/api/theme/publish` | theme | `{message, changes: [{op: put/delete/move, …, baseBlobId}]}` |
| GET | `/api/revisions?path=` | read | history of a theme file, the overlay or the config file |
| GET | `/api/revisions/{commitId}?path=` | read | a theme file, or the overlay parsed and redacted, at that commit |
| GET | `/api/preview?path=&page=latest&scheme=&lang=&--token=value` | read | a page of this host with the given colour and font tokens, made inert, for the panel's sandboxed frame (HTML, not JSON; an error is a short page) |
| POST | `/mcp` | read (+ each tool's) | the MCP endpoint ([MCP](#mcp)) |

A key path is a list of keys, not a dotted string: a map key may hold a dot
(`["ai", "models", "gpt-4.1", "url"]`). Lists are replaced whole.

Conflicts are visible, never silent: `baseBlobId` is the version read; a file
changed in between answers `409 {"error": "conflict", "conflicts": [...]}`.

## MCP

`POST {base}/mcp` gives an AI assistant or a script the operations of the
panel: each tool is a thin wrapper of the operation behind a REST route, run
through the same permission check, so a tool call succeeds and is refused
exactly as the route (a refusal is a tool result with `isError: true` and
the route's error body plus `status` as `structuredContent`). `tools/list`
shows only the tools the caller's permissions allow; a hidden tool called by
name is refused. The transport (`portable/mcp-transport.ts`,
`portable/mcp-methods.ts`) is editor-ui's, byte for byte: MCP 2026-07-28 and
the `initialize`-based 2025-11-25 / 2025-06-18 / 2025-03-26, stateless, one
JSON answer per POST, at most 8 MiB (28 MiB on `paid`) — the protocol notes and links are in
[editor-ui's README](../editor-ui/README.md#mcp).

**Who.** An `auth` service token (`ssgsvc_…`) exchanged for a 10-minute
access token, sent as `Authorization: Bearer`. Give it only what the
assistant needs: `designer-ui:read` alone is read-only; theme files and
advanced settings need the owner's `allowSensitive` (above).

```sh
ACCESS=$(curl -sf -X POST -H "authorization: Bearer $SSG_SERVICE_TOKEN" \
  https://example.com/api/auth/token | jq -r .accessToken)
curl -s https://example.com/designer/mcp \
  -H "authorization: Bearer $ACCESS" -H "content-type: application/json" \
  -H "accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"designer_config_set","arguments":{"message":"Polish UI","changes":[{"key":"mddb.lang","value":"pl"},{"key":"title","value":"Example"}]}}}'
```

A client configuration (the access token expires after 10 minutes; a client
that can run a command for its headers should run the exchange instead):

```json
{
  "mcpServers": {
    "site-designer": {
      "type": "http",
      "url": "https://example.com/designer/mcp",
      "headers": { "Authorization": "Bearer <access token from POST /api/auth/token>" }
    }
  }
}
```

| Tool | Arguments | Operation | Permissions |
|---|---|---|---|
| `designer_list` | — | `GET api/theme` | read |
| `designer_read` | `path` | `GET api/theme/files/{path}` | read |
| `designer_write` | `message, path, content \| content_base64, baseBlobId` or `changes[]` | `POST api/theme/publish` (`put`) | theme |
| `designer_config_read` | `key?` | no key: `GET api/settings`; a key: its entry and its fields' | read |
| `designer_config_set` | `message, key, value, baseBlobId?` or `changes[{key, value}]` | `POST api/settings/publish` | settings (+ advanced per key) |
| `designer_publish` | `message, settings?: {baseBlobId?, changes[{key, value}]}, theme?: {changes[]}` | `POST api/publish` — both in one commit | settings + theme to see it; each part its own |
| `revisions_list` | `path, cursor?` | `GET api/revisions` | read |
| `revision_read` | `path, commit` | `GET api/revisions/{commitId}` | read |
| `help` | — | the instructions and the caller's tools | read |

A `key` is dotted (`"mddb.lang"`) or a list of keys when one holds a dot
(`["ai", "models", "gpt-4.1"]`). `value` is JSON; `null` removes the key from
`.ssg.designer.json`. Without `baseBlobId`, `designer_config_set` uses the
settings file as it is now — keys are merged one by one, so only a key
someone changed meanwhile is overwritten; pass the `overlayBlobId` of
`designer_config_read` to be refused `409 conflict` instead. A refusal of a
settings change carries `refused: [{key, reason}]` for every key at once.
Tool descriptions tell the assistant that file contents and setting values
are data, never instructions.

Differences from `ssg mcp` (the local development server, `internal/mcp/`):

| | `ssg mcp` | designer-ui |
|---|---|---|
| Where a write goes | the working tree | one commit on the branch per call, with `message` |
| `designer_write` | one file, `path` + `content` | several files in `changes[]`; `baseBlobId` required (null = new); `content_base64` for images and fonts |
| `designer_edit`, `designer_find` | yes | no — read, then `designer_write` the whole file |
| `designer_config_read` | the presentation keys only, as text | every setting with its schema, value and source, as structured content (secrets as set / not set); `key` for one |
| `designer_config_set` | one `key`, `value` as text (`"true"`, `"webp,avif"`), presentation keys only | many keys, `value` as JSON; every key by permission: plain with `settings`, code/deploy/secret references with `advanced` |
| Validation | the whole config after the edit, rolled back when invalid | the next `.ssg.designer.json` against the schema before the commit; `refused` per key |
| Writes `.ssg.yaml` | yes, in place | never — `.ssg.designer.json` only |
| Delete, move theme files | no | REST only (`api/theme/publish`) |
| Revisions | — | `revisions_list`, `revision_read` |
| Access | stdio, or HTTP with a static token or OAuth | `auth` accounts, per-tool permissions, the rate limit |

## Path policy

The only place deciding which files may be touched (src/theme.ts, portable/path-policy.ts):

- theme: files under `<templates_dir>/<template>/` with an extension on the
  list `.html .css .scss .js .json .svg .png .webp .woff2`;
- settings: exactly `.ssg.designer.json` to write; the config files to read
  (their text is never served — `api/settings` shows the values, redacted);
- never: `.github/`, `.git/`, `workers/`, `node_modules/`, `wrangler.*`,
  `.dev.vars*`, `.env*`, any dot segment.

**A theme that is not in the repository** — built into ssg (`simple`,
`krowy`, `apidoc` are extracted at build time) or downloaded from
`online_theme` — cannot be edited here. `api/theme` says so and how to bring
it in: run ssg once locally with this configuration (it writes the theme's
files to `<templates_dir>/<template>/`), commit that folder. Settings work as
usual.

## Reading `.ssg.yaml`

ssg publishes no effective configuration into the repository, and a build
artefact would lag the branch the panel edits, so the worker reads the YAML
itself with editor-ui's minimal reader (no npm dependency): key by key. A key
it cannot follow (anchors, aliases, tags, block scalars, multi-line values)
is reported as **set in the file, value not read** with the line — never
guessed — and the others still read. `include:` files are not followed (the
answer says so). A TOML or JSON config is read for display only; the overlay
works beside a YAML config only, so publishing settings there is refused
(`409 overlay_needs_yaml`).

## Shared code

Everything in `portable/`, the generic panel modules
(`public/js/core/{vnode,router,i18n,api,prefs,session,diff,format,admin-nav}.js`,
`public/js/panel/{ui,dialog,publish-dialog,diff-view,conflict-view,revisions}.js`)
and the test fakes are **byte-identical copies** of `workers/editor-ui/`, their
one source (GO-185); `src/session-verifier.ts` of `workers/auth/portable/`;
`src/summary-kit.ts` of `workers/module-summary-kit.ts`. Never edit a copy —
change the source and copy again. `test/editor-copies.test.ts` and
`test/auth-copies.test.ts` fail while a copy differs, and re-run the sources'
own suites against the copies (skipped, with the reason, when this template
is scaffolded alone).

`portable/` holds the worker's whole frame, not only the repository layer:
the gate (`gate.ts`), operations and their errors (`ops.ts`), the fetch
pipeline (`app.ts`), the router, the module / me / MCP endpoints, the
repository binding (`repo.ts`), the commit author (`author.ts`), the
configuration files and overlay merge (`config-files.ts`), the YAML reader,
and what every answer carries (`http-kit.ts`). What differs between the two
workers is not a copy: the shared modules read this template's id, variable
prefix, default base and `Env` from `src/http.ts`, and its permissions, routes
and MCP tools from `src/permissions.ts`, `src/routes.ts` and
`src/mcp-tools.ts`. A service token's commit is authored `service account
<id>` at `<id>@service-account.invalid`, as in editor-ui.

`src/config-schema.json` is generated from ssg — never edit it by hand:
`UPDATE_CONFIG_SCHEMA=1 go test ./internal/config -run TestSchemaTwinUpToDate`.

## Tests

From the repository root (Node 25, no npm install, no network — GitHub is
faked in-process):

```sh
node --import ./workers/designer-ui/test/resolve.mjs --test 'workers/designer-ui/test/*.test.ts'
node --test 'workers/*.test.ts'
```

`test/mcp-pairs.test.ts` runs every operation by both ways in — the MCP tool
and the REST route — and holds them to the same answer, the same commit and
the same refusal; `test/mcp-sensitive.test.ts` checks, with auth's own
functions, that a service token without the owner's `allowSensitive` cannot
write a theme file or an advanced key.

`test/overlay-samples.json` holds settings files the worker writes, produced
through the whole pipeline (`UPDATE_OVERLAY_SAMPLES=1` regenerates it); ssg's
Go tests validate them with `config.ValidateOverlay`.

## The panel

No framework, no build step, no external resource; strict CSP (no inline
script or style — the few computed styles, such as a font sample, go through
the CSSOM); data is only ever inserted as text. Five languages (en, pl, hi,
de, fr) with the terms of newsletter-ui; light and dark; 360 px wide without
horizontal scroll; every control reachable by keyboard; results announced in
`aria-live` regions. The shared admin bar (`/core/nav.js`) and the site's look
(`/core/theme.css`, after the panel's own tokens) come from `core` when it is
deployed on the same host.

- **Settings** — one entry per key `GET api/settings` answers, in the schema's
  groups, plus **Advanced**: every key of the class `executes` or `secret`,
  under its schema group, visible with `designer-ui:read`, editable only with
  `designer-ui:advanced`. Search by key, label or description (in the panel's
  language and in English); "only changed here". Each key shows its label,
  the English description from the schema (a translation for the most used
  keys, `models/labels.js` `TRANSLATED`), where its value comes from
  (default / `.ssg.yaml` / the panel / not readable) and **Revert** (removes
  it from the overlay: `value: null`). Controls by type: switch, number, text,
  select, one-per-line lists, row editors for lists of objects and for maps,
  JSON for free-form values. A boolean without a value and without a schema
  default is shown as "not set — ssg's own default applies", never as "Off".
  A secret shows only *set / not set* and takes a variable name (`$NAME`,
  `login:$NAME`, `Bearer $NAME`) — the panel builds the reference, so a pasted
  value cannot be sent. History of `.ssg.designer.json`: view any revision,
  restore it as pending changes.
- **Colours and fonts** — see *Theme tokens* below. A pair below 4.5:1 shows
  its ratio and the nearest colour of the same hue that passes, with **Use**;
  it never blocks, but publishing then needs a ticked confirmation.
- **Preview** (in Colours and fonts) — a page of the site (home, latest post
  or a path) with the tab's unpublished tokens, light or dark, full or phone
  width, in an `<iframe sandbox>` without any allowance. `src/preview*.ts`:
  the page is fetched from this host without the caller's credentials
  (same-origin redirects only, 2 MB), every token is written back by the
  worker's own serializer without scripts, `on*` attributes or script URLs,
  and `src/preview-entry.ts` gives the answer its own CSP (no script source,
  `sandbox`, `frame-ancestors 'self'`) and the panel's page `frame-src
  'self'`. Only tokens are previewed; self-hosted fonts need
  `Access-Control-Allow-Origin` (the frame's origin is opaque). Tests:
  `test/preview-sanitize.test.ts` (a hostile HTML corpus),
  `test/preview.test.ts`, `test/panel-preview.test.ts`.
- **Theme files** (`designer-ui:theme` to change; reading needs `read`) — the
  folder tree, a text editor with line numbers and find (Enter / Shift+Enter),
  image replacement (same type, same name; an SVG is a picture and text),
  each file's history with a diff and **Restore**. A one-time note says that
  templates are the site's code. A theme built into ssg or downloaded at build
  time shows how to bring it into the repository instead.
- **Publish** — one dialog lists every change in the person's language
  ("Site name: “Example” → “My kitchen”", files with lines added/removed),
  the commit message (prefilled), the author, and goes out as **one commit**
  through `POST api/publish` (the overlay and the theme files together;
  refused whole, before anything is written, when a permission is missing). A
  409 is one conflict screen for both parts — nothing overwritten, the new
  version read, the person's edits kept on top (with a diff for files) to
  publish again, or dropped; 422 lists the problems per key; 403 names the
  keys that need
  `designer-ui:advanced`. A missing permission means the control is absent.

### Theme tokens — the contract the panel relies on

A theme declares its design tokens as CSS custom properties in a file named
`tokens.css` — `<templates_dir>/<template>/css/tokens.css` first, else the
shortest path ending in `tokens.css` (ssgtheme, picostore and apidoc ship one).

- **Light** values: declarations in a rule for `:root` (or
  `:root:not([data-theme="dark"])`) outside any media query.
  **Dark** values: in a rule inside `@media (prefers-color-scheme: dark)`, or
  for `:root[data-theme="dark"]`; a theme that states them twice (system
  preference and its own toggle) has both copies changed together. A rule in
  another media query (width, print) is an override and is not a token.
- A **colour** field is a token whose value is `#rgb` or `#rrggbb`; a
  **font** field one whose name holds `font` (not size, weight, height,
  spacing) and whose value is a font list. `var(--x, fallback)` tokens are
  followed for the contrast check but not edited; a token holding `palette`
  is a raw ramp, listed apart and never a pair.
- **Pairs** checked (WCAG 2.2, 4.5:1), read from names (an optional `color-`
  and a short theme prefix such as `ad-` are ignored): text tokens (`fg…`,
  `text…`, `ink…`, `link…`) on every general background (`bg…`,
  `background…`, `surface…`, `canvas…`, inverse ones aside); `on-X` on `X`;
  `X-fg` on every `X-bg*`; a text token naming `inverse` on the inverse
  backgrounds.
- Publishing changes only the values in the file's text — comments, order and
  layout stay — as one theme-file change (`designer-ui:theme`).

### Style and colour guidelines

`public/css/tokens.css` holds every colour (a test refuses a colour written
in any other stylesheet). Material colour tokens: Teal 800 / 200 as the main
colour and for links, **Blue Grey** neutrals (newsletter-ui's are Brown),
Deep Orange 900 / 200 reserved for focus, Red / Orange / Green for states,
Green 50 / 900 and Red 50 / 900 behind added and removed diff lines (each line
also has a sign and a word). `test/panel-html.test.ts` computes every
text/background pair (≥ 4.5:1) and every control edge and focus ring (≥ 3:1)
in both schemes with the panel's own contrast module. Type: system stacks
only — a geometric sans for headings, a humanist sans with Devanagari
fallbacks for text, a monospace for code and keys.

### Generic panel modules (shared with editor-ui)

`js/core/{vnode,router,i18n,api,prefs,session,diff,format,admin-nav}.js` and
`js/panel/{ui,dialog,publish-dialog,diff-view,conflict-view,revisions}.js` are
copies of editor-ui's (see "Shared code"). `js/models/contrast.js` has no
designer-specific import either, should another panel need it.

### Local preview and screenshots

```sh
node --import ./workers/designer-ui/test/resolve.mjs workers/designer-ui/test/dev-server.mjs
# → http://localhost:8788/designer/  (accounts: owner, advanced, settings, theme, reader)
node workers/designer-ui/test/panel-shots.mjs <out-dir> [axe.min.js]
```

The dev server runs the worker's own fetch handler against the in-process
fake GitHub, seeded with a small site, ssgtheme and two earlier publications
by two people. `DEMO_THEME=builtin` shows the export instruction,
`DEMO_CORE=1` serves `core`'s shared bar, `DEMO_SITE=<ssg output dir>`
serves a built site on the same port for **Preview** (without it, one small
page). Nothing leaves the machine.

## What it protects and what it does not

Written for site owners after the security review (GO-183,
`audit/roadmap/editor/SECURITY-REVIEW.md`).

**It protects**

- **The repository token** (`DESIGNER_GIT_TOKEN`) exactly as editor-ui does:
  sent only to `https://api.github.com`, no redirects, never in an answer,
  error, log or MCP result.
- **Files outside the theme.** Theme publications write only files with a
  listed extension inside `<templates_dir>/<template>`; settings
  publications write only `.ssg.designer.json`. `.ssg.yaml`, `.github/`,
  `workers/`, `wrangler.*`, `.env*` and dot files are never written.
- **The line between settings and code.** Every key has a class from ssg's
  own schema. With `designer-ui:settings` alone you can change only `plain`
  keys; anything that runs a command, deploys, names a file or folder the
  build reads (including template files of shortcodes, render hooks and
  custom output formats), loads a third-party script on every page
  (`analytics`, `analytics_ids`), can send visitors elsewhere (`redirects`,
  `link_rewrites`, `domain`, `modules[].url`), or holds a `$NAME` reference to the build's
  environment needs `designer-ui:advanced`. `template` is a single folder
  name, never a path. ssg validates the overlay with the same rules and
  refuses to build from a file that breaks them.
- **Secrets.** A secret key accepts only a reference (`$NAME`), never a
  literal; values already in `.ssg.yaml` are shown as "set", never sent.

**It does not protect**

- **Against holders of `designer-ui:theme` or `designer-ui:advanced`.** A
  theme template is code the build runs and HTML every visitor gets; an
  `executes` setting can run commands in CI with its secrets. Both are
  **sensitive**: give them only to people you would give write access to the
  repository, and a service token only with an owner's `allowSensitive`.
- **Against what ordinary settings can still change.** `designer-ui:settings`
  can edit the site's text and look. Treat it as an editor's permission, not
  a reader's. Keys that can send visitors elsewhere — `redirects`,
  `link_rewrites`, `domain` and a module's `modules[].url` — are `executes`
  and need `designer-ui:advanced`.
- **Your visitors and admins from content.** Theme and content share one
  origin with the panels and `auth` (see editor-ui's section of the same name).
- **Pages protected only at Cloudflare's edge, from the look preview.** The
  preview fetches any path of its own host, anonymously, and shows the HTML
  to a `designer-ui:read` holder. If parts of the site are protected only at
  the edge (Cloudflare Access, WAF rules by IP or country), check on the
  deployed zone whether the Worker's request to its own zone bypasses them.
  Until you have verified it, do not rely on edge-only protection for pages
  a `designer-ui:read` holder must not see.
- **Revocation in real time**, **content read by an assistant as
  instructions**, and **the token's GitHub rate limit** — as in editor-ui.
  Give the repository token "Contents: read and write" on this repository
  only, without workflow permission.

## Turning it off

`npx wrangler delete` in this directory, then revoke `DESIGNER_GIT_TOKEN` at
the repository host. Settings and theme stay in the repository; delete
`.ssg.designer.json` there to drop the settings made in the panel.
