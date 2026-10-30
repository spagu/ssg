# web-push worker

Browser notifications for new posts (#368): visitors subscribe with one button,
subscriptions live in Cloudflare D1, and a call after each deploy announces
whatever the site's feed gained since the last one. Scaffold it with
`ssg new worker web-push` and switch the site side on with `web_push: true`.

Web Push needs a server for two things a static site cannot do: keep the
subscriptions, and sign each message with a private key. This worker is that
server and nothing more — the encryption (RFC 8291) and the signing (VAPID,
RFC 8292) use WebCrypto, with no dependencies.

Deliberate choices:

- **Permission only on a click.** The prompt never appears on page load; the
  button stays hidden where the browser cannot do push at all.
- **No archive flood.** The first notify call only remembers the newest item.
  When the remembered item has left the feed (deleted, or the feed keeps only
  the latest few), nothing is sent and the marker moves on — guessing would
  re-announce old posts.
- **At most once.** The marker moves *before* sending, so a retried call after
  a partial failure does not notify anyone twice.
- **Known push services only.** A subscription endpoint is a URL the worker
  POSTs to later, so only the browsers' push services are accepted (Google,
  Mozilla, Apple, Microsoft). `PUSH_ENDPOINT_HOSTS` replaces the list.
- **Gone means gone.** A subscription its push service answers `404`/`410`
  for is deleted at once.
- **No IP stored**, the User-Agent only as a salted hash and only with a salt.
- **The public key lives in one place.** `push.js` asks `GET /api/push/key`, so
  rotating keys never needs a rebuild of the site.

## Files

```
workers/web-push/
├── schema.sql                           the D1 table
├── wrangler.snippet.toml                D1/KV bindings + [vars] stub for `ssg new wrangler`
├── functions/api/push/key.ts            GET the VAPID public key
├── functions/api/push/subscribe.ts      POST a PushSubscription
├── functions/api/push/unsubscribe.ts    POST {"endpoint"} to forget it
├── functions/api/push/notify.ts         POST (Bearer PUSH_KEY): announce new feed items
├── functions/api/push/_*.ts             shared modules (not routes): crypto, feed, store
├── public/sw.js                         the service worker: shows and opens notifications
├── public/push.js                       the button's behaviour
├── public/push-button.html              the button snippet (also a demo page)
├── scripts/vapid-keys.mjs               prints a key pair
└── test/                                node --test suite
```

`public/` is served from the site root: `/sw.js`, `/push.js`, and the demo at
`/push-button.html` (marked `noindex`).

## 1. Keys, database, KV

```sh
node workers/web-push/scripts/vapid-keys.mjs     # prints both keys
npx wrangler d1 create ssg-push
npx wrangler kv namespace create ssg-push
# paste both ids into workers/web-push/wrangler.snippet.toml (uncomment the blocks)
npx wrangler pages secret put PUSH_VAPID_PRIVATE_KEY
npx wrangler pages secret put PUSH_KEY            # any long random string
```

Put the public key in `[vars]` as `PUSH_VAPID_PUBLIC_KEY`, and set
`PUSH_SUBJECT` to a `mailto:` or `https:` contact — push services use it to
reach you about a misbehaving sender. Keep the pair: a new one invalidates every
subscription. The worker **creates its table on first use**; to apply it by hand,
`npx wrangler d1 execute ssg-push --file=workers/web-push/schema.sql --remote`.

## 2. Wire it into `.ssg.yaml`

```yaml
feed: true          # the worker announces /feed.xml by default
web_push: true      # ssg writes manifest.webmanifest and adds push.js to every page
workers:
  - name: web-push
    dir: workers/web-push
    routes_include: [/api/push/*]
```

With `web_push: true` ssg puts these in each page's `<head>` (with your base
path, if the site has one):

```html
<link rel="manifest" href="/manifest.webmanifest">
<script src="/push.js" defer data-endpoint="/api/push"></script>
```

