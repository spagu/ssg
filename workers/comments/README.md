# comments worker

Comments for an SSG site — for blog posts especially — stored in Cloudflare D1,
protected by Turnstile, held for moderation. Scaffold it with
`ssg new worker comments`.

Deliberate choices:

- **No accounts.** A comment is a name, a required email (kept only in D1 for the
  duplicate check + avatar hash — never returned by the API or shown on the page),
  and a body. No login, no passwords, no OAuth.
- **Threaded replies.** Readers can reply to a top-level comment (one level deep);
  a reply is moderated like any other comment. Nothing to configure.
- **Compliance, not tracking.** For "who said what" the row keeps a **salted
  hash** of the IP plus the user-agent — the raw IP (PII) is never stored.
- **Moderated by default.** New comments are `pending` and invisible until an
  admin approves them, so nothing a visitor writes appears unreviewed.
- **Spam-filtered.** Turnstile on submit, plus a heuristic score; drop in an
  Akismet key for real scoring.
- **JS by default, static optional.** The widget fetches approved comments at
  read time. To bake them into the HTML instead, fetch `/api/comments?url=…` in
  a build step and render server-side — the API is the same.

## Files

```
workers/comments/
├── schema.sql                              the D1 table + indexes
├── wrangler.snippet.toml                   D1 binding + [vars] stub for `ssg new wrangler`
├── functions/api/comments/index.ts         GET (list approved) + POST (submit)
├── functions/api/comments/admin.ts         GET queue + POST approve/spam/delete/purge
├── functions/api/comments/admin/whoami.ts  GET the admin mode, account and permissions (the panel's first call)
├── functions/api/comments/import.ts        POST a JSON batch of existing comments
├── functions/api/comments/_*.ts            shared helpers: lib, access, admin-auth, events, mail, schema (not routes)
├── functions/api/comments/_session-verifier.ts  copy of workers/auth/portable/session-verifier.ts (never edit)
├── public/comments.js                      the reader widget
├── public/comments.css                     its styles
├── public/comments-admin.html              the moderation panel
├── public/comments-admin.js                its script
├── public/comments-admin.css               its styles
├── public/comments-admin-tokens.css        their colours and fonts (core's token names)
└── tools/wordpress-comments.sh             WordPress REST → import JSON
```

`public/` is served from the site root: the widget at `/comments.js`, the panel
at `/comments-admin.html`.

## 1. Create the database

```sh
npx wrangler d1 create ssg-comments
# paste the id into workers/comments/wrangler.snippet.toml (uncomment the block)
```

That's the only step — the worker **creates its table on first use**, so once the
`COMMENTS_DB` binding is in place it just works, with no separate schema command.
(To seed the table by hand instead, apply `schema.sql`:
`npx wrangler d1 execute ssg-comments --file=workers/comments/schema.sql --remote`.)

`ssg new wrangler` folds the D1 binding stub into your `wrangler.toml`.

## 2. Wire it into `.ssg.yaml`

```yaml
workers:
  - name: comments
    dir: workers/comments
    routes_include: [/api/comments, /api/comments/*]
```

## 3. Mount the widget on posts

In your post template:

```html
<div id="ssg-comments" data-url="{{ .Post.GetURL }}"></div>
<script id="ssg-comments-config" type="application/json">
  { "turnstileSiteKey": "0xYOUR_SITE_KEY", "api": "/api/comments", "order": "newest" }
</script>
<script src="/comments.js" defer></script>
```

The site key is public; get it (and the secret) from the Cloudflare Turnstile
dashboard.

**Caching:** `comments.js`/`comments.css` are served from the site root and
updated in place, so don't let them inherit a long immutable asset cache or a
browser will keep a stale copy after a deploy. Serve them revalidating with a
`headers:` override in your config:

```yaml
headers:
  /comments.js:  { Cache-Control: "public, max-age=0, must-revalidate" }
  /comments.css: { Cache-Control: "public, max-age=0, must-revalidate" }
```

## Localisation

The widget's UI strings (`Leave a comment`, `Post comment`, `No comments yet`, …)
are translated and follow the page: it reads `<html lang>` and shows English,
Polish, German or French accordingly — no config needed. ssgtheme already sets
`<html lang>` from the post's language, so a Polish post gets a Polish form.

To force a language regardless of the page, or to override a single string, add
to the config JSON:

