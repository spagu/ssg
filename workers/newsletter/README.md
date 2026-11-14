# newsletter worker

A newsletter or waitlist sign-up for an SSG site: addresses in Cloudflare D1,
Turnstile on the form, consent recorded with its wording, one-click unsubscribe
(RFC 8058), optional double opt-in. Scaffold it with `ssg new worker newsletter`.

It collects the list; sending the newsletter is the job of its companion,
[`newsletter-ui`](../newsletter-ui/README.md) — a standalone Worker on the same
D1 database with a panel, campaigns, a send engine and bounce handling
(`ssg new worker newsletter-ui`). Without it, export the confirmed
subscribers as CSV and mail them with whatever you already use — each row
carries the unsubscribe URL that mailer needs.

Deliberate choices:

- **No enumeration.** Every accepted sign-up gets the same answer — new address,
  pending, confirmed or previously unsubscribed — so the form cannot be used to
  find out who is on the list.
- **Consent with its words.** The consent box is required, and the record keeps
  the exact text the visitor agreed to and when (GDPR Art. 7(1): you must be
  able to *demonstrate* consent, not just claim it).
- **Unsubscribing never needs a page view — and never happens by accident.**
  Mailbox providers POST the one-click request; a reader following the link gets
  a page with one button. A plain GET changes nothing, because mail scanners
  fetch every link in a message.
- **No email in any URL.** Confirm and unsubscribe links carry a random 43-
  character token stored per subscriber.
- **IP and User-Agent only as salted hashes**, and only when a salt is set.
- **Works without JavaScript.** The form is a plain HTML form; `newsletter.js`
  only enhances it.

## Files

```
workers/newsletter/
├── schema.sql                                 the D1 table + indexes
├── wrangler.snippet.toml                      D1 binding + [vars] stub for `ssg new wrangler`
├── functions/api/newsletter/index.ts          POST sign-up
├── functions/api/newsletter/confirm.ts        GET page + POST confirm (double opt-in)
├── functions/api/newsletter/unsubscribe.ts    GET page + POST unsubscribe (RFC 8058)
├── functions/api/newsletter/admin.ts          GET counts / CSV, POST purge
├── functions/api/newsletter/_*.ts             shared modules (not routes)
├── functions/api/newsletter/_transport.ts     mail: Resend → MailChannels → SMTP, one interface
├── functions/api/newsletter/_smtp*.ts         the SMTP client (cloudflare:sockets)
├── functions/api/newsletter/_mime.ts          message checks + MIME rendering
├── functions/api/newsletter/_session-verifier.ts  `auth` sessions: byte-identical copy of workers/auth/portable/
├── public/newsletter-form.html                the form snippet (also a demo page)
├── public/newsletter.js                       optional progressive enhancement
└── test/                                      Node tests (not deployed; see *Tests*)
```

`public/` is served from the site root: `/newsletter.js`, and the demo at
`/newsletter-form.html` (marked `noindex`).

## 1. Create the database

```sh
npx wrangler d1 create ssg-newsletter
# paste the id into workers/newsletter/wrangler.snippet.toml (uncomment the block)
```

The worker **creates its table on first use**, so the binding — D1 bound as
`NEWSLETTER_DB` — is the only step.
To apply the schema by hand instead:
`npx wrangler d1 execute ssg-newsletter --file=workers/newsletter/schema.sql --remote`.
`ssg new wrangler` folds the binding stub into your `wrangler.toml`.

**Migrations.** `schema.sql` is version 1 and every statement is
`IF NOT EXISTS`, so re-applying it is safe. A later version that adds a column
does it with a guarded `ALTER TABLE` in `_schema.ts` (as the comments worker
did for `parent_id`) and lists it in the file's header. If you run
`wrangler d1 migrations`, copy `schema.sql` to `migrations/0001_newsletter.sql`.

## 2. Wire it into `.ssg.yaml`

```yaml
workers:
  - name: newsletter
    dir: workers/newsletter
    routes_include: [/api/newsletter, /api/newsletter/*]
```

## 3. Put the form on a page

Copy the marked block from `public/newsletter-form.html` into your theme. Fill
in `language`, `source_page` and `tags` per page, and your public Turnstile site
key. Keep the hidden `consent_text` identical to the visible consent label — it
is what gets stored (with JavaScript on, `newsletter.js` reads the label itself).

