# newsletter-ui API reference

Every route, its body and its answer. The machine-readable description is
[`../openapi.yaml`](../openapi.yaml) (OpenAPI 3.1); `test/openapi.test.ts`
fails when it and `src/routes.ts` drift apart. The [README](../README.md)
explains what the worker does; this file is the reference. Import, sending
and bounces have their own pages: [IMPORT.md](IMPORT.md),
[SENDING.md](SENDING.md), [BOUNCES.md](BOUNCES.md).

All paths are under the base path (`NEWSLETTER_UI_BASE`, default
`/newsletter-admin`). Everything under `/api/` needs admin authentication and,
for any method other than `GET`/`HEAD`, the header `x-newsletter-admin: 1`
(else `403`). Errors are `{"error": "<snake_case_code>", "detail": "…"}`; list
endpoints answer `{items, nextCursor}` — pass `nextCursor` back as `cursor`
until it is `null`.

Answers any `/api/` route can give: `401` (no or wrong credentials), `403`
(CSRF header missing, Access denied), `404` / `405` (no such route / method,
with `allow`), `429` (the `RATE_LIMITER` binding or the failed-login lockout,
with `retry-after`), `500 internal error` (the details go to the log, never to
the answer), `503` (no admin auth configured, the database unreachable, or the
`subscribers` table missing — `{"error": "newsletter not installed", "hint":
"…"}`; `/api/health` and `/api/settings*` answer without the table).

## Health

| Method | Path | Answer |
|---|---|---|
| `GET` | `/api/health` | `{ok, schemaVersion, subscribersTable, auth: "access" \| "basic", bindings: {queue, rateLimiter, assets}, hint?}` |

## Subscribers

Responses never contain a subscriber's `token`, `ip_hash` or `ua_hash`.

| Method | Path | Does |
|---|---|---|
| `GET` | `/api/subscribers` | list: `{items, nextCursor}`, newest first; `status`, `language`, `tag`, `q` (part of the address), `limit` (default 50, max 200), `cursor` |
| `GET` | `/api/subscribers/:id` | one person: record, consent text and time, `sends` (with campaign subject), `suppression` |
| `PATCH` | `/api/subscribers/:id` | `{"tags": [...] or "a,b", "language": "pl" or null}`; any other field is `400 field_not_editable` |
| `POST` | `/api/subscribers/:id/unsubscribe` | manual unsubscribe, the same field changes as the public link; `{ok, changed, subscriber}` |
| `DELETE` | `/api/subscribers/:id` | GDPR erasure (below); `{ok, erased}` |
| `GET` | `/api/subscribers/:id/export` | GDPR access: everything stored about the person, as a JSON download |
| `GET` | `/api/subscribers.csv` | the list as CSV; `status=confirmed` (default) \| `pending` \| `unsubscribed` \| `all` |
| `POST` | `/api/import/preview` | what an import would do; writes nothing ([IMPORT.md](IMPORT.md)) |
| `POST` | `/api/import` | the import ([IMPORT.md](IMPORT.md)) |
| `GET` | `/api/stats/subscribers` | `days=30\|90\|365`: `{days, total, byStatus, byLanguage, series: [{day, signups, unsubscribes}]}` |

A subscriber in a list is `{id, email, status, language, tags, sourcePage,
createdAt, updatedAt, confirmedAt, unsubscribedAt}`; the detail adds
`consentText, consentAt, confirmSentAt, sends: [{campaignId, subject, status,
attempts, error, claimedAt, sentAt}], suppression: {reason, source, count,
createdAt, updatedAt} | null`.

Errors: `400` (`invalid_status`, `invalid_language`, `invalid_tag(s)`,
`invalid_limit`, `invalid_cursor`, `invalid_body`, `field_not_editable`,
`invalid_days`), `404 not_found`.

**Status.** An admin cannot set a status. Unsubscribing is the only change,
and nobody but the subscriber can make a row `confirmed` — except an import
that states where the consent was given.

**Erasure.** `DELETE` removes the row and the person's `campaign_sends` in one
transaction and writes a suppression with reason `manual`, source `erasure`
and **no address** — only its SHA-256. Afterwards the address is in no table
in clear text, and an import cannot bring it back. A suppression that had
kept the address (a bounce) loses it.

