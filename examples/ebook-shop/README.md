# A working shop

A publisher with two books, a basket, VAT for eight countries and an admin
panel — small enough to read in one sitting, complete enough to take a real
payment once you put real keys in it.

It is the [picostore theme](../../templates/picostore/README.md) plus the
[ecommerce worker](../../workers/ecommerce/README.md), which is the intended
pairing: SSG builds the pages, the worker provides `/api/shop/*`, and
`wrangler pages dev` serves both from one origin — which the browser insists on,
because the checkout posts to the same host it was served from.

## Run it

```sh
cp examples/ebook-shop/.dev.vars.example examples/ebook-shop/.dev.vars
cd examples/ebook-shop
ssg --config ssg.yaml --watch
```

That builds the site and starts `wrangler pages dev` on
**http://localhost:8788**, with D1, R2 and KV created locally under
`.wrangler/`. Nothing reaches Cloudflare.

Then fill the shop:

```sh
sh scripts/seed.sh
```

which signs in, writes the seller's details, creates both products, uploads
their PDFs and puts them on sale. It uses the same HTTP API the panel uses, so
everything it does can be undone by hand.

- Storefront — <http://localhost:8788>
- Panel — <http://localhost:8788/ecommerce-admin/>, `owner@example.com` /
  `correct-horse-battery-staple`

## What you can try

**Buy something.** With the placeholder Stripe key in `.dev.vars` the payment
itself is refused — the order stays `pending` with no provider reference, which
is exactly what you want to see, and the panel explains it. Put a real
`sk_test_…` key in `.dev.vars` and the redirect to Stripe works.

**Deliver it anyway.** Open the order in the panel and *Mark as paid*. That runs
the same fulfilment a webhook runs: the invoice is issued (`FV/2026/000001`), a
download token is created, and the email is queued. Without `SHOP_MAIL_URL` the
queue keeps the message and says `mail_not_configured` rather than pretending it
was sent — the *Log* tab shows it.

**Watch the VAT.** Buy the €24.00 book with Germany as the country and the order
is €22.43 net plus €1.57 tax: Germany's 7% ebook rate, applied because the place
of supply for a digital service is the customer's place. Change the country to
Poland and it becomes 5%. The rate table is in the panel under *VAT*.

**Try to cheat.** Post your own price to `/api/shop/checkout`. It is ignored —
the endpoint takes product codes and quantities and prices everything itself.

## The files

```text
examples/ebook-shop/
├── ssg.yaml                    the site: theme, worker, variables
├── wrangler.toml               local bindings; ids are placeholders
├── .dev.vars.example           secrets for local development
├── content/shop/
│   ├── metadata.json
│   ├── pages/                  two books, the shop pages, terms and privacy
│   └── posts/                  two posts
├── static/images/              the covers, drawn as SVG
├── files/                      the PDFs the shop sells (generated)
└── scripts/
    ├── make-sample-ebook.py    writes those PDFs
    └── seed.sh                 fills the shop over the API
```

Nothing in `files/` is published: it is what the shop *sells*, and it reaches a
buyer only through a signed, expiring link. The build does not copy it.

## Before this takes real money

Replace every placeholder in `wrangler.toml` with a real binding id, set the
secrets with `wrangler pages secret put` rather than `.dev.vars`, delete
`SHOP_ADMIN_BOOTSTRAP` after the first sign-in, and read the terms and privacy
pages properly — they are a starting point written to be edited, not legal
advice. The panel's *Overview* lists whatever is still missing.
