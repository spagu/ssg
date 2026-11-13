# How sending works

The cron starts due campaigns and plans the work; the queue consumer sends it
in batches. Nothing else is needed: schedule a campaign
(`POST /api/campaigns/:id/schedule`) and it goes out. Back to the
[README](../README.md) · [API reference](API.md).

## How a campaign goes out

1. *Cron, every minute.* A `scheduled` campaign whose time has come is moved
   to `sending` with the atomic `scheduled → sending` transition, so of two
   overlapping ticks exactly one starts it. Its plan is created and
   `total_count` counted. Then, for every `sending` campaign, the cron cuts the
   next subscriber ids (segment order by `id`) into ranges of the batch size
   and queues one job per range, keeping about a minute of work queued
   (`ceil(rate / batch)` ranges, at most 100). A job carries
   `{campaignId, after, until}` — a keyset range, never an address.
2. *Queue, one job per invocation.* The consumer checks the campaign's status
   (paused → the range is parked, cancelled → closed, nothing sent), selects
   the range with the segment and the suppression list, reserves rate budget,
   opens **one** mailer session (one SMTP connection, or the HTTP provider),
   and for each recipient, one at a time: claims the row in `campaign_sends`
   and, in the same statement, checks once more that the person is still
   `confirmed` and not suppressed and that the campaign is still `sending`;
   renders the mail in their language, sends it with the idempotency key
   `{campaignId}:{subscriberId}`, records `sent` / `failed`.
3. *Finish.* When every range is planned and done and no row is `claimed` or
   waiting for a retry (`uncertain` rows do not hold a campaign open), the
   cron sets `sent_count` / `failed_count` from `campaign_sends` (recounted,
   never incremented) and moves the campaign to `sent`. A campaign with nobody
   to send to finishes on its first tick.

## Never twice, never lost

Idempotence lives in `campaign_sends` (`UNIQUE(campaign_id, subscriber_id)`),
not in hoping a job arrives once:

- a row is claimed immediately before *its own* message goes out — one row at
  a time, never a batch ahead — so "no row" always means "never handed to the
  transport", and a consumer holds at most one claim at any moment;
- a redelivered job skips `sent`, `failed`, `skipped` and `uncertain` rows,
  and `claimed` rows younger than `NEWSLETTER_SEND_STALE_MINUTES` (another
  consumer may be mid-send); an older claim becomes `uncertain` — never
  re-sent by the engine;
- a range whose job was lost (dropped, dead-lettered, a crash) stays `queued`
  and is re-queued by the cron after the same timeout; a parked range is
  re-queued as soon as the campaign is resumed.

## Uncertain sends: a crash never sends twice by itself

The provider's "accepted" and our write of `sent` are two systems; if the
worker dies between them — the isolate is killed, the platform ends the
invocation, the database write fails — nobody can know whether that message
left. The engine's rule (an owner decision): *a possible duplicate is worse
than one visibly missing message.* So:

- only the message being handed to the transport can be affected — rows are
  claimed one by one, so the rest of the batch has no claim (a crash before
  the send hands its claim back at once; a kill leaves nothing else behind).
  A kill in the milliseconds between that row's claim and its send (the last
  check, rendering) also leaves it `uncertain` although nothing left — the
  cautious side of the same rule;
- after the stale timeout that one row becomes **`uncertain`**: it is not
  retried, not counted as `sent` or `failed`, does not hold the campaign
  open — the campaign finishes `sent` with `uncertain: 1` in `Progress` and
  the statistics, and `remaining` still reaches 0; `/api/engine/status`
  reports `uncertainTotal` across all campaigns;
- the admin sees it in `GET /api/campaigns/:id/uncertain` (who, and when it
  was claimed) and decides: **re-send** gives exactly one more attempt through
  the normal path — the same rate budget, the same last check (still
  `confirmed`, not suppressed; otherwise `skipped`), the same idempotency
  key — or **dismiss** closes the row as `skipped` (`error: "dismissed"`).
  Both work on a campaign that has already finished: it stays `sent`,
  `finished_at` keeps marking the end of its run, the counters follow the
  re-sent rows. The ranges holding re-sent rows are reopened *re-send only*
  (`send_batches.retry_only`): their job reads those ledger rows, not the
  segment, so nobody who signed up since is mailed. A paused campaign's
  reopened ranges wait for resume; a cancelled campaign refuses (`409`).

## Idempotency keys — what they do and do not guarantee

Every campaign message carries the key `{campaignId}:{subscriberId}` (test
sends and confirmation mails carry none: each of those is meant as a new
message).

| Transport | What the key does | `safeResend` |
|---|---|---|
| Resend | sent as the `Idempotency-Key` header. Resend's documented behaviour: a request with a key it has seen in the last **24 hours** and the same payload returns the original answer *without sending again*; a different payload with that key is refused (409). So a re-send of an uncertain row within 24 hours of its claim cannot produce a second copy; after 24 hours the key is forgotten and a re-send is a new message | `true` (per item: only while the claim is under 24 hours old) |
| MailChannels | nothing: its Email API documents no idempotency mechanism (checked 2026-11), so none is sent | `false` |
| SMTP | no such thing in SMTP. The key becomes a deterministic `Message-ID` (`<key with every character outside [A-Za-z0-9_-] as =XX>@{sender domain}`). That is a **hint**, not a guarantee: some mailbox providers drop a second message with a Message-ID they already hold, many do not | `false` |

