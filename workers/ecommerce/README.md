# ecommerce — a small shop for digital products

Sells ebooks (or any file) from a static site, worldwide, with VAT worked out
per country, invoices that survive an audit, and download links that expire.
Runs on Cloudflare Pages Functions with D1, R2 and KV. No npm packages at
runtime: the deployed tree is the source tree.

It is deliberately a *small* shop — a few products, one seller, one person
looking after it. It is not a platform and has no stock control.

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
GET  /api/shop/module              the module manifest (behind the admin gate)
GET  /api/shop/module/summary      numbers and to-dos for the admin home page
     /api/shop/admin/*             everything behind authentication
```

The admin panel is at **/ecommerce-admin/**.

### Defaults

What a new shop does before anyone changes anything. Each is changeable later.

| Setting | Default | Where it lives |
|---|---|---|
| Prices | gross — the price on the page includes VAT, and the buyer's country decides how much of it is tax | `pricing.mode` in the panel's *Settings* |
| Currency | EUR | `currency.base` in the panel's *Settings* |
| Payment providers | Stripe and PayPal, Stripe's button first; each is offered once its secrets are set | `SHOP_GATEWAYS` (default `stripe,paypal`) and the panel's *Payments* |
| Invoices | HTML, numbered without gaps; the buyer prints it or saves it as PDF from the browser (A4) | the `invoices` module |

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
npx wrangler pages secret put SHOP_ADMIN_PASSWORD   # openssl rand -base64 24
npx wrangler pages secret put SHOP_IP_SALT          # openssl rand -base64 24
npx wrangler pages secret put STRIPE_SECRET_KEY
npx wrangler pages secret put STRIPE_WEBHOOK_SECRET
```

Point a Stripe webhook at `https://<your-site>/api/shop/webhooks/stripe` for
`checkout.session.completed`, `checkout.session.expired`,
`payment_intent.payment_failed` and `charge.refunded`.

Open `/ecommerce-admin/`, sign in with that password, and the panel tells you
what is still missing. The shared password is one of three ways in — see
[Who can use the admin panel](#who-can-use-the-admin-panel).

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
| `_auth.ts` | who may use the admin API: Access → `auth` → the shared password, CSRF |
| `_permissions.ts` | every admin route and the one permission it needs; the routes a service token may call |
| `_gate.ts` | the one gate in front of the admin API and the module routes |
| `_summary.ts`, `_summary-text.ts` | the module summary: the numbers, and their words in five languages |
| `_summary-kit.ts`, `_module.ts` | byte-for-byte copies of `workers/module-summary-kit.ts` and of `module.json`, never edited here |
| `_admin-password.ts` | the shared password over HTTP Basic, and its lockout |
| `_access.ts` | Cloudflare Access verification |
| `_session-verifier.ts` | the `auth` worker's portable verifier — a byte-for-byte copy, never edited here |

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

### Where an order came from: web, ACP, UCP

Every order records its **channel**: `web` (this site's checkout), `acp` (an
AI agent platform, [below](#selling-through-ai-agents-acp)) or `ucp` (Google's
agentic checkout). The panel shows it as a labelled badge — *Web*, *ACP*, *UCP*
— in the order list and on the order, never as colour alone, and the order list
filters by it: `GET /api/shop/admin/orders?channel=acp` (exact value; anything
else answers 422 `invalid_channel`; works with the other filters, the sort and
the cursor).

The overview's numbers (`GET /api/shop/admin/stats`) split the same way:
`byChannel` has orders, gross, refunds and net per channel and currency, next
to the per-currency totals in `byCurrency`. **Gross is every order paid in the
window — paid, fulfilled, or refunded since — and the refunds made in the
window come off it once**, as in the `core` summary — leaving a refunded order
out of gross *and* subtracting its refund would take its money off twice. VAT
collected leaves out fully refunded orders,
whose credit note gave the VAT back.

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

### Who can use the admin panel

The panel (`/ecommerce-admin/`) and its API (`/api/shop/admin/*`) use the same
three admin modes as every other worker in this repository. The first one that
is configured wins, in this order; with none of them the admin API answers
**503** — it is never open by default.

| Mode | Set | Choose it when |
|---|---|---|
| **Shared password** | the `SHOP_ADMIN_PASSWORD` secret | one person runs the shop — the simplest way in, no accounts, no second database |
| **Cloudflare Access** | `SHOP_ACCESS_TEAM` + `SHOP_ACCESS_AUD` | you already run Cloudflare Zero Trust: your identity provider signs people in, and there is no password here to leak |
| **`auth`** | `SHOP_AUTH_ISSUER`, `SHOP_AUTH_LOGIN_URL`, and `SHOP_AUTH_PUBLIC_KEY` or `SHOP_AUTH_JWKS_URL` | several people, or different permissions per person — see [`workers/auth`](../auth/README.md) |

- **Shared password.** The panel asks for it and sends it as HTTP Basic on
  every request; it stays in the page's memory only, so a reload asks again.
  `curl -u :<password>` works for scripts. Wrong passwords are counted per
  client address: `SHOP_ADMIN_MAX_FAILURES` (default 10) within
  `SHOP_ADMIN_LOCKOUT_MINUTES` (default 10) lock that address for one window
  — **429**, even for the right password, until the window passes. The count
  lives in the `auth_attempts` table.
- **Cloudflare Access.** Put `/ecommerce-admin/*` and `/api/shop/admin/*`
  behind an Access application first, then set both variables. The shop checks
  the signed token Access forwards (team keys, audience, issuer, expiry).
- **`auth`.** The `auth` worker's session cookie, checked here with its public
  key — the shop never asks `auth` per request. Any one `SHOP_AUTH_*` variable
  turns the mode on; an incomplete set is a **503** naming what is missing,
  never a fall-through to the password. While this mode is on, the shared
  password is **refused**: it would get past every permission, unrecorded. To
  switch to `auth`, set the variables, sign in through `auth`, then delete the
  `SHOP_ADMIN_PASSWORD` secret. A browser without a session is sent to
  `SHOP_AUTH_LOGIN_URL`; the panel shows who is signed in, hides what the
  account may not do, and names the missing permission when the API refuses.
  Scripts use `auth` service tokens — see
  [Scripts and CI](#scripts-and-ci-service-tokens).

| Variable | Mode | Meaning |
|---|---|---|
| `SHOP_ADMIN_PASSWORD` (secret) | password | the shared password |
| `SHOP_ADMIN_MAX_FAILURES` | password | wrong passwords per address before it is locked (10) |
| `SHOP_ADMIN_LOCKOUT_MINUTES` | password | the counting window and the lock, in minutes (10) |
| `SHOP_ACCESS_TEAM` | Access | `myteam` or `myteam.cloudflareaccess.com` |
| `SHOP_ACCESS_AUD` | Access | the Access application's AUD tag |
| `SHOP_AUTH_ISSUER` | auth | the `iss` to accept, e.g. `https://example.com/api/auth` |
| `SHOP_AUTH_LOGIN_URL` | auth | where a browser without a session signs in, e.g. `/auth/login` |
| `SHOP_AUTH_PUBLIC_KEY` | auth | `auth`'s public JWK or JWKS (recommended) — or: |
| `SHOP_AUTH_JWKS_URL` | auth | `/api/auth/jwks` (exactly one of the two) |
| `SHOP_AUTH_AUDIENCE` | auth | the `aud` to accept; default the site's host |

**Permissions.** In `auth` mode every admin route needs exactly one
permission (declared in `module.json`, granted through roles in `auth`). The
shared password and Access grant all of them.

| Permission | Allows | Routes |
|---|---|---|
| `ecommerce:read` | orders, products, sales figures, VAT rates, the audit log and the outbox | `GET` stats, audit, outbox, orders, orders/:id, products, products/:id, vat-rates; `GET /api/shop/module`, `/api/shop/module/summary` |
| `ecommerce:orders` | handling orders | `POST` orders/:id/resend, mark-paid, cancel; `PATCH` orders/:id (note); `POST` outbox (retry, send now) |
| `ecommerce:refund` | refunds — money leaves the shop | `POST` orders/:id/refund |
| `ecommerce:products` | products, their files and the VAT rates | `POST` products, products/:id/file, vat-rates; `PATCH`, `DELETE` products/:id; `DELETE` vat-rates |
| `ecommerce:settings` | settings and payment providers | `GET`, `PUT` settings; `GET`, `POST` gateways |
| `ecommerce:export` | exports with buyers' personal data | `GET` export |

`GET /api/shop/admin/me` needs a session only and tells the panel who is
signed in: `{mode, user, permissions, loginUrl, logoutUrl, refreshUrl, …}`.
The table lives in `functions/api/shop/_permissions.ts`; a test generated
from the route files fails for a route without a permission.

**Changes from the browser** in the Access and `auth` modes, whose credentials
the browser attaches by itself, must carry the `x-ssg-auth: 1` header and come
from the shop's own origin — the panel sends the header on every request.

**The audit log** records who did what: the `auth` account (`auth:<id>
<email>`), an `auth` service account (`service:<id>`), the Access address
(`access:<email>`) or `password`.

**An install from before this change** (the shop's own accounts, roles and
login, `SHOP_JWT_SECRET`, `SHOP_ADMIN_BOOTSTRAP`, `SHOP_ACCESS_OWNERS`): those
variables are no longer read — delete them and set one of the modes above.
Under Access, everyone Access lets through now has every permission; to limit
people, use `auth`. The tables `admin_users` and `admin_sessions` are no longer
used and are left in place; drop them by hand once you no longer need the old
accounts' history (audit entries name them as `admin:<id>`):

```sh
npx wrangler d1 execute ssg-shop --remote --command "DROP TABLE IF EXISTS admin_sessions; DROP TABLE IF EXISTS admin_users;"
```

### Scripts and CI: service tokens

A script — the accountant's monthly export, a monitoring check — should not
hold a person's session or the shared password. In the `auth` mode it uses an
`auth` [service token](../auth/docs/SERVICE-TOKENS.md): exchange the
`ssgsvc_…` secret for a short access token at `POST /api/auth/token`, then send
that as `Authorization: Bearer <token>`. It needs no `x-ssg-auth` header: a
Bearer token is nothing a browser attaches by itself.

A service token is accepted on these routes only — reads, chosen one by one
(`MACHINE_ROUTES` in `functions/api/shop/_permissions.ts`):

| Route | Permission the token must name |
|---|---|
| `GET /api/shop/admin/export` | `ecommerce:export` |
| `GET /api/shop/admin/stats` | `ecommerce:read` |
| `GET /api/shop/module` | `ecommerce:read` |
| `GET /api/shop/module/summary` | `ecommerce:read` |

- **By its exact name.** The token's permissions must list the route's
  permission itself; a wildcard (`ecommerce:*`, `*`) is refused with a 403
  that names what is missing. Grant the service account exactly what the
  script does.
- **Nothing that changes the shop.** Refunds, resends, marking paid, prices,
  settings: a person's decision, recorded under their name. Every other route
  answers a service token `403 {"error": "service_token_not_accepted"}`, with
  any permission. Adding a route to the list is a decision to make again and
  write down there; a test refuses one that is not a `GET`.
- The export a token downloads is recorded in the audit log as
  `service:<account id>`.
- Payment webhooks are not part of this: Stripe and PayPal stay public,
  checked by their own signatures.

```sh
curl -fsS -H "Authorization: Bearer $TOKEN" \
  "https://example.com/api/shop/admin/export?type=invoices&from=2026-10-01&to=2026-10-31" -o invoices.csv
```

### Part of the site's admin: the `core` worker

The shop is a module (`module.json`, [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)).
With the [`core`](../core/README.md) worker on the site:

- the admin home page lists the shop with its **summary**
  (`GET /api/shop/module/summary`, behind the same gate as the admin API,
  `ecommerce:read`): paid orders, revenue after refunds in the base currency
  and refunds over the last 30 days, products on sale; orders waiting for a
  review or for their download links, payment notifications that failed,
  outgoing messages that keep failing, Stripe test keys or the PayPal sandbox
  on a public host, VAT rates about to end; and what is missing — D1, R2, KV,
  a payment provider, mail, the seller's details for invoices. Counts only:
  no buyer's name, address or email is ever in it. Without D1, Access and
  `auth` still get a summary naming that; the password mode gets the admin
  API's 503;
- a shared admin bar appears above the panel — Admin home and a link to every
  panel the account may open — and the site's own look (`/core/theme.css`)
  restyles it. When the bar shows who is signed in and its own *Sign out*, the
  panel hides its own. **Without `core` nothing changes**, not a pixel
  (checked against screenshots): both files answer 404 and the empty bar is
  zero pixels tall. The page carries a strict CSP of its own (`<meta
  http-equiv>`: nothing inline, nothing from another origin; product images
  may be `https:`). Styles load in this order: `tokens.css` (every colour,
  font and shadow, named as in `core`'s tokens where the meaning is the same,
  `--link` for links and `--focus` for the focus ring), `/core/theme.css`,
  `admin.css`.

### Payments, and where the keys are

The panel has a payments screen. It does **not** have a box to paste an API key
into, and that is the deliberate part.

Keeping provider keys in the database behind an admin form is the obvious
design, and it is why "database dump" and "stolen payment credentials" are so
often the same incident: the keys are in every backup, readable by anything
with a database connection, and the admin panel becomes the thing worth
breaking into.

On Cloudflare there is another option. Keys are wrangler **secrets**: encrypted
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

## Selling through AI agents (ACP)

The shop can take orders from an AI agent platform through the
[Agentic Commerce Protocol](https://github.com/agentic-commerce-protocol/agentic-commerce-protocol)
(ACP) checkout API. It is **off** until you switch it on, and it is a
**starting point, not a certified integration**: it was tested against fakes
built from the published specification, never against a live platform.
[ACP.md](ACP.md) has the details — what the specification left open and what
this code chose, every setting, and the feed push.

**Where it can be used today** (checked 2026-11-14; partly from industry
reports rather than official statements, so check with each platform before
you apply):

- **Microsoft Copilot Checkout** runs ACP checkouts (with Stripe and PayPal).
  This is the live consumer of an ACP checkout as of that date.
- **ChatGPT** does not offer in-chat purchase any more: OpenAI's own
  commerce pages (developers.openai.com/commerce, read 2026-11-14) describe
  product feeds for discovery, with the purchase on the merchant's website
  (browser use, ChatGPT Wallet) or through a plugin. The ACP *feed* still
  matters there; this checkout does not.
- Google uses its own protocol (UCP, added to ssg separately); Anthropic and
  DeepSeek had no shopping or checkout protocol to implement on that date.

**What it does.** The agent creates, updates, reads, completes and cancels a
checkout session at `https://<your-site>/api/shop/acp/checkout_sessions…`.
Prices, VAT and currency come from the same code as the storefront checkout,
so an agent order costs exactly what the same basket costs on your site. The
buyer pays with a **Stripe Shared Payment Token** the platform grants your
Stripe account; the order then goes through the same fulfilment as any other
— an invoice number without gaps, download links, the buyer's email — and is
stored with `channel: "acp"` — an *ACP* badge in the panel, a filter on the
order list and its own row in the overview's figures. The platform is
told about the order with a signed `order_create` event through the shop's
outbox, with its retries.

**What you must do yourself** — ssg sells nothing and cannot do this for you:

1. Check the platform's current programme and apply as a merchant; agree to
   its terms. Being accepted is the platform's decision.
2. Have a Stripe account that can receive Shared Payment Tokens (Stripe lists
   the countries; the API is a preview and needs your Stripe profile).
3. Exchange credentials with the platform: the API key it will send you, the
   webhook URL and signing secret for order events, your Stripe account id.
4. Publish the product feed (`feeds:` with `format: acp` in `.ssg.yaml`) so
   the item ids the agent sends are your shop's SKUs.
5. Pass whatever conformance testing or certification the platform runs.
   Its current requirements could not be read from official pages on
   2026-11-14; ask the platform.

**Switching it on**: `SHOP_ACP_ENABLED = "true"`, `SHOP_ACP_MERCHANT_ID`,
`SHOP_ACP_WEBHOOK_URL` in `[vars]`; `STRIPE_SECRET_KEY`, `SHOP_ACP_API_KEY`,
`SHOP_ACP_WEBHOOK_SECRET` (and optionally `SHOP_ACP_SIGNING_SECRET`) as
secrets (`wrangler.snippet.toml` lists them). Give the platform
`https://<your-site>/api/shop/acp` as the base URL. With ACP on but a secret
missing every ACP route answers 503; with it off they answer 404 and nothing
else in the shop changes.

**The feed push** is not part of the build. If your platform takes a pushed
feed rather than fetching it from your site, deploy the small Worker in
[`acp-feed-push/`](acp-feed-push/wrangler.toml): a Cron Trigger that fetches
the published feed and sends it with your credential (Pages Functions cannot
run a Cron Trigger, so it is a Worker of its own; the site build never copies
it). Steps in [ACP.md](ACP.md#the-feed-push).

**Limitations**: digital products only (stock is unlimited; stock for
physical goods would be your own system); one payment handler (Stripe SPT —
no 3-D Secure step, no PayPal); no discounts, affiliate attribution, marketing
consent or intent traces; refunds made later are not yet sent to the platform
as `order_update`.

## Development

No npm install: the tests run on Node's built-in test runner (Node 23.6 or
newer, which runs TypeScript directly). From the repository root:

```sh
node --import ./workers/ecommerce/test/resolve.mjs --test 'workers/ecommerce/test/*.test.ts'
# with coverage
node --import ./workers/ecommerce/test/resolve.mjs --test --experimental-test-coverage \
  --test-coverage-include='workers/ecommerce/functions/**' 'workers/ecommerce/test/*.test.ts'
# refuse npm imports, which Pages Direct Upload cannot bundle
node workers/ecommerce/scripts/check-bare-imports.mjs
```

`make test-workers` runs the same suite with every other template's.

The panel against the real handlers, on the test fakes with a demo shop (six
products, sixty orders in every state and every channel — web, ACP, UCP —
invoices, refunds, a failing outbox). The screenshot script also reports any
screen wider than the window:

```sh
node --import ./workers/ecommerce/test/resolve.mjs workers/ecommerce/test/dev-server.mjs
# → http://localhost:8788/ecommerce-admin/, password "demo"
# DEMO_CORE=1 adds core's bar; DEMO_CORE=theme also a demo site theme
node workers/ecommerce/test/panel-shots.mjs /tmp/shots   # every screen, light and dark, wide, 390 and 360 px (needs Playwright)
```

The bindings are fakes in `test/fakes.ts`: D1 runs real SQL on `node:sqlite`
(SQLite, the engine D1 is built on), so every statement the shop sends is
executed; R2 and KV are in-memory, with R2's ranged reads and KV's expiry.
Handlers are called directly with a constructed request, env and context, and
the payment providers, mail API and tracking endpoints are answered by a
replaced `globalThis.fetch`.

## What it does not do yet

No subscriptions, no physical shipping, no VIES validation of the VAT numbers
buyers type (a well-formed number is *not* treated as verified, so reverse
charge does not apply on the strength of a string), no PDF invoices (the HTML
prints correctly to A4), no coupon codes. Each is a module that fits the shape
above; see the planning notes for the order they were meant to arrive in.

## License

Part of ssg, under the [Business Source License 1.1](../../LICENSE) (BUSL-1.1)
like the rest of the repository since 1.9.0.
