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

## 4. Moderate

Open `/comments-admin.html`, enter `COMMENTS_ADMIN_PASSWORD`, and approve, mark
spam, or delete. Approved comments appear in the widget on the next load.

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
| `COMMENTS_AKISMET_URL` | Akismet endpoint, paired with the key |

## Compliance notes

The `ip_hash` and `user_agent` columns exist to answer abuse reports and to
deduplicate, not to profile. Salt the hash (`COMMENTS_IP_SALT`) so it is not a
plain rainbow-table lookup, document the retention in your privacy policy, and
gate any avatar (Gravatar) load behind your cookie banner's preferences
category if you use one.