A theme that adds the `push.js` script itself is left alone — ssg skips the
injection on any page that already mentions `push.js`.

## 3. Put the button on a page

The theme needs only the button, from `public/push-button.html`:

```html
<button type="button" data-push-subscribe hidden
        data-label-on="Notify me about new posts"
        data-label-off="Stop notifications">Notify me about new posts</button>
```

It is unhidden when the browser supports push, asks for permission on the
first click, and turns into "Stop notifications" once subscribed. `aria-pressed`
tells assistive technology which state it is in.

**iPhone and iPad** deliver web push only to a site added to the home screen
(Safari 16.4 or later) — that is what the manifest is for. In plain Safari the
button stays hidden; tell readers to use *Share → Add to Home Screen*.

## 4. Announce after each deploy

`notify` is not on a schedule — Pages Functions have none, and right after a
deploy is exactly when there is something new. Add a step after the deploy:

```yaml
- name: Announce new posts
  run: curl -fsS -X POST -H "Authorization: Bearer ${{ secrets.PUSH_KEY }}" https://example.com/api/push/notify
```

It answers `{"sent":…, "failed":…, "removed":…, "items":[{"title","url"}]}`, or
`{"primed":"<id>"}` on the very first call. Each item becomes one notification
— title, the summary shortened to about 140 characters, and the post's URL —
sent oldest first so the newest ends up on top. One call makes
*subscribers × items* requests to push services, which counts against your
plan's subrequest limit per invocation; `PUSH_MAX_ITEMS` caps the items.

## The API

| Endpoint | Does |
|---|---|
| `GET /api/push/key` | `{"publicKey":"…"}`, cached 5 minutes |
| `POST /api/push/subscribe` | stores `PushSubscription.toJSON()`; `201`, or `422` `invalid_subscription` |
| `POST /api/push/unsubscribe` | `{"endpoint":"…"}`; always `200` `{"ok":true}`, stored or not |
| `POST /api/push/notify` | `Authorization: Bearer <PUSH_KEY>`; `401` without it, `503` when not configured, `502` when the feed cannot be read |

`subscribe` and `unsubscribe` refuse a browser request from another origin
(`403`).

## Config / secrets

| Secret | Purpose |
|---|---|
| `PUSH_VAPID_PRIVATE_KEY` | signs every message (required) |
| `PUSH_KEY` | Bearer token for `notify` (required) |
| `PUSH_HASH_SALT` | salt for the User-Agent hash; unset ⇒ nothing stored |

| Var | Effect |
|---|---|
| `PUSH_VAPID_PUBLIC_KEY` | the public key (required) |
| `PUSH_SUBJECT` | VAPID contact, `mailto:` or `https:` (required to send) |
| `PUSH_FEED_URL` | the feed to announce; default `/feed.xml`. Atom, RSS or JSON Feed |
| `PUSH_MAX_ITEMS` | most items per notify call; default `3` |
| `PUSH_TTL` | seconds a push service keeps an undelivered message; default `86400` |
| `PUSH_ENDPOINT_HOSTS` | accepted push services as host suffixes, comma-separated |

| Binding | |
|---|---|
| `PUSH_DB` | D1: the subscriptions |
| `PUSH_KV` | KV: the last item announced (`last-item`) |

## Privacy notes

Stored per subscriber: the push endpoint, its two keys, timestamps and — only
with a salt — a salted hash of the User-Agent. The endpoint is an address at the
browser vendor's push service, not the visitor's; it identifies a browser
profile, so treat it as personal data. Say so in your privacy policy. A visitor
who clicks *Stop notifications*, or revokes the permission in the browser (the
next send gets `410`), is deleted, not flagged.

## Tests

From the repository root, with Node 22.18 or later (TypeScript runs natively):

```sh
node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
```

`resolve.mjs` lets Node resolve the extensionless imports Pages bundles. The
suite checks the encryption against the RFC 8291 worked example byte for byte,
verifies the VAPID signature, and runs every route against in-memory D1, KV and
push services.