```json
{ "turnstileSiteKey": "0x...", "api": "/api/comments",
  "defaultLang": "pl",
  "i18n": { "en": { "submit": "Send it" } } }
```

`defaultLang` is used when the page's `<html lang>` is one the widget doesn't
ship; `i18n.<lang>` overrides individual keys or adds a whole new language.

## 4. Moderate

Open `/comments-admin.html` and approve, mark spam, or delete. Approved
comments appear in the widget on the next load. How you sign in depends on the
mode below: with the shared password the panel asks for
`COMMENTS_ADMIN_PASSWORD`; behind Cloudflare Access or with `auth` it asks
nothing.

**With the [`core`](../core/README.md) worker** on the site, a shared admin
bar appears at the top of the page — Admin home and a link to every panel the
account may open, Comments marked as current — and the site's own look
(`/core/theme.css`) restyles the page. Without `core` nothing changes: both
files answer 404 and the page looks exactly as before. The page carries its
own strict CSP (`<meta http-equiv>`: nothing inline, nothing from another
origin — `core`'s files are on the same host, so `'self'` allows them). Its
styles load in this order: `/comments-admin-tokens.css` (every colour and font
as a custom property, named as in `core`'s tokens), `/core/theme.css`,
`/comments-admin.css`. Local preview, with the real handlers and invented
comments: `node --import ./workers/comments/test/resolve.mjs workers/comments/test/dev-server.mjs`
(password `demo`; `DEMO_CORE=1` adds the bar, `DEMO_CORE=theme` a demo theme).

### Who can moderate

Three modes. The worker uses the first one that is configured, in this order:

| Mode | Turned on by | Which to choose |
|---|---|---|
| **Cloudflare Access** | `COMMENTS_ACCESS_TEAM` + `COMMENTS_ACCESS_AUD` | you already run Cloudflare Zero Trust: moderators sign in through your identity provider at the edge |
| **`auth`** | any `COMMENTS_AUTH_*` variable | several people, or different permissions — a moderator who may approve but not delete, a reader who may only look; see [workers/auth](../auth/README.md) |
| **Shared password** | `COMMENTS_ADMIN_PASSWORD` | one person: the simplest — set one secret and the panel works |

Nothing configured: every admin request answers `503 {"error": "moderation not configured"}`.
Access and the password allow every operation; with `auth` each operation
needs its permission:

| Operation | Permission |
|---|---|
| `GET /api/comments/admin` (the queue), `GET /api/comments/module`, `GET /api/comments/module/summary` | `comments:read` |
| `POST /api/comments/admin` `approve`, `spam` | `comments:moderate` |
| `POST /api/comments/admin` `delete`, `purge` | `comments:delete` |
| `POST /api/comments/import` | `comments:import` |

`comments:*` or `*` in an account's roles grants all four. The table lives in
code as `OPERATIONS` (`functions/api/comments/_admin-auth.ts`), and
`test/permissions.test.ts` is generated from it.

#### Shared password

By default the panel and its API use HTTP Basic with `COMMENTS_ADMIN_PASSWORD`
(`wrangler pages secret put COMMENTS_ADMIN_PASSWORD`). The panel asks for it;
`curl -u :<password>` works from scripts. It is a fully supported mode: one
person, one secret, nothing else to run. When more people moderate, or some
should only approve and not delete, the next step is the `auth` mode below
([which mode to choose](../../docs/WORKERS.md#admin-sign-in-which-mode-to-choose)).

**Known limitation: no brute-force limit.** In this mode the worker does not
count failed passwords, so nothing slows down someone guessing it. Use a long
random password (`openssl rand -base64 24`), and add a limit in front: the
[`rate-limit`](../rate-limit/README.md) template (its middleware covers
`/api/comments/admin` and `/api/comments/import` like every other Function),
Cloudflare Access, or `auth`, whose sign-in locks an address and an IP after
repeated failures.

#### Cloudflare Access

Create an Access application covering `/comments-admin.html`,
`/api/comments/admin*` and `/api/comments/module*`, then set two vars:

```toml
[vars]
COMMENTS_ACCESS_TEAM = "myteam"            # or "myteam.cloudflareaccess.com"
COMMENTS_ACCESS_AUD  = "<application-aud>"  # Access → your app → Overview → Application Audience (AUD) Tag
```

With those set, the worker ignores the password and instead verifies the signed
JWT Access forwards (`Cf-Access-Jwt-Assertion`) against your team's public keys —
checking the signature, that the audience is *this* application, the issuer is
your team, and the token hasn't expired. The moderator signs in through your IdP;
the panel detects the Access session and skips its own password prompt. There is
no shared secret to store or rotate.

#### `auth` — accounts and permissions

Deploy the [`auth` worker](../auth/README.md) on the same host, give the people
who moderate roles holding the permissions above, and set:

```toml
[vars]
COMMENTS_AUTH_ISSUER     = "https://example.com/api/auth"  # auth's AUTH_ISSUER (default https://<host>/api/auth)
COMMENTS_AUTH_LOGIN_URL  = "/auth/login"                   # where a moderator without a session is sent
COMMENTS_AUTH_PUBLIC_KEY = '{"kty":"EC","crv":"P-256",…}'  # auth's public key: the AUTH_PUBLIC_KEY= line of scripts/auth-keys.mjs, or /api/auth/jwks
# COMMENTS_AUTH_JWKS_URL = "/api/auth/jwks"                # instead of the public key: fetched and cached (set exactly one of the two)
# COMMENTS_AUTH_AUDIENCE = "example.com"                   # only if auth's AUTH_AUDIENCE is not the site host
```

The worker checks the session cookie (`__Host-ssg-auth`) itself with the
public key — it never asks `auth` per request
([contract](../auth/docs/CONTRACT.md)). The panel shows who is signed in and
only the actions the account may perform (the API refuses the others with
`403 {"error": "forbidden", "permission": "comments:…"}`, and the panel names
the missing permission); without a session it sends the browser to
`COMMENTS_AUTH_LOGIN_URL`, which returns to the panel afterwards. A revoked
role or a sign-out elsewhere reaches this worker when the access token expires,
10 minutes 30 seconds by default.

Rules worth knowing:

- **A half-configured set is refused, loudly.** Some `COMMENTS_AUTH_*` set but
  not `ISSUER`, `LOGIN_URL` and exactly one of `PUBLIC_KEY` / `JWKS_URL` →
  every admin request is `503 {"error": "not_configured", "detail": "…"}`
  naming what is missing. It never falls back to the password.
- **While `auth` is on, the password is not accepted.** Accepting it would let
  anyone who knows it past every permission, and nothing would record who it
  was. The secret stays stored, unused; delete it
  (`wrangler pages secret delete COMMENTS_ADMIN_PASSWORD`) once `auth` works,
  so removing the `COMMENTS_AUTH_*` vars later does not quietly bring it back.
  Access, when set, still comes first.
- **Writes need the panel's header.** In `auth` mode the session rides in a
  cookie the browser attaches by itself, so `POST` requests must carry
  `x-ssg-auth: 1` and no foreign `Origin` / `Sec-Fetch-Site` (the `auth`
  worker's own CSRF rule) — otherwise `403 {"error": "csrf_check_failed"}` or
  `{"error": "cross_origin"}`. The panel sends it; a machine token sent as
  `Authorization: Bearer …` (an `auth` service token) needs no header. Access
  mode applies the same rule (security review SR-10): Access lets through any
  request that carries its `CF_Authorization` cookie, which the browser
  attaches by itself. The panel sends the header in every mode; the password
  mode needs none (only the panel's script adds the password).
- `GET /api/comments/admin/whoami` answers `{mode, user, permissions}` in every
  mode (`user` is `null` with the password); in `auth` mode its `401` also
  carries `{"mode": "auth", "login": "<login URL>?next=/comments-admin.html"}`.

#### Who did what

Every approve, spam, delete, purge and import that changes something adds a
row to `comment_events`: the action, the comment id (none for a purge or an
import, whose `detail` holds the row count), the `actor` — the `auth` account
id, the Access email (or `access` when the token has none), or `password` —
and the `mode`. The table is created on first use (and is in `schema.sql`):

```sh
npx wrangler d1 execute ssg-comments --remote \
  --command "SELECT created_at, action, comment_id, actor, mode FROM comment_events ORDER BY created_at DESC LIMIT 50"
```

## 5. Import existing comments

Migrating from another comment system or a spreadsheet? Convert its export to
this **normalised JSON** — an array of comments — and post it once:

```json
[
  { "url": "/blog/hello/", "author": "Ada", "email": "ada@example.com",
    "body": "First!", "created_at": "2021-05-01T10:00:00Z" },
  { "url": "/blog/hello/", "author": "Bo", "body": "Nice post" }
]
```

Only `url`, `author` and `body` are required; `email` feeds the avatar hash,
`created_at` defaults to now, and `status` defaults to `approved` (imported
comments are already-vetted — pass `"status": "pending"` per item, or
`defaultStatus` for the batch, to re-moderate).

Easiest: sign in to `/comments-admin.html`, open **Import comments**, choose the
`.json` file (or paste it), pick a default status, and click Import.

Or via the API (admin Basic auth, same password as moderation):

```sh
curl -u :$COMMENTS_ADMIN_PASSWORD \
  -H 'content-type: application/json' \
  --data @comments.json \
  https://your-site/api/comments/import
# {"ok":true,"total":2,"imported":2,"duplicate":0,"invalid":0}
```

The import is **idempotent** — each row's id is a hash of its content, inserted
with `INSERT OR IGNORE`, so re-running the same file adds nothing new
(`duplicate` counts the skips). Up to 1000 items per request; chunk larger
exports. Rows missing `url`/`author`/`body` are counted in `invalid` and skipped
rather than failing the whole batch.

### From WordPress

`tools/wordpress-comments.sh` (bash + `curl` + `jq`, no plugin) pulls comments
straight from the old site's public REST API and converts them to the import
JSON above. Each comment's URL is built from its post's slug.

```sh
# convert to JSON…
./tools/wordpress-comments.sh https://old-blog.example.com > comments.json
curl -u :"$COMMENTS_ADMIN_PASSWORD" -H 'content-type: application/json' \
     --data @comments.json https://your-site/api/comments/import

# …or fetch, convert AND post in one go (chunked):
./tools/wordpress-comments.sh https://old-blog.example.com \
    --post https://your-site/api/comments/import --password "$COMMENTS_ADMIN_PASSWORD"
```

Match the URL to your permalink pattern with `URL_TEMPLATE` (default
`/blog/{slug}/`):

```sh
URL_TEMPLATE='/{slug}/' ./tools/wordpress-comments.sh https://old-blog.example.com
```

Caveats of the REST API path: it returns **approved** comments only and does
**not** expose commenter emails (so no Gravatar). If you need pending/spam
comments or the emails, export the **WXR** file instead (WordPress → Tools →
Export → *All content*) — it nests each post's comments under the post, so a
small converter can map them the same way; the REST script covers the common
"keep the approved history" case without a database or plugin.

## Config / secrets

`wrangler pages secret put <NAME>`:

| Secret | Purpose |
|---|---|
| `COMMENTS_TURNSTILE_SECRET` | verifies the submit token (required; falls back to the shared `TURNSTILE_SECRET` when unset) |
| `COMMENTS_ADMIN_PASSWORD` | moderation panel password (the shared-password mode — see *Who can moderate*) |
| `COMMENTS_IP_SALT` | salt for the stored IP hash |
| `COMMENTS_AKISMET_KEY` | optional — enables Akismet spam scoring |
| `COMMENTS_MAIL_KEY` | optional — bearer token for the email API (email-on-comment) |

Prefer `COMMENTS_TURNSTILE_SECRET` over the shared `TURNSTILE_SECRET`: every
worker in one Pages project reads the same environment, so an unprefixed secret
is seen by all of them. The fallback keeps projects configured before the prefix
existed working unchanged — see *Secrets* in
[docs/WORKERS.md](https://github.com/spagu/ssg/blob/main/docs/WORKERS.md#secrets).

`[vars]` (or `wrangler.toml`):

| Var | Effect |
|---|---|
| `COMMENTS_ORDER` | `newest` (default) or `oldest` |
| `COMMENTS_ACCESS_TEAM`, `COMMENTS_ACCESS_AUD` | Cloudflare Access mode (see *Who can moderate*) |
| `COMMENTS_AUTH_ISSUER`, `COMMENTS_AUTH_LOGIN_URL`, `COMMENTS_AUTH_PUBLIC_KEY` or `COMMENTS_AUTH_JWKS_URL`, `COMMENTS_AUTH_AUDIENCE` | `auth` mode (see *Who can moderate*) |
| `COMMENTS_CLOSE_AFTER_DAYS` | auto-close a thread `N` days after its last activity (`0` = never) |
| `COMMENTS_RETENTION_DAYS` | delete spam and pending comments older than `N` days (`0` = keep). Approved comments are never deleted. The purge runs after new comments, at most hourly; `POST /api/comments/admin {"action":"purge"}` runs it now, e.g. from a cron |
| `COMMENTS_AKISMET_URL` | Akismet endpoint, paired with the key |
| `COMMENTS_MAIL_URL` | email API endpoint accepting `{from,to,subject,text}` (e.g. Resend); enables email-on-comment |
| `COMMENTS_MAIL_FROM` | sender address for the notice |
| `COMMENTS_MAIL_TO` | comma-separated recipient(s) |
| `COMMENTS_MAIL_SUBJECT` | optional subject prefix (default "New comment awaiting review") |

### Email on a new comment

Set `COMMENTS_MAIL_URL`, `COMMENTS_MAIL_FROM` and `COMMENTS_MAIL_TO` (plus
`COMMENTS_MAIL_KEY` if the provider needs a bearer token) and the worker emails a
moderation notice whenever a **non-spam** comment lands — spam is filtered
silently and never mailed. The request is a JSON `POST` of
`{from, to, subject, text}`, which fits providers like Resend or your own relay;
it runs in the background (`waitUntil`), so it never delays the visitor's
response, and a gateway error is logged, never fatal. Unset ⇒ no email is sent.

### Auto-closing old threads

Set `COMMENTS_CLOSE_AFTER_DAYS` (e.g. `30`) and a thread stops accepting new
comments once that many days have passed **since its last activity** — the
newest comment, or the post's publish date while it has no comments yet. So a
lively discussion stays open as long as people keep replying, and a post nobody
has touched for a month locks itself. The widget hides the form and shows
"Comments are closed for this post." on a closed thread (`GET` returns
`"closed": true`); existing comments stay visible. The publish date comes from
the theme (ssgtheme renders `data-published` on the widget), so empty old posts
close correctly too.

## Wiring keys from CI (GitHub Actions)

To keep keys out of the repo, feed them from **GitHub Actions secrets** at deploy
time. There are three kinds of value, and they go to three different places —
not all of them are "secrets":

| Value | Kind | Where it goes | How |
|---|---|---|---|
| Turnstile **site key** | public (ships in the page) | into the build config | inject at build (see below) |
| Turnstile **secret**, admin password, IP salt | secret | the Pages **project** | `wrangler pages secret put` (persists) |
| D1 database | a **binding**, not a secret | the Pages **project** | bind once (dashboard/API) + apply the schema |

The SSG documentation site's own workflow (`.github/workflows/docs-site.yml`)
does this and is a working example:

1. **Site key** — a step rewrites `turnstileSiteKey` in the config from a GitHub
   secret before building; if the secret is unset, the committed demo/test key
   stays, so the build never breaks:

   ```yaml
   - name: Inject Turnstile site key
     env:
       TURNSTILE_SITE_KEY: ${{ secrets.TURNSTILE_SITE_KEY }}
     run: |
       [ -n "$TURNSTILE_SITE_KEY" ] &&
         sed -i "s|turnstileSiteKey: \".*\"|turnstileSiteKey: \"$TURNSTILE_SITE_KEY\"|" docs-site.yaml
   ```

2. **Server secrets** — a step pushes them onto the Pages project. Pages secrets
   persist across deploys, so this just re-asserts them (and skips any that
   aren't set):

   ```yaml
   - name: Sync secrets to the Pages project
     env:
       TURNSTILE_SECRET: ${{ secrets.TURNSTILE_SECRET_KEY }}
     run: printf '%s' "$TURNSTILE_SECRET" |
       npx wrangler pages secret put COMMENTS_TURNSTILE_SECRET --project-name "$PROJECT"
   ```

   (The docs site's workflow still pushes the shared `TURNSTILE_SECRET`, which
   the worker reads as a fallback. The prefixed name is what keeps that secret
   away from the other workers in the same project.)

   Needs `CLOUDFLARE_API_TOKEN` + `CLOUDFLARE_ACCOUNT_ID` in the job env.

3. **D1 is not a secret.** A database binding is a project setting, so **nothing
   goes into GitHub secrets for D1**. Set it up once:

   ```sh
   npx wrangler d1 create ssg-comments          # note the database_id it prints
   npx wrangler d1 execute ssg-comments --remote --file=workers/comments/schema.sql
   ```

   Then bind it as `COMMENTS_DB` on the Pages project — either in the dashboard
   (Workers & Pages → *project* → Settings → Functions → **D1 database bindings**
   → variable `COMMENTS_DB` → your database), or via the Pages API in CI (the
   only datum you'd add to GitHub is the non-secret **database_id**, best kept as
   a repository *Variable*). The binding persists across deploys.

## Compliance notes

The `ip_hash` and `user_agent` columns exist to answer abuse reports and to
deduplicate, not to profile. Salt the hash (`COMMENTS_IP_SALT`) so it is not a
plain rainbow-table lookup, document the retention in your privacy policy, and
gate any avatar (Gravatar) load behind your cookie banner's preferences
category if you use one.

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, its panel and navigation, the permissions its admin surface distinguishes, and how to switch it off. It is a description only — no secrets, no configuration.

| Permission | Allows |
|---|---|
| `comments:read` | View the comments in the moderation queue: pending, spam and approved. |
| `comments:moderate` | Approve a comment or mark it as spam. |
| `comments:delete` | Delete a comment for good, and run the purge of old spam and pending comments now. |
| `comments:import` | Import comments in bulk from another system, without the spam check. |

With the password or Cloudflare Access whoever passes may do everything; in `auth` mode each operation needs its permission (*Who can moderate*).

It also serves the manifest at `GET /api/comments/module`, behind the same gate as the moderation endpoints (Cloudflare Access, `auth` with `comments:read`, or `COMMENTS_ADMIN_PASSWORD`); a refusal is exactly the one `/api/comments/admin` gives. The route imports `functions/api/comments/_module.ts`, a generated copy of `module.json` (Pages Functions cannot read files at runtime) — edit `module.json` and keep the copy identical.

### Summary

`GET /api/comments/module/summary` answers the module summary (format in [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md#the-summary-endpoint)) that an admin home page shows: the counts awaiting moderation, approved and held as spam; an `action` item while comments wait, plus a `warning` once the oldest has waited 7 days, both linking to `/comments-admin.html`; and configuration problems — `no_database`, `no_turnstile_secret` (every new comment is refused), `access_incomplete` (only one of `COMMENTS_ACCESS_TEAM` / `COMMENTS_ACCESS_AUD` set). Texts come in en, pl, hi, de and fr. It is one aggregate statement over the status index — never an author, address or comment text — behind the same gate as moderation, refusing exactly as `/api/comments/admin` does. With Cloudflare Access, let the Access application cover `/api/comments/module*` as well, so Access sends its token to the manifest and summary routes too.

## Tests

```sh
node --import ./workers/comments/test/resolve.mjs --test 'workers/comments/test/*.test.ts'
```

`test/compat-admin.test.ts` pins every answer of `/api/comments/admin` and
`/api/comments/import` (status, headers, body, rows left behind) in the
password and Cloudflare Access modes, as they were before the manifest route
was added; later changes to this worker must keep it passing unedited.
`test/admin-modes.test.ts` covers the three modes side by side (half-configured
`auth`, `auth` with the password also set, a JWKS URL, `whoami`);
`test/permissions.test.ts` is generated from `OPERATIONS` (no session, a
session without the permission, with it — for every operation);
`test/csrf-actors.test.ts` the CSRF rule and `comment_events`;
`test/auth-copies.test.ts` keeps `_session-verifier.ts` and
`test/auth-copy-guard.ts` byte-identical to `workers/auth/portable/` and, next
to `workers/auth`, runs the verifier's own suites against this copy;
`test/auth-tokens.ts` mints test sessions with a key generated per run;
`test/lib.test.ts` and `test/access.test.ts` the helpers.
`test/module.test.ts` checks the manifest route against the admin endpoint,
`test/summary.test.ts` the summary (auth, severities, problems, no personal
data, one statement over 100 000 comments); `test/summary-sample.ts` is what
the cross-template test `workers/modules.test.ts` validates.
`test/panel-page.test.ts` checks the moderation page as files (no inline
style or script, its CSP, every file it names, the shared bar and theme
adopted in order, colours only as tokens); `test/dev-server.mjs` is the local
preview (not a test).
Node ≥ 23.6 runs the TypeScript directly; `test/resolve.mjs` adds the two
things esbuild does and Node does not (extensionless imports, interfaces
imported as values).
