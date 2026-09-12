# ecommerce — a small shop for digital products

Sells ebooks (or any file) from a static site, worldwide, with VAT worked out
per country, invoices that survive an audit, and download links that expire.
Runs on Cloudflare Pages Functions with D1, R2 and KV. No npm packages at
runtime: the deployed tree is the source tree.

It is deliberately a *small* shop — a few products, one seller, one person
looking after it. It is not a platform, has no stock control, and does not try
to be Shopify.

```
GET  /api/shop/products            the catalogue
GET  /api/shop/products/:sku       one product
POST /api/shop/checkout            basket → order → payment URL
GET  /api/shop/orders/:id/status   what the thank-you page waits on
GET  /api/shop/download/:token     the file, with Range support
POST /api/shop/resend              lost the email
GET  /api/shop/invoices/:number    the invoice, as HTML you can print
POST /api/shop/webhooks/stripe     Stripe tells us the money arrived
POST /api/shop/webhooks/paypal     PayPal does the same
     /api/shop/admin/*             everything behind authentication
```

The admin panel is at **/ecommerce-admin/**.

## Five minutes to a working shop

```sh
npx wrangler d1 create ssg-shop
npx wrangler r2 bucket create ssg-shop-files
npx wrangler kv namespace create SHOP_KV
```

Paste the three ids into `wrangler.toml` (copy the stubs from
`wrangler.snippet.toml`; SSG appends them for you when it generates a starter
config). Then set the secrets:

```sh
npx wrangler pages secret put SHOP_JWT_SECRET       # openssl rand -base64 48
npx wrangler pages secret put SHOP_ADMIN_BOOTSTRAP  # owner@example.com:a-long-password
npx wrangler pages secret put SHOP_IP_SALT          # openssl rand -base64 24
npx wrangler pages secret put STRIPE_SECRET_KEY
npx wrangler pages secret put STRIPE_WEBHOOK_SECRET
```

Point a Stripe webhook at `https://<your-site>/api/shop/webhooks/stripe` for
`checkout.session.completed`, `checkout.session.expired`,
`payment_intent.payment_failed` and `charge.refunded`.

Open `/ecommerce-admin/`, sign in with the bootstrap credentials, and the panel
tells you what is still missing. **Delete `SHOP_ADMIN_BOOTSTRAP` once you are
in** — it keeps warning you until you do.

A complete working example, with content, a theme and two sample books, is in
[`examples/ebook-shop/`](../../examples/ebook-shop/README.md).

## The database sets itself up

There is no migration step. The worker brings its own tables up to date on the
first request of each isolate (`functions/api/shop/_schema.ts`), and seeds the
EU + UK VAT rates so a new shop is not unusable on day one. `schema.sql` is the
same thing written out for a human; a test proves the two cannot drift apart.

**The seeded rates are a starting point, not tax advice.** The panel warns when
the newest rate is over a year old. Your accountant has the last word.

## How it is put together

Files with a leading underscore are modules, not routes — Pages ignores them
when building the route table.

| File | What it is for |
|---|---|
| `_env.ts` | every binding and variable, and the one place that decides whether a feature is configured |
| `_lib.ts` | responses, ids, hashing, validation, the CSV and filename guards |
| `_money.ts` | integer minor units, per-currency decimals, half-up rounding |
| `_schema.ts` | numbered migrations, run once per isolate |
| `_tax.ts` | place of supply, evidence, reverse charge, the rate lookup |
| `_settings.ts` | what the owner edits, cached for five seconds |
| `_gateway.ts` | what a payment provider is; the registry |
| `_gateways.ts` | the registry with Stripe and PayPal actually in it |
| `_orders.ts` | server-side pricing, customers, order creation |
| `_invoice.ts` | gapless numbering, the frozen snapshot, the printable HTML |
| `_fulfil.ts` | the one path from "paid" to "delivered", safe to run twice |
| `_webhook.ts` | verify → deduplicate → apply, and always 200 to the provider |
| `_downloads.ts` | tokens, use counting, Range windows |
| `_outbox.ts` | email, webhooks and tracking, retried with backoff |
| `_auth.ts` | PBKDF2 passwords, HS256 tokens, rotating refresh |
| `_access.ts` | Cloudflare Access verification, for shops that prefer an IdP |

### Two decisions worth knowing about

**Nothing trusts the browser about money.** The checkout endpoint is sent
product codes and quantities. Prices, tax rates and totals come from the
database, every time. A basket with a price in it would be a price a buyer can
edit.

**Everything is safe to run twice.** D1 has no transaction spanning several
requests, so instead of a rollback that does not exist, each step carries its
own "have I already done this": the move to `paid` names the states it may move
from, the webhook inbox row is the lock (`INSERT OR IGNORE`), a download token
is issued only where no live one exists. Two webhooks racing produce one email.

### Modules

What the shop **does**, as opposed to what it is configured with. Each switch is
read where the work happens, not only where it is drawn — turning emails off
stops them being queued at all — and the panel has a screen for them.

| Module | Default | Off means |
|---|---|---|
| `invoices` | on | orders are still paid and delivered, with no invoice of ours |
| `emails` | on | the buyer gets the thank-you page and nothing in their inbox |
| `admin_notices` | on | no "you sold something" line to the owner |
| `webhooks` | on | your own systems are not told |
| `tracking` | **off** | nothing is sent to GA4 or Meta |
| `turnstile` | on | no anti-spam challenge, even with a secret set |
| `resend` | on | the public "lost my download" endpoint does not exist |

Two states are not enough to describe them, so there are three: on, off, and
**unavailable** — switched on but unable to run because a secret or a binding is
missing. The API refuses to switch on a module in that state and says what it
needs, rather than leaving the panel showing "on" and the shop doing nothing.

`tracking` is off by default on purpose. A shop should not start sending
purchases to Google because somebody happened to set a measurement id; and even
switched on, it only ever fires for a buyer who ticked the marketing box.

### What bounds how often it can be called

Turnstile raises the cost of abusing a public endpoint. It does not cap it, a
solved token can be replayed inside its validity window, and it is optional in
the first place — so every endpoint a stranger can reach has a budget:

| Bucket | Default | Fails | What it protects |
|---|---|---|---|
| `checkout` | 10 / minute | closed | writing an order and calling a payment provider |
| `resend` | 3 / 10 minutes | closed | sending email to an address the caller chose |
| `download` | 60 / minute | open | streaming files; ranged continuations are already free |
| `status` | 120 / minute | open | the thank-you page, which polls while it waits |
| `admin` | 300 / minute | open | the panel, against a stolen token |

Signing in is not in that table: it has a tighter throttle of its own, per
address **and** per account, with a lockout.

"Fails closed" is what happens when the backend itself errors. Open is right for
a download — losing a buyer's book because KV had a bad minute costs more than
serving one extra — and wrong for anything that writes or spends money.

`SHOP_KV` is enough to enforce all of it; KV is eventually consistent, so a
burst arriving at several points of presence at once can overshoot. A Workers
rate-limiting binding is exact and free, and replaces the counters for whichever
bucket has one (`RATE_LIMIT_CHECKOUT`, `RATE_LIMIT_RESEND`, … or a generic
`RATE_LIMITER`) — see `wrangler.snippet.toml`. With neither bound nothing is
enforced, and the panel says so rather than letting you assume otherwise.

The provider webhooks are deliberately **not** capped: Stripe and PayPal
legitimately burst, they retry on anything but a 200, and their requests are
signature-verified before anything is written. A budget there would drop real
payments.

This is separate from `workers/rate-limit`, the project-wide middleware with one
budget for everything under `/api/`. The two compose; they answer different
questions.

### Authentication, two ways

`SHOP_ACCESS_TEAM` + `SHOP_ACCESS_AUD` put the panel behind Cloudflare Access:
your identity provider does the authenticating and there is no password here to
leak. The login endpoint stops existing entirely in that mode.

Otherwise `SHOP_JWT_SECRET` enables the built-in login: PBKDF2-SHA256 at 600 000
iterations, a 15-minute access token held in memory only, and a rotating refresh
token in a `__Host-` cookie with reuse detection — a stolen refresh token used
twice ends every session for that account.

## Development

```sh
npm install          # tooling only; the runtime has no dependencies
npm run typecheck
npm test
npm run coverage     # 96% floor, the same as the rest of the repository
npm run check:bare-imports
```

Tests run inside workerd through `@cloudflare/vitest-pool-workers`, with real
local D1, R2 and KV. A Node mock of D1 would only prove the mock works.

> The pool pins vitest to 4.1.x. `@cloudflare/vitest-pool-workers@0.22.0` peers
> `vitest@^4.1.0`, so vitest 5 cannot be used here until the pool supports it.

## What it does not do yet

No subscriptions, no physical shipping, no VIES validation of the VAT numbers
buyers type (a well-formed number is *not* treated as verified, so reverse
charge does not apply on the strength of a string), no PDF invoices (the HTML
prints correctly to A4), no coupon codes. Each is a module that fits the shape
above; see the planning notes for the order they were meant to arrive in.
