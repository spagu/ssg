# newsletter worker

A newsletter or waitlist sign-up for an SSG site: addresses in Cloudflare D1,
Turnstile on the form, consent recorded with its wording, one-click unsubscribe
(RFC 8058), optional double opt-in. Scaffold it with `ssg new worker newsletter`.

It collects the list; it does not send the newsletter. Export the confirmed
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
├── functions/api/newsletter/index.ts          POST sign-up
├── functions/api/newsletter/confirm.ts        GET page + POST confirm (double opt-in)
├── functions/api/newsletter/unsubscribe.ts    GET page + POST unsubscribe (RFC 8058)
├── functions/api/newsletter/admin.ts          GET counts / CSV, POST purge
├── functions/api/newsletter/_*.ts             shared modules (not routes)
├── public/newsletter-form.html                the form snippet (also a demo page)
└── public/newsletter.js                       optional progressive enhancement
```

`public/` is served from the site root: `/newsletter.js`, and the demo at
`/newsletter-form.html` (marked `noindex`).

## 1. Create the database

```sh
npx wrangler d1 create ssg-newsletter
# paste the id into workers/newsletter/wrangler.snippet.toml (uncomment the block)
```

The worker **creates its table on first use**, so the binding is the only step.
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

Without a mail provider it is **single opt-in**: a valid sign-up is `confirmed`
at once. That means anyone can put any address on the list (Turnstile only stops
bots doing it in bulk). In Germany and for many senders elsewhere, double opt-in
is the expected standard.

Set `NEWSLETTER_MAIL_FROM` and **one** provider key and every new or returning
sign-up is `pending` until the confirmation link is followed:

- `NEWSLETTER_RESEND_API_KEY` — Resend; or
- `NEWSLETTER_MAILCHANNELS_API_KEY` — MailChannels Email API.

The names are prefixed on purpose, with no fallback to contact-form's
`RESEND_API_KEY`: a shared key set for another template must not switch this
flow on unnoticed. A pending address gets at most one confirmation mail per 15
minutes, so the public form cannot be used to flood someone's inbox.

Re-submitting a **confirmed** address changes nothing (an unauthenticated request
must not rewrite someone's entry). Re-submitting a pending or unsubscribed one is
a fresh sign-up: new consent, `unsubscribed_at` cleared, same token.

## Sending: the RFC 8058 headers

Put the row's `unsubscribe_url` from the CSV in every message:

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
Access JWT instead of a password, exactly as the comments worker does. The CSV
is UTF-8 with a BOM (so Excel shows Polish and Hindi correctly), and any cell
starting with `=`, `+`, `-` or `@` is prefixed with `'` so a spreadsheet cannot
run it as a formula.

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
| `NEWSLETTER_ADMIN_PASSWORD` | admin endpoint password (unless Cloudflare Access is used) |
| `NEWSLETTER_RESEND_API_KEY` | optional — double opt-in via Resend |
| `NEWSLETTER_MAILCHANNELS_API_KEY` | optional — double opt-in via MailChannels |

`[vars]`:

| Var | Effect |
|---|---|
| `NEWSLETTER_THANKS_URL` | where a form post goes: `/{lang}/thanks/`, or a JSON map `{"en":"/thanks/","pl":"/pl/dziekujemy/","default":"/thanks/"}`. Unset: back to the sign-up page at `#newsletter-thanks` |
| `NEWSLETTER_RETENTION_DAYS` | purge pending/unsubscribed rows after N days (`0` = keep) |
| `NEWSLETTER_MAIL_FROM` | sender for confirmation mail; with a provider key, enables double opt-in |
| `NEWSLETTER_MAIL_FROM_NAME` | optional sender display name |
| `NEWSLETTER_ACCESS_TEAM`, `NEWSLETTER_ACCESS_AUD` | Cloudflare Access for the admin endpoint |

Prefer `NEWSLETTER_TURNSTILE_SECRET` over the shared `TURNSTILE_SECRET`: every
worker in one Pages project reads the same environment — see *Secrets* in
[docs/WORKERS.md](https://github.com/spagu/ssg/blob/main/docs/WORKERS.md#secrets).

## Privacy notes

Stored per subscriber: the email, status, language, source page, tags, the
consent text and time, timestamps, the link token and — only with a salt — the
salted hashes of IP and User-Agent. Say so in your privacy policy, with the
retention you chose. The hashes exist to spot one source signing up hundreds of
addresses, not to identify anyone; the admin export leaves them out. The pages
the worker renders send no `Referer` (their URL carries a token), are not cached
and are marked `noindex`.
