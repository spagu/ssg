# newsletter-ui worker

![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)
![Node tests](https://img.shields.io/badge/tests-node%3Atest-339933?logo=nodedotjs&logoColor=white)
![OpenAPI 3.1](https://img.shields.io/badge/OpenAPI-3.1-6BA539?logo=openapiinitiative&logoColor=white)
![npm dependencies: 0](https://img.shields.io/badge/npm%20dependencies-0-brightgreen)

The sending side of the `newsletter` template: a panel in the browser, an
admin API, campaigns, a send engine with a cron trigger and a queue, bounce
and complaint handling, statistics that track nobody. The `newsletter`
template collects the list on your site; this worker mails it. Scaffold it
with `ssg new worker newsletter-ui`.

More detail lives next to this file:
[API reference](docs/API.md) ([`openapi.yaml`](openapi.yaml), OpenAPI 3.1) ·
[how sending works](docs/SENDING.md) · [importing a list](docs/IMPORT.md) ·
[bounces and webhooks](docs/BOUNCES.md) · [the panel](docs/PANEL.md) ·
[who can do what](docs/ACCESS.md).

## What it is

`newsletter` is a set of Pages Functions inside your site's Pages project.
Pages Functions answer requests and nothing else: they have no cron trigger and
cannot consume a queue. A campaign has to start on schedule, go out in
batches, pause, resume after a failure and never lose or repeat a message, so
it needs both. `newsletter-ui` is therefore a **standalone Worker** with its
own `wrangler.toml`, deployed with `wrangler deploy`, sharing one thing with
`newsletter`: the D1 database bound as `NEWSLETTER_DB`.

```
workers/newsletter      (Pages Functions, on your site)
   sign-up · confirm · unsubscribe · counts · CSV · purge
   owns the `subscribers` table (schema v1 — never changed)
        │   the same D1 database: NEWSLETTER_DB
        ▼
workers/newsletter-ui   (a Worker, under {base} on the same domain)
   panel · admin API · campaigns · cron + queue · sending · bounces
   its own tables: campaigns, campaign_sends, suppressions, settings, …
```

Do **not** add it to `worker:` / `workers:` in `.ssg.yaml`. That section copies
Pages Functions into the site build, and `ssg` refuses a standalone Worker
there with a message saying so.

What it does, in one list:

- **Subscribers**: list, search, edit tags and language, unsubscribe, GDPR
  export and erasure, a CSV export that leaves suppressed addresses out, an
  import that reads lists exported from common mailing tools as they are.
- **Campaigns**: Markdown or HTML, a segment by language and tags, a preview
  that is exactly the mail, test sends, scheduling, pause, resume, cancel, a
  draft from a post on the site's feed.
- **Sending**: through Resend, MailChannels or your own SMTP server; at a
  rate you set; each recipient at most once; a crash never sends twice by
  itself.
- **Bounces and complaints**: from the SMTP reply, from signed provider
  webhooks or from a CSV; a suppression list that every send obeys.
- **Statistics**: counts per campaign, optional click counting per link —
  no open pixel, nothing that points at a person.

## Install from zero

Everything below runs from `workers/newsletter-ui/` with `npx wrangler`
(Wrangler 4; nothing is installed into the repository). You need the
`newsletter` template deployed first, because its table holds the list.

1. **The database.** Use the one `newsletter` already uses: `npx wrangler d1
   list` shows its id; paste it into `database_id` in `wrangler.toml`. With no
   newsletter yet, deploy it first (`ssg new worker newsletter`, see its
   README). The database is shared, the tables are not.
2. **The queues.** One for send jobs and one for jobs that keep failing:

   ```sh
   npx wrangler queues create ssg-newsletter-send
   npx wrangler queues create ssg-newsletter-send-dlq
   ```

3. **Admin auth.** Cloudflare Access (recommended: an Access application
   covering `{base}*`, then set `NEWSLETTER_ACCESS_TEAM` and
   `NEWSLETTER_ACCESS_AUD`) or a password:
   `npx wrangler secret put NEWSLETTER_ADMIN_PASSWORD`.
4. **Mail.** Set `NEWSLETTER_SITE_URL` and `NEWSLETTER_MAIL_FROM` in `[vars]`
   and one transport — `npx wrangler secret put NEWSLETTER_RESEND_API_KEY`,
   or `NEWSLETTER_MAILCHANNELS_API_KEY`, or `NEWSLETTER_SMTP_HOST` /
   `_PORT` / `_USER` (and optionally `_HELO`) plus the
   `NEWSLETTER_SMTP_PASSWORD` secret. These are the
   `newsletter` template's names; set the same values there and its
   confirmation mail goes the same way. For SMTP: port `465` (TLS) or `587`
   (STARTTLS); port 25 is blocked by Cloudflare and refused. The sender
   domain needs SPF allowing your server, DKIM signing that covers
   `List-Unsubscribe` and `List-Unsubscribe-Post`, and DMARC alignment — see
   "Mail transports" in the `newsletter` README. To run your own SMTP server
   for this: [docs/SMTP-SERVER.md](../../docs/SMTP-SERVER.md)
   (`services/smtp-relay`, Docker or a Raspberry Pi).
5. **The route.** Replace `<example.com>` in `routes` with your zone (keep it
   equal to `NEWSLETTER_UI_BASE`), then:

   ```sh
   npx wrangler deploy
   curl -u admin:<password> https://example.com/newsletter-admin/api/health
   ```

   `{"ok": true, "subscribersTable": true, …}` means it found the list. Open
   `https://example.com/newsletter-admin/` in a browser.
6. **Bounces (optional, recommended).** Register the provider's webhook and
   set its secret: [BOUNCES.md](docs/BOUNCES.md).

The tables are created on the first request. To create them by hand:
`npx wrangler d1 execute ssg-newsletter --file=schema.sql --remote`.

`npx wrangler deploy --dry-run` validates this `wrangler.toml` and bundles the
worker without an account; it was run for this release with Wrangler 4.161.0.
The deploy itself, the queues and the routes were not exercised against a live
Cloudflare account.

## Configuration

Every variable and secret in one place. `[vars]` go in `wrangler.toml`;
secrets with `npx wrangler secret put <NAME>`, never in a file. The mail
names are the `newsletter` template's, so one set of values serves both.

| Name | Kind | Default | Use |
|---|---|---|---|
| `NEWSLETTER_DB` | D1 binding | — | the `newsletter` template's database (required) |
| `NEWSLETTER_QUEUE` | queue producer | — | send jobs (required to send) |
| `RATE_LIMITER` | rate-limit binding | 120 / 60 s per IP | optional; delete the block to go without |
| `ASSETS` | assets binding | `./public` | the panel |
| `NEWSLETTER_UI_BASE` | var | `/newsletter-admin` | the path the worker owns; keep `routes` in step |
| `NEWSLETTER_ACCESS_TEAM`, `NEWSLETTER_ACCESS_AUD` | var | — | Cloudflare Access (both, or neither) |
| `NEWSLETTER_ADMIN_PASSWORD` | secret | — | HTTP Basic, when neither Access nor `auth` is set |
| `NEWSLETTER_AUTH_ISSUER`, `_LOGIN_URL`, `_PUBLIC_KEY` or `_JWKS_URL`, `_AUDIENCE` | var | — | accounts of the `auth` worker ("Who can do what"); any one set turns the mode on, an incomplete set is `503` |
| `NEWSLETTER_AUDIT_RETENTION_DAYS` | var | `365` | days the audit trail keeps a row; `0` keeps them forever |
| `NEWSLETTER_AUTH_MAX_FAILURES` | var | `10` | failed Basic logins per IP … |
| `NEWSLETTER_AUTH_WINDOW_MINUTES` | var | `10` | … per window, also the lockout length |
| `NEWSLETTER_HASH_SALT` | secret | — | salts the short-lived failed-login records |
| `NEWSLETTER_SITE_URL` | var | — | the site's `https://` origin, where `newsletter` runs; needed to preview, test, schedule and confirm |
| `NEWSLETTER_MAIL_FROM`, `NEWSLETTER_MAIL_FROM_NAME` | var | — | default sender (a campaign may set its own) |
| `NEWSLETTER_RESEND_API_KEY` | secret | — | transport: Resend |
| `NEWSLETTER_MAILCHANNELS_API_KEY` | secret | — | transport: MailChannels |
| `NEWSLETTER_SMTP_HOST`, `_PORT`, `_USER`, `_HELO` | var | port `587` | transport: your SMTP server (`465` = TLS, `587` = STARTTLS) |
| `NEWSLETTER_SMTP_PASSWORD` | secret | — | SMTP login |
| `NEWSLETTER_SEND_RATE` | var | `60` | messages per minute, whole worker (campaigns and confirmations) |
| `NEWSLETTER_SEND_BATCH` | var | `10` | messages per queue job, never more than the rate; see "Limits per Cloudflare plan" |
| `NEWSLETTER_SEND_STALE_MINUTES` | var | `30` | after this a claimed row becomes `uncertain` and a lost job is re-queued; keep it above 15 (a consumer's wall-clock limit) |
| `NEWSLETTER_SEND_MAX_ATTEMPTS` | var | `3` | tries per recipient on temporary failures |
| `NEWSLETTER_BOUNCE_BREAKER_MIN` | var | `5` | hard bounces in one batch (or reported by webhook for a sending campaign since its last trip) before the campaign pauses … |
| `NEWSLETTER_BOUNCE_BREAKER_PERCENT` | var | `50` | … if they are also this share of the batch's messages (of the campaign's sent messages, for webhook bounces) |
| `NEWSLETTER_DEAD_LETTER_QUEUE` | var | `ssg-newsletter-send-dlq` | only if you renamed the queue |
| `NEWSLETTER_SOFT_BOUNCE_LIMIT` | var | `3` | soft bounces in a row before a suppression |
| `NEWSLETTER_RESEND_WEBHOOK_SECRET` | secret | — | the Resend webhook's `whsec_…`; unset → `/hooks/resend` answers 503 |
| `NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY` | secret | — | MailChannels' Ed25519 public key (PEM); unset → 503 |
| `NEWSLETTER_TRACK_CLICKS` | var | off | `true` counts clicks per link |
| `NEWSLETTER_UTM` | var | off | `on` adds `utm_*` parameters to links to the site |
| `NEWSLETTER_UTM_SOURCE` | var | `newsletter` | the `utm_source` value |
| `NEWSLETTER_UTM_HOSTS` | var | — | more hosts to tag (commas or spaces), besides the host of `NEWSLETTER_SITE_URL` |
| `NEWSLETTER_UTM_STRICT` | var | off | `on`: a link to a tagged host that already has a `utm_*` parameter stops scheduling (`409 content_check_failed`); needs `NEWSLETTER_UTM` on, otherwise every schedule answers `503 content_rules_invalid`; a value that is neither on (`1`, `true`, `yes`, `on`) nor off (`0`, `false`, `no`, `off`, empty) → 503 too ("Content rules") |
| `NEWSLETTER_BANNED_PHRASES` | var | — | phrases separated by `\|`, matched as whole words in any case; a hit stops scheduling (`409 content_check_failed`); more than 200 phrases or one longer than 100 characters → `503 content_rules_invalid` ("Content rules") |
| `NEWSLETTER_BLOCKED_IMAGE_PATTERNS` | var | — | substrings separated by `\|` an image URL of the mail must not contain, in any case; a hit stops scheduling; more than 50 patterns or one longer than 200 characters → 503 ("Content rules") |
| `NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS` | var | `14` | how long after a send an unsubscribe is credited to it |
| `NEWSLETTER_FEED_URL` | var | `/feed.xml` on the site | the feed for "campaign from a post" |
| `NEWSLETTER_CONFIRM_BATCH` | var | `10` | confirmation mails per call, never more than the rate |
| `NEWSLETTER_IMPORT_MAX_ROWS` | var | `20000` | rows per CSV import (1 KiB of body per row) |
| `NEWSLETTER_ASSETS` | R2 binding | — | optional campaign image store, the switch ("Campaign images"); unbound → the asset routes answer `404 assets_disabled` |
| `NEWSLETTER_ASSETS_URL` | var | — | the bucket's own `https://` address (an R2 custom domain), without credentials, query or fragment; unset → this worker serves images at `{NEWSLETTER_SITE_URL}{base}/hooks/a/`; an unusable value → `503 assets_misconfigured` |
| `NEWSLETTER_ASSETS_MAX_BYTES` | var | `2097152` (2 MiB) | largest image upload; not a positive integer → the default, above 10 MiB → 10 MiB |
| `NEWSLETTER_IMAGES` | Images binding | — | optional; unbound → files are stored as uploaded; bound → an image wider than `NEWSLETTER_ASSETS_MAX_WIDTH` is stored as a width-capped copy ("Campaign images") |
| `NEWSLETTER_ASSETS_MAX_WIDTH` | var | `1200` | that width cap in pixels; not a positive integer → the default, then clamped to 320–4000; read only with `NEWSLETTER_IMAGES` |
| `NEWSLETTER_IMAGES_REQUIRE` | var | — | any of `alt`, `store`, `load`, separated by commas (or spaces), any case; unset or empty → the image check only warns; set → scheduling refuses images that break these rules (`409 image_check_failed`); an unknown word, or `store` without a usable image store → every schedule answers `503 image_rules_unusable` ("Campaign images → Requiring images to pass") |

The transport is the first one set: **Resend → MailChannels → SMTP**. An SMTP
setting that can never work (port 25, a port that is not a number, a user
without a password, a bad EHLO name) is reported by `GET /api/settings`
(`transportProblem`) and refused up front by every route that sends
(`503 transport_unusable`) — never a failed send later.

## Base path, authentication, headers

Everything lives under `NEWSLETTER_UI_BASE`:

| Path | What | Auth |
|---|---|---|
| `{base}/` and its files | the panel (`public/`) | admin |
| `{base}/api/…` | admin API, JSON | admin + the route's permission + CSRF header |
| `{base}/hooks/…` | provider webhooks and the click redirect | none; each verifies its signature or carries no person |
| anything else | `404` | — |

An invalid base (spaces, `..`) answers `503`; the worker must not shadow a
page of your site.

- **Cloudflare Access:** the JWT is verified against your team's keys:
  audience, issuer and expiry. Setup is below.
- **HTTP Basic:** any user name; the password is compared in constant time.
  After `NEWSLETTER_AUTH_MAX_FAILURES` failed logins from one IP within the
  window, that IP gets `429` for one window, even with the right password —
  otherwise the lock would only change the error code of a wrong guess. The
  count lives in `auth_attempts`, keyed by a hash of the IP.
- **`auth` accounts:** the session of the `auth` worker on the same host,
  checked locally with its public key; each route needs its permission
  (next section). With `auth` configured the password is not accepted.
- With none configured, every admin request is `503`. Nothing is open by
  default. The order is Access → `auth` → password.
- **CSRF.** Every request other than `GET`/`HEAD` must send
  `x-newsletter-admin: 1`, or it is refused with `403`. A form on another site
  can make the browser send your credentials or Access cookie along, but it
  cannot add a header. With `auth` a cross-origin `Origin` or
  `Sec-Fetch-Site` is refused too.
- **Headers.** Every response carries `cache-control: no-store`,
  `x-robots-tag: noindex, nofollow`, `referrer-policy: no-referrer`,
  `x-content-type-options: nosniff`, `x-frame-options: DENY` and a CSP with no
  `unsafe-inline`.

### Putting the panel behind Cloudflare Access (Zero Trust)

Access signs you in with your identity provider before a request reaches the
worker, and the worker then verifies Access's signed token itself — so a
request that somehow skips Access is still refused. There is no password to
store or rotate.

In the Zero Trust dashboard, under **Access → Applications**:

1. **Add an application → Self-hosted.** Application domain: your site's
   host, path `newsletter-admin` (your `NEWSLETTER_UI_BASE` without the
   leading slash). Add a policy with action **Allow** that names who may
   administer the list — specific emails or a group, not "everyone in the
   domain" unless that is what you mean. Choose a short session duration.
2. **Add a second application for the public hooks.** Same host, path
   `newsletter-admin/hooks`, with one policy: action **Bypass**, include
   **Everyone**. Access applies the most specific path, so this lets the
   provider webhooks and the click redirect through while everything else
   under the base stays protected. Without it Access answers the providers
   with its login page: bounces and complaints are never recorded and tracked
   links stop working. The hooks are safe to expose — each verifies its
   provider's signature or carries no person.
3. **Copy the first application's Application Audience (AUD) tag** from its
   overview, and your team name (the `<team>` in
   `<team>.cloudflareaccess.com`), into `wrangler.toml`:

   ```toml
   [vars]
   NEWSLETTER_ACCESS_TEAM = "myteam"
   NEWSLETTER_ACCESS_AUD = "<the application's AUD tag>"
   ```

4. **Remove the password** if one was set
   (`npx wrangler secret delete NEWSLETTER_ADMIN_PASSWORD`) and deploy. With
   the Access variables set the worker ignores HTTP Basic.

Check it: `{base}/` in a private window redirects to your sign-in;
`curl -i https://example.com/newsletter-admin/api/health` is answered by
Access with a redirect to its sign-in, never by the panel; `curl -i -X POST
https://example.com/newsletter-admin/hooks/resend` reaches the worker and is
refused by the worker itself (`401` or `503`), not by an Access login page.

The `newsletter` template's admin endpoint (`/api/newsletter/admin*`) takes
the same two variables. Add that path to the first application as a second
destination and one AUD covers both; or give it its own application and set
that application's AUD in the Pages project.

## Who can do what

Three ways in; pick by how many people run the newsletter:

| Mode | Set | Permissions | Which to choose |
|---|---|---|---|
| Shared password | secret `NEWSLETTER_ADMIN_PASSWORD` | everything | one person — the simplest setup, nothing else to deploy |
| Cloudflare Access | `NEWSLETTER_ACCESS_TEAM` + `_AUD` | everything | you already run Cloudflare Zero Trust |
| `auth` accounts | `NEWSLETTER_AUTH_*` | each account's own | several people, or different permissions for different people |

The password is a first-class mode and stays one. More than one person,
more than one panel, or different permissions → see the `auth` worker
(`ssg new worker auth`).

What each permission (from [`module.json`](module.json)) lets an account do:

| Permission | Actions |
|---|---|
| `newsletter:read` | overview, subscribers, campaigns and statistics, suppression list, configuration, a campaign's preview |
| `newsletter:edit` | campaign drafts (new, from a post, edit, duplicate, delete); a subscriber's tags and language; unsubscribe someone |
| `newsletter:send` | test send, schedule, unschedule, pause, resume, cancel; re-send or dismiss uncertain sends; confirmation mail |
| `newsletter:import` | import a subscriber list (with `newsletter:suppress`: a suppression list) |
| `newsletter:export` | CSV of the list, one person's data |
| `newsletter:erase` | erase a person's data |
| `newsletter:suppress` | add or lift a suppression |
| `newsletter:settings` | SMTP connection test, the audit trail (Activity) |

`newsletter:*` holds all of them. Every route states its permission in
`src/routes.ts`; the full route table, `/api/me`, the panel's behaviour and
the variables are in [docs/ACCESS.md](docs/ACCESS.md). A missing permission
answers `403 {"error": "forbidden", "permission": "…"}`; the panel hides what
the account cannot use and names what is missing. The cron and the queue
need no session; the hooks stay public.

**The audit trail.** Every state-changing admin request is recorded in the
worker's own table `admin_audit`: who (the `auth` account, or just
`password` / `access`), what (`campaign.schedule`, `subscriber.erase`, …),
on what (an id — never an address, so an erasure leaves nothing of the
person), when, and the answer. Panel: **Activity**; API: `GET {base}/api/audit`
(`newsletter:settings`). Kept `NEWSLETTER_AUDIT_RETENTION_DAYS` (365) days.

**Switching from the password to `auth`:**

1. Deploy `auth` on the same host as the panel, invite the people, give each
   a role with the permissions above.
2. Set `NEWSLETTER_AUTH_ISSUER`, `NEWSLETTER_AUTH_LOGIN_URL` and
   `NEWSLETTER_AUTH_PUBLIC_KEY` in `wrangler.toml`; deploy. The panel now
   sends a browser without a session to the login page.
3. Sign in as each kind of account and check what it sees.
4. **Remove the password when you switch:** `npx wrangler secret delete
   NEWSLETTER_ADMIN_PASSWORD` (and `npx wrangler pages secret delete
   NEWSLETTER_ADMIN_PASSWORD` for the `newsletter` template). While `auth` is
   configured the password is not accepted — accepting it would bypass every
   permission — but if the `NEWSLETTER_AUTH_*` variables were ever removed,
   it would quietly become the way in again. Nothing removes it for you.

## Sending and its guarantees

Schedule a campaign and it goes out: every minute the cron starts due
campaigns and cuts the segment into id ranges; the queue consumer sends one
range per invocation through one mailer session (one SMTP connection, or the
HTTP provider), at `NEWSLETTER_SEND_RATE` for the whole worker. The details,
step by step, are in [SENDING.md](docs/SENDING.md). What you can rely on:

- **At most once per person per campaign.** A ledger row is claimed
  immediately before that person's message goes out, in one statement with a
  last check (still `confirmed`, not suppressed, campaign still `sending`).
  A redelivered job skips everyone already handled.
- **A crash never sends twice by itself.** If a consumer dies between the
  provider's "accepted" and the database write, that one row becomes
  **`uncertain`** after `NEWSLETTER_SEND_STALE_MINUTES`: not retried, not
  counted, shown in the panel, where an admin re-sends or dismisses it. A
  possible duplicate is treated as worse than one visibly missing message.
- **Idempotency per transport.** Every campaign message carries the key
  `{campaignId}:{subscriberId}`. Resend honours it as `Idempotency-Key` for
  24 hours, so a re-send within that window cannot make a second copy. The
  MailChannels Email API documents no such mechanism; over SMTP the key only
  becomes a deterministic `Message-ID`, a hint some receivers de-duplicate on.
  The panel says, per row, whether a re-send is safe.
- **Nothing lost.** A job that fails is retried by the queue; one that runs
  out of retries lands in the dead-letter queue, is counted, and its range is
  re-queued by the cron.
- **Circuit breaker.** When one batch gets at least
  `NEWSLETTER_BOUNCE_BREAKER_MIN` hard bounces that are also at least
  `NEWSLETTER_BOUNCE_BREAKER_PERCENT` % of its messages, the campaign pauses
  and nobody from that batch is suppressed: a server refusing everyone is a
  configuration problem, not a list problem. A render error or a transport
  that refuses the login pauses it too, with the reason in the panel. Hard
  bounces a provider reports later by webhook, attributed to a campaign that
  is still sending, go through the same rule and pause it the same way
  ([BOUNCES.md](docs/BOUNCES.md#webhook-bounce-breaker)).
- **Unsubscribes take effect at once**, mid-campaign included.

## Bounces and webhooks

The suppression list fills itself from the SMTP reply while sending
(classified conservatively: a refusal caused by the sender never suppresses
anyone), from Resend's and MailChannels' signed webhooks at
`{base}/hooks/resend` and `{base}/hooks/mailchannels`, and from a CSV for SMTP
users whose bounces arrive as mail. A hard bounce or a complaint suppresses at
once (a complaint also unsubscribes); soft bounces only after
`NEWSLETTER_SOFT_BOUNCE_LIMIT` in a row. A webhook without its secret
configured answers `503`, never accepts unsigned events, and a redelivered
event counts once. Classification tables and the registration steps:
[BOUNCES.md](docs/BOUNCES.md).

A report counts for a campaign only when it names the campaign: SMTP
refusals while sending, and Resend events that carry the
`newsletter_campaign` tag, which the Resend transport sends with every
campaign message. MailChannels events name no campaign and are never
attributed. Every verified webhook, `delivered`
included, is counted per hour, so Settings and `GET /api/health` show when
the last one arrived, with a warning when a webhook transport has been
silent since the last campaign started
([BOUNCES.md](docs/BOUNCES.md#webhook-health)).

## Import

Upload a list exported from any common mailing tool as it is: the delimiter,
encoding, header row, columns (by a table of the header names those tools use), status words and
date formats are detected, and the preview shows what each row will become
before anything is written. Names are recognised and dropped (data
minimisation). An import never undoes an unsubscribe, never touches an
existing row, sends no mail, and marks people `confirmed` only when you say
where they consented. Everything else: [IMPORT.md](docs/IMPORT.md).

## Campaign images

Optional. Images in a campaign must be public `https:` addresses. The site's
own pages are a poor place for them: bot protection may answer a webmail image
proxy with `403`, and an author who has a photo but no page for it has nowhere
to put it. With an R2 bucket bound to the worker, the panel stores the
image and gives it an address of its own. Without the binding nothing
changes: every asset route answers `404 assets_disabled` and mail renders byte
for byte as before.

### Turn it on

Create the bucket, then uncomment the `[[r2_buckets]]` block in
`wrangler.toml` (the binding name is `NEWSLETTER_ASSETS`). The binding is the
only switch.

| Name | Kind | Default | When unset or out of range |
|---|---|---|---|
| `NEWSLETTER_ASSETS` | R2 binding | — | the feature is off; the asset routes answer `404 assets_disabled` |
| `NEWSLETTER_ASSETS_URL` | var | — | this worker serves the images at `{NEWSLETTER_SITE_URL}{base}/hooks/a/`; set, it must be an `https://` base without credentials, query or fragment (an R2 custom domain) |
| `NEWSLETTER_ASSETS_MAX_BYTES` | var | `2097152` (2 MiB) | not a positive integer: the default; above 10 MiB (`10485760`): 10 MiB |
| `NEWSLETTER_IMAGES` | Images binding | — | no width cap: files are stored as uploaded (the `[images]` block in `wrangler.toml`) |
| `NEWSLETTER_ASSETS_MAX_WIDTH` | var | `1200` | not a positive integer: the default; clamped to 320–4000; read only with `NEWSLETTER_IMAGES` |

An address that cannot be built is a configuration problem, never a silent
fallback to another address: `NEWSLETTER_ASSETS_URL` that is not `https://`,
or, without it, no `https://` `NEWSLETTER_SITE_URL` (an `http://localhost`
preview included: the sanitizer keeps `https:` images only, and that rule is
not widened; set `NEWSLETTER_ASSETS_URL` for a preview). Then the three
`/api/assets` routes answer `503 assets_misconfigured {detail}`, and
`GET /api/settings` (`assets.problem`) and the module summary
(`assets_unusable`) say why. `GET /api/settings` also reports `assets.enabled`,
`assets.baseUrl`, `assets.maxBytes` and `assets.maxWidth` (`null` without
`NEWSLETTER_IMAGES`).

### Where images are served from

| | Worker route (`NEWSLETTER_ASSETS_URL` unset) | R2 custom domain (`NEWSLETTER_ASSETS_URL` set) |
|---|---|---|
| Image address | `{NEWSLETTER_SITE_URL}{base}/hooks/a/{key}` | `{NEWSLETTER_ASSETS_URL}/{key}` |
| Who answers | this worker, one R2 read per load | R2; the request never reaches the worker |
| Worker requests per load | 1, counted toward the daily limit of the plan | 0 |
| Response headers | set by the worker (below) | set by R2 and the domain, not by this worker |

R2's own storage and operation pricing applies either way:
[R2 pricing](https://developers.cloudflare.com/r2/pricing/).

The worker route is public, like the click redirect, because a mail client or
its image proxy fetches it without credentials. Only a name the store itself
writes (`<64 hex>.jpg|.png|.webp`) reaches the bucket; any other path is
`404` before R2 is asked. The answer carries the stored content type,
`cache-control: public, max-age=31536000, immutable`,
`x-content-type-options: nosniff`, an `etag`, and `304` on a matching
`If-None-Match`; `HEAD` answers the same headers.

### Upload rules

- **Accepted.** JPEG, PNG and WebP, recognised by their first bytes; the
  `content-type` header and any file name are ignored. SVG, GIF and everything
  else get `415 unsupported_type`. The file is the raw request body (no
  multipart).
- **Size.** A body over `NEWSLETTER_ASSETS_MAX_BYTES` gets `413 too_large`
  (refused by `content-length` when sent, and counted on the bytes actually
  read either way); an empty one `400 empty_body`. With the width cap the limit
  applies to the file as uploaded.
- **Stored once.** The key is the SHA-256 of the stored bytes plus the
  extension, so uploading the same file again stores nothing new and answers
  `200` with the existing record (`201` for a new one). Objects are written
  with `cache-control: public, max-age=31536000, immutable`.
- **Location in photos.** `metadata: true` means the file carries Exif or XMP
  (JPEG APP1, PNG `eXIf`, WebP's EXIF/XMP flags), where phones keep the
  location a picture was taken. The worker does not strip or rewrite it (only
  a width-capped copy, below, loses most of it as a side effect). The panel
  warns, and the author decides whether to upload a cleaned copy.
- **Deleting.** `DELETE /api/assets/:key` is refused with `409 asset_in_use`
  while any campaign's body or text version contains the key, whatever the
  campaign's status: a sent mail stays in inboxes indefinitely, and a draft
  would break too. An unknown key is `404`.
- **When R2 and the table disagree.** The `assets` table is the listing, R2
  holds the bytes. Upload writes R2 first, then the row; delete removes the R2
  object first, then the row. A failure between the two leaves either an
  unlisted object (the repeat upload lists it) or a listed image without
  bytes (the repeat delete finishes; a repeat upload puts the bytes back), so
  every step can be repeated.

### Width cap (optional)

With the Cloudflare Images binding bound as `NEWSLETTER_IMAGES`, an upload
wider than `NEWSLETTER_ASSETS_MAX_WIDTH` (default `1200`, twice a 600 px mail
column) is stored as a copy of that width. A narrower file, or one whose header
gives no width, is stored as uploaded, and nothing is enlarged. The type is
kept: JPEG and WebP at quality 80, PNG without a quality setting (for PNG it
would switch to the lossy palette variant); WebP is not converted, so its
uneven support in mail clients stays the author's choice.

The key is the hash of the stored copy, and the row remembers the upload's
hash (`source_sha256`, schema version 8), so the same file uploaded again
answers `200` with that copy instead of making another; a transform need not
give the same bytes twice. A file stored before the binding was set stays as
stored. The binding keeps only the Exif copyright tag by default, and
`metadata` describes the stored bytes, so a resized photo usually reports
`false`.

With the binding, the upload answer carries `resize: {resized, maxWidth,
original: {width, height, bytes}, error}`; without it the field is absent. If
the transform fails or returns something unexpected, the original is stored and
`resize.error` is `transform_failed` or `unexpected_output`.

Each transform is a billable Images operation (unique transformations, see
[Images pricing](https://developers.cloudflare.com/images/pricing/)); a repeat
upload of a stored file makes none.

### Pre-send image check

`GET /api/campaigns/:id/images?language=` renders the campaign with the real
renderer and sanitizer and lists the images of the mail a reader gets, in order.
Each item is `{url, alt, stored, probe, problems}`, and the answer adds
`checked` (distinct images probed) and `skipped` (distinct images not probed).
Problems:

- `not_stored`: the store is on, has a usable address, and the image is not
  under its base URL (a misconfigured store is reported in the settings, not
  on each image);
- `no_alt`: the alt text is missing or blank;
- `unreachable`: the probe did not get `200` with an `image/*` type.

By itself it warns and never blocks (to make findings block, see "Requiring
images to pass" below). The campaign is not changed, nothing about the check is stored, and it works with the store
off (then `not_stored` is never reported). It needs `NEWSLETTER_SITE_URL`
(`503 no_site_url` otherwise).

How an image is probed:

- a `GET` with the User-Agent
  `ssg-newsletter-image-check/1.0 (mail image proxy test; GoogleImageProxy)`,
  then the same with `YahooMailProxy` (the second only when the first
  passed): a labelled agent that carries the token of a webmail image proxy,
  so a bot rule keyed on that token is exercised;
- one redirect, `https:` only; 5 s per request; only the headers are read;
- at most 8 distinct images per call, four at a time (at most 32
  subrequests, within the 50 a Workers Free invocation allows). The rest are
  listed with `probe: null`, counted in `skipped`, and still judged on alt text
  and address;
- an image this worker serves itself (`{site}{base}/hooks/a/…`) is judged from
  the R2 object (`probe.via: "store"`) instead of a request to its own route.

The `probe` object is `{via, ok, status, contentType, error, agent}`; its
fields are in [API.md](docs/API.md#pre-send-image-check).

What a pass does not prove: for an image served here, only that the bytes are
in the bucket, not that the zone's bot rules let a proxy through; for a
fetched one, that a request from Cloudflare's network got an image, so a
rule keyed on the real proxies' IP ranges or verified-bot status is not
exercised. Authors see the check where they prepare a campaign
([SENDING.md](docs/SENDING.md#before-you-schedule)).

### Requiring images to pass

`NEWSLETTER_IMAGES_REQUIRE` (#448) turns chosen findings of the check into a
refusal to schedule. Unset or empty, nothing changes: every response is the
same as without it. Each rule enforces problem codes of the check:

| Rule | Refuses | Notes |
|---|---|---|
| `alt` | `no_alt` | alt missing, empty or only whitespace, judged on the rendered, sanitized mail: Markdown `![](…)` and `![  ](…)`; in HTML a missing `alt`, a bare `alt`, `alt=""`, `alt=''`, `alt="  "`, `alt=` with no value, `alt="&nbsp;"`, and an alt of only invisible characters (soft hyphen, zero-width space and joiners). No network. |
| `store` | `not_stored` | the image is not under the store's base URL. Needs the store on with a usable address. No network. |
| `load` | `unreachable`, `not_probed` | the probe above must pass. One check probes at most 8 distinct images (the Workers Free budget of 50 subrequests); an image beyond that was never shown to load and fails with `not_probed`. Use at most 8 different images, or leave `load` out. |

What is gated: only `POST /api/campaigns/:id/schedule` (draft → scheduled),
the one move that commits a draft's content to the list. Only drafts can be
edited, so resume (paused → sending) and the send engine's scheduled →
sending carry content that was already checked when it was scheduled; a
campaign scheduled before the switch was set is not checked again (unschedule
and schedule it to check it). A test send and saving a draft are never
refused, so the author can look at the mail while fixing it.

The refusal is `409 {error: "image_check_failed", detail, items, rules}`:
`items` are the failing images in the shape of the check's items, each with
only its enforced problems; `rules` is the enforced list. The campaign stays a
draft, unchanged. With `load`, the probe takes up to a few seconds; it runs
before the state moves, and the move only succeeds if the draft was not saved
in the meantime (`409 campaign_changed` otherwise: check and schedule again).
A second click while the first request runs gets `409 invalid_transition`
once the first has scheduled.

The mail is checked as rendered in the campaign's first segment language,
else `en`. An image address built with `{{language}}` is therefore checked
for that one language only.

A network failure during the probe (`timeout`, `fetch_failed`) refuses the
schedule like any failed image; try again once the image host answers. The
limits of a pass above still hold: `load` proves what a request from
Cloudflare's network got, not what every mail proxy will get.

Fail closed: an unknown word, or `store` while the store is off or has no
usable address, is a configuration problem. `GET /api/settings` reports it in
`imageRules.problem`, the module summary lists `image_rules_unusable`, the
panel's dashboard shows it, and every schedule answers
`503 {error: "image_rules_unusable", detail}` until the value is fixed. The
check's own answer then carries `rules`, `rulesProblem` and, per item,
`blocking` (the problems that refuse scheduling) — those fields are absent
while the switch is unset.

### Nothing is counted

Serving an image reads and stores nothing about the request: no IP,
User-Agent or cookie, no per-load row. It is not open tracking, and every
reader gets the same URL. See also "Statistics and privacy" below.

### Not verified

Against a live Cloudflare account, none of this was exercised: R2 and Images
are stand-ins in the tests. The Images stand-in follows the documented call
chain ([Optimize with Workers](https://developers.cloudflare.com/images/optimization/binding/)),
so what a real transform returns for a given file, and what Cloudflare bills
for it, is not confirmed. The probe was tested against stubbed responses, so
how a real bot rule answers it is not known.

## Content rules

Optional house rules a campaign must pass before it is scheduled (#449). All
three are off unless set in `wrangler.toml`; with none set, no answer and no
mail differs from a worker without them. They live in `wrangler.toml` only:
the panel is read-only about configuration, so a panel session cannot loosen
them. They are a check of the site's own rules, not a spam filter, and they
say nothing about whether a mail is delivered.

What is checked is what the author wrote, rendered by the real renderer with
the preview token: the subject, the body as readers get it, and the text
version when it is written by hand (`textOverride`). The footer the worker
adds (the unsubscribe line) is never checked. There is no preheader field in
this worker, so there is nothing else to check.

- **`NEWSLETTER_BANNED_PHRASES`** — phrases separated by `|`, each trimmed;
  empty items are dropped, items that differ only in case count once. A
  phrase is compared literally (it is never a regular expression), in any
  case (Unicode simple case folding: `ß` does not match `SS`), as a whole
  word or run of words. A phrase edge that is a letter, a digit or a
  combining mark must not touch another one in the text, which is right for
  Polish and Hindi where `\b` is not: `kup teraz` is not found in
  `zakup teraz`, and `कर` is not found in `करें`. Both sides are normalised to
  NFC, soft hyphens and zero-width spaces are removed, and any run of
  whitespace (line breaks included) counts as one space. Scanned: the
  subject; the body's words, link texts and image alt texts; the
  hand-written text version. Not scanned: addresses (`href`, `src`, and a URL
  written out in the text, `https://…`, `http://…`, `mailto:…`), `title`
  tooltips, the footer. When the text version is generated from the body it
  is not scanned again. Bounds: at most 200 phrases of at most 100
  characters; matching is linear in the length of the mail.
- **`NEWSLETTER_BLOCKED_IMAGE_PATTERNS`** — substrings separated by `|`. An
  image of the rendered mail whose URL contains one, compared in any case
  (host and path alike: on most servers `/WP-Content/` and `/wp-content/`
  are the same files, and a false hit costs one edit), is a finding naming
  the image and the first pattern it contains. The URL is compared as the
  mail has it (after the sanitizer). Bounds: at most 50 patterns of at most
  200 characters.
- **`NEWSLETTER_UTM_STRICT=on`** — with `NEWSLETTER_UTM` on, the worker
  leaves a link that already has a `utm_*` parameter exactly as written
  ("Statistics and privacy"). With strict mode on, such a link to a host the
  worker tags (the site's host plus `NEWSLETTER_UTM_HOSTS`) is a finding
  naming the link and its `utm_*` parameters instead; the worker never
  rewrites or replaces them. The links checked are exactly those UTM tagging
  is offered (http(s) links of the body and of a hand-written text version,
  never the unsubscribe link or `mailto:`). With `NEWSLETTER_UTM` off strict
  mode has nothing to compare with, so it is a configuration problem rather
  than a silent no-op.

**When it runs.** `POST /api/campaigns/:id/schedule` runs it after the
content checks it already had and before the image rules ("Requiring images
to pass"), rendered in the segment's first language (else `en`; only
`{{language}}` depends on it). A finding answers
`409 {error: "content_check_failed", detail, findings}` and the campaign stays
a draft. Each finding is `{rule, where, value, …}`:

| `rule` | `where` | `value` | more fields |
|---|---|---|---|
| `banned_phrase` | `subject`, `body`, `text` | the phrase as configured | `match` (the first occurrence as the mail spells it), `count` |
| `blocked_image` | `body` | the image URL | `pattern` |
| `utm_conflict` | `body`, `text` | the link as written | `host`, `parameters` (the `utm_*` names as written) |

Not affected: saving a draft, test sends, previews. Not re-checked: resume
(`paused → sending`) and re-sending uncertain sends continue a send whose
content was checked when it was scheduled and can no longer be edited —
refusing them would leave a half-sent campaign that can only be cancelled. A
campaign already scheduled when the rules change is not re-checked either:
unschedule and schedule it again to apply them.

**Fails closed.** A configuration that cannot be honoured (too many or too
long phrases or patterns, `NEWSLETTER_UTM_STRICT` with a value that is
neither on nor off, or on with `NEWSLETTER_UTM` off) makes every schedule
answer `503 content_rules_invalid` with the reason, never a check that runs
with part of the rules. The reason is in `GET /api/settings`
(`contentRules.problem`), in the module summary's problems
(`content_rules_invalid`) and on the panel's overview.

**Reading it.** `GET /api/campaigns/:id/content-check?language=` (read
permission) answers `{findings, rules}` without changing anything; the
panel's "Content rules" card in the editor uses it. `rules` and
`GET /api/settings` → `contentRules` say
`{active, bannedPhrases, blockedImagePatterns, utmStrict, problem}`: counts
and switches, never the phrase list. A reader-level account sees only the
phrases its campaign actually contains; the list itself stays with whoever
can read `wrangler.toml`, since it may hold wording a site does not want
shown to every panel account.

## Statistics and privacy

Counts per campaign, from data the worker already keeps — every figure is a
count, and no table the statistics use links a click or an open to anyone.

- **Recipients:** total, sent, failed, skipped, pending and uncertain, from
  the send ledger.
- **Unsubscribes — an attribution by time window, not an exact count.** The
  unsubscribe link belongs to the `newsletter` template and carries only the
  reader's token, so nothing records which mail it was clicked in. An
  unsubscribe is credited to the most recent campaign actually sent to that
  subscriber before their `unsubscribed_at`, if that send is at most
  `NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS` old. Any unsubscribe counts — the
  link, an admin, a complaint. Someone erased since, or who signed up again,
  is no longer counted.
- **Clicks**, only with `NEWSLETTER_TRACK_CLICKS` on: every `http(s)` link is
  rewritten to `{NEWSLETTER_SITE_URL}{base}/hooks/c/{campaignId}/{linkId}`,
  `linkId` being the first 16 hex digits of `sha256(url)` — the same in every
  recipient's mail. Never rewritten: the unsubscribe link, `mailto:` links,
  images; at most 100 distinct links per campaign. The redirect goes only to a
  target stored for that campaign (not an open redirector), reads and stores
  nothing about the request, and `HEAD` is not counted. Scanners follow links
  too and nothing is de-duplicated, so read clicks as a relative signal
  between links and campaigns, not as a number of people. With tracking off
  the mail is byte for byte what it was.
- **UTM parameters**, only with `NEWSLETTER_UTM` on (to refuse links that
  already carry hand-written `utm_*` instead of leaving them, see
  `NEWSLETTER_UTM_STRICT` in "Content rules"): a link whose host is the
  site's (the host of `NEWSLETTER_SITE_URL`, plus `NEWSLETTER_UTM_HOSTS`;
  hosts are compared whole, so list `www.example.com` beside `example.com`)
  gets `utm_source` (`NEWSLETTER_UTM_SOURCE`, default `newsletter`),
  `utm_medium=email`, `utm_campaign` (a slug of the subject: ASCII lower case
  with dashes, at most 60 characters; the campaign id when the subject leaves
  nothing) and `utm_content` (the `linkId` above, computed from the link as
  written). They name the campaign and the link, not the reader, and add no
  table and no stored data. Never tagged: a link that already has any `utm_*`
  parameter, other hosts, the unsubscribe link, `mailto:` links, images, the
  confirmation mail, and links past the first 100 of a campaign. Tagging runs
  before the click rewrite, so with click counting on the stored redirect
  target carries the parameters, and the preview and a test send show what
  readers get. A link whose target was stored before `NEWSLETTER_UTM` was
  turned on keeps its stored target for that campaign. With it off the mail
  is byte for byte what it was.
- **Images are not a tracking channel.** A campaign image has the same URL
  for every reader and its route reads and stores nothing about the request
  ("Campaign images"), so loading one is not counted as an open.
- **Bounces and complaints per campaign — only when the report names the
  campaign.** Hard, soft and complaint counts come from `campaign_bounces`:
  SMTP refusals while sending, and webhook events that carry the campaign's
  tag. A report that names no campaign (MailChannels, mail sent before
  attribution) is never credited by address or time; the figures are then
  `null` with the reason ([BOUNCES.md](docs/BOUNCES.md#attribution-to-a-campaign)).
- **Not counted, by design:** opens (no tracking pixel — privacy proxies make
  open rates meaningless, and a pixel only works by giving each person their
  own URL) and who clicked.

Personal data: subscriber rows are the `newsletter` template's; this worker
adds the send ledger (who got which campaign, when), suppressions (an
address, or only its SHA-256 after an erasure), soft-bounce counts and hashed
IPs of failed logins (kept for the lockout window). An erasure removes the
person's row and sends in one transaction and leaves only the hash, so an
import cannot bring them back. Say so in your privacy policy.

## The panel and local preview

`{base}/` in a browser: overview, subscribers, import, campaigns with an
editor and live progress, suppressions, settings, activity, help — in English, Polish,
Hindi, German and French, light and dark, no framework, nothing loaded from
another origin. Details, security model and charts: [PANEL.md](docs/PANEL.md).

Try it locally with invented data, the real worker on an in-memory database
and a pretend SMTP server:

```sh
node --import ./workers/newsletter-ui/test/resolve.mjs workers/newsletter-ui/test/dev-server.mjs
# → http://localhost:8787/newsletter-admin/  (any user name, password "demo")
```

`DEMO_AUTH=1` runs it with `auth` accounts instead of the password: pick an
owner, editor, sender, reader or an account with no permission at
`/auth/login`, and see what each one is shown.

`DEMO_ASSETS=1` runs it with the image store on, so the upload, the library
and the image check can be tried. With the store on, the panel's
`Content-Security-Policy` allows images from the store's origin, and only
from it, so thumbnails and the preview can show them; see
[docs/PANEL.md](docs/PANEL.md).

## Abuse protection

Three layers, each against something the others cannot see. The template is
safe with only the second and third; the first is optional.

| Layer | Where | Against |
|---|---|---|
| WAF / Rate Limiting rules | Cloudflare dashboard, in front of the worker | volume, cut off before the worker runs (no request billed); not versioned with the site |
| `RATE_LIMITER` binding | `wrangler.toml` (`[[ratelimits]]`, 120 requests per 60 s per IP) | the same, versioned and scaffolded with the site; counted per Cloudflare location and approximate, by Cloudflare's own description |
| Limits in code | the worker | what Cloudflare cannot know: failed logins per IP (`auth_attempts`), 10 test sends per hour for the whole worker, one confirmation mail per address per 15 minutes, the send rate, import size, webhook body size and signature age |

`{base}/hooks/` is outside the per-IP binding on purpose: providers deliver
webhooks from a few shared addresses, and a per-IP cap would drop their
events. Their signatures protect them instead; the click redirect carries no
person and leads only where the mail already pointed.

## Limits per Cloudflare plan

Checked against Cloudflare's documentation on 2026-11-13; plans change, so
follow the links before relying on a number.

| Limit | Workers Free | Workers Paid | Source |
|---|---|---|---|
| Requests | 100 000 / day | no limit | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| CPU time per invocation | 10 ms | 30 s default, up to 5 min (`limits.cpu_ms`) | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| Subrequests per invocation (`fetch` to a provider) | 50 | 10 000 default, configurable | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| D1 queries per invocation | 50 | 1 000 | [D1 limits](https://developers.cloudflare.com/d1/platform/limits/) |
| D1 rows written | 100 000 / day (00:00 UTC) | 50 million / month included | [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/) |
| D1 rows read | 5 million / day | 25 billion / month included | [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/) |
| D1 per statement | 100 bound parameters, 100 KB of SQL, 30 s | same | [D1 limits](https://developers.cloudflare.com/d1/platform/limits/) |
| Queues operations | 10 000 / day | 1 million / month included | [Queues pricing](https://developers.cloudflare.com/queues/platform/pricing/) |
| Queue consumer wall-clock | 15 min | 15 min | [Queues limits](https://developers.cloudflare.com/queues/platform/limits/) |
| Cron triggers | 5 per account | 250 per account | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| Outbound TCP (`cloudflare:sockets`) | port 25 blocked; Cloudflare IP ranges blocked; 6 connections open at once | same | [TCP sockets](https://developers.cloudflare.com/workers/runtime-apis/tcp-sockets/) |

What this means here:

- **Every index update is a row written.** D1 counts each index an insert or
  update touches as an extra row. A recipient costs about **7 rows**: the
  ledger row and its three index entries when it is claimed, the row and two
  index entries when the result is written — plus a share of each job's
  bookkeeping. This, not the send rate, is what bounds Workers Free.
- **A queue job costs 3 operations** (written, read, deleted; a retry adds a
  read); the rate wait re-queues a job, which costs 3 more.
- **Batch size.** One job per invocation (`max_batch_size = 1`). The engine
  makes 2 D1 queries per message plus about 10 per job, and an HTTP
  transport one `fetch` per message: a batch of 10 is about 30 queries and 10
  subrequests, inside Free's 50 and 50; a batch of 100 is about 210 queries,
  inside Paid's 1 000. `NEWSLETTER_CONFIRM_BATCH` (10) fits Free the same way.
- **The panel counts as requests.** `run_worker_first = true` sends every
  panel file through the worker (so it sits behind the login), and each such
  request counts toward Free's daily 100 000.
- **Campaign images.** An image the worker serves is one request per load
  against the daily 100 000 and one R2 read; one on an R2 custom domain is
  none ("Campaign images"). The pre-send check makes at most 32 subrequests
  in a call, inside Free's 50.
- **Not verified:** whether Workers Free's 10 ms CPU limit applies to a queue
  consumer as it does to a request (the Queues limits page lists a 30 s
  default without a Free column); whether the subrequest limit counts a
  consumer or cron invocation differently from a request (the docs say only
  "per invocation"); and whether the rate-limit binding is offered on the Free
  plan (the binding's page does not say). A Free-plan campaign that hits the
  CPU limit fails its job, which the queue retries; nothing is sent twice.

**How long 50 000 recipients take.**

| Setup | Time | Bound by |
|---|---|---|
| Defaults (60 a minute, batch 10), Workers Paid | ≈ 14 h (50 000 ÷ 60 = 833 min; 5 000 jobs) | the send rate |
| Workers Paid, `NEWSLETTER_SEND_RATE=480`, `NEWSLETTER_SEND_BATCH=80` | ≈ 1 h 45 min (625 jobs, ~170 D1 queries each) | the send rate; 480 a minute is 8 per second, under Resend's documented default of 10 per second per team — check your provider |
| Workers Free, defaults | ≈ 4 days | D1's 100 000 rows written a day ÷ ~7.5 rows per recipient ≈ 13 000 recipients a day (Queues would allow about 3 300 jobs ≈ 33 000 a day) |

On the free plan a campaign that exhausts the day's quota stalls — its jobs
fail and are retried or dead-lettered — and resumes from the ledger once the
quota resets at 00:00 UTC; nobody is mailed twice. Plan Workers Paid for lists
beyond about 10 000, or for sending in one evening.

## Schema: `subscribers` is never altered

`subscribers` belongs to `newsletter` and stays at its schema v1. This worker
never creates, alters, drops or indexes it, and it adds no columns or
statuses; it reads rows and changes them row by row (unsubscribe, erase,
import, a complaint). Everything else lives in its own tables, all `CREATE
TABLE IF NOT EXISTS`; nothing is ever dropped, and a new column arrives as a
guarded `ALTER TABLE … ADD COLUMN` listed in `MIGRATIONS` in `src/schema.ts`.
The result: it works on a database an existing `newsletter` deployment
already filled, with no migration, and a site that only runs `newsletter`
behaves exactly as before. `test/compat-subscribers.test.ts` proves it on the
table exactly as the 1.9.7 template created it — every route, the cron, a
whole campaign, an import, a webhook and an erasure leave its definition
untouched.

If the table does not exist yet, the API answers `503` with an instruction to
deploy `newsletter` first; `GET {base}/api/health` still answers and reports
`"subscribersTable": false`.

| Table | Holds |
|---|---|
| `schema_meta` | the schema version per component (`newsletter-ui` = 9) |
| `settings` | panel settings as JSON values (and the test-send counter); never secrets |
| `campaigns` | subject, body, segment, status, schedule, counters, source post |
| `campaign_sends` | the send ledger: one row per campaign and subscriber — `claimed` → `sent` / `failed` / `retry` / `skipped`, or `uncertain` |
| `suppressions` | addresses not to mail, by hash: `hard`, `soft`, `complaint`, `manual` |
| `auth_attempts` | failed Basic logins per hashed IP |
| `send_plans`, `send_batches` | per campaign: the keyset cursor and the reason the engine paused it; per job: the id range and its state |
| `send_rate` | messages handed to the transport per UTC minute |
| `send_dead_letters` | queue jobs that ran out of retries |
| `soft_bounces` | soft bounces in a row per address, below the limit |
| `hook_events` | webhook event ids already applied, kept 7 days |
| `campaign_bounces` | hard / soft / complaint counts attributed to a campaign, per source — no addresses |
| `hook_breaker` | per campaign: webhook hard bounces since the breaker's last trip, and the sent count at that trip |
| `hook_activity` | verified webhook events per UTC hour, provider and class, with the latest time — counts only, kept 8 days |
| `click_links` | per campaign link: the target and a counter — no column that can point at a person |
| `admin_audit` | the audit trail of admin actions ("Who can do what") |
| `assets` | campaign images in the optional R2 bucket: key (SHA-256 + extension), type, size, dimensions, whether Exif/XMP is present, and the upload's hash behind a width-capped copy (`source_sha256`) — the bytes are in R2 |

## Files

```
workers/newsletter-ui/
├── wrangler.toml     the Worker: D1, cron, queue + dead-letter queue, rate limit, assets, optional R2
├── schema.sql        this worker's own tables (created on first use anyway)
├── openapi.yaml      the API, OpenAPI 3.1
├── module.json       the module manifest, served at {base}/api/module
│                     (the summary: {base}/api/module/summary, src/module-summary.ts)
├── docs/             API, sending, import, bounces, panel
├── public/           the panel: index.html, css/, js/, img/
├── src/index.ts      entry points: fetch, scheduled, queue
├── src/app.ts        the request pipeline: base path → schema → hooks → rate limit → auth → CSRF → route / panel
├── src/routes.ts     the route registry, one line per route: permission and audit action included
├── src/auth.ts       admin sign-in: Access → auth → password; src/permissions.ts, src/me.ts
├── src/audit.ts      the audit trail: written from the pipeline, GET {base}/api/audit
├── src/session-verifier.ts  byte-identical copy of workers/auth/portable/session-verifier.ts
├── src/mail/         byte-identical copies of the `newsletter` template's mail modules
├── src/…             one feature per group of files: subscriber*, import*, campaign*, engine*,
│                     bounce*, hook*, suppression*, stats*, click*, feed*, confirm*, render*, settings,
│                     asset*, image-* (campaign images: config, type sniffing, width cap, API, serving; the pre-send check and its probe),
│                     content-rules* (the content rules checked before scheduling)
└── test/             node:test suites; fakes.ts runs D1 on real SQLite
```

`src/mail/` holds byte-identical copies of `newsletter`'s portable mail
modules (`_transport.ts`, `_smtp.ts`, `_smtpwire.ts`, `_mime.ts`), so a
scaffolded `newsletter-ui` stands alone; `test/mail-copies.test.ts` fails when
a copy drifts.

## Tests

From the repository root (Node 23.6 or newer, no npm packages), or
`make test-workers` for every worker template:

```sh
node --import ./workers/newsletter-ui/test/resolve.mjs --test 'workers/newsletter-ui/test/*.test.ts'
# coverage:
node --import ./workers/newsletter-ui/test/resolve.mjs --experimental-test-coverage \
  --test-coverage-include='workers/newsletter-ui/src/**' --test 'workers/newsletter-ui/test/*.test.ts'
```

`test/fakes.ts` runs D1 on `node:sqlite`, so tests execute real SQL. The
panel's logic is tested the same way (`test/panel-*.test.ts`, with a tiny DOM
stand-in); for its coverage add
`--test-coverage-include='workers/newsletter-ui/public/**'`.
`test/compat-subscribers.test.ts` is the backward-compatibility guarantee
above; `test/openapi.test.ts` keeps `openapi.yaml` and `src/routes.ts` in step
(each operation's `x-permission` included). `test/route-registry.test.ts`
runs every route against a session without its permission, with exactly
it and as the owner; `test/auth-copies.test.ts` keeps the verifier copy
byte-identical and runs `workers/auth`'s verifier suites against it.
Pull requests that change `workers/` run `make test-workers` in CI (the
`Workers` workflow).

**Extending.** A route is one line in `src/routes.ts`; the handler gets
`{request, env, ctx, url, params, admin}` and returns a `Response`, and the
pipeline adds auth, the permission check, CSRF, the audit row, headers,
404/405 and the missing-subscribers 503 (set `needsSubscribers: false` only
for a route that must work without the table). Every route names its
`permission` (one from `module.json`); a state-changing one also its `audit`
action, or `audit: false` when it changes nothing.
Document it in `openapi.yaml` — the test insists. A cron task goes in
`CRON_TASKS`, a queue job handler in `JOB_HANDLERS` (`src/jobs.ts`); a
throwing job is retried with backoff and dead-lettered after `max_retries`.

## Style and colour guidelines

The panel's rules, in short — the full list is in [PANEL.md](docs/PANEL.md#style-and-colour-guidelines)
and the tokens, with their reasoning, in `public/css/tokens.css`:

- Material colour tokens: Teal as the one primary hue, the warm Brown family
  as neutrals, Deep Orange only for the focus ring, a five-hue chart set
  (Teal, Deep Orange, Deep Purple, Pink, Blue) checked for colour-vision
  separation; an entity keeps its colour in every chart.
- System fonts only: a serif stack for headings, a humanist sans for the
  rest, tabular figures for numbers.
- Light and dark from the system, with a manual switch.
- WCAG 2.2 AA: text ≥ 4.5:1, controls and chart marks ≥ 3:1 —
  `test/panel-contrast.test.ts` checks every pair in both themes; a 3 px
  focus ring; targets ≥ 40 px; colour is never the only signal.

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, its panel and navigation, the permissions its admin surface distinguishes, and how to switch it off. It is a description only — no secrets, no configuration.

| Permission | Allows |
|---|---|
| `newsletter:read` | View subscribers, counts, campaigns and statistics. |
| `newsletter:edit` | Edit campaign drafts and their images, and a subscriber's tags and language. |
| `newsletter:send` | Send a test, schedule, pause and resume a campaign, and retry uncertain sends — mail that reaches real people. |
| `newsletter:import` | Import subscribers and suppression lists from a file. |
| `newsletter:export` | Download subscribers as CSV and export one person's data. |
| `newsletter:erase` | Erase a person's data for good (right to erasure). |
| `newsletter:suppress` | Add addresses to the suppression list and lift a suppression. |
| `newsletter:settings` | Maintenance: test the SMTP connection and run the retention purge now. |

With the shared password or Cloudflare Access whoever passes may do everything; with `auth` each action needs its permission (*Who can do what*).

It also serves the manifest at `GET {base}/api/module`, behind the worker's admin auth like every `/api/` route, without needing the `subscribers` table. `src/module.ts` imports `module.json` directly; wrangler bundles it.

### Summary

`GET {base}/api/module/summary` answers the module summary (format in [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md#the-summary-endpoint)) an admin home page shows: `confirmed`, `pending`, `campaignsInProgress` and `suppressed`; things to do — campaigns paused by the send engine (`action`, `/#/campaigns?status=paused`), uncertain sends (`action`, `/#/`), pending subscribers never sent a confirmation (`action`, `/#/subscribers?status=pending`), dead-lettered jobs (`warning`, `/#/`); the dashboard's configuration warnings as problems (`no_transport`, `transport_unusable`, `no_site_url`, `no_sender`, `no_queue`, and `no_subscribers_table`); and sign-ups per day for 30 days. Labels and problem texts are the panel's own strings in all five languages (`test/module-summary.test.ts` holds them equal). At most three D1 statements — the dashboard's status counts and sign-up series (`src/subscriber-stats.ts`) and one SELECT of scalar counts built from the engine's and the confirmation sender's own SQL — and never an address, subject or engine error text. Behind the worker's admin auth like every `/api/` route; answers without the `subscribers` table. Its `confirmed` and `pending` agree with the `newsletter` template's summary on the same database, and a home page prefers this one when both exist.
