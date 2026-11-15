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
   to: [github.com/Universal-Commerce-Protocol/conformance](https://github.com/Universal-Commerce-Protocol/conformance).
   **It was not run against the shop.** Reading its harness (commit `0642adf`,
   2026-11-13) shows it will not pass as the shop stands: it sends no
   `X-API-Key`, so every call gets 401; its agent profile is an `http://`
   URL, which the shop refuses; its order tests call a
   `/testing/simulate-shipping/{id}` endpoint the shop does not have; and its
   `google` profile requires a `dev.ucp.shopping.fulfillment` capability the
   shop does not advertise, and a handler named `google_pay` (the shop's
   handler key is `com.google.pay`, its id `gpay` unless you set
   `SHOP_UCP_HANDLER_ID`).

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
  fetched).
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
- The conformance suite was read, not run.
- Microsoft Merchant Center also asks for a "UCP-ready" feed with different
  attribute names; whether it reads Google's names is not confirmed
  ([CONFIGURATION.md](CONFIGURATION.md#universal-commerce-protocol-ucp-on-the-merchant-feed)).