A re-send with `safeResend: false` may deliver a second copy if the first one
did arrive — that is the admin's call, with the list in front of them.

**Resend's 409 answers.** A keyed request can be refused with `409`, and the
transport reads the error `name` in the body to tell the two documented cases
apart: `invalid_idempotent_request` (the key was used for a different payload,
i.e. a message under this key was already accepted) comes back as
`{ok: true, duplicate: true}` and the engine records the row `sent`, with
`error` = "already accepted earlier under this idempotency key";
`concurrent_idempotent_requests` (a request with this key is still in
progress) and any other or unreadable 409 are temporary — the row is retried
like a `429`. A request without a key keeps the old reading (a `409` is a
permanent refusal).

## Pause, resume, cancel, unsubscribes

The status is read at the start of every job and again before every message:
a pause or cancel takes effect at the next message — at the latest after the
current batch. The one claim the check found is handed back without counting
an attempt; the rows after it were never claimed. Resume re-queues the parked
ranges on the next tick; the ledger makes sure nobody already mailed is mailed
again.

**Unsubscribes during a campaign** take effect at once: the last check before
each message reads the subscriber's status and the suppression list. A person
who left is recorded as `skipped` (`not_confirmed`, `subscriber_gone`,
`suppressed`) and does not use rate budget.

**Suppressions.** Both steps of the exclusion: `NOT_SUPPRESSED_SQL` in the
range query, `dropHashOnly` in the worker. A hash-only `manual` suppression
(what an erasure leaves) stops applying when the subscriber's `consent_at` is
newer than the suppression — the person signed up again; `hard` and
`complaint` (and any suppression that kept the address) always apply.
`total_count` sees only the SQL step, so a re-subscribed erased person can make
it one too high.

## Failures

| Result | What happens |
|---|---|
| sent | `sent` |
| temporary (HTTP 429/5xx, network, SMTP 4xx) | `retry`, attempt counted; the job throws, the queue retries it with backoff and only the unfinished rows are tried again; after `NEWSLETTER_SEND_MAX_ATTEMPTS` → `failed` |
| permanent (other HTTP 4xx, SMTP 5xx) | `failed` with the transport's words in `error`; `onPermanentFailure` in `src/engine-hooks.ts` classifies it (a mailbox refusal suppresses, a sender-side refusal does not — [BOUNCES.md](BOUNCES.md)). The hook runs at the end of the batch, after the breaker below had its say; a consumer killed mid-batch loses that batch's suppressions (the rows keep the server's words; the next campaign bounces and suppresses them) |
| render error (an unknown `{{variable}}`) | the campaign is **paused** with `error: "render: …"`; nobody else is tried |
| hard bounces from one batch reach the breaker (default ≥ 5 **and** ≥ 50 % of the batch's messages so far) | the campaign is **paused** with `error: "bounces: N of M messages in one batch were refused as unknown mailboxes; nothing was suppressed …"`; that batch's permanent-failure hooks never run (nobody from it is suppressed, no soft count moves) and its hard-bounced rows go back to `retry` without counting the attempt, so a resume after fixing the server tries them again |
| no site URL, no sender, transport refuses login | paused with `error: "config: …"` / `"transport: …"` |
| transport unreachable | the job throws and is retried; nothing was claimed |

A job that runs out of queue retries lands in the dead-letter queue, which this
worker also consumes: it is recorded in `send_dead_letters` (the count in
`/api/engine/status`) and its range is re-queued by the cron after the stale
timeout, so the work itself is not lost.

**Headers.** Every message carries what the renderer returns:
`List-Unsubscribe: <{site}/api/newsletter/unsubscribe?token=…>` with the
subscriber's own token, `List-Unsubscribe-Post: List-Unsubscribe=One-Click`,
`List-Id: <newsletter.{host}>`, `Precedence: bulk`.

## Rate

One counter row per UTC minute in `send_rate`, reserved with one atomic
upsert, so several consumers at once never hand out more than the rate;
unused budget (skipped people, a stopped batch) is given back. A job that gets
less than it asked for sends what it got and puts itself back on the queue with
a delay to the next minute — no worker sleeps. 60 a minute is a pace every
provider tier and most SMTP servers accept; raise it to what your provider
allows. Resend documents a default of 10 requests per second **per team**,
shared by every key and everything else the team sends
([resend.com/docs/api-reference/introduction](https://resend.com/docs/api-reference/introduction),
"Rate limit", checked 2026-11-13). Confirmation mails sent from the panel
take from the same budget.

The plan limits that decide the batch size, and how long a list takes, are in
the README's "Limits per Cloudflare plan".

## For code built on the engine

`src/engine-hooks.ts` (`onPermanentFailure`), `dropHashOnly(env, rows, true)`
/ `suppressionApplies` in `src/suppress.ts`, `refreshCounters` /
`finishIfDone` in `src/engine-finish.ts`, and in tests
`test/engine-fixtures.ts`: `engineEnv`, `addCampaign`, `fakeMailer`,
`perPerson`, `truth` and `pump()` — runs the cron and the queue until nothing
is left (with `advance` to move a mocked clock between rounds).