**CSV export.** The same columns, order and safety as the `newsletter`
template's `?format=csv` (UTF-8 BOM, CRLF, formula cells prefixed with `'`,
`unsubscribe_url` per row pointing at `/api/newsletter/unsubscribe` on this
origin), but suppressed addresses are left out, with the send engine's
fresh-consent rule — the file lists exactly who a campaign would mail. The
`newsletter` template's own export (`/api/newsletter/admin?format=csv`) does
not know the suppression list: mail from this export instead.

**Stats.** Counts only. The series covers `days` UTC days ending today and
reads the table as it is: an erased person is no longer a sign-up, and a
re-subscribed person keeps their first `created_at`.

## Campaigns

| Method + path | Body | Answer |
|---|---|---|
| `GET /api/campaigns?status=&cursor=&limit=` | — | `{items, nextCursor}`, newest first, bodies omitted; `limit` 1–100 (50) |
| `POST /api/campaigns` | any writable field | `201` the campaign (a `draft`) |
| `GET /api/campaigns/:id` | — | the campaign |
| `PATCH /api/campaigns/:id` | writable fields | the campaign; `409 not_draft` unless `draft` |
| `DELETE /api/campaigns/:id` | — | `204`; `409 not_deletable` unless `draft`/`cancelled` (its send records and click links go too) |
| `POST /api/campaigns/:id/duplicate` | — | `201` a new draft with the same content, sender, segment |
| `POST /api/campaigns/:id/preview` | `{language?}` (default `en`) | `{subject, html, text, headers, recipients: {total, byLanguage}}` |
| `POST /api/campaigns/:id/test` | `{to, language?}` | `{ok: true, transport}`; `429 test_limit`; `502 send_failed`; `503 no_site_url` / `no_transport` / `transport_unusable` |
| `POST /api/campaigns/:id/schedule` | `{at?}` (ISO 8601; omitted or past = now) | the campaign + `recipients` |
| `POST /api/campaigns/:id/unschedule` · `pause` · `resume` · `cancel` | — | the campaign; `409 invalid_transition` |

A campaign is `{id, subject, body, bodyFormat, textOverride, fromEmail,
fromName, segment, status, scheduledAt, startedAt, finishedAt, totalCount,
sentCount, failedCount, sourcePost, createdAt, updatedAt}`.

Writable fields: `subject` (one line, ≤ 250), `body` (≤ 256 KiB),
`bodyFormat` (`markdown` | `html`), `textOverride` (string or `null`; blank =
`null`), `fromEmail`, `fromName` (`null` = `NEWSLETTER_MAIL_FROM` /
`NEWSLETTER_MAIL_FROM_NAME`), `segment`
(`{languages: [], tags: [], tagMode: "any" | "all"}`). Errors are
`{error, detail}`: `invalid_json`, `invalid_input`, `unknown_variable`,
`not_found`, and for test and schedule also `no_site_url` / `no_transport` /
`transport_unusable` (503), `no_sender`, `empty_subject`, `empty_body`,
`missing_unsubscribe`, `empty_segment` (400).

**States.** `draft → scheduled → sending → sent`, `sending ⇄ paused`,
`scheduled → draft` (unschedule), and `scheduled | sending | paused →
cancelled`. `sent` and `cancelled` are final; any other move is
`409 invalid_transition`. Every move is one guarded `UPDATE … WHERE status =
<from>`, so of two concurrent requests exactly one wins. Only a `draft` can
be edited. Resume only flips `paused → sending`; the engine picks the work up.

**Segment.** Always `status = confirmed`. `languages` match the stored value;
a bare language also matches its regions (`pt` → `pt`, `pt-BR`). `tags` match
whole tags only (`news` never matches `newsletter`); `any` = at least one,
`all` = every one.

**Variables.** `{{unsubscribe_url}}`, `{{language}}`, `{{site_url}}`, in the
subject, body and text override. Any other `{{…}}` is refused when saving
(`400 unknown_variable`), so it can never become an empty string in a mail.

**What a reader gets.** One renderer serves preview, test send and the real
send, so the preview is exactly the mail:

- the body, Markdown rendered and then — like an HTML body — passed through an
  allowlist sanitizer: only `p br hr h1–h6 strong b em i u s del sup sub small
  span div blockquote pre code ul ol li a img table thead tbody tr th td`;
  `script`, `style`, `iframe`, `form`, `svg`… are removed with their content;
  no `style`, `class` or `on*` attributes; links `https:` / `mailto:` only,
  images `https:` only (anything else is unwrapped or dropped);
- a plain-text part made from the same sanitized content (links as
  `text (url)`), unless `textOverride` is set;
- a footer with the unsubscribe link, in the reader's language (en, pl, hi,
  de, fr; others get en), appended to both parts always;
- a minimal layout: inline styles, system fonts, `lang` attribute, light and
  dark colours meeting WCAG 2.2 AA, no remote resources of its own;
- headers `List-Unsubscribe: <{site}/api/newsletter/unsubscribe?token=…>`,
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click` (RFC 8058, handled by the
  `newsletter` template), `List-Id: <newsletter.{host}>`, `Precedence: bulk`.