Without JavaScript the browser posts the form and gets a `303` to the thank-you
URL. With `newsletter.js`, the form posts JSON and the result appears in its
`aria-live` status region, in the page's language (`<html lang>`): English,
Polish, Hindi, German and French ship built in. Override strings or add a
language like the other SSG widgets:

```html
<script id="ssg-newsletter-config" type="application/json">
  { "defaultLang": "en", "i18n": { "en": { "ok": "Welcome aboard!" } } }
</script>
```

Turnstile's own `api.js` must load either way — the token comes from it.

## The API

| Endpoint | Does |
|---|---|
| `POST /api/newsletter` | sign up. JSON, or `application/x-www-form-urlencoded` / multipart from a form |
| `GET /api/newsletter/confirm?token=` | localised page with a *Confirm* button (double opt-in) |
| `POST /api/newsletter/confirm?token=` | `pending` → `confirmed` |
| `GET /api/newsletter/unsubscribe?token=` | localised page with an *Unsubscribe* button |
| `POST /api/newsletter/unsubscribe?token=` | unsubscribes; `List-Unsubscribe=One-Click` gets a bare `200` |
| `GET /api/newsletter/admin` | counts by status and language (admin) |
| `GET /api/newsletter/admin?format=csv` | export; `&status=pending\|confirmed\|unsubscribed\|all`, default `confirmed` |
| `POST /api/newsletter/admin` `{"action":"purge"}` | run the retention purge now (admin) |

Sign-up fields: `email`, `consent` (`true`/`yes`/`on`/`1`), `consent_text`,
`language`, `source_page`, `tags` (a list, repeated fields, or comma-separated),
and the Turnstile token as `cf-turnstile-response` or `token`. A JSON caller
gets `{"ok":true,"confirm":<double opt-in on?>}` or `{"error":"<code>"}` with
`invalid_email`, `consent_required`, `captcha_failed` or `not_configured`. A
form post that fails gets a short localised page instead.

The confirm and unsubscribe pages speak the subscriber's stored language, then
the browser's `Accept-Language`, then English. Links never act on a GET.

## Single or double opt-in

Without a mail transport it is **single opt-in**: a valid sign-up is `confirmed`
at once. That means anyone can put any address on the list (Turnstile only stops
bots doing it in bulk). In Germany and for many senders elsewhere, double opt-in
is the expected standard.

Set `NEWSLETTER_MAIL_FROM` and **one** transport and every new or returning
sign-up is `pending` until the confirmation link is followed:

