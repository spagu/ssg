# Selling through Google's agents with UCP

The **Universal Commerce Protocol** ([ucp.dev](https://ucp.dev/), repository
[github.com/Universal-Commerce-Protocol/ucp](https://github.com/Universal-Commerce-Protocol/ucp))
is the protocol Google uses to let a shopper buy in AI Mode in Google Search
and in the Gemini app, paying with Google Pay. This guide is for a site owner
running the ssg shop (`workers/ecommerce`). It is a separate protocol from
ACP, with its own steps; ACP has its own guide: [ACP.md](ACP.md).

ssg sells nothing and is not part of Google's programme. The Merchant Center
account, Google's approval, the payment accounts and their terms are yours.
Facts about Google's process below were read on **2026-11-15**; each has a
link so you can check it again.

## The versions involved

- The shop implements UCP release **`2026-04-08`**
  (`functions/api/shop/_ucp-http.ts`). The protocol's latest release on
  2026-11-15 was `v2026-08-25`; Google's native checkout guide covers
  `2026-04-08` and `2026-01-23`.
- Google Pay payment handler `2026-01-23`; Google Pay tokens are processed
  by Stripe, the only processor the code implements.

## What ssg and the shop do, and what they do not

| Part | Done by |
|---|---|
| UCP attributes in the Merchant feed (`native_commerce`, variants, notices, `merchant_item_id`) | ssg, at build time: `ucp:` on a `google-merchant` feed |
| Checking them | ssg: `check_ucp: warn` or `strict` |
| Getting the feed into Merchant Center | you: Merchant Center fetches the URL on the schedule you set |
| Business profile at `/.well-known/ucp` | the shop worker |
| Checkout endpoints, Google Pay through Stripe, signed order events | the shop worker, off by default |
| Merchant Center settings, waitlist, Google's approval | **you** |

The shop sells **digital products only**, with no shipping, no discounts, no
cart transfer and no identity linking.

## Before you start (Google's requirements)

From Google's
[*Prepare your Merchant Center account*](https://developers.google.com/merchant/ucp/guides/overview/merchant-center)
(page dated 2026-10-29) and
[*UCP implementation overview*](https://developers.google.com/merchant/ucp/guides/overview):

1. A Merchant Center account in good standing, with products approved for
   free listings.
2. A return policy in Merchant Center with return cost, return window and a
   link to the full policy.
3. Customer support information: at least one of a website URL, an email
   address or a phone number.
4. Join the waitlist at
   [support.google.com/merchants/contact/ucp_integration_interest](https://support.google.com/merchants/contact/ucp_integration_interest).
   Google: "Your integration must be approved by Google before you can go
   live on AI Mode in Google Search and Gemini."
5. A Stripe account (for the card processing behind Google Pay) and, for
   production, a Google Pay merchant id.

**Check that your products may be sold this way.** Google lists categories
that are ineligible for checkout. One of its headings is "Digital goods and
services", with services, rentals, virtual items (its examples are in-game
goods) and goods that need software installed first. Whether e-books and
other downloads fall under it is not stated. **Ask Google before you build
on it.**

## Step 1: UCP attributes on the Merchant feed

UCP has no feed format of its own: Google reads checkout eligibility from the
products it already has in Merchant Center. So the attributes go on the
`google-merchant` feed, switched on with `ucp:`:

```yaml
feeds:
  - path: /merchant.xml
    format: google-merchant
    source: shop
    merchant:
      brand: Example Press
    ucp:
      checkout_eligibility: true   # the default once ucp: is present
check_ucp: strict
```

Product pages need `Product` JSON-LD in `schema:` as for any Merchant feed;
the example in [ACP.md](ACP.md#step-1-give-each-product-page-structured-data)
applies unchanged. Every key and attribute:
[CONFIGURATION.md](CONFIGURATION.md#universal-commerce-protocol-ucp-on-the-merchant-feed).

**The id the checkout receives is your shop's SKU.** Google sends
`merchant_item_id` when a product has one, otherwise `id`. ssg writes `id`
from the page's `sku`. The shop looks the product up by SKU, so either keep
`sku` equal to the SKU in the shop panel, or set `merchant.merchant_item_id`
to it on the page.

ssg writes `checkout_eligibility` `FALSE` for what it can see is ineligible:
pre-orders, used or refurbished items, and offers with no returns. For any
other ineligible product set `merchant.checkout_eligibility: false` on its
page.

`check_ucp` messages name the source file, feed, item and field. A few:

| Message contains | Meaning |
|---|---|
| `the row is left out` | a required Merchant attribute or the currency is missing |
| `written FALSE: Google lists … as ineligible` | ssg overrode checkout eligibility |
| `is stated by no page` | availability was assumed, and checkout sells what the feed says is in stock |
| `notice_type` / `notice_message` | a `consumer_notice` Google would reject |
| `variant_option` / `item_group_title` | a variant Google cannot show as one |

As with ACP, a feed with **no rows at all** is a finding (since 1.9.10):
`warn` reports it with the likely cause (no page with `schema:` Product, or
every product missing a required attribute), `strict` fails the build, and
with `check_ucp` off the build prints one line about it.
`examples/ebook-shop` in the repository declares such a feed, one row per
book.

Add the feed URL to Merchant Center as a data source. Google recommends a supplemental data source for these attributes;
ssg puts them in the main feed, so they arrive with the products.

## Step 2: switch on the shop's UCP checkout

Route the profile to Functions. In `.ssg.yaml`, the shop's `workers:` entry:

```yaml
    routes_include:
      - /api/shop/*
      - /.well-known/ucp
```

Make the signing key for order events, an EC P-256 private JWK with a `kid`
(Node 23.6 or newer; keep the output secret):

```sh
node --input-type=module -e 'const k=await crypto.subtle.generateKey({name:"ECDSA",namedCurve:"P-256"},true,["sign"]);const j=await crypto.subtle.exportKey("jwk",k.privateKey);console.log(JSON.stringify({kty:j.kty,crv:j.crv,x:j.x,y:j.y,d:j.d,kid:"shop-2026-11"}))'
```

In the shop's `wrangler.toml` `[vars]`:

```toml
SHOP_UCP_ENABLED = "true"
SHOP_UCP_STRIPE_PUBLISHABLE_KEY = "pk_test_…"   # public: it is in the profile
SHOP_UCP_GPAY_ENVIRONMENT = "TEST"              # "PRODUCTION" once approved
# SHOP_UCP_GPAY_MERCHANT_ID = "…"               # required with PRODUCTION
# SHOP_UCP_HANDLER_ID = "gpay"                  # this shop's id for its Google Pay handler
SHOP_UCP_WEBHOOK_URL = "https://webhooks.example.com/v1/webhooks/partners/PARTNER_ID/events/order"
```

Secrets, with `npx wrangler pages secret put <NAME>`: `SHOP_UCP_API_KEY` (the
key Google sends in `X-API-Key`), `SHOP_UCP_SIGNING_KEY` (the JWK above),
`SHOP_UCP_WEBHOOK_API_KEY` (sent as `X-Goog-Api-Key` with order events), and
`STRIPE_SECRET_KEY`. Google's
[order lifecycle page](https://developers.google.com/merchant/ucp/guides/overview/order-lifecycle)
says it shares a partner id and an API key with you during onboarding; put
the URL without `?key=` in `SHOP_UCP_WEBHOOK_URL`.

With `SHOP_UCP_ENABLED` unset, `/.well-known/ucp` and every UCP route are 404.
With it on and something missing (the API key, the Stripe secret key, a
`pk_test_`/`pk_live_` publishable key, a valid signing key, or the Google Pay
merchant id in `PRODUCTION`) they are 503. Google's
[authentication guide](https://developers.google.com/merchant/ucp/guides/overview/authentication)
offers an API key, OAuth 2.0 (which it recommends) or mTLS; **the shop
implements the API key only**.

| Route under `https://<your-site>/api/shop/ucp` | Operation |
|---|---|
| `POST /checkout-sessions` | create |
| `GET /checkout-sessions/{id}` | read |
| `PUT /checkout-sessions/{id}` | update |
| `POST /checkout-sessions/{id}/complete` | pay and place the order |
| `POST /checkout-sessions/{id}/cancel` | cancel |

The profile at `https://<your-site>/.well-known/ucp` names this endpoint, the
checkout and order capabilities, the Google Pay handler and the public half
of your signing key. Google finds everything else from it.

## Step 3: test, then watch it

Testing before Google does, what Google publishes about its checks, the
monitoring, the "my product disappeared" checklist, how the code handles
security and idempotency, and what was never tried against Google's live
systems: [UCP-OPERATIONS.md](UCP-OPERATIONS.md). Where the specification left
room and what the shop's code chose:
[workers/ecommerce/UCP.md](../workers/ecommerce/UCP.md).
