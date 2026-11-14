# core — the site's modules, at run time

`core` is an optional Pages Functions worker that knows which modules (workers
with a [`module.json`](../../docs/WORKER-MODULES.md)) your site runs, asks
them how they are, and hands that to the admin panels in a form ready to show:
titles in the reader's language, links on this host, a status per module. On
top of that it ships two things people see:

- **the admin home page** at `/core/` — what needs attention across all
  modules, a card per module, and the site's basics ([below](#the-admin-home-page));
- **the shared navigation** `/core/nav.js` — one line in any panel's HTML
  gives it a bar linking every panel the person may open
  ([below](#how-a-panel-adopts-the-shared-navigation)).

## What it is not

- **Not a security boundary.** `core` never proxies a request to a module and
  never authorizes anything on a module's behalf. Every module keeps checking
  access itself; `core` only lists and links.
- **Hiding is cosmetic.** In the `auth` mode `core` leaves out modules the
  caller holds no permission for, so a navigation shows no doors they cannot
  open. A module left out is still served at its own address, where it refuses
  (or admits) the caller on its own terms.
- **Not an installer.** A module appears because the build listed it, and
  disappears when you remove it from `workers:` (or `modules:`) and deploy.
- `core` has no list of known modules in its code. A module describes itself
  with its manifest; your own worker shows up once it has one.

## Install

```sh
ssg new worker core
```

```yaml
workers:
  - name: comments
    dir: workers/comments
  - name: core
    dir: workers/core          # this entry makes the build write the registry
modules:                       # standalone Workers the build does not bundle
  - id: newsletter-ui
    url: /newsletter-admin     # its base path on this host
    dir: workers/newsletter-ui # optional: read its manifest at build time
```

With `core` in `workers:`, the build writes `functions/_ssg-modules.json` — the
module registry ([format](../../docs/WORKER-MODULES.md#the-module-registry)) —
and wrangler bundles it into the function. The file is never in this template
directory; a deploy without it (core copied by hand, an ssg too old to write
it) answers every route with `503 registry_missing` and how to fix it, instead
of failing the deploy.

Then pick a sign-in mode and deploy. The variables are in
[`wrangler.snippet.toml`](wrangler.snippet.toml).

## Sign-in

Three modes, the same as the other panels. The first one configured wins:
**Access → `auth` → password**. None configured → every route answers 503
(`signin_not_configured`); a mode configured by halves → 503 naming what is
missing (`access_misconfigured`, `not_configured`). It is never open.

| | Shared password | `auth` worker | Cloudflare Access |
|---|---|---|---|
| Good for | one person, or a few who share it — the simple first step | a team — the recommended next step | an organisation already on Cloudflare Zero Trust |
| What you set | the `CORE_ADMIN_PASSWORD` secret | `CORE_AUTH_ISSUER`, `CORE_AUTH_LOGIN_URL`, `CORE_AUTH_PUBLIC_KEY` (or `CORE_AUTH_JWKS_URL`) | `CORE_ACCESS_TEAM` + `CORE_ACCESS_AUD` |
| Who is signed in | nobody in particular (`user: null`) | the account (`id`, `email`) | the Access address (`email`) |
| Modules shown | all | only those the account's permissions open | all |
| Brute-force defence | lockout per IP (below) | the `auth` worker's | Access |

**Shared password.** HTTP Basic with any user name. After
`CORE_LOGIN_MAX_FAILURES` failed logins (default 10) within
`CORE_LOGIN_WINDOW_MINUTES` (default 10) the address is locked for one window:
even the right password then gets `429` with `Retry-After`. **With the
optional `CORE_DB` database** ([below](#module-management)) the count is kept
there and holds across isolates: every isolate sees every failure. **Without
it**, the lockout is kept in the isolate's memory, so it is best effort:
Cloudflare runs many isolates and recycles them, and an attacker spread across
them gets more guesses. Either way, the optional rate-limit binding
`CORE_LOGIN_LIMITER` adds a ceiling across isolates; it counts every request
that carries a password (it cannot count only failures), so size it for normal
use — 60 a minute is plenty for a panel. A database failure during a sign-in
refuses it (500), never lets the guess through. Parallel guesses gain nothing
(security review SR-12): a wrong guess counted beyond the limit gets `429`
like a locked one, and the right password is accepted only if the address is
still unlocked after the comparison.

**`auth` worker.** `core` carries a byte-identical copy of the `auth` worker's
portable session verifier
(`functions/api/core/_session-verifier.ts` ← `workers/auth/portable/session-verifier.ts`,
contract in `workers/auth/docs/CONTRACT.md`). It checks the session cookie (or
a service token in `Authorization: Bearer`) locally, without asking `auth`.
No session → `401` for an API call, a redirect to `CORE_AUTH_LOGIN_URL` for a
page; keys unavailable → `503`.

**Cloudflare Access.** Put `/api/core/*` behind an Access application; `core`
verifies the Access JWT (signature, audience, issuer, expiry).

## API

Every route is behind the sign-in and answers JSON with `cache-control:
no-store`, `x-robots-tag: noindex, nofollow`, a strict CSP and the usual
security headers — refusals and errors included. Errors are
`{error: "<snake_case>", detail?}`. A request other than GET/HEAD must carry
`x-core-admin: 1` (CSRF rule; the routes that change state are the module
settings under `/api/core/manage`).

### `GET /api/core/modules[?lang=pl]`

```json
{
  "mode": "auth",
  "user": { "id": "…", "email": "ada@example.com" },
  "lang": "pl",
  "items": [
    {
      "id": "comments", "kind": "pages-function",
      "title": "Komentarze", "description": "…", "version": "1.0.0",
      "panel": "/comments-admin.html",
      "nav": [{ "label": "Kolejka moderacji", "path": "/comments-admin.html", "permission": "comments:read" }],
      "status": "registered"
    }
  ]
}
```

- Language: `?lang=`, then `Accept-Language`, then English; a text a module
  lacks in that language is given in English.
- `panel` and `nav[].path` are absolute paths on this host (the module's base
  plus the manifest path); a path that would leave the host is dropped.
- `status`: `registered` (manifest from the build), `live` (a module the build
  could not read, asked for its manifest just now), `unreachable`,
  `invalid_manifest`. A module that refuses the caller when asked is left out.
- Visibility: password and Access → every module. `auth` → a module is listed
  only when the caller holds the permission of at least one of its `nav`
  entries, or — for a module without navigation — any permission it declares;
  `nav` entries are filtered the same way. A module that declares no
  permission at all, or whose manifest `core` does not have, is not listed in
  the `auth` mode.
- With `CORE_DB`, the [module settings](#module-management) apply for
  everyone: hidden modules are left out, the rest come in the set order, and a
  label for the asked language replaces `title`. Without it, or when the
  database fails, the items are as the build listed them.

### `GET /api/core/status`

```json
{
  "checkedAt": "2026-11-13T10:00:00.000Z",
  "items": [
    { "id": "comments", "version": "1.0.0", "status": "reachable", "ms": 12, "httpStatus": 200 },
    { "id": "newsletter-ui", "version": null, "status": "unreachable", "ms": 2000, "reason": "timeout" }
  ]
}
```

| `status` | Meaning |
|---|---|
| `reachable` | it answered its `health` endpoint, or its manifest |
| `unreachable` | `reason`: `timeout`, `deadline`, `network`, `http_status`, `redirect` (never followed), `refused_target` (its base leaves this host) |
| `denied` | it refused the caller's credential (401/403) |
| `invalid_manifest` | `reason`: `registry` (the build's copy), `not_json`, `rejected` (the reader's first finding in `detail`), `too_large` |
| `unknown` | it declares no endpoint to ask (`reason: no_endpoint`) |

What is asked: a module whose manifest is not in the registry → `{base}/api/module`;
otherwise its `endpoints.health`, else `endpoints.manifest`. `core` itself is
not asked. `cached: true` marks an answer from the cache. The same visibility
rule as `/modules` applies.

### `GET /api/core/module`

`core`'s own manifest (`module.json`; the route serves the generated twin
`functions/api/core/_module.ts`). Its panel is the home page, `/core/`. It
declares `core:manage` — managing order, visibility and labels — and
**no navigation entry**: every entry must name a permission, and the home page
is open to every signed-in account (it shows each person only what their
other permissions open), so any permission there would either be decoration
or hide the home page from people who should see it. The shared bar links the
home page through `home` in `/api/core/modules` instead. It declares no
summary either: it has nothing of its own to report.

### `GET /api/core/modules` — the fields the navigation reads

Besides `items`: `home` (the home page, `/core/`) and `signOut` — in the
`auth` mode the `auth` worker's sign-out route on this host (the `auth`
module's base from the registry + `/api/auth/logout`, else the path of a
same-origin `CORE_AUTH_ISSUER` + `/logout`), otherwise `null` — and `brand`:
`{name, logo, logoDark}` from `admin_theme:` (see [Panel look](#panel-look-admin_theme)),
or `null` when the site sets no name or logo. Each field is `null` when unset;
logos are paths on this host.

### `GET /api/core/overview[?lang=pl]`

Every module the caller may see (by the `/modules` rule), with its summary
([format](../../docs/WORKER-MODULES.md#the-summary-endpoint)), and what needs
a person across all of them. `core` itself is not an item. `canManage` says
whether this caller may arrange modules (the home page offers the link only
then; the manage routes refuse anyone else regardless).

```json
{
  "checkedAt": "2026-11-13T10:00:00.000Z", "mode": "auth", "user": { "id": "…", "email": "ada@example.com" }, "lang": "en",
  "canManage": false,
  "items": [
    { "id": "comments", "title": "Comments", "panel": "/comments-admin.html", "…": "…",
      "summary": { "state": "ok", "generatedAt": "…", "metrics": [{ "key": "pending", "label": "Awaiting moderation", "value": 12, "unit": null }],
                   "problems": [], "series": null } },
    { "id": "newsletter-ui", "…": "…", "summary": { "state": "unavailable", "reason": "timeout" } }
  ],
  "attention": {
    "items": [{ "module": "comments", "title": "Comments", "severity": "action", "text": "12 comments await moderation.", "href": "/comments-admin.html" }],
    "total": 1
  }
}
```

- `summary`: `null` when the module declares no `endpoints.summary`; else a
  `state` — `ok`, `unavailable` (`reason` as in `/status`), `denied`
  (401/403, `httpStatus`), `invalid` (`not_json`, `too_large`, `rejected` with
  the reader's first finding in `detail`, at most 300 characters). One module's
  trouble is never the whole answer's.
- Summaries are asked exactly as modules are asked for their status (same
  host, no redirect, per-module timeout, one deadline for the whole answer,
  only the caller's `cookie`/`authorization`, per-caller cache) and read with
  `readSummary` against a copy of `module-summary.schema.json`; the body is
  read up to 64 KiB, the value held to 32 KiB.
- `attention.items`: most serious first (`action`, `warning`, `info`), then by
  module id, each module's own order kept; at most 20, `total` says how many
  there were. Every `href` is the module's base joined with the summary's path
  and resolved by the URL parser; a link the parser would change (a `..`
  segment, `//`) is refused and becomes `null`, so a link can never leave the
  host or the module's base.
- With both `newsletter` and `newsletter-ui` answering, the numbers are shown
  once, by `newsletter-ui` (`newsletter`'s `metrics` are `[]`, `series` `null`,
  `metricsIn: "newsletter-ui"`); `newsletter`'s problems stay.
- Texts come in the asked language and are plain text: the home page inserts
  them as text, never as markup.
- With `CORE_DB`, the module settings apply as in `/modules`: a hidden
  module's card **and its attention items** are left out (its problems still
  reach `/info`), and each item carries `unreachableSince` — when the module
  was last seen to stop answering, if it has not answered since (`null`
  otherwise). Without `CORE_DB` the field is absent.

### `GET /api/core/info[?lang=pl]`

```json
{
  "checkedAt": "…", "mode": "password", "user": null, "lang": "en",
  "registry": { "format": 1, "modules": 7 },
  "modules": [{ "id": "comments", "title": "Comments", "kind": "pages-function", "version": "1.4.0", "contract": 1, "status": "registered" }],
  "problems": [{ "module": "newsletter", "title": "Newsletter sign-up", "code": "no_turnstile_secret", "text": "…" }],
  "build": { "generator": "ssg", "version": "1.9.9", "buildTime": "…", "finished": "…", "durationSeconds": 8.6, "pages": 412, "files": 1288, "content": { "post": 120 } },
  "buildState": "ok", "buildOption": "build_report",
  "theme": null
}
```

- `build` comes from the site's own `build.json`, written by ssg's
  `build_report` option ([CONFIGURATION.md](../../docs/CONFIGURATION.md)) and
  published as a static file; it is fetched like a module (same host, same
  limits) and read field by field — only these fields, each of its own type.
  `buildState`: `ok`, `absent` (404: the option is off — the home page says
  how to turn it on), `unreadable`, `invalid`. A report under another name:
  `CORE_BUILD_REPORT_PATH`.
- `theme`: the registry's `theme` as the build wrote it (name, logos, font
  stacks and files, `main`/`accent` and which was `derived`, the `light` and
  `dark` token values, `adjustments`, `checks`), or `null`; the Appearance
  view draws it. Each field is re-checked against the shape the build writes
  (`_theme.ts`): a hand-edited value outside it becomes `null` or is left out.
- `problems`: every configuration problem the visible modules' summaries
  report. Nothing comes from `core`'s environment — no value of any variable
  or secret (a test scans the answer for every one).

## Module management

`/core/#modules` — which modules the shared navigation and the home page show,
in which order, under which name, with a note for administrators; what else
the site could add; when each module stopped or started answering; who
changed what; and the settings as a file to move between environments. It
manages **visibility, order and labels — never installing, enabling or
deploying**: next to every switch the page says that hiding is cosmetic (the
module keeps running, its panel keeps working at its own address, and it
still decides who may use it), and gives the module's own `disable` text: how
to really switch it off.

**The database.** Everything here needs the optional D1 binding `CORE_DB`.
Without it the view is read-only and says why and how to enable it, and the
rest of `core` works exactly as before. With it, `core` creates its tables on
first use (`CREATE … IF NOT EXISTS`; [`schema.sql`](schema.sql) is the same,
for applying by hand). Every table is prefixed `core_` and `schema_meta` has
the other templates' shape, so the database may be one another worker
already uses.

```sh
wrangler d1 create core          # then bind it as CORE_DB (wrangler.snippet.toml)
```

**Who.** In the `auth` mode only an account holding `core:manage` — for
reading too, since the view carries admin notes and who changed what. With a
shared password or Cloudflare Access, every signed-in admin. Every route that
changes something carries `x-core-admin: 1`.

| Route | What |
|---|---|
| `GET /api/core/manage[?lang=pl]` | the view: `writable`, `readOnly` (`null`, `no_database`, `database_error`), `items` (every module, hidden ones too, in bar order: `title` from the manifest, `hidden`, `position`, `labels`, `note`, `updatedAt`, `updatedBy`, `disable`, `status: {status, reason, at}`), `available`, `history`, `changes`, `retention`, `limits` |
| `PATCH /api/core/manage/modules/{id}` | `{hidden?, labels?, note?}` — `labels` is the whole map `{lang: label}` (60 characters, one line; an empty label removes it), `note` up to 500 characters → `{item, changed}` |
| `PUT /api/core/manage/order` | `{order: [ids]}` — every module of the registry exactly once → `{order, changed}` |
| `GET /api/core/manage/export` | `core-settings.json`: `{kind: "ssg-core-settings", format: 1, exportedAt, modules: [{id, hidden, position, labels, note}]}` |
| `POST /api/core/manage/import` | the file (at most 64 KiB) → a preview: `{applied: false, fingerprint, changes: [{module, field, from, to}]}`; again with `?apply=<fingerprint>` → applied in one batch, or `409 settings_changed` when the settings moved since that preview |

Refusals: `403 forbidden` (`permission: "core:manage"`), `503 no_database`,
`404 unknown_module`, `413 too_large`, `400 invalid_json`, `422
invalid_settings` / `unknown_modules` with a `detail` naming the place. An
import is checked strictly — the kind and format, no unknown field, only this
site's modules, each once — and **replaces** the settings: modules the file
does not name go back to the defaults, and the preview lists those too.

- **Order** is changed with "Move up" / "Move down" buttons — keyboard
  operable, no dragging — and the new position is announced in the page's
  polite live region ("Comments moved to position 2 of 7."). At either end the
  button stays focusable and says the module is already first or last.
- **Available, not connected**: the templates the ssg that built the site
  ships and the site does not use — from the registry's `available`, never the
  network — each with `ssg new worker <id>` and the configuration it then
  needs (`workers:`, or `modules:` with a `url` for a standalone Worker).
- **Status history**: a row each time a module is *seen* to change between
  reachable and unreachable — when someone opens or refreshes the home page
  (it asks `/api/core/status`), never on a timer, so a module nobody looks at
  costs nothing. Kept for `CORE_HISTORY_DAYS` (default 90), purged with every
  write; each module's latest row stays, and the table never holds more than
  5 000 rows.
- **Change log**: who (the account's address in the `auth` mode, the Access
  address, or "someone with the shared password"), what and when — written in
  the same batch as the change. Kept for `CORE_CHANGES_DAYS` (default 365),
  at most 5 000 rows.

## The admin home page

`/core/` — static files from this template's `public/core/` (the site build
copies a worker's `public/` to the output root), no framework, no build step.
It asks `/api/core/overview` and `/api/core/status` together, then
`/api/core/info` (by then from core's cache), **on load and on Refresh only —
never on a timer**: every summary is a few D1 statements counting whole
tables in each module.

- **Needs attention** first: every waiting item as a sentence with its
  severity in words and a link into the owning panel; an explicit "nothing
  waits" — which says so honestly when some modules could not report.
- **Modules**: a card each — an original illustration (`img/modules/<id>.svg`,
  a neutral jigsaw piece for a module of your own; never fetched from a
  module), the state in a word (working, needs configuration, unavailable, no
  access, not checked), up to four numbers, a small chart when the summary
  has a series (with its sentence and "Show data"), what is wrong and what to
  check, and the way into the panel.
- **Site**: the build (or how to publish `build.json`), the sign-in mode —
  and, only with a shared password and more than one panel, one calm sentence
  pointing to the `auth` worker for teams — configuration problems, module
  versions and manifest contracts, the registry format, and a link to
  **Appearance**.
- **Modules** (`/core/#modules`): [module management](#module-management),
  linked from the Modules section ("Arrange modules") and from Site.
- **Appearance** (`/core/#appearance`, read-only): the panels' look as the
  last build computed it — name, colours, the logo on a light page, a dark
  page and the bar; every colour that had to change, in words; a button, a
  link, a field and a two-series chart in the scheme shown now; every
  contrast pair of both schemes with its ratio and minimum; the font stacks.
  Changing it means editing `.ssg.yaml` and rebuilding; the page says so.
- Five languages with a switcher, light/dark/system with a remembered choice,
  WCAG 2.2 AA, 360 px without horizontal scroll, its own CSP in a `<meta>`
  (a static page on Pages carries no headers of its own), `noindex`, no
  analytics. Settings without editing the page, in its JSON block:
  `{"lang": "pl", "loginUrl": "/auth/login", "i18n": {"pl": {"app.title": "…"}}}`
  (`loginUrl`: where the `auth`-mode "Sign in" link goes, default `/auth/login`).
- The page is public static HTML with no data in it; everything it shows comes
  from `/api/core/*`, behind the sign-in. With Access you can put `/core/*`
  behind the same application.

### Paths this template serves as static files

| Path | What |
|---|---|
| `/core/` (`index.html`), `/core/css/`, `/core/js/`, `/core/img/` | the home page |
| `/core/nav.js`, `/core/nav.css` | the shared navigation |
| `/core/tokens.css` | the shared design tokens |
| `/core/theme.css` | the site's own look; ships empty, the build writes it when `admin_theme:` sets colours or fonts |

**A page of your own at `/core/` stops the build.** The build copies these
files after everything else it writes; since ssg 1.9.9 (#420) a worker's
public file that lands on any file the build already wrote — a page, a static
file, a generated file — is an error naming both, like two pages at one
address: give your page another `link:`/`slug:` or leave the worker out. A
byte-identical file is not a clash, and neither is core's own copy left from
an earlier build in an output that is not cleaned. The base is fixed: the
build copies `public/` as it is, so it cannot be moved without changing ssg.

## How a panel adopts the shared navigation

One line, where the bar should appear (the top of `<body>`):

```html
<script type="module" src="/core/nav.js"></script><ssg-admin-nav current="newsletter-ui"></ssg-admin-nav>
```

- `current`: the panel's module id; that link is marked `aria-current="page"`
  (`core` marks the home link).
- **CSP.** Everything is same-origin: the script, its stylesheet (a `<link>`
  inside the element's shadow root — allowed by `style-src 'self'`, no inline
  style), the request to `/api/core/modules` (`connect-src 'self'`) and the
  sign-out form (`form-action 'self'`). A panel with `script-src 'self';
  style-src 'self'` needs no change. A **standalone Worker on another base
  path of the same host** (`newsletter-ui` under `/newsletter-admin/`, whose
  CSP is exactly that) loads `/core/nav.js` from the site root of the same
  origin — still `'self'`. A Worker on a different host cannot adopt it (that
  would need a looser CSP; don't).
- **Shadow DOM**, deliberately: the panel's own `a`, `nav`, `button` rules
  cannot reach in and the bar's cannot leak out; custom properties do cross,
  so the bar follows `--nav-bg`, `--on-nav`, `--nav-muted`, `--nav-hover`,
  `--nav-control`, `--nav-current`, `--font-ui`, `--radius` when the page sets
  them (as `/core/theme.css` does), with its own defaults otherwise.
- **The site's look.** Load `/core/theme.css` **after** the panel's own
  tokens (same selectors, so the later file wins by order alone):
  ```html
  <link rel="stylesheet" href="css/tokens.css">
  <link rel="stylesheet" href="/core/theme.css">
  ```
  Without `core` it 404s and the panel keeps its own colours; without
  `admin_theme:` it is empty. It sets only token names `public/core/tokens.css`
  defines, so a panel that styles itself with those tokens follows the brand
  with no other change.
- **Ready signal.** Don't look inside the shadow root: after each drawing the
  element carries `data-ready` and, when it shows a sign-out button,
  `data-signout="<path>"`, and dispatches a bubbling, composed
  `ssg-admin-nav:ready` event with `{user, signOut}` in `detail` (`user` is
  `{email}` or `null`). When it draws nothing both attributes are removed and
  no event is sent. A panel hides its own sign-out, say, with
  `nav.matches("[data-signout]")` or by listening for the event.
- **Brand.** The bar shows the site's name, or its logo instead (`<img>`
  only, `alt` = the name, a 32 px × up to 160 px box that fits any shape; the
  bar is dark in both schemes, so `logo_dark` when set). A logo that fails to
  load is replaced by the name.
- **Degrades to nothing.** No `core` (the script 404s): the browser logs the
  failed request in its network panel and nothing else happens. `core` there
  but the request refused, timed out (6 s), redirected or not JSON, or the
  stylesheet failing: the element stays empty — no bar, no message, no thrown
  error, no layout shift (it draws only once its stylesheet has loaded).
- **Shared password.** The bar asks with `x-core-quiet: 1`, so a refusal
  carries no password challenge and never pops up a dialog; the browser sends
  the password only after it has been asked once on `/api/core/…` — open the
  home page once and the bar appears in the other panels too. The home page
  fires `ssg-admin-nav:redraw` on its document after it signs in; any page may.
- **`auth` mode**: the signed-in address and a **Sign out** button — a plain
  form POST to the `auth` worker's logout, which accepts a same-origin form
  (its CSRF rule checks `Origin` / `Sec-Fetch-Site`; no script needed).
- Keyboard: links and buttons in order; under 720 px the module links fold
  behind a **Modules** disclosure button (`aria-expanded`), Escape closes it
  and returns focus. Not sticky, so it never covers a focused element; every
  target at least 40 px; five languages from the page's `<html lang>` (it
  follows a change of that attribute).
- A panel keeps working exactly as before without it: adoption is additive.

## Style and colour guidelines

`public/core/tokens.css` is the shared palette: every custom property of
newsletter-ui's tokens with the same value (a test compares them all, its
layout-only `--sidebar` aside), plus the bar's `--nav-*`. Material colour
tokens only — Teal as the main colour (and, until a theme sets them apart,
for links: `--link`), warm Brown neutrals, Deep Orange reserved for focus and
the current page, fixed status colours (red always
means an error); system font stacks only (an editorial serif for headings, a
humanist sans with Devanagari fallbacks), tabular figures. Every visual value
is a custom property with a light and a dark value, so `/core/theme.css`
(written by the build from `admin_theme:`, see [Panel look](#panel-look-admin_theme))
restyles everything from one place.
The contrast tests in `test/home-static.test.ts` compute every
text/background pair (≥ 4.5:1) and every mark and control edge (≥ 3:1) in both
themes from the tokens file. Colour is never the only signal: states are
words with differently shaped marks, severities are words in chips, the
current page is bold and underlined.

## Panel look (`admin_theme`)

Set once in `.ssg.yaml` ([CONFIGURATION.md](../../docs/CONFIGURATION.md#admin-panels-look)),
rebuilt with the site, served by `core` to every panel that adopts
`/core/theme.css` and the bar:

```yaml
admin_theme:
  name: "Anna's Kitchen"
  logo: /img/logo.svg          # logo_dark: for dark backgrounds (the bar)
  main: "#1b3a4b"
  accent: "#e07a5f"
  font: "Georgia, serif"       # headings; font_body: everything else
```

**Choosing the two colours.** `main` is the brand's surface colour: the
navigation bar, primary buttons, the current menu item, the first chart
series. `accent` calls attention: **links** (`--link`), the focus ring, the
current-page marker in the bar, the second chart series. Without a theme
`--link` equals the shipped `--primary`, so links look as they always did. Give `main` a colour that can carry white text (a dark
or saturated one works best — the bar needs 7:1 against white) and `accent` a
clearly different hue. Give only one and the other is derived: an accent of
the opposite hue, or a darker, quieter main from the accent.

**What is derived, and how.** From those two the build computes, for the
light and the dark scheme: `primary`, `on-primary`, `primary-soft`,
`on-primary-soft`, `link`, `focus`, `c1`–`c5` and every `--nav-*` token. One
algorithm (`internal/admintheme`, OKLCH): a brand colour that can serve a
role as given is used exactly; one that cannot (too light for text on white,
too dark for a dark-scheme link) keeps its hue and moves to the nearest
lightness that works — less colourful only where sRGB cannot hold it — and
the Appearance view says so in words ("Light scheme: for links, #ae4e34 is
used instead of #e07a5f …"). Guaranteed in both schemes,
and proven by property tests over thousands of colour pairs including white,
black, greys, pure primaries, very light yellow and near-identical pairs:
text ≥ 4.5:1 on its background — links on the page, on cards and on inset
areas included; focus ring, chart marks and the bar's edges
≥ 3:1 (WCAG 2.2); the focus ring told apart from the primary colour; every
two chart series told apart (OKLab distance ≥ 0.12). Charts never rely on
colour alone either: lines differ in style and markers in shape, and every
chart has its numbers as text.

**What cannot be themed, and why.** The neutral backgrounds and text stay as
shipped — every other pair is built against them, so readability holds
whatever the brand. The state colours (`danger`, `critical`, `warning`, `ok`,
`info`) never come from a brand: red always means an error. Advanced
`tokens:` overrides may set single derived tokens per scheme; each overridden
pair is checked again and a failure is a finding under `check_admin_theme`.

**Fonts.** Font-family lists only, checked against a strict grammar; the
build adds Devanagari fallbacks and a generic family, so Hindi always has a
font. Files for `@font-face` (`font_files:` — `.woff2`, `.woff`, `.ttf`,
`.otf`) must be on the site's own host: the panels' CSP allows `font-src
'self'` and nothing else, and that stays.

**Logo.** A `.svg`, `.png` or `.webp` file on the site's own host (put it in
`static/`), shown only through `<img>` — an SVG is never inserted into the
page — at 32 px high and up to 160 px wide, with the name as its alternative
text. The bar is a dark surface in both schemes: give `logo_dark` when the
logo needs a dark-background version. No uploads: the logo is a file of the
site, like any other.

**Public pages are not themed.** Subscription confirmations, unsubscribe
pages and other pages visitors see keep their own settings and never depend
on `core`.

## How modules are asked

- Only the **same origin** as the incoming request, built from the registry's
  base and a manifest path; anything else is refused before a request exists.
- `redirect: "manual"`: a redirect is a failure, never followed.
- In parallel, each with `CORE_PROBE_TIMEOUT_MS` (default 2000), the whole
  answer within `CORE_STATUS_DEADLINE_MS` (default 4000).
- A manifest body is read up to 256 KiB; larger is `invalid_manifest`.
- The request carries **only the caller's own `cookie` and `authorization`
  headers** — nothing invented, no key of `core`'s own. With Access, the
  `CF_Authorization` cookie lets Access admit the request again.
- Answers are cached per isolate for `CORE_STATUS_CACHE_SECONDS` (default 15),
  keyed by the URL and a hash of the caller's credential: one caller's view is
  never served to another.

## What a module must provide to show up

1. A valid `module.json` ([format](../../docs/WORKER-MODULES.md)).
2. Be in `workers:` (Pages) or `modules:` (a standalone Worker, with `url`).
3. For a live status: an `endpoints.health` or `endpoints.manifest` it serves
   (a standalone Worker without `dir`: its manifest at `{base}/api/module`).
4. In the `auth` mode, `nav` entries (or declared permissions) the people who
   use it hold.

## Variables

| Name | Default | Meaning |
|---|---|---|
| `CORE_ADMIN_PASSWORD` (secret) | — | shared-password mode |
| `CORE_LOGIN_MAX_FAILURES` | `10` | failed logins per IP per window |
| `CORE_LOGIN_WINDOW_MINUTES` | `10` | the window and the lock's length |
| `CORE_LOGIN_LIMITER` (binding) | — | optional rate-limit binding for password attempts |
| `CORE_AUTH_ISSUER` | — | `auth` mode: the token issuer |
| `CORE_AUTH_LOGIN_URL` | — | `auth` mode: where to sign in |
| `CORE_AUTH_PUBLIC_KEY` / `CORE_AUTH_JWKS_URL` | — | `auth` mode: exactly one; the inline key is recommended |
| `CORE_AUTH_AUDIENCE` | the request's host | `auth` mode |
| `CORE_ACCESS_TEAM`, `CORE_ACCESS_AUD` | — | Cloudflare Access mode, both |
| `CORE_PROBE_TIMEOUT_MS` | `2000` | per module |
| `CORE_STATUS_DEADLINE_MS` | `4000` | per status answer |
| `CORE_STATUS_CACHE_SECONDS` | `15` | per caller and module |
| `CORE_BUILD_REPORT_PATH` | `/build.json` | where the site publishes its build report, when `build_report` names another file |
| `CORE_DB` (D1 binding) | — | [module management](#module-management), status history, change log, and a lockout that holds across isolates |
| `CORE_HISTORY_DAYS` | `90` | how long the status history is kept |
| `CORE_CHANGES_DAYS` | `365` | how long the change log of the settings is kept |

## Limits

- Password mode forwards the caller's Basic credential to each module. A module
  with a **different** password answers `denied` — and counts a failed login
  at that module (its own lockout). Use one password for all panels, or the
  `auth` worker.
- The status cache lives in one isolate's memory, and so does the lockout
  without `CORE_DB`.
- Modules the build cannot read and that do not follow the `/api/module`
  convention show as `unreachable`.
- `wrangler pages dev` run from the build output serves the `functions/`
  directory as static files locally; `wrangler pages deploy` does not upload it.

## Local preview

The real handlers and `public/`, invented modules (`test/demo.ts`), a strict
CSP on every page; nothing leaves the machine:

```sh
node --import ./workers/core/test/resolve.mjs workers/core/test/dev-server.mjs
# → http://localhost:8790/core/   (any user name, password "demo")
# switch: /dev/set?scenario=good|todo|down&mode=password|auth&as=owner|limited|manager&core=on|off&theme=none|harbour|lemon|plum&db=on|off|reset
# a bare panel adopting the bar: /dev/host.html
```

`test/core-shots.mjs` (needs Playwright on the machine, never as a dependency
here) captures every state light and dark at 1440×900 and 390×844, the bar in
a bare panel (and that panel with `core` absent), the three demo looks (home,
the bar, Appearance; into `<out-dir>/themes/`), checks 360 px, runs axe-core
when given its file and walks the bar with the keyboard:
`node workers/core/test/core-shots.mjs <out-dir> [axe.min.js]`.

## Tests

From the repository root (Node 23.6 or newer, no npm packages):

```sh
node --import ./workers/core/test/resolve.mjs --test 'workers/core/test/*.test.ts'
```

The browser modules are tested in node too (a stand-in DOM in `test/dom.ts`);
`public/core/js/main.js` is the only file without unit tests — DOM glue that
wires the tested modules to the document, kept thin on purpose.

The copies of `workers/module-manifest.ts`, `workers/module.schema.json`,
`workers/module-summary.schema.json` and
the `auth` worker's portable verifier are checked byte for byte and run
against their originals' own suites; scaffolded alone, those checks are
skipped with a visible reason.