Preview and test mails use a token no subscriber has, so trying their
unsubscribe link changes nothing.

**Markdown.** Supported: `#` headings, paragraphs (two trailing spaces or `\`
= line break), `>` quotes, one-level `-`/`*`/`+` and `1.` lists, fenced code,
`---` rules, `**strong**`, `*em*`/`_em_`, `~~strike~~`, `` `code` ``,
`[text](url "title")`, `![alt](url)`, `<https://…>` and bare `https://` links,
`\` escapes. Not supported: setext headings, indented code, nested lists,
tables, reference links. HTML inside Markdown is shown as text; set
`bodyFormat: "html"` to write HTML.

**Test sends** go through the configured transport, subject prefixed
`[TEST] `, at most 10 per hour for the whole worker (an attempt counts even if
the transport fails; a refusal before sending — no transport, unusable SMTP
settings — does not). The counter is one row in `settings`
(`campaign_test_sends`); the window opens with the first test send.

## Send engine

| Method + path | Answer |
|---|---|
| `GET /api/engine/status` | `{campaigns: [Progress…], deadLetters: {count, lastAt}, uncertainTotal, rate: {perMinute, minute, used}, batchSize, staleMinutes, maxAttempts, queue}` — `campaigns` are those `sending` or `paused`; `uncertainTotal` counts uncertain sends across all campaigns, finished ones included |
| `GET /api/campaigns/:id/progress` | `Progress`; `404 not_found` |
| `GET /api/campaigns/:id/uncertain?limit=50&cursor=…` | `{items: [{subscriberId, email, claimedAt, safeResend}], nextCursor, transport, safeResend}` — `limit` 1–200 (`400 invalid_limit`); `email` is null for a subscriber erased since; `404 not_found` |
| `POST /api/campaigns/:id/uncertain/resend` | body `{ids?: string[]}` (1–1000; omitted = all) → `{resent, ranges, safeResend}`; `400 invalid_ids`; `404`; `409 not_resendable` (cancelled, or never started) |
| `POST /api/campaigns/:id/uncertain/dismiss` | body `{ids?: string[]}` → `{dismissed}` — the rows become `skipped` with `error: "dismissed"`; `400 invalid_ids`; `404` |

`Progress` is `{id, subject, status, total, sent, failed, skipped, claimed,
uncertain, remaining, error, startedAt, finishedAt}`: `total` is counted when
the campaign starts, `claimed` is being sent or waiting for a retry, `skipped`
left the list (or was suppressed, or was dismissed) before their turn,
`uncertain` was in flight when its consumer died ([SENDING.md](SENDING.md)),
`remaining = total − sent − failed − skipped − uncertain` (never below 0),
`error` is why the engine paused the campaign. All numbers come from the
tables; a queue cannot be asked what it holds.

## Campaign statistics

| Method | Path | Answer |
|---|---|---|
| `GET` | `/api/campaigns/:id/stats` | `{id, subject, status, recipients: {total, sent, failed, skipped, pending, uncertain}, bounces: {hard, soft, complaints, detail}, unsubscribes: {count, windowDays, attribution: "time-window"}, clicks: {tracking, total, links: [{linkId, url, clicks}]}, timing: {scheduledAt, startedAt, finishedAt, durationSeconds}}`; 404 `not_found` |
| `GET` | `/api/stats/campaigns?limit=10&cursor=…` | `{items: [{id, subject, status, total, sent, failed, skipped, pending, uncertain, unsubscribes, clicks, startedAt, finishedAt, durationSeconds}], nextCursor, windowDays}` — started campaigns, newest first, `limit` 1–50 (400 `invalid_limit` / `invalid_cursor`) |
| `GET` / `HEAD` | `/hooks/c/:campaign/:link` | public: 302 to the link's stored target (`GET` counts one click, `HEAD` does not); 404 for any pair not stored |

What these numbers mean, and what is deliberately not counted, is in the
README's "Statistics and privacy".

## Campaign from a post

| Method + path | Body | Answer |
|---|---|---|
| `GET /api/feed/posts` | — | `{items: [{guid, title, link, summary, published, language, campaign}], nextCursor: null, feedUrl}` — up to 50, feed order (ssg: newest first); `campaign` = `{id, status}` of the newest campaign made from the post, else `null` |
| `POST /api/campaigns/from-post` | `{guid, confirm?}` | `201` the draft; `409 already_exists` `{id}` when a campaign was made from this post before, unless `confirm: true`; `404 post_not_found` |

The draft is subject = title, body = excerpt + link, segment language = the
post's language. It is never sent or scheduled by itself, and nothing happens
on publish.

- **The feed:** `NEWSLETTER_FEED_URL` — an absolute `https://` URL or a path
  on the site; unset = `/feed.xml` on `NEWSLETTER_SITE_URL`'s origin, which is
  where ssg's `feed: true` writes the Atom feed (a multilingual site has
  `/pl/feed.xml` etc. — point the variable at the one you want). RSS 2.0 and
  Atom are read; JSON Feed is not.
