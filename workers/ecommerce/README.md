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

### Lists, at the size a shop actually reaches

Every list — products, orders, the queue, the audit log — is filtered, sorted
and paged by the API rather than in the browser. The paging is by **cursor**,
not by offset: an offset page shifts under you as rows arrive, so page two skips
the row page one pushed down and shows another twice. The cursor is the last
row's sort value *and* its id, because two orders can share a timestamp — or,
sorted by amount, a total — and a cursor that cannot break that tie loses rows.

The panel remembers the cursor that started each page, which is what makes
"previous" exact rather than arithmetic.

Two things this fixed rather than improved:

- The catalogue looked its prices up with `WHERE product_id IN (?, ?, …)` — one
  bound parameter per product. **D1 refuses a statement with too many**, so the
  page did not slow down as a shop grew; it stopped loading, at a size a demo
  never reaches. Prices are read for one page at a time and in chunks even then
  (`_catalogue.ts`).
- The public catalogue answered with every active product. A storefront page
  shows a handful, so it now asks for those by code — `/api/shop/products?sku=A,B`
  — and gets a page otherwise.

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

### Who can get in

Two roles, and the difference is the answer to one question: can this person
change what the shop is or what it charges?

| | Staff | Owner |
|---|---|---|
| Read orders, open one, take a note | ✓ | ✓ |
| Resend a buyer's download links | ✓ | ✓ |
| Products, prices, files | | ✓ |
| Refund, mark paid, cancel | | ✓ |
| Settings, VAT rates, modules, payment providers | | ✓ |
| Accounts | | ✓ |

Staff exists so the person who answers the email does not hold the credentials
that could change your VAT number.

An account can be **suspended** rather than deleted: its sessions end
immediately — including the fifteen-minute token it is holding, which is checked
on every request, not only at sign-in — and the audit entries it left keep its
name, because "who did this" has to go on having an answer. Deleting works too,
and the log still names them.

The shop refuses to be left without an owner who can sign in: the last one
cannot demote, suspend or delete themselves. Changing a password ends every
session for that account, and an owner can reset a colleague's without knowing
it; changing **your own** asks for the current one first, so a borrowed session
cannot become a permanent one.

### Payments, and where the keys are

The panel has a payments screen. It does **not** have a box to paste an API key
into, and that is the deliberate part.

Magento, PrestaShop and WooCommerce all keep provider keys in the database
behind an admin form. It is the obvious design, and it is why "database dump"
and "stolen payment credentials" are so often the same incident: the keys are in
every backup, readable by anything with a database connection, and the admin
panel becomes the thing worth breaking into. Shopify sidesteps it by not having
the keys — the platform is the payment provider.

On Cloudflare there is a third option. Keys are wrangler **secrets**: encrypted
at rest, injected into the running worker, and readable by nothing else — not
the panel, not this API, not a database export, not a screen share. A test
asserts that the payments endpoint's response contains no key material at all.

What the screen does instead is the part sellers actually get stuck on:

- whether each provider is configured, and **exactly which secret is missing**
- whether the keys in use are test or live keys
- the **exact webhook URL** to paste into the provider's dashboard, and the list
  of events it must send — the half that fails quietly, because without it a
  buyer pays and is never sent their book
- whether a webhook has ever arrived, what it was, and when
- how many payments each provider has actually taken
- which providers are offered at checkout and in what order

A provider with missing credentials cannot be offered: the button would take a
buyer to a 502 at the last step of a purchase. One taken off the list is not
reachable by posting its name either.

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
