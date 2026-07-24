# comments worker

Comments for an SSG site — for blog posts especially — stored in Cloudflare D1,
protected by Turnstile, held for moderation. Scaffold it with
`ssg new worker comments`.

Deliberate choices:

- **No accounts.** A comment is a name, an optional email (used only for the
  avatar hash), and a body. No login, no passwords, no OAuth.
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
├── functions/api/comments/index.ts         GET (list approved) + POST (submit)
├── functions/api/comments/admin.ts         GET queue + POST approve/spam/delete
├── functions/api/comments/_lib.ts          shared helpers (not a route)
├── public/comments.js                      the reader widget
├── public/comments.css                      its styles
└── public/comments-admin.html              the moderation panel
```

`public/` is served from the site root: the widget at `/comments.js`, the panel
at `/comments-admin.html`.

## 1. Create the database

```sh
npx wrangler d1 create ssg-comments
# paste the id into workers/comments/wrangler.snippet.toml (uncomment the block)
npx wrangler d1 execute ssg-comments --file=workers/comments/schema.sql --remote
```

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

Open `/comments-admin.html`, enter `COMMENTS_ADMIN_PASSWORD`, and approve, mark
spam, or delete. Approved comments appear in the widget on the next load.

### Moderation auth: password or Cloudflare Access

By default the panel and its API use HTTP Basic with `COMMENTS_ADMIN_PASSWORD`.

For a team — or to avoid managing a shared password — put the moderation surface
behind **Cloudflare Access** instead. Create an Access application covering
`/comments-admin.html` and `/api/comments/admin*`, then set two vars:

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

## 5. Import existing comments

Migrating from Disqus, WordPress, Commento or a spreadsheet? Convert the export
to this **normalised JSON** — an array of comments — and post it once:

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

## Config / secrets

`wrangler pages secret put <NAME>`:

| Secret | Purpose |
|---|---|
| `TURNSTILE_SECRET` | verifies the submit token (required) |
| `COMMENTS_ADMIN_PASSWORD` | moderation panel password (required to moderate) |
| `COMMENTS_IP_SALT` | salt for the stored IP hash |
| `COMMENTS_AKISMET_KEY` | optional — enables Akismet spam scoring |

`[vars]` (or `wrangler.toml`):

| Var | Effect |
|---|---|
| `COMMENTS_ORDER` | `newest` (default) or `oldest` |
| `COMMENTS_CLOSE_AFTER_DAYS` | auto-close a thread `N` days after its last activity (`0` = never) |
| `COMMENTS_AKISMET_URL` | Akismet endpoint, paired with the key |

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

## Compliance notes

The `ip_hash` and `user_agent` columns exist to answer abuse reports and to
deduplicate, not to profile. Salt the hash (`COMMENTS_IP_SALT`) so it is not a
plain rainbow-table lookup, document the retention in your privacy policy, and
gate any avatar (Gravatar) load behind your cookie banner's preferences
category if you use one.
