# Who can do what

The panel and the admin API answer to one of three sign-in modes. Two of them
open everything to whoever gets in; the third, accounts from the `auth`
worker, gives each person only the permissions their role holds and records
who did what. The [README](../README.md#who-can-do-what) has the short
version; this page is the reference.

## Which mode to choose

| Mode | Set | Who gets in | Permissions | Choose it when |
|---|---|---|---|---|
| **Shared password** | the secret `NEWSLETTER_ADMIN_PASSWORD` | anyone with the password (HTTP Basic, any user name) | all of them | one person runs the newsletter — the simplest setup, nothing else to deploy |
| **Cloudflare Access** | `NEWSLETTER_ACCESS_TEAM` + `NEWSLETTER_ACCESS_AUD` | whoever your Access policy lets in | all of them | you already run Cloudflare Zero Trust and want its sign-in |
| **`auth` accounts** | `NEWSLETTER_AUTH_*` (below) | accounts of the `auth` worker on the same host | each account's own | several people, or different permissions for different people |

The worker checks them in this order: **Access → `auth` → password**. The
first one configured decides; the ones after it are not consulted.

- With Access configured, `auth` and the password are ignored, as before.
- With any `NEWSLETTER_AUTH_*` variable set, the password is **not accepted**
  — even when it is still set. Accepting it would let anyone with the shared
  password bypass every per-action permission. A set that is incomplete
  (an issuer without a login URL, no key or two keys, a key that is not a
  public P-256 key) answers `503 {"error": "not_configured", "detail": "…"}`
  naming what is missing; it never falls back to the password.
- Nothing configured: `503`. The panel is never open by default.

The shared password stays a first-class mode: nothing about it changed, and
nothing in this worker depends on `auth`.

## The `auth` variables

The same names in this worker and in the `newsletter` template (they share
the `NEWSLETTER` prefix, as they share the Access variables), set in
`wrangler.toml` `[vars]`:

| Variable | Required | Meaning |
|---|---|---|
| `NEWSLETTER_AUTH_ISSUER` | yes | the `iss` to accept, e.g. `https://example.com/api/auth` |
| `NEWSLETTER_AUTH_LOGIN_URL` | yes | where a page without a session is sent, e.g. `/auth/login` |
| `NEWSLETTER_AUTH_PUBLIC_KEY` | one of these two | the `auth` public key: the `AUTH_PUBLIC_KEY=` line `scripts/auth-keys.mjs` prints, or the body of `/api/auth/jwks` (recommended: no request per token, works while `auth` is down) |
| `NEWSLETTER_AUTH_JWKS_URL` | one of these two | `/api/auth/jwks` (fetched, cached five minutes) |
| `NEWSLETTER_AUTH_AUDIENCE` | no | the `aud` to accept; default the request's host |

`NEWSLETTER_AUTH_MAX_FAILURES` and `NEWSLETTER_AUTH_WINDOW_MINUTES` are not
part of this set: they belong to the password mode's failed-login lockout and
do not switch `auth` on.

The worker checks the session token itself with the public key
(`src/session-verifier.ts`, a byte-identical copy of
`workers/auth/portable/session-verifier.ts`); it never asks `auth` per
request. A permission granted or revoked in `auth` reaches the panel when the
person's token is renewed — at most **10 minutes 30 seconds** later with the
default token lifetime (`workers/auth/docs/CONTRACT.md`, "How stale
permissions can be").

## Switching from the password to `auth`

1. Deploy the `auth` worker on the **same host** as the panel (the session
   cookie is per host) and invite the people who run the newsletter. Give
   each a role holding the `newsletter:*` permissions below that they need.
2. Copy the public key into `NEWSLETTER_AUTH_PUBLIC_KEY`, set
   `NEWSLETTER_AUTH_ISSUER` and `NEWSLETTER_AUTH_LOGIN_URL` in
   `wrangler.toml`, and deploy. From now on the panel sends a browser without
   a session to the login page; the password is no longer accepted.
3. Sign in as each kind of account and check what it sees.
4. **Remove the password** — `npx wrangler secret delete
   NEWSLETTER_ADMIN_PASSWORD` here, and `npx wrangler pages secret delete
   NEWSLETTER_ADMIN_PASSWORD` in the Pages project of the `newsletter`
   template. It is not accepted while `auth` is configured, but if the
   `NEWSLETTER_AUTH_*` variables were ever removed it would silently become
   the way in again. Taking it out is a separate step on purpose: nothing
   switches it off for you.

Back to the password: delete the `NEWSLETTER_AUTH_*` variables and set the
secret again. The audit trail keeps its rows.

## Permissions

Declared in [`../module.json`](../module.json) — the list the `auth` worker
shows when an owner builds a role. A role may also hold `newsletter:*` (every
one of them) or `*` (everything, every worker).

| Permission | Lets an account |
|---|---|
| `newsletter:read` | see the overview, subscribers, campaigns and their statistics, the suppression list, the configuration, a campaign's preview |
| `newsletter:edit` | write and change campaign drafts (also from a post), duplicate and delete them; change a subscriber's tags and language; unsubscribe someone |
| `newsletter:send` | send a test, schedule, unschedule, pause, resume and cancel a campaign, re-send or dismiss uncertain sends, mail confirmations to pending subscribers — everything that mails real people |
| `newsletter:import` | import a subscriber list from a file |
| `newsletter:export` | download subscribers as CSV, export one person's data |
| `newsletter:erase` | erase a person's data for good |
| `newsletter:suppress` | add an address to the suppression list, lift a suppression |
| `newsletter:settings` | test the SMTP connection, read the audit trail (Activity), run the `newsletter` template's retention purge |

Every route needs exactly what is listed here; `src/routes.ts` states it per
route and `test/route-registry.test.ts` refuses a route without one, or with
a permission `module.json` does not declare. An account that only holds
`newsletter:read` provably cannot call any route that changes something.

| Route | Permission | Audit action |
|---|---|---|
| `GET /api/health`, `/api/module`, `/api/module/summary`, `/api/settings` | `newsletter:read` | — |
| `GET /api/subscribers`, `/api/subscribers/:id`, `/api/stats/subscribers` | `newsletter:read` | — |
| `GET /api/campaigns`, `/api/campaigns/:id`, `…/progress`, `…/stats`, `…/uncertain`, `/api/stats/campaigns`, `/api/engine/status`, `/api/feed/posts` | `newsletter:read` | — |
| `GET /api/suppressions`, `/api/stats/suppressions` | `newsletter:read` | — |
| `POST /api/campaigns/:id/preview` | `newsletter:read` | — (changes nothing) |
| `GET /api/subscribers.csv`, `/api/subscribers/:id/export` | `newsletter:export` | — |
| `PATCH /api/subscribers/:id` | `newsletter:edit` | `subscriber.edit` |
| `POST /api/subscribers/:id/unsubscribe` | `newsletter:edit` | `subscriber.unsubscribe` |
| `DELETE /api/subscribers/:id` | `newsletter:erase` | `subscriber.erase` |
| `POST /api/subscribers/:id/confirmation` | `newsletter:send` | `subscriber.confirmation` |
| `POST /api/subscribers/confirmations` | `newsletter:send` | `subscribers.confirmations` |
| `POST /api/import/preview` | `newsletter:import` | — (changes nothing) |
| `POST /api/import` | `newsletter:import` | `subscribers.import` |
| `POST /api/campaigns`, `/api/campaigns/from-post` | `newsletter:edit` | `campaign.create`, `campaign.from_post` |
| `PATCH /api/campaigns/:id`, `DELETE …`, `POST …/duplicate` | `newsletter:edit` | `campaign.edit`, `campaign.delete`, `campaign.duplicate` |
| `POST …/test`, `…/schedule`, `…/unschedule`, `…/pause`, `…/resume`, `…/cancel` | `newsletter:send` | `campaign.test`, `campaign.schedule`, … |
| `POST …/uncertain/resend`, `…/uncertain/dismiss` | `newsletter:send` | `campaign.uncertain_resend`, `campaign.uncertain_dismiss` |
| `POST /api/suppressions`, `DELETE /api/suppressions/:hash` | `newsletter:suppress` | `suppression.add`, `suppression.lift` |
| `POST /api/suppressions/import/preview` | `newsletter:import` **and** `newsletter:suppress` | — (changes nothing) |
| `POST /api/suppressions/import` | `newsletter:import` **and** `newsletter:suppress` | `suppressions.import` |
| `POST /api/settings/smtp-test` | `newsletter:settings` | `settings.smtp_test` |
| `GET /api/audit` | `newsletter:settings` | — |
| `GET /api/me` | a session, nothing more | — |
| `/hooks/…` (webhooks, click redirect) | public — each verifies its own signature | — |

Two routes do two things. The suppression import writes suppressions in
bulk, so it needs both `newsletter:import` and `newsletter:suppress`. The
subscriber import also suppresses addresses the file itself marks bounced or
complained; that only stops mail and never sends any, so it needs
`newsletter:import` alone.

A missing permission is answered `403 {"error": "forbidden", "permission":
"newsletter:send"}`. In the Access and password modes every permission is
held and nothing about the answers changed.

## The panel

At start the panel asks `GET {base}/api/me` once:

```json
{"mode": "auth", "user": {"id": "…", "email": "ada@example.com"},
 "permissions": ["newsletter:read", "newsletter:edit"],
 "loginUrl": "/auth/login", "logoutUrl": "/api/auth/logout", "refreshUrl": "/api/auth/refresh"}
```

(`{"mode": "password" | "access", "user": null, "permissions": "all"}` in the
other modes.) It hides the navigation entries and the buttons the account
cannot use, shows who is signed in with a **Sign out** button, and a view the
account may not open says which permission is missing. Hiding is a courtesy:
the worker refuses every call on its own. When an API call answers `401` —
the access token lives minutes — the panel asks `auth` once for a new one and
retries; failing that it goes to the login page, which brings the admin back
to the same view.

## CSRF

Every state-changing request keeps needing `x-newsletter-admin: 1`. With
`auth` it must also carry no cross-origin evidence: an `Origin` header, when
sent, is the worker's own origin, and `Sec-Fetch-Site`, when sent, is
`same-origin` — else `403 {"error": "cross-origin request refused"}`. A script
using a service token in `Authorization: Bearer` sends neither and passes. The
password and Access modes are unchanged.

## The audit trail

Every state-changing admin request that reached its route is recorded in the
worker's own table `admin_audit` (schema version 6) — never in
`subscribers`:

| Column | Holds |
|---|---|
| `at` | when (ISO 8601, UTC) |
| `mode` | `auth`, `password` or `access` |
| `actor_id`, `actor_email` | the `auth` account; empty in the other modes, where the mode is all that is known (this worker does not read Access's identity) |
| `action` | the route's audit action, e.g. `campaign.schedule` |
| `target` | ids from the path — `campaign:<id>`, `subscriber:<id>`, `suppression:<hash>` — or the id a create answered with; never an address |
| `status` | the HTTP status the request was answered with (a refused schedule is worth knowing about too) |

Reads, previews and the import previews are not recorded. A GDPR erasure
leaves no trace of the person: the trail holds the subscriber's id only, and
`test/audit.test.ts` scans it for the erased address. The rows are written in
one place (`src/app.ts`, from the route registry), so no handler can forget
them.

Rows older than `NEWSLETTER_AUDIT_RETENTION_DAYS` (default **365**; `0` keeps
them forever) are deleted each time a row is written. Read them in the
panel's **Activity** view (`#/activity`) or from the API:

```sh
curl -b "__Host-ssg-auth=…" 'https://example.com/newsletter-admin/api/audit?actor=ada@example.com&action=campaign.schedule'
# {"items": [{"id": "…", "at": "…", "mode": "auth", "actor": {"id": "…", "email": "ada@example.com"},
#             "action": "campaign.schedule", "target": "campaign:…", "status": 200}], "nextCursor": null}
```

`actor` matches an account id or address, or `password` / `access` for the
rows that name nobody.

## The engine is not a person

The cron trigger and the queue consumer send campaigns without any session,
in every mode: they never pass through the admin gate. The provider webhooks
and the click redirect under `{base}/hooks/` stay public and unchanged.