- `NEWSLETTER_RESEND_API_KEY` — Resend; or
- `NEWSLETTER_MAILCHANNELS_API_KEY` — MailChannels Email API; or
- `NEWSLETTER_SMTP_HOST` (+ user and password) — your own mail server, see
  [Mail transports](#mail-transports).

**Mail settings that can never send are refused, not ignored.** When
`NEWSLETTER_MAIL_FROM` is set and the chosen transport is SMTP with a setting
no server could fix — port `25`, a port that is not a number or outside
1–65535, `NEWSLETTER_SMTP_USER` without `NEWSLETTER_SMTP_PASSWORD`, an invalid
`NEWSLETTER_SMTP_HELO` — the sign-up endpoint answers `not_configured` (`503`
JSON, or the localized error page for a form post) and logs why
(`newsletter: sign-up refused, double opt-in is configured but cannot send:
smtp: …`). It does not fall back to single opt-in, which would confirm
addresses without proof of consent, and it does not store sign-ups that would
stay `pending` forever. Installs with a Resend or MailChannels key, and
installs with no mail settings at all, behave exactly as before.
`transportProblem(env)` in `_transport.ts` answers the question (a string
saying what is wrong, or `null`) without connecting.

The names are prefixed on purpose, with no fallback to contact-form's
`RESEND_API_KEY`: a shared key set for another template must not switch this
flow on unnoticed. A pending address gets at most one confirmation mail per 15
minutes, so the public form cannot be used to flood someone's inbox.

Re-submitting a **confirmed** address changes nothing (an unauthenticated request
must not rewrite someone's entry). Re-submitting a pending or unsubscribed one is
a fresh sign-up: new consent, `unsubscribed_at` cleared, same token.

## Mail transports

The confirmation mail goes through the first transport configured, in this
order: **Resend → MailChannels → SMTP**. The order is fixed on purpose: a site
that already has a Resend or MailChannels key keeps sending through it, even
after SMTP settings are added next to it. Every transport gets the same
message, and a message is refused — never repaired — when an address, the
subject, the sender name or a header holds a CR or LF (header injection).

**SMTP** talks to your own server (or any provider's submission port) over a
Cloudflare TCP socket (`cloudflare:sockets`):

| Port | Connection |
|---|---|
| `465` | TLS from the first byte |
| `587` (default) | plain, then `STARTTLS`; any other port is treated the same way |
| `25` | refused with a message — Cloudflare blocks outbound port 25 |

TLS is required: a server on 587 that does not offer `STARTTLS` gets no
password and no mail. Login uses `AUTH PLAIN`, or `AUTH LOGIN` when only that
is offered; leave `NEWSLETTER_SMTP_USER` unset for a server that relays without
login. Each command waits at most 10 seconds for a reply. A `4xx` reply, a
dropped connection or a timeout counts as temporary, a `5xx` as permanent; the
log line names the SMTP code and the step (`smtp 550 at RCPT TO: …`).

Messages are UTF-8, `text/plain` (or `multipart/alternative` with an HTML
part), quoted-printable, with RFC 2047 encoded subject and sender name,
`Message-ID` and `Date`. `EHLO` names the sender's domain unless
`NEWSLETTER_SMTP_HELO` says otherwise.

A message may carry an optional `idempotencyKey` (1–256 visible ASCII
characters; the confirmation mail sends none). Resend receives it as the
`Idempotency-Key` header and, by its documentation, sends a message at most
once per key within 24 hours. The MailChannels Email API documents no such
mechanism, so it is not sent there. Over SMTP the key becomes a deterministic
`Message-ID` — a hint some receivers de-duplicate on, not a guarantee.
Without a key every request is byte for byte what it was before.
`honoursIdempotencyKey(env)` says whether the configured transport itself
guarantees the de-duplication (Resend only).

Resend refuses a keyed request with `409` in two documented cases, told
apart by the error `name` in the JSON body: `invalid_idempotent_request` (the
key was used in the last 24 hours with a different payload — a message under
it was already accepted) returns `{ok: true, duplicate: true}`, so a caller
that reads only `ok` treats it as sent; `concurrent_idempotent_requests` (a
request with this key is still in progress) and any other or unreadable `409`
are temporary. A request without a key reads a `409` as before (permanent).

Whether `cloudflare:sockets` is available to Pages Functions could not be
verified: Cloudflare's [TCP sockets](https://developers.cloudflare.com/workers/runtime-apis/tcp-sockets/)
page documents `connect()` for Workers and says nothing about Pages Functions
(checked 2026-11-13), and no live deployment was tried for this release. If
the import fails, the worker logs it once and the confirmation mail is not
sent (sign-ups still work, as `pending`). The same page confirms that port 25
is blocked and that connections to Cloudflare's own IP ranges are refused.
`newsletter-ui` is a Worker, where sockets are documented.

**Deliverability is the mail server's job.** It signs DKIM, and for a
newsletter that signature must cover `List-Unsubscribe` and
`List-Unsubscribe-Post` (RFC 8058) — check the server's list of signed
headers. SPF and DMARC belong to the sender domain's DNS: the server that
sends must be allowed by SPF, and the DKIM domain must align with the `From`
domain for DMARC.

## Sending: the RFC 8058 headers

`newsletter-ui` adds these headers to every campaign message by itself. If you
mail the CSV with another tool, put the row's `unsubscribe_url` in every
message:

```
List-Unsubscribe: <https://example.com/api/newsletter/unsubscribe?token=YOUR_ROWS_TOKEN>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
```

Gmail and Yahoo require one-click unsubscribe from bulk senders, and RFC 8058
requires the message's DKIM signature to cover both headers. The provider then
POSTs `List-Unsubscribe=One-Click` to the URL itself when the reader clicks its
Unsubscribe button.

## Admin

`GET /api/newsletter/admin` with HTTP Basic (`NEWSLETTER_ADMIN_PASSWORD`):

```sh
curl -u :"$NEWSLETTER_ADMIN_PASSWORD" https://example.com/api/newsletter/admin
# {"total":3,"byStatus":{"pending":0,"confirmed":2,"unsubscribed":1},"byLanguage":{"pl":{…}}}
curl -u :"$NEWSLETTER_ADMIN_PASSWORD" -o list.csv 'https://example.com/api/newsletter/admin?format=csv'
```

Or put `/api/newsletter/admin*` behind **Cloudflare Access** and set
`NEWSLETTER_ACCESS_TEAM` + `NEWSLETTER_ACCESS_AUD`; the worker then verifies the
Access JWT instead of a password, exactly as the comments worker does. Or use
**accounts of the `auth` worker** (below). The CSV
is UTF-8 with a BOM (so Excel shows Polish and Hindi correctly), and any cell
starting with `=`, `+`, `-` or `@` is prefixed with `'` so a spreadsheet cannot
run it as a formula. This export does not know the suppression list (bounces,
complaints, erasures) kept by the `newsletter-ui` worker — if you run it, mail
from its `GET {base}/api/subscribers.csv` instead, which leaves suppressed
addresses out.

### Who can do what

Three ways in, checked in this order: **Cloudflare Access → `auth` →
password**. The first one configured decides.

| Mode | Set | Permissions | Which to choose |
|---|---|---|---|
| Shared password | secret `NEWSLETTER_ADMIN_PASSWORD` | everything | one person — the simplest setup |
| Cloudflare Access | `NEWSLETTER_ACCESS_TEAM` + `_AUD` | everything | you already run Cloudflare Zero Trust |
| `auth` accounts | `NEWSLETTER_AUTH_*` | each account's own | several people, or different permissions |

The password is a first-class mode and stays one. More than one person, more
than one panel, or different permissions → see the `auth` worker
([which mode to choose](../../docs/WORKERS.md#admin-sign-in-which-mode-to-choose)).

**Known limitation of the password mode: no brute-force limit.** This
template does not count failed passwords on `/api/newsletter/admin`, so
nothing slows down someone guessing it. Use a long random password
(`openssl rand -base64 24`), and add a limit in front: the
[`rate-limit`](../rate-limit/README.md) template (its middleware covers the
admin endpoint like every other Function), Cloudflare Access, or `auth`, whose
sign-in locks an address and an IP after repeated failures. (`newsletter-ui`
has its own per-IP lockout.)

With `auth` the worker checks the session cookie of the `auth` worker on the
same host locally, with its public key
(`functions/api/newsletter/_session-verifier.ts`, a byte-identical copy of
`workers/auth/portable/session-verifier.ts`, guarded by
`test/auth-copies.test.ts`). The variables are the ones `newsletter-ui` reads
— one set serves both:

| Variable | Meaning |
|---|---|
| `NEWSLETTER_AUTH_ISSUER` | required: the `iss` to accept, e.g. `https://example.com/api/auth` |
| `NEWSLETTER_AUTH_LOGIN_URL` | required: where a browser without a session is sent, e.g. `/auth/login` |
| `NEWSLETTER_AUTH_PUBLIC_KEY` or `NEWSLETTER_AUTH_JWKS_URL` | exactly one: the public key (recommended) or `/api/auth/jwks` |
| `NEWSLETTER_AUTH_AUDIENCE` | optional: the `aud` to accept; default the request's host |

| Route | Permission |
|---|---|
| `GET /api/newsletter/admin` (counts) | `newsletter:read` |
| `GET /api/newsletter/admin?format=csv` | `newsletter:export` |
| `POST /api/newsletter/admin {"action":"purge"}` | `newsletter:settings` |
| `GET /api/newsletter/module`, `/module/summary` | `newsletter:read` |

A missing permission answers `403 {"error": "forbidden", "permission": "…"}`;
no session `401` (a browser page: a redirect to the login page); an
incomplete `NEWSLETTER_AUTH_*` set `503 {"error": "not_configured", "detail":
"…"}` — never a fall-back to the password. The purge, the one state-changing
admin request, must come from the site's own origin when it rides on the
session cookie (`Origin` / `Sec-Fetch-Site`; a service token in
`Authorization: Bearer` needs neither). With Access or the password nothing
changed. The public endpoints — sign-up, confirm, unsubscribe — never look at
any of this.

**Switching from the password:** set the `NEWSLETTER_AUTH_*` variables, deploy,
check, then **remove the password**: `npx wrangler pages secret delete
NEWSLETTER_ADMIN_PASSWORD`. While `auth` is configured it is not accepted (it
would bypass every permission), but if the `auth` variables were ever removed
it would quietly become the way in again; nothing removes it for you.

## Retention

`NEWSLETTER_RETENTION_DAYS = "30"` deletes `pending` and `unsubscribed` rows
untouched for 30 days; confirmed subscribers are never purged. `0` (the default)
keeps everything. The purge runs lazily after sign-up, confirm and unsubscribe
writes, at most hourly per isolate. A quiet site can run it from cron:

```sh
curl -u :"$NEWSLETTER_ADMIN_PASSWORD" -H 'content-type: application/json' \
     -d '{"action":"purge"}' https://example.com/api/newsletter/admin
```

Purging an unsubscribed row also deletes the record that the person left; keep
the window long enough to answer a complaint.

## Behind the rate-limit worker

The sign-up is a public `POST`, so bound it with the `rate-limit` template:

```sh
ssg new worker rate-limit
cp workers/rate-limit/functions/_middleware.ts workers/newsletter/functions/
```

Set `RATE_LIMIT_SKIP = "/api/newsletter/unsubscribe"`. One-click unsubscribes
come from the mailbox provider's servers, a few IPs for thousands of readers;
counted per IP they would hit the cap and be refused with a `429` the provider
may never retry. `newsletter.js` shows a "too many attempts" message on `429`.

## Config / secrets

`wrangler pages secret put <NAME>`:

| Secret | Purpose |
|---|---|
| `NEWSLETTER_TURNSTILE_SECRET` | verifies the sign-up token (required; falls back to the shared `TURNSTILE_SECRET` when unset) |
| `NEWSLETTER_HASH_SALT` | salt for the IP / User-Agent hashes; unset ⇒ neither is stored |
| `NEWSLETTER_ADMIN_PASSWORD` | admin endpoint password (unless Cloudflare Access or `auth` is used) |
| `NEWSLETTER_RESEND_API_KEY` | optional — double opt-in via Resend |
| `NEWSLETTER_MAILCHANNELS_API_KEY` | optional — double opt-in via MailChannels |
| `NEWSLETTER_SMTP_PASSWORD` | optional — the SMTP login password |

`[vars]`:

| Var | Effect |
|---|---|
| `NEWSLETTER_THANKS_URL` | where a form post goes: `/{lang}/thanks/`, or a JSON map `{"en":"/thanks/","pl":"/pl/dziekujemy/","default":"/thanks/"}`. Unset: back to the sign-up page at `#newsletter-thanks` |
| `NEWSLETTER_RETENTION_DAYS` | purge pending/unsubscribed rows after N days (`0` = keep) |
| `NEWSLETTER_MAIL_FROM` | sender for confirmation mail; with a transport, enables double opt-in |
| `NEWSLETTER_MAIL_FROM_NAME` | optional sender display name |
| `NEWSLETTER_SMTP_HOST` | your mail server; used only when no Resend or MailChannels key is set |
| `NEWSLETTER_SMTP_PORT` | `465` (TLS) or `587` (STARTTLS, the default); `25` or a non-number makes sign-ups answer `not_configured` |
| `NEWSLETTER_SMTP_USER` | SMTP login (with the `NEWSLETTER_SMTP_PASSWORD` secret) |
| `NEWSLETTER_SMTP_HELO` | optional `EHLO` name; default: the domain of `NEWSLETTER_MAIL_FROM` |
| `NEWSLETTER_ACCESS_TEAM`, `NEWSLETTER_ACCESS_AUD` | Cloudflare Access for the admin endpoint |
| `NEWSLETTER_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` | accounts of the `auth` worker for the admin endpoint ("Who can do what"); shared with `newsletter-ui` |

Prefer `NEWSLETTER_TURNSTILE_SECRET` over the shared `TURNSTILE_SECRET`: every
worker in one Pages project reads the same environment — see *Secrets* in
[docs/WORKERS.md](https://github.com/spagu/ssg/blob/main/docs/WORKERS.md#secrets).

## Backward compatibility

An install deployed before 1.9.8 keeps working after the template is updated,
with no migration and no new setting:

- the `subscribers` table is the same schema v1, and nothing in this round
  alters, indexes or drops it (`newsletter-ui` included);
- every endpoint answers as before — status codes, headers, JSON bodies, the
  HTML pages byte for byte, the CSV header row and cell rules — and leaves the
  same rows behind;
- an install with only a Resend key, or only a MailChannels key, sends the
  confirmation mail with a byte-identical provider request.

Two behaviours changed on purpose, both only for settings that did nothing
before: an SMTP host next to `NEWSLETTER_MAIL_FROM` now makes sign-ups double
opt-in through SMTP (it used to be ignored, so sign-ups were confirmed at
once), and SMTP settings that can never send make sign-ups answer
`not_configured` (see *Single or double opt-in*). The provider name in two log
lines changed too (`resend returned 500` where it said `provider returned
500`).

The `compat-*` tests enforce this: they run every endpoint scenario against
the current code and compare the answers with `test/compat-golden.json`,
recorded from the 1.9.7 release.

## Tests

The mail code is tested against an in-memory SMTP server and a fake `fetch`;
the endpoints run on a real SQLite database (`node:sqlite`, the engine D1 is
built on). Node 23.6 or newer runs the TypeScript directly; nothing to
install. From the repository root (or `make test-workers` for every
template):

```sh
node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
```

`_transport.ts`, `_smtp.ts`, `_smtpwire.ts` and `_mime.ts` import nothing
else from this worker, so they can be copied unchanged into another template.

`test/compat-*.test.ts` pin the answers of 1.9.7 byte for byte and never
change. `test/auth.test.ts` runs every admin route against no session, a
session without its permission, exactly it and the owner;
`test/auth-copies.test.ts` keeps the verifier copy identical to
`workers/auth/portable/` and runs that worker's verifier suites against it.

## Privacy notes

Stored per subscriber: the email, status, language, source page, tags, the
consent text and time, timestamps, the link token and — only with a salt — the
salted hashes of IP and User-Agent. Say so in your privacy policy, with the
retention you chose. The hashes exist to spot one source signing up hundreds of
addresses, not to identify anyone; the admin export leaves them out. The pages
the worker renders send no `Referer` (their URL carries a token), are not cached
and are marked `noindex`.

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, the permissions its admin surface distinguishes, and how to switch it off. It is a description only — no secrets, no configuration.

| Permission | Allows |
|---|---|
| `newsletter:read` | View subscribers, counts, campaigns and statistics. |
| `newsletter:export` | Download subscribers as CSV and export one person's data. |
| `newsletter:settings` | Maintenance: test the SMTP connection and run the retention purge now. |

With the shared password or Cloudflare Access whoever passes may do everything; with `auth` each action needs its permission (*Who can do what*).

It also serves the manifest at `GET /api/newsletter/module`, behind the same gate as `/api/newsletter/admin` (Cloudflare Access, an `auth` session holding `newsletter:read`, or `NEWSLETTER_ADMIN_PASSWORD`). The route imports `functions/api/newsletter/_module.ts`, a generated copy of `module.json` (Pages Functions cannot read files at runtime) — edit `module.json` and keep the copy identical.

### Summary

`GET /api/newsletter/module/summary` answers the module summary (format in [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md#the-summary-endpoint)): subscribers `confirmed`, `pending` and `unsubscribed`, and the configuration problems that refuse sign-ups — `no_database`, `no_turnstile_secret`, `mail_unusable` (double opt-in is configured but the transport can never send; the text names why, e.g. SMTP port 25, never a password), `access_incomplete`. No attention items: this template has no panel to link to. One `GROUP BY` (the one `/api/newsletter/admin` answers), behind the same gate, refusing exactly as that endpoint does. `newsletter-ui` reads the same table and reports the same `confirmed` and `pending` numbers; when both are deployed, a home page shows `newsletter-ui`'s summary. With Cloudflare Access, let the Access application cover `/api/newsletter/module*` too.
