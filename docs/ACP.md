# Selling through AI agents with ACP

This guide is for a site owner who runs the ssg shop (`workers/ecommerce`) and
wants an AI agent platform to show their products and, where the platform
allows it, sell them. It covers what you set up in ssg and in the shop, in the
order you do it. Testing, certification, monitoring and the "my product
disappeared" checklist are in [ACP-OPERATIONS.md](ACP-OPERATIONS.md). Google's
own protocol, UCP, has its own guide: [UCP.md](UCP.md).

ssg sells nothing and is not part of any platform's programme. Applying,
the payment account, the platform's terms and any certification are yours.
This page says what the code does and what the protocol asks. It does not say
which platform or provider to use.

## What ACP is

The **Agentic Commerce Protocol** is an open specification, maintained by
OpenAI and Stripe and marked *beta*, for an AI agent to buy from a seller on
a buyer's behalf. It has two parts that matter here:

- a **product feed**: a file describing your products, which the platform
  reads to show them;
- a **checkout API**: REST endpoints on your side that the agent calls to
  create a checkout session, update it, and complete it with a payment token.

Specification: [github.com/agentic-commerce-protocol/agentic-commerce-protocol](https://github.com/agentic-commerce-protocol/agentic-commerce-protocol).
On 2026-11-15 its latest dated release was `2026-04-17` (`spec/2026-04-17/`).
The shop implements that release. The feed file follows OpenAI's
[product feed schema](https://developers.openai.com/commerce/specs/file-upload/products),
which carries no version or date.

## Where it can be used (checked 2026-11-15)

These facts change often. Check each platform's own pages before you apply.

- **The protocol's site** ([agenticcommerce.dev](https://www.agenticcommerce.dev/))
  says each AI platform runs its own process for which businesses may take
  part, names ChatGPT as the first platform, and says businesses that want to
  take part in ChatGPT must apply. It lists no other platform.
- **OpenAI** ([developers.openai.com/commerce](https://developers.openai.com/commerce/)):
  you claim your business in Business Hub, verify your identity, then send a
  product feed by manual upload, a hosted HTTPS URL or SFTP. The feed schema
  says checkout "requires a separately enabled integration" and that setting
  `is_eligible_checkout` "does not complete checkout onboarding". The public
  pages do not say how that integration is enabled, and they describe no
  checkout-session API for merchants. In-chat purchase is described through
  ChatGPT Wallet on your own website or through a plugin.
- **Microsoft Copilot Checkout**
  ([Microsoft Advertising, agentic commerce](https://about.ads.microsoft.com/en/solutions/technology/agentic-commerce)):
  the page names Microsoft Merchant Center with a UCP feed as the approved
  onboarding path, and English-language merchants selling to US buyers as the
  only ones eligible. It does not describe connecting your own ACP checkout
  endpoint.

**So, on that date:** the ACP **feed** has a documented use (ChatGPT
discovery). No platform we could find documents how a merchant connects its
own ACP **checkout** endpoint. The shop's checkout is ready for a platform
that asks for one, and is off until you switch it on.

## What ssg and the shop do, and what they do not

| Part | Done by | Notes |
|---|---|---|
| The feed file | ssg, at build time | `feeds:` with `format: acp`, JSON Lines |
| Checking the feed | ssg, at build time | `check_acp: warn` or `strict` |
| Sending the feed | you, or `acp-feed-push/` | a hosted URL needs nothing; a push needs the small Cron Worker |
| The checkout endpoints | the shop worker | `/api/shop/acp/checkout_sessions…`, off by default |
| Charging the buyer | the shop worker, through Stripe | Shared Payment Token only |
| Order events to the platform | the shop worker | signed, through the shop's outbox |
| Applying, terms, certification | **you** | the platform decides |
| Stripe account and its agreements | **you** | the shop only uses the key you give it |

The shop sells **digital products only**: a physical product is refused at
checkout. There is no 3-D Secure step, no PayPal for ACP and no discounts. A
refund made later is sent to the platform as an `order_update` once Stripe's
webhook confirms it.

## Before you start

You arrange these yourself:

1. A working shop: the `ecommerce` worker deployed, products created in its
   panel with a SKU and a price, and a normal web sale tested
   ([shop README](../workers/ecommerce/README.md)).
2. Acceptance by the platform, under its terms. Being accepted is its decision.
3. A Stripe account able to receive Shared Payment Tokens. Stripe's
   [seller page](https://docs.stripe.com/agentic-commerce/concepts/shared-payment-tokens)
   (read 2026-11-15) lists the countries, asks for a Stripe profile, and ties
   the feature to its own preview terms of service.
4. From the platform: the API key it will send you, the URL that receives
   order events and the secret to sign them, and what it wants as your
   merchant id.

## Step 1: give each product page structured data

The feed is read from the page's `Product` JSON-LD (`schema:` in front matter,
merged with site and section `schema:`), the same data as the Google Merchant
feed. A page with only `sku:` and `price:` as plain front matter gives no row.

```yaml
---
title: The One-Person Shop
schema:
  "@type": Product
  name: The One-Person Shop
  description: Selling your own work online, the tax and the invoices.
  sku: EBOOK-SHOP              # must equal the product's SKU in the shop panel
  image: /images/one-person-shop.jpg
  offers:
    price: "24.00"
    priceCurrency: EUR
    availability: https://schema.org/InStock
---
```

**The SKU is the link between feed and checkout.** The feed writes it as
`item_id`; the agent sends that id back to the checkout, and the shop looks
the product up by SKU (`functions/api/shop/_orders.ts`). If they differ, the
checkout answers "This shop has no such product" and the sale stops there.
Keep SKUs stable: to the platform a new id is a new product.

Use a JPEG or PNG for the image: OpenAI's schema asks for a direct image URL
"such as a JPEG or PNG". The build does not check the image type, on
purpose: on 2026-11-15 the schema gave those two as examples, not as a closed
list, so a warning would state a rule OpenAI does not; and a URL's extension
does not prove the file's format (an image CDN often has none). OpenAI's
upload report says what it rejected.

`examples/ebook-shop` in the repository does exactly this: each book's page
has a `schema:` Product with its shop SKU and a PNG cover, and its `ssg.yaml`
declares this feed (and a Merchant feed with `ucp:`), one row per book.

## Step 2: add the feed to `.ssg.yaml`

```yaml
feeds:
  - path: /acp/products.jsonl
    format: acp
    source: shop
    merchant:
      brand: Example Press
    acp:
      seller_name: Example Press      # required
      target_countries: [US]          # required, ISO 3166-1 alpha-2
      store_country: US               # required
      seller_url: /
      seller_privacy_policy: /privacy/   # needed when a row asks for checkout
      seller_tos: /terms/                # needed when a row asks for checkout
      is_digital: true
      is_eligible_checkout: false     # true only once the platform has enabled checkout
check_acp: strict
```

Every key and how each feed field is filled:
[CONFIGURATION.md](CONFIGURATION.md#product-feed-for-the-agentic-commerce-protocol-acp).
Without `seller_name`, `target_countries` or `store_country` the build stops
and names them. OpenAI's schema says standard uploads use `US` whatever
`target_countries` says, and other markets need OpenAI's confirmation.

## Step 3: read what the build says

With `check_acp: warn` the build lists problems; with `strict` it fails on
them. Each line names the source file, the feed, the item and the field:

| Message ends with | Meaning | Fix |
|---|---|---|
| `required and missing — the row is left out` | no title, description, url, brand, image or price | add it to `schema:` or the feed's `merchant:` |
| `has no currency; set offers.priceCurrency` | a bare price | add `priceCurrency` |
| `is stated by no page` | availability came from the feed default or was assumed `in_stock` | set `offers.availability`, or `merchant.availability: unknown` when you truly do not know (written as `unknown`, which OpenAI's feed spec accepts — "Use `unknown` explicitly when stock status is unavailable", read 2026-11-15) |
| `is not "amount CURRENCY"` / `not in the price's currency` | a malformed `sale_price` or `shipping_price` | write `12.00 EUR`, same currency as the price |
| `must be unique in the feed` | two rows share an `item_id` or `offer_id` | give each product its own SKU |
| `does not resolve in the output` | a product or policy URL on this site leads nowhere | fix the link or the page |
| `required when a row has is_eligible_checkout: true` | no privacy policy or terms URL | set `acp.seller_privacy_policy`, `acp.seller_tos` |
| `true while is_eligible_search is false` | checkout asked for on a row hidden from search | turn search on, or checkout off |

| `has no rows — no selected page has Product JSON-LD` | the feed is empty: no selected page has a `schema:` Product (plain `sku:`/`price:` front matter gives no row) | add `schema:` as in step 1, or check the feed's `source:`/`type:` selection |
| `has no rows — all N product(s) are left out` | every product lacks a required field | fix the fields named on the lines for each item |

A feed with **no rows at all** is a finding (since 1.9.10): `warn` reports
it, `strict` fails the build, and with `check_acp` off the build prints one
line about it. The count the build prints (`acp/products.jsonl (N item(s))`)
is the quickest check.

## Step 4: switch on the checkout endpoint

Only when a platform has given you its credentials. In the shop's
`wrangler.toml` `[vars]`:

```toml
SHOP_ACP_ENABLED = "true"
SHOP_ACP_MERCHANT_ID = "profile_…"     # the id the platform asks for (your Stripe account or profile)
SHOP_ACP_WEBHOOK_URL = "https://agent-platform.example.com/order_events"
```

Secrets, with `npx wrangler pages secret put <NAME>`: `SHOP_ACP_API_KEY` (the
bearer token the platform sends), `SHOP_ACP_WEBHOOK_SECRET` (signs order
events), `STRIPE_SECRET_KEY` (already set for web sales), and optionally
`SHOP_ACP_SIGNING_SECRET` (makes the shop require signed requests).
`SHOP_ACP_STRIPE_VERSION` overrides the Stripe API version, default
`2026-09-30.preview`, the version Stripe's seller page used on 2026-11-15.

Give the platform `https://<your-site>/api/shop/acp` as the base URL. With
`SHOP_ACP_ENABLED` unset every ACP route is 404; with it on but the API key,
the Stripe key or the merchant id missing, every ACP route is 503. Routes,
headers, and the places the specification contradicts itself:
[workers/ecommerce/ACP.md](../workers/ecommerce/ACP.md).

## Step 5: deliver the feed

- **The platform fetches a URL** (OpenAI's hosted HTTPS URL option): give it
  `https://<your-site>/acp/products.jsonl`. It changes when you rebuild.
- **The platform wants it pushed**: deploy `workers/ecommerce/acp-feed-push/`,
  a Worker with one Cron Trigger (every six hours by default). Set
  `ACP_FEED_SOURCE_URL`, `ACP_FEED_TARGET_URL` and `ACP_FEED_METHOD` in its
  `wrangler.toml`, the platform's credential as the `ACP_FEED_TOKEN` secret,
  then `npx wrangler deploy`. It sends the file as it is, never an empty one.
  It does not speak the protocol's own Feed API (`PATCH /feeds/{id}/products`
  with a different JSON shape).
- **Manual upload or SFTP**: upload the built file yourself; ssg does not
  talk SFTP.

Then: [test before certification, monitor, and what to do when a product
disappears](ACP-OPERATIONS.md).
