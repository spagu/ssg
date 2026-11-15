# ACP checkout and feed push — reference

The short version is in [README.md](README.md#selling-through-ai-agents-acp).
This page is for whoever wires the shop to a platform or reviews the code.

## Which specification

- **Agentic Commerce Protocol, release `2026-04-17`**, the latest dated
  release in the protocol's repository
  ([github.com/agentic-commerce-protocol/agentic-commerce-protocol](https://github.com/agentic-commerce-protocol/agentic-commerce-protocol),
  `spec/2026-04-17/openapi/openapi.agentic_checkout.yaml`,
  `openapi.agentic_checkout_webhook.yaml`, `rfcs/rfc.agentic_checkout.md`,
  `rfcs/rfc.payment_handlers.md`), read on **2026-11-14** (repository head of
  2026-07-18). The site named in issue #197, agentic-commerce-protocol.com,
  did not resolve on that date.
- **Stripe, "Shared payment tokens — Sellers"**
  ([docs.stripe.com/agentic-commerce/concepts/shared-payment-tokens](https://docs.stripe.com/agentic-commerce/concepts/shared-payment-tokens)),
  read 2026-11-14: `POST /v1/payment_intents` with
  `payment_method_data[shared_payment_granted_token]`, `confirm=true`,
  `Stripe-Version: 2026-09-30.preview`.
- **OpenAI's commerce pages** ([developers.openai.com/commerce](https://developers.openai.com/commerce/)),
  read 2026-11-14: they describe product feeds (manual upload, a hosted URL or
  SFTP in Business Hub), browser use, ChatGPT Wallet and plugins — no in-chat
  checkout API. That matches industry reports that Instant Checkout in
  ChatGPT was withdrawn in March 2026 (not confirmed by an official statement
  we could find).

## Routes

Base URL to give the platform: `https://<your-site>/api/shop/acp`.

| Route | Protocol operation |
|---|---|
| `POST /checkout_sessions` | createCheckoutSession → 201 |
| `GET /checkout_sessions/{id}` | getCheckoutSession |
| `POST /checkout_sessions/{id}` | updateCheckoutSession |
| `POST /checkout_sessions/{id}/complete` | completeCheckoutSession |
| `POST /checkout_sessions/{id}/cancel` | cancelCheckoutSession (405 once final) |

Every call: `Authorization: Bearer <SHOP_ACP_API_KEY>` (compared in constant
time) and `API-Version` (`2026-04-17`, `2026-01-30` or `2026-01-16`; anything
else is 400 with `supported_versions`). Every POST: `Idempotency-Key` (≤ 255
characters). `Request-Id` and `Idempotency-Key` are echoed. Errors are the
protocol's flat `{type, code, message, param?}`; problems with a session that
can still be shown (unknown product, missing country, declined payment) are
`messages[]` on a 2xx session, as the specification describes.

Idempotency follows RFC §6: same key + same body (compared as canonical
JSON) replays the stored response with `Idempotent-Replayed: true`; another
body is 422 `idempotency_conflict`; a key still running is 409
`idempotency_in_flight` with `Retry-After` (one that has run for over 60 s is
taken to belong to a request that died, and the next request takes it over);
a 5xx is never stored. A key is honoured for 24 hours, the RFC's minimum;
after that the same key is a new request. Rows older than 7 days are deleted
when a new key is stored (`_acp-idempotency.ts`).

## Configuration

| Name | Kind | Meaning |
|---|---|---|
| `SHOP_ACP_ENABLED` | var | `"true"` turns ACP on; anything else is a 404 on every ACP route |
| `SHOP_ACP_API_KEY` | secret | the bearer token the platform sends |
| `SHOP_ACP_MERCHANT_ID` | var | your Stripe account/profile id, in the payment handler's `config.merchant_id` |
| `STRIPE_SECRET_KEY` | secret | the shop's existing Stripe key |
| `SHOP_ACP_WEBHOOK_URL` | var | the platform's order-events receiver; unset = no events |
| `SHOP_ACP_WEBHOOK_SECRET` | secret | HMAC key for `Merchant-Signature` on those events |
| `SHOP_ACP_SIGNING_SECRET` | secret, optional | when set, requests must be signed (below) |
| `SHOP_ACP_STRIPE_VERSION` | var, optional | `Stripe-Version` for SPT calls (default `2026-09-30.preview`) |

Missing `SHOP_ACP_API_KEY`, `STRIPE_SECRET_KEY` or `SHOP_ACP_MERCHANT_ID`
with ACP on is a 503 — the routes are closed, never open.

## How a session becomes an order

- **State** is in D1 (`acp_sessions`, migration 5), next to the orders. Every
  write is a compare-and-swap on a `version` column, so two concurrent updates
  cannot lose a write and a session cannot be completed twice (of two
  concurrent completes, one gets 409). Why D1 rather than a Durable Object is
  written down in `functions/api/shop/_acp-session.ts`.
- **Price** comes from `priceBasket` — the storefront's code. An agent's
  `unit_amount` is compared (a `price_change` warning when it differs), never
  used. The tax country is `fulfillment_details.address.country`; the
  request's own address is the platform's server, so it is not evidence.
- **Complete** re-prices (a moved total puts the session back with a
  `price_change` warning), writes the order (`channel = 'acp'`, gateway
  `stripe`), charges the token with Stripe's `Idempotency-Key`
  `acp:<session>:<token hash>`, records the payment, runs `fulfilOrder` (the
  storefront's fulfilment), marks the session completed and queues
  `order_create`. A decline puts the session back to `ready_for_payment` with
  a `payment_declined` message; the same pending order is reused for the next
  token. Stripe failing is a 502 and the session goes back too. A completion
  that died after charging is finished by the next one, without a new charge.
- **Order events** are `{type, data: Order}` POSTed to `SHOP_ACP_WEBHOOK_URL`
  with `Merchant-Signature: t=<unix>,v1=<hex HMAC-SHA256 of "t.body">`,
  through the outbox (kind `acp`): retried with backoff, visible in the
  panel's outbox list when they fail. The secret is read at delivery, never
  queued.
- **The waiver**: when `legal.digital_waiver` is on, every line carries a
  disclosure and the session an info message (`requires_buyer_review`) saying
  the download is immediate and the right of withdrawal is lost; the order
  records `consent_waiver = 1` on completion. Whether that counts as the
  buyer's consent where you sell is a legal question for you.

## Where the specification is ambiguous, and what this code does

1. **Version header.** The release is `2026-04-17`, but its RFC text and
   OpenAPI example still say `API-Version: 2026-01-16` (and the checklist
   `2026-01-15`). Accepted: `2026-04-17`, `2026-01-30`, `2026-01-16`; the
   shapes answered are `2026-04-17`'s. `2026-01-15` stays refused (checked
   again for 1.9.11, GO-187): it appears only in the RFC's compliance
   checklist (repository head `7fdd78d`, 2026-07-17), no release carries that
   date (the releases are `2025-09-29`, `2025-12-12`, `2026-01-16`,
   `2026-01-30`, `2026-04-17`) and the OpenAPI lists no such value. A client
   that sends it gets `400 unsupported_api_version` with `supported_versions`,
   as the RFC asks, and can retry with one of them.
2. **`line_items` or `items`.** The create schema requires `line_items`; the
   examples and the RFC use `items`. Both are read.
3. **Item quantity.** The schema's `Item` has no `quantity` (and forbids extra
   fields); the RFC's has one. `quantity` is read, default 1.
4. **Required `currency` and `capabilities` on create.** Required by the
   schema, absent from the examples. Both are optional here; the currency
   defaults to the shop's base currency.
5. **`payment_data`.** The schema says `{handler_id, instrument: {type,
   credential: {type: "spt", token}}}`; the same release's complete example
   sends `{token, provider: "stripe"}`. Both are accepted. The RFC's SPT
   credential schema says tokens look like `vt_…`, Stripe's are `spt_…`; any
   `[A-Za-z0-9_]{1,255}` is accepted and Stripe decides.
6. **Payment handler URLs.** The RFC gives two different sets
   (`/specs/handlers/card` and `/schemas/handlers/tokenized.card/config.json`)
   under acp.dev; the §8.1 example's set is used. Whether those URLs resolve
   was not checked.
7. **A declined payment on complete.** The complete response schema requires
   an `order`; the `MessageError` description says messages go on 2xx
   sessions. A decline answers 200 with the session (no `order`), status
   `ready_for_payment` and a `payment_declined` message.
8. **Request signatures.** The RFC recommends a `Signature` "over canonical
   JSON" with a `Timestamp`, algorithm "published out of band". When
   `SHOP_ACP_SIGNING_SECRET` is set: HMAC-SHA256 over `<Timestamp>.<body>`,
   base64url or base64, body as sent or as canonical JSON, Timestamp within
   300 s. Tell the platform this is what you verify, or leave the secret unset
   (the bearer token is always required).
9. **Order status words.** Storefront states map to `created`, `confirmed`,
   `manual_review`, `canceled`, `completed`; an unknown one passes through, as
   the webhook spec allows.
10. **Not implemented** (all optional in the release): 3-D Secure /
    `authentication_required`, discounts, affiliate attribution, marketing
    consent, intent traces, the delegate authentication and cart APIs.
    A refund confirmed later by Stripe's webhook *is* sent: an
    `order_update` with the full order and the refund as an adjustment
    (`_agent-refund.ts`, `_webhook.ts`).

## The feed push

The feed file is written by the ssg build (`feeds:` with `format: acp`). A
platform that fetches it from your site (OpenAI's Business Hub accepts a
hosted HTTPS URL) needs nothing more. A platform that wants it pushed gets it
from **`acp-feed-push/`**, a standalone Worker with one Cron Trigger — Pages
Functions cannot run a Cron Trigger, and the site build copies only
`functions/`, so this directory never becomes part of the site.

1. `cd workers/ecommerce/acp-feed-push`
2. In `wrangler.toml`: `ACP_FEED_SOURCE_URL` (the published feed),
   `ACP_FEED_TARGET_URL` (the platform's endpoint, from its docs),
   `ACP_FEED_METHOD` (POST, PUT or PATCH), the schedule in `[triggers]`.
3. `npx wrangler secret put ACP_FEED_TOKEN` — sent as `Authorization: Bearer`.
4. `npx wrangler deploy`, then `npx wrangler tail` to watch a run.

Each run fetches the feed (https only, `ACP_FEED_TIMEOUT_MS` per request,
default 30 s), refuses one larger than `ACP_FEED_MAX_BYTES` (default 20 MiB,
ceiling 100 MiB) or empty (an empty upload can wipe a catalogue), sends the
bytes as they are with the feed's content type, and logs one JSON line —
outcome, status, bytes, time, the two hosts; never paths, query strings or the
token. A run that did not push throws, so the dashboard shows it as failed.

The feed's format is the ssg build's (JSON Lines in the OpenAI product-feed
field names). The protocol's own Feed API (`PATCH /feeds/{id}/products` with
`{products: [...]}`) is a different shape; this Worker does not convert.

## Tests

`test/acp-*.test.ts`, in the shop's suite. Real SQL on the D1 fake; Stripe and
the platform's receiver are fakes written from the documents above
(`test/acp-helpers.ts`). Nothing was run against a live platform or Stripe.
