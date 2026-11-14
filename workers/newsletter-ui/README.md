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
   "Mail transports" in the `newsletter` README.
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
| `NEWSLETTER_BOUNCE_BREAKER_MIN` | var | `5` | hard bounces in one batch before the campaign pauses … |
| `NEWSLETTER_BOUNCE_BREAKER_PERCENT` | var | `50` | … if they are also this share of the batch's messages |
| `NEWSLETTER_DEAD_LETTER_QUEUE` | var | `ssg-newsletter-send-dlq` | only if you renamed the queue |
| `NEWSLETTER_SOFT_BOUNCE_LIMIT` | var | `3` | soft bounces in a row before a suppression |
| `NEWSLETTER_RESEND_WEBHOOK_SECRET` | secret | — | the Resend webhook's `whsec_…`; unset → `/hooks/resend` answers 503 |
| `NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY` | secret | — | MailChannels' Ed25519 public key (PEM); unset → 503 |
| `NEWSLETTER_TRACK_CLICKS` | var | off | `true` counts clicks per link |
| `NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS` | var | `14` | how long after a send an unsubscribe is credited to it |
| `NEWSLETTER_FEED_URL` | var | `/feed.xml` on the site | the feed for "campaign from a post" |
| `NEWSLETTER_CONFIRM_BATCH` | var | `10` | confirmation mails per call, never more than the rate |
| `NEWSLETTER_IMPORT_MAX_ROWS` | var | `20000` | rows per CSV import (1 KiB of body per row) |

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
  that refuses the login pauses it too, with the reason in the panel.
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

## Import

Upload a list exported from any common mailing tool as it is: the delimiter,
encoding, header row, columns (by a table of the header names those tools use), status words and
date formats are detected, and the preview shows what each row will become
before anything is written. Names are recognised and dropped (data
minimisation). An import never undoes an unsubscribe, never touches an
existing row, sends no mail, and marks people `confirmed` only when you say
where they consented. Everything else: [IMPORT.md](docs/IMPORT.md).

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
- **Not counted, by design:** opens (no tracking pixel — privacy proxies make
  open rates meaningless, and a pixel only works by giving each person their
  own URL), who clicked, and bounces per campaign (a bounce reported later
  carries no campaign id, and the worker does not guess).

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
| `schema_meta` | the schema version per component (`newsletter-ui` = 5) |
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
| `click_links` | per campaign link: the target and a counter — no column that can point at a person |

## Files

```
workers/newsletter-ui/
├── wrangler.toml     the Worker: D1, cron, queue + dead-letter queue, rate limit, assets
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
│                     bounce*, hook*, suppression*, stats*, click*, feed*, confirm*, render*, settings
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
| `newsletter:edit` | Edit campaign drafts, and a subscriber's tags and language. |
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