- **Fetch rules:** `https:` only; the host must be the site's or exactly the
  host of `NEWSLETTER_FEED_URL`; redirects are followed by hand (at most 3) and
  only to those hosts over `https:`; 10 s timeout; 2 MiB cap (read as a
  stream, cut off); an XML content type (`…xml`, `rss`, `atom`); the charset
  from the header or the XML declaration, decoded strictly. A feed that
  declares XML entities is refused (nothing is ever expanded). Any violation =
  `{error, detail}` and no draft: `no_feed_url`, `feed_not_https` (503),
  `feed_unavailable`, `feed_redirect`, `feed_content_type`, `feed_too_large`,
  `unsupported_charset`, `invalid_feed` (502).
- **The draft** goes through the same validation as `POST /api/campaigns`
  (`createDraft` in `src/campaign-crud.ts`): a title holding `{{…}}` is refused
  with `400 unknown_variable`, as a hand-written one would be. The excerpt
  (≤ 500 characters, HTML removed) and the title are Markdown-escaped, so a
  post's `*`, `[`, `{{` stay text. A title over 250 characters is shortened.
  `campaigns.source_post` holds the post's guid (RSS `<guid>` / Atom `<id>`,
  else the link).

## Confirmation mail for pending subscribers

An import without proof of consent leaves rows `pending`, and the `newsletter`
template mails a confirmation only at sign-up. These routes send it:

| Method + path | Body | Answer |
|---|---|---|
| `POST /api/subscribers/confirmations` | `{after?}` | `{sent, failed, skipped, failures: [{id, detail, temporary}], remaining, next}`; call again with `{after: next}` while `next` is not `null` |
| `POST /api/subscribers/:id/confirmation` | — | `{ok: true, transport}`; `404`; `409 not_pending` / `suppressed`; `429 confirm_throttled` with `retry-after`; `502 send_failed` |

- **The mail is the template's:** the same subject and text per language (en,
  pl, hi, de, fr by the subscriber's stored language; others get en), the
  link `{NEWSLETTER_SITE_URL}/api/newsletter/confirm?token=…`, sender
  `NEWSLETTER_MAIL_FROM` / `_NAME`. The strings are a copy in
  `src/confirm-strings.ts`; `test/confirm-copies.test.ts` fails when it drifts
  from the template's `_i18n.ts` (skipped, with a reason, when the template is
  not next to this directory).
