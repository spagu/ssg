# Module requirements: what a worker must contain to work with `auth` and `core`

A worker becomes a **module** when it describes itself, and a module can join
two optional workers: `core` (the admin home page and the navigation shared
between panels) and `auth` (accounts, roles, one sign-in for every panel).
This page is the checklist: what your worker must contain for each. The
formats themselves are specified elsewhere and linked from each rule — this
page says what is required, those say exactly how.

**MUST** is a condition for working at all; **SHOULD** is what every bundled
template does and what people who run your module will expect.

## Three levels

Each level includes the one before it. Stop where your module's needs stop.

| Level | You get | You add |
|---|---|---|
| 1. Module | The build checks your worker and can list it | `module.json` |
| 2. Works with `core` | A place in the shared navigation, a card on the admin home page, a live status, your numbers and to-dos in one overview | manifest endpoint, health or summary endpoint, registry entry, one line of HTML in the panel |
| 3. Works with `auth` | People sign in once for every panel; each action needs a permission; scripts use service tokens | the verifier file, permissions per action, the `auth` variables, a CSRF check |

A worker with no admin side (a public endpoint, a webhook checked by a
provider's signature) stops at level 1: `panel: null`, empty `nav` and
`permissions`.

## What you get in return

Everything below already exists; your worker uses it instead of building it.

### From `auth` — in your code

One call, `verifySession(request, config)`, answers who is asking. No request
to `auth`, no database of your own.

| You receive | Use it for |
|---|---|
| `user.id`, `user.email` | who acted: the author of a change, the name in your audit trail |
| `perms`, checked with `can(perms, "<id>:<action>")` | a decision per action, not one gate for the whole panel |
| `kind` — `user` or `service` | telling a person in a browser from a script; a bearer request needs no CSRF check |
| `sessionId`, `expiresAt` | audit entries; knowing when the panel must refresh |
| `denied()`, `forbidden()`, `loginRedirect()` | the right answer to every refusal — a redirect to sign-in for a page, 401 or 403 JSON for an API call |

### From `auth` — what you do not have to build

- **Sign-in**: the login page, passwords and their storage, lockout after
  failed attempts, reset links, sessions, refresh, logout and "log out
  everywhere". Your worker has no login form at all.
- **More ways to sign in**, with no change in your worker: Google, Microsoft,
  Apple, GitHub, Facebook, Cloudflare Access, a second factor (authenticator
  app, backup codes, passkeys).
- **People and rights**: invitations, roles, groups and grants, managed in the
  accounts panel at `/auth/admin/`. Your permissions appear there once
  declared; an administrator grants them, you only check them.
- **A second factor for risky actions**: a role that needs one simply lacks
  its permissions until the person has passed it — your `can()` check is
  enough.
- **Access for scripts**: service accounts and service tokens for CI, cron
  and assistants, narrowed to the permissions a script needs, with expiry and
  revocation. Your API accepts them through the same `verifySession`.
- **A link to your panel** on each person's "Your panels" page.

### From `core` — in your panel

- **The shared navigation**: one line of HTML gives your panel a bar linking
  every panel the person may open, the site's name or logo, who is signed in,
  and a sign-out button. Your panel needs no menu of other panels and no
  sign-out of its own.
- **The site's look**: `/core/theme.css` restyles your panel in the colours,
  fonts and logo the site's owner set once in `admin_theme:` — with contrast
  already checked for light and dark.
- **A place on the admin home page** at `/core/`: a card with your title and
  description in the reader's language, your numbers, a small chart, and your
  to-dos in the one list of what needs attention across the whole site.
- **A live status**: whether your module answers, since when it does not, and
  your configuration problems shown where the administrator looks first —
  instead of in a log nobody reads.
- **Management by the administrator**: your module's place in the order, its
  label and a note, without a line of your code.

### Asking `core` yourself

The bar is one use of an API your panel can call too. To build your own menu
of the site's other panels — a sidebar, a "switch to" list, a link from one
panel into another — ask `GET /api/core/modules`:

```js
// The site's other panels, as the signed-in person may see them.
async function otherPanels(currentId) {
  try {
    const lang = document.documentElement.lang || "en";
    const res = await fetch(`/api/core/modules?lang=${encodeURIComponent(lang)}`, {
      headers: { "x-core-quiet": "1" }, // a refusal never pops up a password dialog
      redirect: "error",
    });
    if (!res.ok) return [];              // no core, or not signed in: show nothing
    const { items } = await res.json();
    return items.filter((m) => m.id !== currentId && m.panel);
  } catch {
    return [];                           // core is optional: its absence is not an error
  }
}
```

Each item is ready to show — insert the texts as text, never as HTML:

| Field | What it is |
|---|---|
| `id`, `kind`, `version` | the module's manifest values |
| `title`, `description` | already in the asked language (English where the module lacks it); `title` is the administrator's label when one is set |
| `panel` | the panel's address: an absolute path on this host, or `null` |
| `nav[]` | `{label, path, permission}` — the module's own pages, paths absolute on this host |
| `status` | `registered`, `live`, `unreachable` or `invalid_manifest` |

The same answer carries `home` (the admin home page), `signOut` (where to
post a sign-out, or `null`), `brand` (`{name, logo, logoDark}` or `null`),
`user` and `mode`.

What you do not have to work out yourself:

- **Who may see what.** In the `auth` mode the list holds only modules the
  caller has a permission for, and only the `nav` entries they may open. With
  a password or Access it holds every module.
- **Order, visibility and names** set by the administrator are already
  applied.
- **Addresses.** A standalone Worker's base path is already joined in; a path
  that would leave the host has been dropped.

To link to one particular module, find it by `id` and use its `panel` — never
hard-code another worker's address: a standalone Worker may be mounted on any
base path. When it is not in the list, the person cannot open it (or it is
not installed): show no link.

Other routes, the same way: `GET /api/core/status` (which modules answer, and
how fast), `GET /api/core/overview` (every module's numbers and to-dos),
`GET /api/core/info` (the site, the build, versions) —
[workers/core/README.md](../workers/core/README.md#api). All of them are
behind `core`'s sign-in and work from a page on the same host; a worker
calling them from its own code forwards the caller's `cookie` and
`authorization` headers and nothing else.

### What stays yours

Neither worker stores your data, proxies your requests or refuses on your
behalf. `core` hides a link a person cannot use; only your module can refuse
the request. `auth` says who is asking and what they hold; what each
permission means is decided in your handlers.

## What the directory contains

```
workers/<id>/
├── module.json                  level 1 — the manifest
├── README.md                    install, variables, permissions, how to switch it off
├── wrangler.snippet.toml        a Pages template: bindings and [vars] to merge
│   or wrangler.toml             a standalone Worker: its own deployment
├── functions/api/<id>/…         a Pages template   (src/… in a standalone Worker)
│   ├── _module.ts               level 2 — generated copy of module.json (Pages only)
│   ├── _summary-kit.ts          level 2 — copy of workers/module-summary-kit.ts, if it serves a summary
│   └── _session-verifier.ts     level 3 — copy of workers/auth/portable/session-verifier.ts
├── public/…                     the panel, if any
└── test/
    ├── auth-copy-guard.ts       level 3 — copy of workers/auth/portable/copy-guard.ts
    ├── auth-copies.test.ts      level 3 — fails while a copy differs from its source
    └── summary-sample.ts        level 2 — prints a summary for the cross-template test
```

Copies are deliberate: a template is scaffolded alone (`ssg new worker <id>`)
and cannot import from another template. A copy is never edited; a test keeps
it identical to its source.

## Level 1 — the manifest

Format and field rules: [WORKER-MODULES.md](WORKER-MODULES.md#fields).

- **MUST** have `module.json` at the worker's root, valid against
  `workers/module.schema.json`, with `id` equal to the directory name and
  `kind` matching the layout (`pages-function` or `worker`).
- **MUST** give every text (`title`, `description`, `nav` labels, permission
  descriptions, `disable`) in all five panel languages: `en`, `pl`, `hi`,
  `de`, `fr`.
- **MUST** list only paths the worker really serves (`panel`, `nav`,
  `endpoints`), relative to the module's base, never a scheme or host.
- **MUST** declare every permission its admin side distinguishes, named
  `<namespace>:<action>` in lower case — one per level of trust: who may only
  look, who may change, who may do something irreversible or that reaches
  people. No wildcards in a declaration.
- **MUST** say in `disable` how to really switch the module off. Hiding it in
  a navigation does not disable it.
- **MUST NOT** contain a secret, a configuration value or data.
- **SHOULD** bump `version` when routes, the panel or permissions change.

## Level 2 — working with `core`

`core` only shows and links. It does not proxy requests and does not
authorise anything on your module's behalf — your module refuses on its own,
always. Details: [workers/core/README.md](../workers/core/README.md#how-modules-are-asked).

**Being found**

- **MUST** be on the same host as the site. A worker on another host cannot
  be a module of this site.
- **MUST** be listed: in `workers:` (a Pages template) or as a `modules:`
  entry with its base path in `.ssg.yaml` (a standalone Worker) —
  [the module registry](WORKER-MODULES.md#the-module-registry).

**Describing itself at run time**

- **MUST** serve its manifest with `GET` at the path declared as
  `endpoints.manifest` (by convention `…/api/module`), behind the same check
  as the rest of its admin API. A refusal is byte for byte what the admin
  endpoint answers.
- **MUST** accept the caller's own credential on that route — the `cookie`
  and `authorization` headers `core` forwards unchanged. `core` has no key of
  its own.
- **MUST** answer within two seconds (`CORE_PROBE_TIMEOUT_MS`) with at most
  256 KiB and without a redirect; anything else shows the module as
  unavailable.
- **SHOULD** serve `endpoints.health` for a status that does not depend on
  who asks.

**The summary** — optional, and the reason people open the home page
([format](WORKER-MODULES.md#the-summary-endpoint)):

- **MUST**, when declared as `endpoints.summary`: follow
  `workers/module-summary.schema.json` — at most 4 metrics, 10 attention
  items, 10 problems, one series of 90 points, 32 KiB in all.
- **MUST** hold aggregates and fixed sentences only: no address, name,
  message text, token, IP or any other per-person value.
- **MUST** link (`href`) only into its own panel.
- **SHOULD** compute it with one or two aggregate statements, never by
  reading rows into the worker.

**The panel**

- **SHOULD** include the shared navigation — one line at the top of `<body>`:
  `<script type="module" src="/core/nav.js"></script><ssg-admin-nav current="<id>"></ssg-admin-nav>`.
- **SHOULD** load `/core/theme.css` after its own tokens and style itself with
  the token names of `workers/core/public/core/tokens.css`, so the site's
  `admin_theme:` restyles it.
- **MUST** keep working unchanged when `core` is absent or down: the bar does
  not appear, the theme file is a 404, nothing else changes.

## Level 3 — working with `auth`

The agreement is [the `auth` contract](../workers/auth/docs/CONTRACT.md). Your
module never asks `auth` anything per request: it checks the signed token
itself with the public key.

**The verifier**

- **MUST** carry `workers/auth/portable/session-verifier.ts` byte for byte
  (a Pages template: with a leading underscore) and guard it with the copy
  guard. Never reimplement token checking.
- **MUST** take its settings from `<PREFIX>_AUTH_ISSUER`,
  `<PREFIX>_AUTH_LOGIN_URL` and exactly one of `<PREFIX>_AUTH_PUBLIC_KEY`
  (recommended) or `<PREFIX>_AUTH_JWKS_URL`; `<PREFIX>_AUTH_AUDIENCE` is
  optional. The prefix is your module's own.

**Sign-in modes**

- **MUST** check modes in this order, for the ones it offers: Cloudflare
  Access → `auth` → shared password.
- **MUST** answer 503 when nothing is configured or the `auth` set is
  incomplete. Never open, and never a silent fall-through from a half-set
  `auth` to the password.
- **MAY** offer `auth` as its only mode when every action needs a known
  author or a permission (a module that publishes on someone's behalf, for
  one). Its README then says so in the first screen.
- **SHOULD** otherwise keep the shared password as a full mode: one person
  with one worker needs no accounts.

**Permissions**

- **MUST** check a declared permission on every admin route and every action
  with `can(perms, required)`, in the handler that does the work — not only in
  the panel, and not only in a router in front of it.
- **MUST** answer with the verifier's own responses: `denied()` for a failed
  session (a page is sent to the login URL, an API call gets 401) and
  `forbidden(permission)` for a missing permission (403 naming it).
- **MUST** treat the `nav` permission as a display hint only. A hidden link
  is not a refusal.
- **SHOULD** follow the naming `auth` treats as sensitive: `<id>:erase` for
  deleting people's data. A sensitive permission reaches a service token only
  with an owner's explicit allowance
  ([service tokens](../workers/auth/docs/SERVICE-TOKENS.md#sensitive-permissions)).
- **SHOULD** declare `"sensitive": true` in module.json on any other
  permission that gives the trust of a person with write access to the
  repository or the site — code the build runs or a visitor gets, settings
  that deploy, references to secrets (`designer-ui:theme`,
  `designer-ui:advanced`). No name rule can guess them; the declaration
  reaches `auth` through `AUTH_KNOWN_PERMISSIONS`.

**Requests**

- **MUST** check `Origin` (or `Sec-Fetch-Site`) on every state-changing
  request authenticated by the cookie, also in the Access mode. A request
  authenticated by a bearer header needs no such check.
- **MUST** accept people's tokens from the cookie only and machines' tokens
  from `Authorization: Bearer` only — the verifier enforces it; do not work
  around `wrong_kind`.
- **SHOULD**, in the panel, answer a 401 by calling `POST /api/auth/refresh`
  once and retrying.
- **SHOULD** record who acted (`user.id`, and `sessionId`) for actions worth
  an audit trail.

**What to tell the people who run it**

- **MUST** document that a removed role, a blocked account or a logout
  reaches the module at the latest after the token lifetime plus 30 seconds
  (10 min 30 s by default) — the module does not see revocation sooner.
- **MUST** document its `<PREFIX>_AUTH_*` variables in the README and in the
  wrangler file, and that its permissions must be in `auth`'s
  `AUTH_KNOWN_PERMISSIONS` to be grantable
  (`node workers/auth/scripts/auth-permissions.mjs workers/*/module.json`
  prints the value).

## Every panel, whatever the level

- **MUST** be served from the site's own host with a strict Content Security
  Policy: `script-src 'self'; style-src 'self'`, no inline script or style, no
  external resource.
- **MUST** insert data as text, never as HTML. Every worker on the host
  shares one session; an injection in one panel acts in all of them.
- **MUST** send `Cache-Control: no-store` on admin answers and keep the panel
  out of search indexes.
- **MUST** never return or log a secret, a token or password material, and
  refuse rather than open when configuration is missing.
- **SHOULD** offer the five panel languages, light and dark, WCAG 2.2 AA
  contrast, full keyboard use, and fit a 360-pixel screen.
- **SHOULD** answer errors as `{"error": "<snake_case>", "detail"?}` and
  lists as `{"items", "nextCursor"}`, like the bundled templates.

## Tests a module ships

- The manifest validates and matches the code — inside the ssg repository
  `node --test 'workers/*.test.ts'` checks every template; outside it,
  [the one-line check](WORKER-MODULES.md#making-your-own-worker-a-module).
- The manifest and summary routes answer exactly as the admin route does when
  refused.
- Each admin route × no session, a session without the permission, a session
  with it; a half-configured `auth` set → 503.
- A state-changing request with a foreign `Origin` is refused.
- The summary, seeded with recognisable personal data, contains none of it.
- The copies are identical to their sources.

## Before you call it done

- [ ] `module.json` is valid, complete in five languages, lists only served paths
- [ ] the module is in `workers:` or `modules:`
- [ ] manifest route is behind the admin check; health or summary declared
- [ ] the panel works with and without `core`
- [ ] every action checks a permission in the `auth` mode
- [ ] nothing configured → 503
- [ ] CSRF check on every write authenticated by a cookie
- [ ] README: install, variables, permissions, revocation delay, how to switch it off

## What is not required

`core` and `auth` are both optional, for the site and for a module: a module
at level 1 or 2 is complete without `auth`, and a level-3 module works without
`core`. A module needs no database for any of this, no particular framework,
and no change in `core` or `auth` to be added — they learn about it from its
manifest.
