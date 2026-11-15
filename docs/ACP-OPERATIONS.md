# ACP: testing, certification and monitoring

What to check before an agent platform tests your shop's ACP integration,
what is known about certification, how to watch it once it runs, and what to
do when a product stops showing. The setup steps come first, in
[ACP.md](ACP.md). Facts about outside
services were checked on **2026-11-15**; links are given so you can check
them again.

## Test before anyone else does

### 1. The feed, at build time

Build with `check_acp: strict` (or `ssg --check-acp=strict`) so a broken row
fails the build instead of quietly leaving the file. Then read the file the
build wrote:

```sh
ssg --check-acp=strict
wc -l output/acp/products.jsonl          # one line per product
head -n 1 output/acp/products.jsonl      # item_id, price "24.00 EUR", seller fields
```

Compare every `item_id` with the SKUs in the shop panel. A row the platform
shows but the shop does not know is a sale that fails at the last step.

### 2. The shop's own tests

In a checkout of the ssg repository (Node 23.6 or newer, no npm install):

```sh
node --import ./workers/ecommerce/test/resolve.mjs --test \
  'workers/ecommerce/test/acp-*.test.ts'
```

They run the real handlers on fakes: SQLite standing in for D1, and a Stripe
and a platform receiver written from the published documents
(`workers/ecommerce/test/acp-helpers.ts`). They prove the code does what the
specification says as we read it. They prove nothing about a live platform.

### 3. Your own shop, with Stripe in test mode

Run the built site locally with the shop's Functions
(`npx wrangler pages dev output`), with a `.dev.vars` holding a Stripe
**test** key and test values for the ACP settings:

```sh
SHOP_ACP_ENABLED="true"
SHOP_ACP_API_KEY="local-test-key"
SHOP_ACP_MERCHANT_ID="profile_test"
STRIPE_SECRET_KEY="sk_test_..."
```

Create a session as an agent would. Every call needs the bearer token and
`API-Version`; every POST an `Idempotency-Key`:

```sh
curl -s http://localhost:8788/api/shop/acp/checkout_sessions \
  -H 'Authorization: Bearer local-test-key' -H 'API-Version: 2026-04-17' \
  -H 'Idempotency-Key: test-1' -H 'Content-Type: application/json' \
  -d '{"line_items":[{"id":"EBOOK-SHOP","quantity":1}],
       "buyer":{"email":"buyer@example.com"},
       "fulfillment_details":{"address":{"country":"PL"}}}'
```

