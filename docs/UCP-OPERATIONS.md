# UCP: testing, checks and monitoring

What to check before Google tests your shop's UCP integration, what Google
publishes about its checks, how to watch it once it runs, and what to do when
a product stops showing. The setup steps come first, in [UCP.md](UCP.md).
Facts about Google's process were read on **2026-11-15**; each has a link so
you can check it again.

## Test before Google does

1. **Build** with `check_ucp: strict` and open `output/merchant.xml`. A feed
   with no rows at all fails the build too (since 1.9.10).
2. **The shop's tests**, from a checkout of the ssg repository:
   `node --import ./workers/ecommerce/test/resolve.mjs --test 'workers/ecommerce/test/ucp-*.test.ts'`.
   They run the real handlers against fakes of Google's platform, Stripe and
   an order-webhook receiver that verifies the signatures. They passed on
   2026-11-15. They prove nothing about Google's live systems.
3. **The profile**: `curl -s https://<your-site>/.well-known/ucp` must answer
   200 with your endpoint and a `signing_keys` entry without `d`.
4. **The UCP conformance suite**, which Google's native checkout guide points
   to. How to run it and what it says about the shop:
   [the conformance suite](#the-conformance-suite) below.

## The conformance suite

[github.com/Universal-Commerce-Protocol/conformance](https://github.com/Universal-Commerce-Protocol/conformance)
was run against the shop on **2026-11-15** (commit `0642adf` of 2026-11-13,
`ucp-sdk` 0.4.6, protocol `2026-04-08`, its flower-shop fixtures with the
SKUs seeded as downloads) — locally: the shop's real Functions on the test
fakes, Stripe faked. Nothing reached Google or Stripe.

**What a run needs from the shop** (since 1.9.11), on a test deployment only:

```toml
SHOP_UCP_TEST_MODE = "true"          # refused (503) next to sk_live_/pk_live_ or PRODUCTION
SHOP_UCP_HANDLER_ID = "google_pay"   # only for the suite's `--platform=google` handler check
SHOP_UCP_WEBHOOK_URL = "http://localhost:8284/webhooks/partners/test_partner/events/order"
```

plus the secret `SHOP_UCP_SIMULATION_SECRET` (pass the same value as
`--simulation_secret`) and `/testing/*` in `routes_include`. Test mode
accepts the suite's `http://localhost` agent profile and receiver, its test
payment handler (`success_token` delivers the product **without payment**)
and `/testing/simulate-shipping/{id}`
([workers/ecommerce/UCP.md](../workers/ecommerce/UCP.md#test-mode-for-the-conformance-suite-since-1911)).
The suite sends **no `X-API-Key`** and has no option for one, so put a proxy
in front of the shop that adds it; the API key is never switched off.

**Results** (83 tests; 28 skipped because the shop does not advertise
discount, fulfillment, buyer consent or AP2):

| Run | Passed | Failed |
|---|---|---|
| before 1.9.11, no API key (as the suite sends) | 3 | 52 (401) |
| before 1.9.11, key added by the proxy | 3 | 52 (400 `invalid_profile_url`: the agent profile is http) |
| 1.9.11, key added, test mode **off** | 3 | 52 — unchanged for a shop not in test mode |
| 1.9.11, key added, test mode on | 13 | 42 |
| the same, suite patched locally to skip fulfillment when the server does not advertise it | 29 | 26 |
| … and to send a buyer email, which a download needs | 40 | 15 |

The 42 failures of the unpatched suite come from one helper: before every
completion it reads `fulfillment.methods[0]` from the session (41 tests
fail with `KeyError: 'fulfillment'`), although fulfillment is an optional
extension that "applies only to items requiring physical delivery"
(fulfillment.md). A downloads-only shop cannot pass those tests without
inventing a shipping method. The last one is `test_version_negotiation`,
which sends `UCP-Agent: profile="..."`: not a URL, so `400
invalid_profile_url`, as overview.md lists it.

The 15 left after both local patches, and why they stay:

| Tests | Why |
|---|---|
| `test_order_update`, `test_order_adjustments`, `test_invalid_adjustment_status`, `test_malformed_adjustment_payload` | they `PUT /orders/{id}`; the protocol defines only Get Order |
| `test_order_fulfillment_retrieval`, `test_ready_for_complete_when_nothing_is_outstanding`, `test_update_checkout`, `test_webhook_order_address_known_customer`, `test_webhook_order_address_new_address` | they need shipping options and fulfillment fields without checking the capability |
| `test_card_credential_payment` | it pays with a raw card number; the shop never accepts one |
| `test_complete_is_idempotent` | completing a completed session answers 200 with the order (a safe replay); the suite wants a 4xx. The protocol only says a completed session is immutable |
| `test_delivery_carries_standard_webhook_headers` | `Webhook-Timestamp` is RFC 3339, as every Google example shows; order.md's table says unix (ambiguity 6 in workers/ecommerce/UCP.md) |
| `test_transient_failure_retries_preserve_event_identity` | a failed event is retried by the outbox with backoff (minutes), not within the suite's few seconds |
| `test_update_inventory_validation` | quantity 10001 is a `400 invalid_request` (`{code, content}`, overview.md's error shape); the suite wants `detail`, an out-of-stock message or a clamped quantity |
| `test_version_negotiation` | as above |

`--platform=google` also fails: Google's profile in the suite requires the
fulfillment capability (with `SHOP_UCP_HANDLER_ID = "google_pay"` that is
the only finding left). Whether Google's approval uses this profile is not
stated.

## What Google checks

- **Approval** before going live ([UCP.md](UCP.md#before-you-start-googles-requirements)).
  Google's pages read on 2026-11-15 do not publish the criteria.
- **Service levels** ([native checkout overview](https://developers.google.com/merchant/ucp/guides/overview/native-checkout)):
  at least 95% availability for create, update and complete; latency p50/p95
  at most 1 s/4 s for create, 1 s/5 s for update, 6 s/10 s for complete, over
  a rolling 24 hours. Nothing in ssg measures these.
- **Conformance**: the suite above has a `google` certification profile.
  Whether Google's approval runs it is not stated.

## Monitoring, and a product that disappeared

- Merchant Center's product diagnostics say why a product is not shown or
  not eligible. ssg cannot see them.
- In the shop panel: orders filter by channel (*UCP*); order events that keep
  failing stay in the outbox list; refunds are sent to Google as order
  updates.
- If a product vanished: check the build log for its SKU; check the
  published `merchant.xml` contains it with `checkout_eligibility` `TRUE`;
  check ssg did not write `FALSE` (pre-order, used, refurbished, no returns);
  check that `id` or `merchant_item_id` still equals the shop's SKU; check
  the product is active, digital and priced in the shop; then Merchant
  Center's diagnostics.
- Every UCP call failing: 404 = not switched on; 503 = a setting is missing
  ([UCP.md, step 2](UCP.md#step-2-switch-on-the-shops-ucp-checkout)); 401 =
  the API key differs; 400 `invalid_profile_url` = the caller sent no
  `UCP-Agent: profile="https://…"`.

## Security, as the code implements it

- `X-API-Key` compared with `SHOP_UCP_API_KEY` in constant time; every call
  must carry `UCP-Agent` with an `https` profile URL (checked for shape, not
  fetched; `http` too in test mode only). OAuth 2.0 and mTLS are options in
  the protocol, not requirements, and are not implemented; mTLS would be a
  Cloudflare zone rule in front of the Worker
  ([workers/ecommerce/UCP.md](../workers/ecommerce/UCP.md), ambiguity 2).
- Test mode (`SHOP_UCP_TEST_MODE`) is off by default and refused (503 on
  every UCP route) next to a live Stripe key or Google Pay `PRODUCTION`.
- Order events are signed as RFC 9421 HTTP Message Signatures with ES256,
  covering method, host, path, `UCP-Agent`, `Idempotency-Key`,
  `Content-Digest` and `Content-Type`, with a `created` time; retries reuse
  the same `Webhook-Id` and `Idempotency-Key`.
- `Idempotency-Key` is optional; when sent, the same body replays the stored
  answer, another body is `409 idempotency_conflict`, and a key is honoured
  for 48 hours (rows are deleted after 7 days). The Stripe charge has its own
  idempotency key per session and token, so a retried completion never
  charges twice.
- A decline or a Stripe failure on complete answers 200 with the session and
  a message, as Google asks, so the buyer can try another card.

Every choice the code made where the specification left room is listed in
[workers/ecommerce/UCP.md](../workers/ecommerce/UCP.md).

## What was not tried against the real thing

- No Google system has called the shop; no Merchant Center account was used.
- No Google Pay token was ever decrypted or charged; Stripe was a fake.
- The conformance suite was run locally against the shop's Functions on
  fakes, not against a deployment.
- Microsoft Merchant Center also asks for a "UCP-ready" feed with different
  attribute names; whether it reads Google's names is not confirmed
  ([CONFIGURATION.md](CONFIGURATION.md#universal-commerce-protocol-ucp-on-the-merchant-feed)).