- **Who:** the batch mails `pending` subscribers never mailed
  (`confirm_sent_at IS NULL`), in id order, at most `NEWSLETTER_CONFIRM_BATCH`
  (default 10, never more than the send rate) per call. Suppressed addresses
  are skipped. `next` moves past failed and skipped rows, so one bad address
  never blocks the rest; a fresh pass (no `after`) retries them. `remaining`
  counts what is left after `next`.
- **Bookkeeping:** `confirm_sent_at` is set only for a mail the transport
  accepted, and it starts the template's 15-minute re-send throttle, which the
  single re-send honours.
- **Rate:** both routes take from the send engine's per-minute budget
  (`send_rate`), so confirmations and campaigns together never exceed
  `NEWSLETTER_SEND_RATE`; with the minute used up the answer is
  `429 rate_limited` with `retry-after`. Unused budget is given back.
- Requires `NEWSLETTER_SITE_URL`, `NEWSLETTER_MAIL_FROM` and a usable
  transport: `503 no_site_url` / `no_sender` / `no_transport` /
  `transport_unusable` otherwise.

## Suppression list

| Method + path | Body → answer |
|---|---|
| `GET /api/suppressions` | `?reason=hard\|soft\|complaint\|manual&q=&cursor=&limit=` → `{items: [{emailHash, email, reason, count, source, createdAt, updatedAt}], nextCursor}`, newest change first; `q` matches kept addresses only (an erased one has none) |
| `POST /api/suppressions` | `{email, reason?: manual (default) \| hard \| complaint}` → `201 {suppression}`, `200` when it was already there (the stronger reason stays) |
| `DELETE /api/suppressions/:hash` | `204`; a `complaint` needs the body `{"confirm": true}`, else `409 confirm_required`; `404` |
| `POST /api/suppressions/import/preview` | a CSV (as the subscriber import: `text/csv` or `{"csv": "…"}`) → `{total, counts: {new, duplicate, existing, invalid}, sample, invalid}`; writes nothing |
| `POST /api/suppressions/import` | the same, written → plus `imported` |
| `GET /api/stats/suppressions` | `?days=30\|90\|365` → `{days, total, byReason, softCounting, series: [{day, hard, soft, complaint, manual}]}` (suppressions created per UTC day) |
| `POST /hooks/resend` | Resend's webhook, signed → `{ok, applied, duplicate, ignored}` |
| `POST /hooks/mailchannels` | MailChannels' webhook, signed → the same |

How the list fills itself, and how to register the webhooks:
[BOUNCES.md](BOUNCES.md).

## Settings

| Method + path | Answer |
|---|---|
| `GET /api/settings` | `{transport, transportProblem, sender: {email, name}, siteUrl, siteUrlValid, keys: {resend, mailchannels}, smtp: {host, port, user, passwordSet, helo} \| null, engine: {rate, batchSize, staleMinutes, maxAttempts, queue, deadLetterQueue}, clicks: {tracking, unsubscribeWindowDays}, bounces: {softLimit}, webhooks: {resend, mailchannels}, feedUrl, confirmBatch, importMaxRows, auth, base, schemaVersion}` — secrets only as booleans; `transportProblem` says why the configured transport can never send (SMTP on port 25, a port that is not a number, a user without a password, a bad EHLO name), else `null` |
| `POST /api/settings/smtp-test` | opens a session with the configured SMTP server exactly as a send would (greeting, EHLO, STARTTLS, AUTH) and closes it without sending: `{ok, transport, stage, tls: "implicit" \| "starttls", authenticated, reply, detail?}` — `reply` is what the *server* said (the client's lines, AUTH included, are never recorded); `409 not_smtp` for an HTTP provider or no transport; `503 transport_unusable` for settings that can never work (no test-send budget used); counts against the 10 test sends per hour (`429 test_limit`) |

Both work without the `subscribers` table. The panel never asks for, stores
or shows the SMTP password: it is a `wrangler secret`.