The answer is `201` with `status: "ready_for_payment"`, the shop's own price
and tax, or `messages[]` saying what is missing. Stripe's sandbox can grant
your account a Shared Payment Token without any agent
([Stripe, "Generate an SPT in a sandbox"](https://docs.stripe.com/agentic-commerce/concepts/shared-payment-tokens)):
`POST /v1/test_helpers/shared_payment/granted_tokens` with a test payment
method and `usage_limits` for the currency and the session's total in minor
units. Complete the session with that token:

```sh
curl -s http://localhost:8788/api/shop/acp/checkout_sessions/<id>/complete \
  -H 'Authorization: Bearer local-test-key' -H 'API-Version: 2026-04-17' \
  -H 'Idempotency-Key: test-2' -H 'Content-Type: application/json' \
  -d '{"payment_data":{"handler_id":"card_tokenized",
       "instrument":{"type":"card","credential":{"type":"spt","token":"spt_..."}}}}'
```

The order then appears in the panel with an *ACP* badge, and the buyer gets
the download email if mail is configured. Send the same complete again with
the same key: the answer is replayed with `Idempotent-Replayed: true` and
Stripe is not called twice.

**This step was not run while writing this guide**: there was no `wrangler`
and no Stripe account on the machine. The commands follow the code
(`functions/api/shop/_acp-*.ts`) and Stripe's page; report anything that
does not match.

## What certification checks

What we could find on 2026-11-15:

- **OpenAI's public commerce pages describe no certification process** for
  merchants: not for feeds, not for checkout. They say checkout needs "a
  separately enabled integration" and do not say how it is enabled. Ask the
  platform.
- **The protocol's site** says each platform runs its own process.
- **The protocol itself** has a conformance checklist (§10 of
  `rfcs/rfc.agentic_checkout.md`). It is a list to check yourself, not a test
  anyone runs for you. How the shop stands against it:

| Checklist item | The shop |
|---|---|
| HTTPS, JSON, `API-Version` | JSON and `API-Version` are enforced; HTTPS is your deployment's. The checklist names `2026-01-15`, which the shop does not accept (it takes `2026-04-17`, `2026-01-30`, `2026-01-16`) |
| Authoritative cart state on every response | yes, priced by the storefront's code |
| Integer minor units | yes |
| Create, update, retrieve, complete, cancel | yes |
| `Idempotency-Key` on every POST, 400/422/409 | yes |
| Flat errors `type/code/message/param?` | yes |
| Verifies auth; signs/verifies requests | bearer always; request signatures when `SHOP_ACP_SIGNING_SECRET` is set |
| Order webhooks | `order_create` on completion; `order_update` when Stripe's webhook confirms a refund |
| `fulfillment_details`, `selected_fulfillment_options[]` | yes; the only option is `digital` |

If a platform runs its own tests, expect it to need a test product it can
buy, your test-mode keys, and the order-events receiver it gave you.

## Monitoring

- **Every build**: the `check_acp` lines and the item count. With `strict` in
  CI a broken row, or a feed with no rows at all, stops the deploy.
- **The feed push**: `npx wrangler tail` in `acp-feed-push/` shows one JSON
  line per run (`outcome`, `status`, `bytes`, `ms`, the two hosts). A run
  that did not push is marked failed in the Cloudflare dashboard. Outcomes:
  `pushed`, `not_configured`, `fetch_failed`, `too_large`, `empty_feed`,
  `push_failed`, `timeout`.
- **The shop panel**: orders filter by channel (*ACP*); failing order events
  stay in the outbox list with their error; an exception inside an ACP call
  is written to the audit log as `acp.error`.
- **The platform's own reports** (for OpenAI, the upload review in Business
  Hub): the only place that says what it rejected.

## A product disappeared: checklist

Work down the list; most causes are at the top.

1. **The build left it out.** Search the build log for its SKU. A missing
   field or a bare price drops the row.
2. **It is not in the published file.** `curl -s https://<your-site>/acp/products.jsonl | grep '"EBOOK-SHOP"'`.
   No hit after a good build: the deploy did not finish, or a cache serves an
   old copy.
3. **The platform did not receive it.** Hosted URL: check the platform's last
   fetch. Push: `wrangler tail`, look for anything but `pushed`.
4. **The id changed.** A new SKU, or a page with no SKU whose URL (used as the
   id) changed with its slug. To the platform that is a new product.
5. **The row says no.** `availability` `out_of_stock`, or
   `is_eligible_search: false` on the page or the feed.
6. **The platform rejected it.** Read its upload report. OpenAI's schema also
   says "Eligibility does not guarantee display".
7. **It shows but cannot be bought.** The shop decides at checkout: the
   product must exist under that SKU, be active, be digital, and have a price
   in the session's currency, and the buyer's country must be in your tax
   table. Each failure is a message on the session (`not_found`,
   `out_of_stock`, `invalid`, `region_restricted`).
8. **Every ACP call fails.** 404: `SHOP_ACP_ENABLED` is not exactly `"true"`.
   503: the API key, the Stripe key or the merchant id is missing. 401: the
   key was rotated on one side only, or a required signature is late or wrong.
   429: the agent budget for that credential is spent; it answers with
   `Retry-After`.

## Security, as the code implements it

- **Who may call**: `Authorization: Bearer` compared with `SHOP_ACP_API_KEY`
  in constant time, on every call.
- **Signed requests** (only with `SHOP_ACP_SIGNING_SECRET`): HMAC-SHA256 over
  `<Timestamp>.<body>`, and a `Timestamp` more than 300 seconds from the
  shop's clock is refused as a replay. The specification leaves the algorithm
  to the seller, so agree it with the platform or leave the secret unset.
- **Order events you send**: `Merchant-Signature: t=<unix>,v1=<hex HMAC-SHA256
  of "t.body">` with `SHOP_ACP_WEBHOOK_SECRET`, delivered with retries. The
  secret is read at delivery and never stored in the queue.
- **Idempotency**: the same key and body replays the stored answer; another
  body is `422 idempotency_conflict`; a key still running is `409` with
  `Retry-After`; 5xx answers are not stored. A key is honoured for 24 hours
  (the protocol's minimum); after that the same key is a new request, and
  the rows are deleted after 7 days. The Stripe
  charge carries its own idempotency key built from the session and the
  token, so a retried completion never charges twice.
- **Price**: an agent's `unit_amount` is compared, never charged. A total
  that moved before completion sends the session back with a warning.

## What was not tried against the real thing

- No live platform has called the checkout, and no feed has been uploaded to
  one by us.
- No live or test Stripe account was used; Stripe was a fake written from its
  documentation.
- Step 3 above was written from the code and Stripe's page, not run.
- No certification or platform test was taken, because none is published.
