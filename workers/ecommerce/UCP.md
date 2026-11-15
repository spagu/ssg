# UCP checkout and business profile — reference

The short version is in [README.md](README.md#selling-through-googles-agents-ucp);
the owner's step-by-step guide is [docs/UCP.md](../../docs/UCP.md), with
testing and monitoring in [docs/UCP-OPERATIONS.md](../../docs/UCP-OPERATIONS.md).
This page is for whoever wires the shop to a platform or reviews the code:
what the documents left open and what this code chose. The code comments
refer to the numbered ambiguities below.

## Which specification

- **Universal Commerce Protocol, release `2026-04-08`**, from the protocol's
  repository ([github.com/Universal-Commerce-Protocol/ucp](https://github.com/Universal-Commerce-Protocol/ucp),
  tag `v2026-04-08`): `docs/specification/checkout.md`, `checkout-rest.md`,
  `overview.md`, `signatures.md`, `order.md` and `source/schemas`
  (`_ucp-http.ts`). Read 2026-11-14.
- **Google's Merchant UCP guides**
  ([developers.google.com/merchant/ucp](https://developers.google.com/merchant/ucp)),
  read 2026-11-14: "Publish your UCP profile", "Authentication", "Order
  lifecycle", "Error codes", "Tax-inclusive pricing", native checkout.
- **Google Pay, "Google Pay payment handler for UCP"**, version `2026-01-23`
  (`https://pay.google.com/gp/p/ucp/2026-01-23/`), read 2026-11-14, with its
  processor table for Stripe (`_ucp-payment.ts`).
- **Stripe**: `POST /v1/payment_intents` with
  `payment_method_data[card][token]`, as Stripe's "Create a PaymentMethod"
  describes for a token from a wallet.

## Routes

Base URL in the profile: `https://<your-site>/api/shop/ucp`.

| Route | Operation |
|---|---|
| `GET /.well-known/ucp` | the business profile — public, no API key |
| `POST /checkout-sessions` | create → 201 |
| `GET /checkout-sessions/{id}` | read |
| `PUT /checkout-sessions/{id}` | update: replaces the whole session |
| `POST /checkout-sessions/{id}/complete` | pay and place the order |
| `POST /checkout-sessions/{id}/cancel` | cancel |
| `GET /orders/{id}` | Get Order (order.md): the full order of a UCP checkout; any other order is `not_found` (since 1.9.11) |
| `POST /testing/simulate-shipping/{id}` | at the site root, test mode only (below) |

Every route goes through `_ucp-route.ts`, in this order: UCP off → 404; body
read (at most 64 KiB, else 413); a request budget per credential and address
(120 a minute, `_ratelimit.ts`; 429 with `Retry-After`); configuration,
`X-API-Key` and `UCP-Agent` (`_ucp-auth.ts`); the shop configured; a JSON
object body; `Idempotency-Key` claimed on `POST`/`PUT` when sent; the
operation. `Request-Id` and `Idempotency-Key` are echoed. Protocol errors
are `{code, content}` with an HTTP status; what is wrong with a basket is
`messages[]` on the session, with the codes Google publishes.

## Configuration

| Name | Kind | Meaning |
|---|---|---|
| `SHOP_UCP_ENABLED` | var | `"true"` turns UCP on; anything else is a 404 on every UCP route and on `/.well-known/ucp` |
| `SHOP_UCP_API_KEY` | secret | the key the platform sends in `X-API-Key` |
| `SHOP_UCP_SIGNING_KEY` | secret | EC P-256 private JWK with `kid`; signs order events |
| `SHOP_UCP_STRIPE_PUBLISHABLE_KEY` | var | Stripe's `pk_test_…`/`pk_live_…`; public, it is in the profile |
| `STRIPE_SECRET_KEY` | secret | the shop's existing Stripe key; charges the token |
| `SHOP_UCP_GPAY_ENVIRONMENT` | var, optional | `"PRODUCTION"`, or `"TEST"` (the default, also for any other value) |
| `SHOP_UCP_GPAY_MERCHANT_ID` | var | Google Pay merchant id; required with `PRODUCTION` |
| `SHOP_UCP_HANDLER_ID` | var, optional | this shop's id for its Google Pay handler (default `gpay`) |
| `SHOP_UCP_WEBHOOK_URL` | var, optional | the platform's order-event URL, https, without credentials; unset = no events |
| `SHOP_UCP_WEBHOOK_API_KEY` | secret, optional | sent as `X-Goog-Api-Key` with order events |
| `SHOP_UCP_TEST_MODE` | var, optional | `"true"` = the conformance-suite test mode (below); anything else is off |
| `SHOP_UCP_SIMULATION_SECRET` | secret, optional | the `Simulation-Secret` of `/testing/simulate-shipping`; unset = that path is a 404 |

With UCP on, a missing `SHOP_UCP_API_KEY`, `STRIPE_SECRET_KEY` or signing
key, a publishable key that does not start `pk_test_`/`pk_live_`, or no
merchant id in `PRODUCTION` is a 503 on every route and on the profile —
closed, never open (`ucpMissing` in `_ucp-auth.ts`). So is the test mode
next to a live configuration (`sk_live_`/`rk_live_` secret key, `pk_live_`
publishable key or Google Pay `PRODUCTION`).

**Routing.** Pages sends a path to Functions only when `routes_include`
names it: add `/.well-known/ucp` next to `/api/shop/*` in the shop's
`workers:` entry, or the profile is a static 404.

**Shop settings.** Set `legal.terms_url` and `legal.privacy_url` in the
panel: every checkout response lists them as `terms_of_service` and
`privacy_policy` links, which Google shows and asks for. An empty setting is
a link left out, not a link to nowhere (`_ucp-render.ts`). The name on the
Google Pay sheet is `seller.name`, else `shop.name`.

## The business profile and the signing key

`/.well-known/ucp` (`_ucp-profile.ts`) is built from configuration on every
request and cached for 300 s (`cache-control: public, max-age=300`), so a
rotated key shows up without a deploy. It carries:

- `ucp.version` `2026-04-08` and one `dev.ucp.shopping` REST service with
  the endpoint above;
- the capabilities `dev.ucp.shopping.checkout` and
  `dev.ucp.shopping.order` (the order capability: the webhooks and, since
  1.9.11, `GET /orders/{id}`);
- `payment_handlers` with one `com.google.pay` handler (below);
- `signing_keys`: the public half of `SHOP_UCP_SIGNING_KEY` —
  `{kid, kty: "EC", crv: "P-256", x, y, use: "sig", alg: "ES256"}`, never
  `d`.

Make the key once, keep it secret, and set it with
`npx wrangler pages secret put SHOP_UCP_SIGNING_KEY` (Node 23.6 or newer):

```sh
node --input-type=module -e 'const k=await crypto.subtle.generateKey({name:"ECDSA",namedCurve:"P-256"},true,["sign"]);const j=await crypto.subtle.exportKey("jwk",k.privateKey);console.log(JSON.stringify({kty:j.kty,crv:j.crv,x:j.x,y:j.y,d:j.d,kid:"shop-2026-11"}))'
```

To rotate: make a new key with a new `kid` and replace the secret. Events
are signed at delivery, retries included, so every event after the change
carries the new `kid`. The profile publishes one key at a time and may be
cached for up to 300 s, so a platform that still holds the old profile
cannot verify events sent in that window until it fetches the profile again.

## Payment: Google Pay through Stripe

The handler (`_ucp-payment.ts`) offers cards (`PAN_ONLY`, `CRYPTOGRAM_3DS`;
Amex, Discover, JCB, Mastercard, Visa) with a minimal billing address — the
country is the VAT evidence for a download — tokenized for the gateway
`stripe` with `stripe:version` `2018-10-31` and the publishable key.

On complete the shop takes the selected instrument (or the only one) of its
own handler id, with `credential.type` `PAYMENT_GATEWAY`; the token must be
Google Pay's tokenization data holding a Stripe token `tok_…`. Anything else
is a `payment_ineligible` message. The charge is a confirmed PaymentIntent,
card only, with Stripe's `Idempotency-Key` `ucp:<session>:<token hash>`: a
retried completion with the same token gets Stripe's stored answer, a new
token after a decline is a new attempt. The credential is never stored or
echoed.

## How a session becomes an order

- **State** is in D1 (`ucp_sessions` and `ucp_idempotency`, migration 6),
  next to the orders, with the same compare-and-swap on a `version` column as
  ACP (`_agent-session.ts`). A session lives 6 hours; an expired one is
  `canceled`.
- **Items**: 1 to 20 lines, quantity 1 to 10, `item.id` = the shop's SKU;
  two lines naming one product become one. Only digital products sell.
- **Price** comes from `priceBasket`, the storefront's code. `context.currency`
  is a preference: a currency the product has no price in falls back to the
  shop's base currency.
- **Country**: the selected (or first) fulfilment destination's
  `address_country`, else `context.address_country`, else the billing country
  from Google Pay on complete. The request's own address is the platform's
  server, so it is not evidence.
- **Buyer email** is required before a session is `ready_for_complete`
  (`missing_buyer_info`): a download cannot be delivered without it.
- **Complete** claims the session (`complete_in_progress`, one CAS), prices
  it again (a moved total puts it back with a `totals_changed` warning),
  writes the order (`channel = 'ucp'`, gateway `stripe`), charges, records
  the payment, runs `fulfilOrder` — the storefront's fulfilment, invoice
  number without gaps — marks the session `completed` and queues the order
  event. A completion that died after charging is finished by the next one
  without a new charge; one stuck `complete_in_progress` for 5 minutes may be
  resumed.
- **Order events** (`_ucp-order.ts`) are the full order, POSTed to
  `SHOP_UCP_WEBHOOK_URL` through the outbox (kind `ucp`, retried with
  backoff, visible in the panel when they fail) on completion and when
  Stripe's webhook confirms a refund (`_agent-refund.ts`). Headers:
  `UCP-Agent` with the shop's profile URL, `Idempotency-Key` and
  `Webhook-Id` (the order id, or `adj_<payment id>` for a refund),
  `Webhook-Timestamp`, `X-Goog-Api-Key` when set, and an RFC 9421 signature:
  `Content-Digest` (SHA-256), `Signature-Input` covering `@method`,
  `@authority`, `@path`, `@query` when present, `ucp-agent`,
  `idempotency-key`, `content-digest` and `content-type` with `created` and
  `keyid`, and `Signature` (ES256, raw r‖s). The URL, its key and the
  signing key are read at delivery, never queued.
- **The waiver**: when `legal.digital_waiver` is on (the default), every
  priced session carries a `digital_waiver` warning with `presentation:
  "disclosure"`, which the platform must show and must not let be dismissed,
  and the order records `consent_waiver = 1`. Whether that counts as the
  buyer's consent where you sell is a legal question for you.

## Test mode for the conformance suite (since 1.9.11)

The UCP conformance suite
([github.com/Universal-Commerce-Protocol/conformance](https://github.com/Universal-Commerce-Protocol/conformance),
read at commit `0642adf`, 2026-11-13) drives the shop from a local machine
with things no real platform sends. `SHOP_UCP_TEST_MODE = "true"`
(`_ucp-testing.ts`) accepts them, and nothing else changes:

- `UCP-Agent: profile="http://…"` as well as `https://`;
- an `http://` `SHOP_UCP_WEBHOOK_URL`, for the suite's local receiver;
- the suite's test payment handler: `handler_id` `mock_payment_handler`,
  `credential: {type: "token", token}` with `success_token` (paid in full,
  nothing charged, the payment row's reference starts `test_`, **the
  product is delivered**) or `fail_token` (answered as `payment_failed`,
  nothing charged). It is not advertised in the profile. Card numbers
  (`credential.type: "card"`) are never accepted, test mode or not;
- `POST /testing/simulate-shipping/{order id}` with `Simulation-Secret`
  equal to `SHOP_UCP_SIMULATION_SECRET` (403 otherwise): it sends the
  order's current snapshot again as an order event, with one `shipped`
  fulfillment event described as simulated, and stores nothing — a
  download never ships. Pages routes it only with `/testing/*` in
  `routes_include`.

The X-API-Key is still required: the suite cannot send one, so put
something in front of the shop that adds it (the run in
[docs/UCP-OPERATIONS.md](../../docs/UCP-OPERATIONS.md#the-conformance-suite)
did). Use the mode on a test deployment only: anyone holding the API key
can take a product for a test payment. It is refused next to a live
configuration.

## Where the documents are ambiguous, and what this code does

1. **Which version.** The protocol's latest release on 2026-11-15 was
   `v2026-08-25`; Google's native checkout guide covers `2026-04-08` and
   `2026-01-23`. The shop speaks `2026-04-08` only, in every `ucp` envelope
   and in the profile.
2. **Who is calling.** The protocol leaves authentication to the business
   (API key in `X-API-Key`, OAuth 2.0, mTLS or HTTP Message Signatures);
   Google lists an API key, OAuth 2.0 (which it recommends) or mTLS. The
   shop implements the API key only, compared in constant time. The protocol
   makes every method optional ("Authentication is optional and depends on
   business requirements", checkout-rest.md), so nothing more is required.
   **mTLS** would be terminated by Cloudflare before the Worker runs (API
   Shield's mTLS rules on the zone); the Worker only sees what Cloudflare
   reports about the client certificate, so the check belongs in a zone
   rule, not in this code, and none is set up here. **OAuth 2.0** would need
   an authorization server that issues the platform's tokens; the shop has
   none.
   `UCP-Agent: profile="https://…"` is required on every call and checked for
   shape and the `https` scheme; the platform's profile is **not fetched** —
   the API key already says who is calling.
3. **`Idempotency-Key` optional or required.** `checkout-rest.md` says "When
   provided, the server MUST …"; the OpenAPI marks it required. It is
   optional here: a request without it runs unkeyed. When sent, the body is
   compared by the SHA-256 of its raw bytes; another body is `409
   idempotency_conflict`; a key still running is `409
   idempotency_in_flight` with `Retry-After` (taken over after 60 s); a 5xx
   is never stored; a key is honoured for the recommended 48 hours and rows
   are deleted after 7 days. A storage failure answers 503, as
   `signatures.md` asks ("fail closed").
4. **Country codes.** The protocol also accepts alpha-3 codes or a country
   name "for backward compatibility". Only ISO 3166-1 alpha-2 is read, because
   a tax rate is looked up by it; anything else counts as no country.
5. **A declined or failed payment.** Google asks for "200 OK even when your
   payment gateway returns a 4xx or 5xx". A decline or a Stripe failure on
   complete answers 200 with the session back at `ready_for_complete` and a
   `payment_declined` or `payment_failed` message, so the sheet can ask for
   another card; the same pending order is reused for the next token.
6. **`Webhook-Timestamp` format.** The protocol's header table says "unix";
   every Google example shows RFC 3339. The shop sends RFC 3339 (the time
   the event was queued). The signature's `created` parameter is unix
   seconds, as RFC 9421 defines it.
7. **Totals with tax included.** For a shop that prices gross (the default)
   the totals follow Google's "Tax-inclusive pricing": a `subtotal` labelled
   "Subtotal (including taxes)" and a `total`, no `tax` line. A net-priced
   shop shows `subtotal`, `tax` and `total`.
8. **Expired and escalated sessions.** The protocol has no status for an
   expired session; it becomes `canceled`. `requires_escalation` is never
   set: everything this shop needs can be given through the API.
9. **PayPal.** Google's processor table lists PayPal too, but its Google Pay
   flow needs PayPal's own client-side confirmation, which a server-side
   completion cannot do. Only Stripe is offered.
10. **Not implemented**: OAuth 2.0 and mTLS, the fulfillment capability and
    shipping, discounts, cart, identity linking, `requires_escalation`, and
    `PUT /orders/{id}` (the suite's order tests write orders that way; the
    protocol defines only Get Order).
11. **Fulfillment for downloads.** fulfillment.md: "Fulfillment applies only
    to items requiring physical delivery. Items not requiring fulfillment
    (e.g., digital goods) do not need to be assigned to a method." The shop
    does not advertise `dev.ucp.shopping.fulfillment`; inventing a shipping
    method for a download would mislead the buyer. Google's certification
    profile in the suite requires it, so that profile cannot pass.
12. **The handler's id.** The protocol names the handler by its key,
    `com.google.pay`; the `id` is the business's own (Google's handler page
    uses a UUID). The suite's `google` profile looks for `google_pay`, the
    id of its sample server. The default stays `gpay`, so nothing changes
    for a shop already set up; `SHOP_UCP_HANDLER_ID = "google_pay"` satisfies
    the suite. Changing it mid-way breaks a platform's cached instruments
    for up to the 300 s the profile may be cached.
13. **`payment` in every checkout response.** Optional in checkout.json;
    always sent (`{instruments: []}` until the platform has paid), because
    platforms read it back on update (since 1.9.11).

## Tests

`test/ucp-*.test.ts` (`ucp-testmode.test.ts` for the test mode and Get
Order) and `test/agent-refund.test.ts`, in the shop's suite.
Real SQL on the D1 fake; Google's platform, Stripe and an order-event
receiver that verifies the RFC 9421 signature are fakes written from the
documents above (`test/ucp-helpers.ts`). Nothing was run against Google,
Google Pay or Stripe.
