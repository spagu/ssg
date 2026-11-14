# Worker modules: the module manifest

Every worker template that `ssg new worker` scaffolds describes itself in a
file at its root, `module.json`: what it is called, where its admin panel lives,
which navigation entries it offers, which permissions its admin surface
distinguishes, which endpoints describe it, and how to really switch it off.
A worker with such a file is a **module**.

The manifest is a description only. It never holds a secret, a configuration
value or data — anyone allowed to see the module's admin endpoints may see it.

- Format: [`workers/module.schema.json`](../workers/module.schema.json)
  (JSON Schema, draft 2020-12) — the one definition of the format.
- Reader: [`workers/module-manifest.ts`](../workers/module-manifest.ts) — a
  dependency-free TypeScript validator that interprets that schema and adds the
  checks a schema cannot express.
- Summary format: [`workers/module-summary.schema.json`](../workers/module-summary.schema.json)
  — what a module's [summary endpoint](#the-summary-endpoint) answers; the same
  reader checks it (`readSummary`).

## What reads the manifest today

| Reader (1.9.9) | What it does with it |
|---|---|
| The module itself | `GET …/api/module` in `comments`, `newsletter`, `newsletter-ui`, `auth` and `core` serves it, behind each module's own admin check |
| The site build | Checks the manifests of the workers in `workers:` and of `modules:` entries and writes them into a [module registry](#the-module-registry) — **only when one of the workers is `core`** |
| The `core` worker | Reads that registry, asks each module for its manifest, status and summary, and serves the admin home page (`/core/`) and the shared navigation (`/core/nav.js`), which the `comments`, `newsletter-ui` and `auth` panels include — see [`workers/core/README.md`](../workers/core/README.md) |
| The `auth` worker | Builds roles from the declared permission names (its own manifest plus the ones you list in `AUTH_KNOWN_PERMISSIONS`; `node workers/auth/scripts/auth-permissions.mjs workers/*/module.json` prints that list) and puts each account's effective permissions in the `perms` of its access token |
| Every module in `auth` mode | Checks the token's `perms` against the permission each action needs: `comments`, `newsletter`, `newsletter-ui`, `core` and `auth` itself for people, `web-push` and `republish-trigger` for service tokens |
| `workers/modules.test.ts` | Checks every bundled template's manifest against its code, and each summary against the summary format |

A module enforces permissions only in the `auth` mode. With a shared password
or Cloudflare Access whoever passes the module's check may do everything, as
before; that is a supported choice for one person, not a gap
([which mode to choose](WORKERS.md#admin-sign-in-which-mode-to-choose)).

## Fields

```json
{
  "id": "comments",
  "contract": 1,
  "version": "1.0.0",
  "kind": "pages-function",
  "title": { "en": "Comments", "pl": "Komentarze", "hi": "टिप्पणियाँ", "de": "Kommentare", "fr": "Commentaires" },
  "description": { "en": "Reader comments stored in D1, …", "pl": "…", "hi": "…", "de": "…", "fr": "…" },
  "panel": "/comments-admin.html",
  "nav": [
    {
      "label": { "en": "Moderation queue", "pl": "Kolejka moderacji", "hi": "मॉडरेशन कतार", "de": "Moderationswarteschlange", "fr": "File de modération" },
      "path": "/comments-admin.html",
      "permission": "comments:read"
    }
  ],
  "permissions": [
    { "name": "comments:read", "description": { "en": "View the comments in the moderation queue: …", "pl": "…", "hi": "…", "de": "…", "fr": "…" } },
    { "name": "comments:moderate", "description": { "en": "Approve a comment or mark it as spam.", "…": "…" } },
    { "name": "comments:delete", "description": { "en": "Delete a comment for good, and run the purge …", "…": "…" } },
    { "name": "comments:import", "description": { "en": "Import comments in bulk from another system, …", "…": "…" } }
  ],
  "endpoints": { "manifest": "/api/comments/module", "summary": "/api/comments/module/summary" },
  "disable": { "en": "Remove workers/comments from the workers: list in .ssg.yaml, …", "pl": "…", "hi": "…", "de": "…", "fr": "…" }
}
```

| Field | Type | Meaning |
|---|---|---|
| `id` | string, `^[a-z][a-z0-9-]*$` | The template's directory name (`workers/<id>`). |
| `contract` | integer ≥ 1 | Version of this manifest format. Currently `1`. |
| `version` | `MAJOR.MINOR.PATCH` | The module's own version, independent of the ssg release. Bump it when the module's routes, panel or permissions change. |
| `kind` | `pages-function` \| `worker` | `pages-function`: merged into the site's Pages build through `workers:`. `worker`: a standalone Worker with its own `wrangler.toml` (e.g. `newsletter-ui`). |
| `title`, `description` | text | Name and one-sentence description. |
| `panel` | path or `null` | The module's admin panel; `null` when it has none. |
| `nav` | array (≤ 12) | Entries a shared navigation may show: `label` (text), `path`, `permission` (a name from `permissions`). Empty when there is no panel. |
| `permissions` | array (≤ 32) | Every permission the admin surface distinguishes: `name` (`namespace:action`) and `description` (text). |
| `endpoints` | object of paths | Endpoints that describe the module. Known keys: `manifest` (this file), `health`, `summary` ([the summary endpoint](#the-summary-endpoint)), `openapi`. Only endpoints the module really serves are listed. |
| `disable` | text | How to really switch the module off. Hiding a module in a navigation does not disable it; this says what does (remove it from `workers:` and deploy, delete the Worker, revoke a webhook…). |

**Text** is an object with a non-empty string for each of the five panel
languages — `en`, `pl`, `hi`, `de`, `fr` — at most 600 characters each. More
language codes may be added.

**Paths** are relative to the module's base. For a `pages-function` module the
base is the site root, so `/comments-admin.html` is literally that URL. For a
`worker` module the base is wherever the Worker is mounted
(`NEWSLETTER_UI_BASE`, default `/newsletter-admin`), so `newsletter-ui`'s panel
`/` is `/newsletter-admin/`. A path starts with `/`, has no empty, `.` or `..`
segment, no query, and no scheme or host — a manifest can never point a
navigation to another site. `panel` and `nav` paths may end in a `#fragment`
for panels with hash routing (`/#/campaigns`); endpoint paths may not.

**Permission names** are `namespace:action`, lowercase. The namespace is the
module's own id, or the id of the module whose data it acts on: `newsletter-ui`
declares `newsletter:*` permissions because it works on the `newsletter`
template's subscribers, and a permission declared by both means the same in
both (same description — the test checks). Wildcards (`newsletter:*`, `*`)
belong to role definitions in `auth`, never to a declaration.

### Permissions of the bundled templates

| Template | Permissions | Derived from |
|---|---|---|
| `comments` | `comments:read`, `comments:moderate`, `comments:delete`, `comments:import` | `GET /api/comments/admin` (read); `approve`/`spam` (moderate); `delete` and `purge` (delete); `POST /api/comments/import` (import) |
| `newsletter` | `newsletter:read`, `newsletter:export`, `newsletter:settings` | `GET /api/newsletter/admin` counts (read); `?format=csv` (export); `purge` (settings) |
| `newsletter-ui` | `newsletter:read`, `newsletter:edit`, `newsletter:send`, `newsletter:import`, `newsletter:export`, `newsletter:erase`, `newsletter:suppress`, `newsletter:settings` | One per level of trust of its admin API: viewing, editing drafts and subscriber fields, mailing real people, importing, exporting, erasing a person, managing suppressions, maintenance |
| `auth` | `auth:admin`, `auth:invite` | Managing people, roles, groups, grants and anyone's sessions (admin); inviting people with roles no wider than one's own (invite) |
| `core` | `core:manage` | [Module management](../workers/core/README.md#module-management): order, visibility, labels and notes |
| `web-push` | `web-push:notify` | `POST /api/push/notify` (the shared key `PUSH_KEY`, or a service token holding it) |
| `republish-trigger` | `republish-trigger:run` | `POST /api/republish` (the shared key `REPUBLISH_KEY`, or a service token holding it) |
| `contact-form`, `conversions-proxy`, `cookie-consent`, `dynamic-price`, `rate-limit`, `stripe-checkout` | none | Public endpoints, or ones authenticated by a provider's signature (Stripe), not by a person or a key you hand out |

The two machine-called templates declare their one action so that an `auth`
service token can be granted exactly that; the shared keys keep working.

## Versioning

- A reader **ignores fields it does not know**. Adding an optional field never
  needs a new contract.
- A reader **refuses a manifest whose `contract` is newer** than the newest it
  understands, with a message that names the module and says to upgrade
  (`module "x" uses manifest contract 2, but this reader understands contract 1
  at most — upgrade ssg (and the core worker) to read it`). The contract number
  changes only when an existing field changes meaning or a new field becomes
  required.
- A module's `version` is its own business; readers display it, they do not
  compare it.

## The manifest endpoint

| Template | Route | Auth |
|---|---|---|
| `comments` | `GET /api/comments/module` | same gate as `/api/comments/admin` (Cloudflare Access or `COMMENTS_ADMIN_PASSWORD`) |
| `newsletter` | `GET /api/newsletter/module` | same gate as `/api/newsletter/admin` |
| `newsletter-ui` | `GET {base}/api/module` | the worker's admin auth, like every `/api/` route |
| `auth` | `GET /api/auth/module` | an access token holding `auth:admin` |
| `core` | `GET /api/core/module` | `core`'s own sign-in, like every `/api/core/` route |

The answer is the manifest as JSON (`cache-control: no-store`); a refusal is
byte for byte what the module's admin endpoint answers. It needs no database.
The other templates have no authenticated admin surface to put it behind, so
they ship the file only: serving it publicly would make every site announce
which workers it runs, and the [module registry](#the-module-registry) reads
the file at build time.

A Pages Function cannot read files at runtime, and the site build copies only a
template's `functions/` tree, so a Pages template imports a generated copy of
its manifest — `functions/api/<name>/_module.ts` (the leading underscore keeps
it out of the route table). `workers/modules.test.ts` fails while a copy
differs from `module.json`; `UPDATE_MODULE_TWINS=1 node --test 'workers/*.test.ts'`
rewrites the copies. A standalone Worker imports `module.json` directly
(`import manifest from "../module.json" with { type: "json" }`); wrangler
bundles it at deploy time.

## The summary endpoint

An admin home page needs the same three things from every module, in one
shape: a few numbers, what needs a person, and what is wrong with the
configuration. A module that can answer that declares
`endpoints.summary` and serves it with `GET`.

```json
{
  "contract": 1,
  "generatedAt": "2026-11-13T10:00:00.000Z",
  "metrics": [
    { "key": "pending", "label": { "en": "Awaiting moderation", "pl": "Czekają na moderację", "…": "…" }, "value": 12 }
  ],
  "attention": [
    { "severity": "action", "text": { "en": "12 comments await moderation.", "pl": "12 komentarzy czeka na moderację.", "…": "…" }, "href": "/comments-admin.html" },
    { "severity": "warning", "text": { "en": "The oldest comment in the queue has been waiting for 9 days.", "…": "…" }, "href": "/comments-admin.html" }
  ],
  "problems": [
    { "code": "no_turnstile_secret", "text": { "en": "Neither COMMENTS_TURNSTILE_SECRET nor TURNSTILE_SECRET is set: every new comment is refused.", "…": "…" } }
  ],
  "series": { "label": { "en": "Sign-ups", "…": "…" }, "points": [{ "day": "2026-11-13", "value": 4 }] }
}
```

| Field | Rule |
|---|---|
| `contract` | Version of the summary format, currently `1` (independent of the manifest's). A reader refuses a newer one, like a manifest. |
| `generatedAt` | ISO 8601 in UTC, when the module computed the answer. |
| `metrics` | At most **4**: `key` (camelCase, `^[a-z][A-Za-z0-9]*$`), `label` (text), `value` (a number ≥ 0), optional `unit` (`percent` or `days`; absent for a count). Two modules that read the same data use the same key, label and value for the same number. |
| `attention` | At most **10**, most serious first: `severity`, `text` (a whole sentence), `href`. |
| `problems` | At most **10**: `code` (snake_case) and `text`. Names what is missing or unusable, never a secret's value. |
| `series` | Optional, one daily series for a small chart: `label` and at most **90** `points` `{day: "YYYY-MM-DD", value}`, oldest first, each day once. |

**Severity.** `action`: waits for a person's decision or work (comments to
moderate, a campaign the send engine paused, uncertain sends).
`warning`: something is going wrong or degrading (a stale queue, jobs that
ran out of retries). `info`: good to know, nothing to do.

**Links.** `href` is a path inside the module's own panel, relative to the
module's base, with the same grammar as a manifest `nav` path — an optional
`#fragment` for hash routing (`/#/campaigns?status=paused`), never a scheme,
host, `//`, `..` or query. A home page joins it to the module's base; a
summary can never send a person to another site. A module without a panel has
no attention items.

**Text.** Every text is a [text](#fields) in the five panel languages, at most
400 characters each, and a reader renders it as **plain text** — never as
HTML. Counts are written into the sentence in each language's plural form
(`Intl.PluralRules`) and number format.

**Size.** Besides the caps above, a whole answer is at most 32 KiB of UTF-8
JSON (`SUMMARY_MAX_BYTES`). Readers ignore fields they do not know.

**Aggregates only.** A summary holds counts and fixed sentences. No address,
name, comment text, campaign subject, token, IP, user agent or any other
per-person value — each template's tests seed recognisable personal data and
scan the serialized summary for it (and for any `@`).

**Auth.** The summary is behind the same gate as the module's manifest and
admin API, and every refusal is byte for byte what the admin endpoint answers.
In the `auth` mode it needs the module's read permission (`comments:read`,
`newsletter:read`, `auth:admin`); with the password or Cloudflare Access
whoever passes may read it.

### Which templates serve one

| Template | Route | Metrics | Attention | Problems | D1 statements |
|---|---|---|---|---|---|
| `comments` | `GET /api/comments/module/summary` | `pending`, `approved`, `spam` | `action` while comments await moderation; also `warning` once the oldest has waited 7 days (`STALE_DAYS`); both link to `/comments-admin.html` | `no_database`, `no_turnstile_secret`, `access_incomplete` (only one of `COMMENTS_ACCESS_TEAM` / `_AUD` set) | 1 |
| `newsletter` | `GET /api/newsletter/module/summary` | `confirmed`, `pending`, `unsubscribed` | none (no panel) | `no_database`, `no_turnstile_secret`, `mail_unusable` (double opt-in configured but the transport can never send — sign-ups are refused), `access_incomplete` | 1 |
| `auth` | `GET /api/auth/module/summary` | `accounts`, `pendingInvitations`, `liveSessions`, `missingFactor` | `action`: accounts without a second factor their roles need; `warning`: a signing key past the yearly rotation advice; `info`: open invitations — all linking into `/auth/admin/` | `mail_broken`, `mfa_key_short`, `mfa_roles_invalid`, `access_misconfigured` | a few, aggregates only |
| `newsletter-ui` | `GET {base}/api/module/summary` | `confirmed`, `pending`, `campaignsInProgress`, `suppressed` | `action`: campaigns paused by the send engine (`/#/campaigns?status=paused`), uncertain sends (`/#/`), pending subscribers never sent a confirmation (`/#/subscribers?status=pending`); `warning`: dead-lettered jobs (`/#/`) | `no_subscribers_table`, `no_transport`, `transport_unusable`, `no_site_url`, `no_sender`, `no_queue` — the dashboard's own warnings, in its own words | ≤ 3 |

`newsletter-ui`'s series is the sign-ups per day of the last 30 days. Without
the subscribers table it still answers: the campaign and suppression numbers
and the problem `no_subscribers_table`.

**`newsletter` and `newsletter-ui` together.** Both read the same
`subscribers` table, and both report `confirmed` and `pending` under the same
keys, with the same labels and the same values — `workers/modules.test.ts`
runs both on one database. When a site has both, a home page should show
**`newsletter-ui`'s** summary and may hide `newsletter`'s: it is a superset
(campaigns, suppressions, the series) and has a panel to link to.
`newsletter`'s summary is for a site that runs the sign-up form without the
panel.

**No summary: no gate.** The other templates have no authenticated admin
surface, and an unauthenticated summary would tell anyone which bindings and
secrets a site lacks — reconnaissance for an attacker. `web-push` and
`republish-trigger` have only a machine key (`PUSH_KEY`, `REPUBLISH_KEY`) held
by CI; a home page forwards the *person's* credential, never such a key, and
handing the key to people would also let them trigger notifications or
rebuilds. `auth` can now authenticate a person for them, but they have no
person-facing admin check yet, so their summaries are still to come:

| Template | What its summary would say, once it has a gate |
|---|---|
| `web-push` | subscriptions, time of the last notification (needs recording), missing VAPID keys or `PUSH_KEY` |
| `republish-trigger` | missing hook or key |
| `cookie-consent` | consents by category over recent days, when the log is on |
| `contact-form`, `conversions-proxy`, `dynamic-price`, `rate-limit`, `stripe-checkout` | configuration state only: which required secrets and bindings are set |

### What a summary costs on D1

Each summary is a fixed, small number of statements (the table above; the
first request of an isolate adds the template's schema check, and
`newsletter-ui`'s password mode reads one lockout row per request). That is
far inside the **50 queries per Worker invocation** of the Workers Free plan
(1 000 on Paid).

The budget that matters is **rows read**: D1 counts every row a statement
scans, and a count over a table scans it (an index makes the scan cheaper, not
shorter). The Free plan includes **5 million rows read per day**. Roughly, per
summary:

| Template | Rows read |
|---|---|
| `comments` | every comment (one pass over the status index) |
| `newsletter` | every subscriber |
| `newsletter-ui` | every subscriber twice (status counts, sign-up series), the pending ones again, every `campaign_sends` row, the suppressions |

So with 100 000 subscribers one `newsletter-ui` summary reads a few hundred
thousand rows — about 20 summaries a day on the Free plan, besides everything
else the site reads. A home page therefore asks for summaries when it is
opened or refreshed by hand, never on a timer, and a large list
belongs on Workers Paid.

### Adding a summary to your module

1. Copy [`workers/module-summary-kit.ts`](../workers/module-summary-kit.ts)
   into your worker unchanged (a Pages Function: as `functions/…/_summary-kit.ts`).
   It holds the types, the caps, `counted(phrase, n)` (a sentence with a count
   in five languages) and `summaryOf(parts)` (sorts attention by severity,
   applies the caps, stamps `generatedAt`).
2. Compute the numbers with one or two aggregate statements (`COUNT`,
   `GROUP BY`) — never by reading rows into the worker.
3. Serve it behind your admin check and declare `endpoints.summary`.
4. In the ssg repository, keep a `test/summary-sample.ts` in the template:
   run with the template's `test/resolve.mjs`, it drives the handler through
   the template's test fakes and prints the body. `workers/modules.test.ts`
   validates it with `readSummary` and checks every `href` against the
   panel's files; a template that declares a summary without serving it fails
   there. `workers/module-summary.test.ts` fails while a copy of the kit
   differs from the original.

## The module registry

The build knows the site's whole worker list; the registry hands that
knowledge to the `core` worker. It is written only when a worker in
`workers:` (or `worker:`) has a `module.json` with `"id": "core"`. **Without such
a worker the build writes nothing new** and its output is byte for byte what
it was before — the golden corpora pin that.

With `core`, the build reads the `module.json` of every worker it bundles and
of every `modules:` entry that has a `dir`, checks each with the Go reader
(`internal/modules`, the same rules and messages as `module-manifest.ts`), and
writes `functions/_ssg-modules.json` in the output. A worker without a
`module.json` is not a module and is simply not listed.

Modules the build cannot see — a standalone Worker deployed on its own — are
declared in the site configuration:

```yaml
modules:
  - id: newsletter-ui
    url: /newsletter-admin      # the module's base path on the same host
    dir: workers/newsletter-ui  # optional: read title/nav/permissions at build time
```

`url` is a path on the site's own host; a scheme, a host, `//`, `..`, a
backslash, a query or a fragment is a configuration error naming the file and
the key. Without `dir` the registry records the id and the base, and `core`
asks the module for its manifest at runtime.

### Where it is, and who can read it

A Pages Function cannot read static files, so the registry lives inside the
Functions tree and the core function imports it:

```ts
import registry from "../../_ssg-modules.json" with { type: "json" };
```

wrangler bundles it into the function at deploy time. The leading underscore
keeps it out of the route table, and `wrangler pages deploy` never uploads the
`functions/` directory as static assets, so the file is not publicly served
(checked with `wrangler pages functions build`: the import bundles and no
route is generated for the file). A worker that ships a file of the same name
is refused. Like the rest of `functions/`, a deploy target other than
Cloudflare Pages uploads it as an ordinary file.

### Shape

```json
{
  "format": 1,
  "modules": [
    {
      "id": "comments",
      "kind": "pages-function",
      "base": "/",
      "bundled": true,
      "contract": 1,
      "version": "1.0.0",
      "manifest": { "contract": 1, "id": "comments", "…": "the whole module.json" }
    },
    {
      "id": "shop",
      "kind": null,
      "base": "/shop",
      "bundled": false,
      "contract": null,
      "version": null,
      "manifest": null
    }
  ],
  "available": [
    { "id": "contact-form", "kind": "pages-function", "title": "Contact form" }
  ],
  "theme": null
}
```

| Field | Meaning |
|---|---|
| `format` | Version of this document. A reader refuses a newer one. |
| `modules` | Sorted by `id`. `base` is `/` for a Pages Function in this build and the configured `url` otherwise; `bundled` says which. `kind`, `contract`, `version` and `manifest` are `null` when the build has no manifest for the module (no `dir`, or a `dir` whose manifest is unusable). `manifest` is the full `module.json`, unknown fields included, keys sorted. |
| `available` | Templates this ssg binary ships that the site does not use: `id`, `kind`, English `title`, sorted by `id`. |
| `theme` | The panels' look computed from `admin_theme:`, or `null` when the site sets none — see below. |

No timestamps and no paths of the machine that built it: the same site gives
the same bytes.

### The panels' look (`theme`)

With `admin_theme:` in `.ssg.yaml` ([CONFIGURATION.md](CONFIGURATION.md#admin-panels-look))
the build validates it, derives the palette (`internal/admintheme`) and
writes the result here — and the colours and fonts again as `/core/theme.css`,
which the build writes over the empty file the `core` template ships. The
worker serves both as they are; it does no colour maths.

```json
"theme": {
  "name": "Anna's Kitchen",
  "logo": "/img/logo.svg", "logoDark": null,
  "fontDisplay": "\"Georgia\", \"Noto Serif\", \"Noto Serif Devanagari\", \"Nirmala UI\", serif",
  "fontUi": null,
  "fontFiles": [{ "family": "Brand Sans", "src": "/fonts/brand-sans.woff2", "format": "woff2", "weight": "400 700", "style": "normal" }],
  "main": "#1b3a4b", "accent": "#e07a5f", "derived": [],
  "light": { "primary": "#1b3a4b", "on-primary": "#ffffff", "…": "every themed token" },
  "dark": { "primary": "#698a9d", "…": "every themed token" },
  "adjustments": [
    { "scheme": "dark", "token": "primary", "requested": "#1b3a4b", "used": "#698a9d", "reason": "contrast", "against": "surface" }
  ],
  "checks": [{ "scheme": "light", "fg": "primary", "bg": "surface", "ratio": 11.97, "minimum": 4.5 }]
}
```

| Field | Meaning |
|---|---|
| `name`, `logo`, `logoDark` | The brand for the shared navigation (`/api/core/modules` → `brand`); `null` when unset. Logos are paths on the site's own host. |
| `fontDisplay`, `fontUi` | The font stacks as written to `theme.css` (`--font-display`, `--font-ui`), fallbacks included; `null` when unset. |
| `fontFiles` | The `@font-face` blocks in `theme.css`. |
| `main`, `accent`, `derived` | The two brand colours (lowercase `#rrggbb`); `derived` lists the one the build made from the other. `null` when no colour is set. |
| `light`, `dark` | Token name (without `--`) → colour, per scheme: `primary`, `on-primary`, `primary-soft`, `on-primary-soft`, `focus`, `c1`–`c5`, `nav-bg`, `on-nav`, `nav-muted`, `nav-hover`, `nav-control`, `nav-current` — exactly the names `workers/core/public/core/tokens.css` defines. Empty when no colour is set. |
| `adjustments` | Each brand colour that could not serve a role as given: `scheme` (`light`, `dark`, or `both` for the bar), `token`, `requested`, `used`, `reason` (`contrast` or `distinct`) and the token it was measured `against`. |
| `checks` | Every contrast pair the panels use that involves a themed token: `ratio` rounded down to two decimals, `minimum` 4.5 (text) or 3 (marks, edges, focus). |

The worker reads the field defensively (`functions/api/core/_theme.ts`): a
value outside the shape above becomes `null` or is left out. `/core/theme.css`
sets the light tokens on `:root`, the dark ones under `@media
(prefers-color-scheme: dark) { :root:not([data-theme="light"]) }` and under
`:root[data-theme="dark"]` — the selectors of `tokens.css` — so a panel loads
it **after** its own tokens and each rule wins by order. `check_admin_theme`
(`off`, `warn` default, `strict`) reports a logo or font file missing from the
output, a `tokens:` override that breaks a pair, and `admin_theme` without a
`core` worker.

### Checks (`check_modules`)

`check_modules: off | warn | strict` — `warn` by default, because findings
only exist on a site that uses modules, and there a module silently missing
from the navigation is what to avoid. `strict` (or a `--strict` build) fails
on any finding. Each names the file and the key:

| Finding | Effect on the registry |
|---|---|
| A bundled worker's `module.json` fails the schema, has a newer `contract` or is unreadable (`workers/x/module.json → manifest.panel: …`) | the worker is left out |
| A `modules:` entry's manifest fails the same way | kept with id and base, no manifest |
| `modules[i].dir` has no `module.json` | kept with id and base |
| `modules[i].id` differs from the manifest's `id` | kept with the declared id, no manifest |
| Two modules with the same `id` | the first in configuration order is kept |
| Two modules whose panels are the same page | both kept |
| Two `modules:` entries with the same or nested base paths | both kept |
| A bundled manifest with `kind: "worker"` | kept |
| `modules:` entries but no `core` worker — they have no effect | nothing is written |

`ssg new worker <template>` in a project that already has `workers/core`
prints one more line: how the new template joins the registry (listing it in
`workers:` for a Pages template, the `modules:` entry for a standalone Worker).

The registry is not part of `site-graph.json` or the `ssg mcp` site tools:
the graph is a public file, and the registry must not be.

## Making your own worker a module

1. Add `module.json` at the root of the worker's directory, following the
   fields above. Start with `contract: 1`, `version: "1.0.0"`, your directory
   name as `id`, five-language texts, `panel: null` and empty `nav`,
   `permissions` and `endpoints` if it has no admin side.
2. If it has an admin side, list one permission per level of trust (who may
   only look, who may change, who may do something irreversible or that
   reaches people) and add a `nav` entry per page worth a link.
3. Serve the manifest at `…/api/module` (or next to your admin route) behind
   the same check as the rest of your admin API, and declare that path as
   `endpoints.manifest`. A Pages Function imports a copy, as above.
   Optionally serve a summary at `…/api/module/summary` behind the same check
   and declare it as `endpoints.summary` (see
   [Adding a summary to your module](#adding-a-summary-to-your-module)).
4. Check it: from an ssg checkout, the reader validates any manifest —

   ```sh
   node -e 'import("./workers/module-manifest.ts").then(async ({ readManifest }) => {
     const fs = await import("node:fs");
     const r = readManifest(JSON.parse(fs.readFileSync(process.argv[1], "utf8")),
                            JSON.parse(fs.readFileSync("workers/module.schema.json", "utf8")));
     console.log(r.ok ? "ok" : r.errors.join("\n"));
   })' path/to/your-worker/module.json
   ```


## Tests

```sh
node --test 'workers/*.test.ts'          # reader + every bundled template's manifest and summary
go test ./internal/modules/              # the Go reader and the registry
```

`internal/modules/testdata/manifests/` holds manifests with the exact errors a
reader must report for each; `workers/module-fixtures.test.ts` holds the
TypeScript reader to them and `internal/modules/fixtures_test.go` the Go one,
so the two cannot drift apart. After an intended change of a message,
`UPDATE_MODULE_FIXTURES=1 node --test 'workers/module-fixtures.test.ts'`
rewrites the expected errors.

The cross-template test treats every directory under `workers/` that has a
`functions/` tree or a `wrangler.toml` as a template and fails when one has no
manifest or an invalid one, when `id` is not the directory name, when `kind`
does not match the layout, when a `panel`, `nav` or endpoint path is not served
by the template's code (a file under `public/`, a Pages Functions route, or a
GET operation in a standalone Worker's `openapi.yaml`), or when a text lacks a
language. Each template with a manifest endpoint also tests that route against
its admin endpoint.
