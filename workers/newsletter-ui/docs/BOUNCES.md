# Bounces, complaints and the suppression list

An address on the suppression list gets no campaign: the engine leaves it out
when it selects a batch and checks again before each message. The list fills
itself from three sources and can be edited by hand. Back to the
[README](../README.md) · routes in the [API reference](API.md).

## Classes

`hard` (the mailbox does not exist) and `complaint` (the recipient reported
spam) suppress at once; a complaint also unsubscribes the subscriber
(`status = unsubscribed`, `unsubscribed_at`, `updated_at` — what the
unsubscribe link does). `soft` (mailbox full) is counted in `soft_bounces`
and suppresses only after `NEWSLETTER_SOFT_BOUNCE_LIMIT` in a row (default 3);
below that the address is mailed as usual. A count resets when a later
message gets through: for SMTP when the next message to the address is
accepted (one statement per batch), for a provider on its `delivered` event —
a provider accepts every message first and reports the bounce later, so its
"accepted" proves nothing. A suppression is never weakened by a later write:
`complaint` > `hard` > `soft` = `manual`.

## Source 1: the SMTP reply while sending

Classified conservatively, because a refusal caused by the *sender* fails
every recipient alike and suppressing on it would wipe the list the first time
the server is misconfigured:

| Reply | Class |
|---|---|
| 5xx at `RCPT TO` or end of data with enhanced code 5.1.1, 5.1.2, 5.1.3, 5.1.6, 5.1.10 | `hard` |
| the same with 5.2.2 (mailbox full), or a bare 552 at `RCPT TO` | `soft` |
| a bare 550 / 551 / 553 at `RCPT TO` whose text has no policy, spam, block, relay, auth, DMARC/SPF/DKIM, rate, content or size words | `hard` |
| any other enhanced code (5.7.x policy, 5.1.8 sender, 5.3.4 size, …), anything at `MAIL FROM`, `AUTH` or connection set-up, any 4xx | nothing |
| an HTTP provider's synchronous error (`resend returned 422`) | nothing — it is about the request; bounces come by webhook |

## Source 2: provider webhooks

Public routes: they skip admin auth, the CSRF rule and the per-IP
`RATE_LIMITER`, because the provider calls from many addresses without a
browser and a shared per-IP budget would drop its events under load (the same
reason the `newsletter` template exempts its unsubscribe endpoint). The
signature protects them instead, checked before anything else: no secret/key
configured → `503` (never open); body over 2 MiB → `413`; a missing or wrong
signature, or a timestamp more than 5 minutes from now in either direction →
`401`, nothing written. A redelivered event (same event id) counts once: its
id is stored in `hook_events` in the same transaction as its effects, and kept
7 days.

| Resend event (`type`) | Class |
|---|---|
| `email.bounced`, `bounce.type` `Permanent` or absent | `hard` |
| `email.bounced`, `Transient` / `Undetermined` | `soft` |
| `email.bounced` with subtype `MessageTooLarge`, `ContentRejected`, `AttachmentRejected`, or a message showing a policy/spam refusal | nothing |
| `email.complained` | `complaint` |
| `email.delivered` | resets the soft count |
| anything else | nothing |

| MailChannels event (`event`) | Class |
|---|---|
| `hard-bounced` | `hard`, unless `status`/`reason` show a policy/spam refusal |
| `complained` | `complaint` |
| `delivered` | resets the soft count |
| `soft-bounced` | nothing: MailChannels retries for hours, may send several for one message, and sends `hard-bounced` when it gives up |
| anything else (`processed`, `dropped`, `open`, `click`, `unsubscribed`, `test`) | nothing |

### Register the Resend webhook

Resend dashboard → Webhooks → add endpoint
`https://example.com/newsletter-admin/hooks/resend` with the events
`email.bounced`, `email.complained` and `email.delivered`; copy its signing
secret (`whsec_…`) and `npx wrangler secret put NEWSLETTER_RESEND_WEBHOOK_SECRET`.
Resend signs with the Svix scheme: `svix-id`, `svix-timestamp`,
`svix-signature` = `v1,` + base64 HMAC-SHA256 of `{id}.{timestamp}.{body}`
keyed with the base64 part of the secret; `svix-id` is the dedup key.

### Register the MailChannels webhook

Enrol `https://example.com/newsletter-admin/hooks/mailchannels` through
MailChannels' webhook API (see their Email API webhook documentation).
MailChannels signs with HTTP Message Signatures (RFC 9421) and Ed25519 over
the `Content-Digest` header (sha-256 of the body); the worker verifies against
a public key you configure — it never fetches one at request time, so a
request cannot choose the key it is checked against. Fetch the key once:

```sh
curl -s 'https://api.mailchannels.net/tx/v1/webhook/public-key?id=<keyid from Signature-Input>'
npx wrangler secret put NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY   # paste the PEM "key" value
```

If MailChannels rotates the key, deliveries answer `401` until you update it.
MailChannels events have no event id; the worker hashes `event`, `smtp_id`,
`request_id`, `timestamp` and the recipients instead. Verified against
MailChannels' published documentation (header names, algorithm, `created`
window, the public-key endpoint, the event names and the `recipients` field
of bounce events) but **not against a live delivery**; the `complained`
event's recipient field is assumed to be `recipients` like the bounces.

## Source 3: CSV import

For SMTP users, whose bounces arrive as mail: columns by header name, `email`
required, `reason` optional — `hard` (the default), `complaint` (also
unsubscribes) or `manual`. Rows already suppressed are left as they are. Same
size limits as the subscriber import. Reading the bounce mailbox
automatically is out of scope. A subscriber import whose file marks people
bounced or complained suppresses them too ([IMPORT.md](IMPORT.md)).

## By hand

`POST /api/suppressions` adds one; lifting a `complaint` needs
`{"confirm": true}` — the person said this mail is spam.

## Exports

`GET /api/subscribers.csv` leaves suppressed addresses out, with the engine's
fresh-consent rule, so the file lists exactly who a campaign would mail. The
**`newsletter` template's own CSV export
(`/api/newsletter/admin?format=csv`) does not know the suppression list**:
mail from this worker's export instead.
