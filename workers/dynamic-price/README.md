# dynamic-price worker

A Cloudflare Pages Function that serves live prices from a KV namespace (or an
upstream pricing API) so a static page can show current amounts without a
rebuild. Ships a small client snippet (`static/js/price.js`) that fills any
`[data-price-sku]` element.

## Config

```yaml
worker:
  dir: workers/dynamic-price
  mode: functions
  routes_include:
    - /api/price/*
```

Copy `static/js/price.js` into your site's `static/js/` and include it on
pricing pages. Keep a server-rendered fallback price in the element for the
no-JS / offline case.

## Bindings & secrets

Bind a KV namespace named `PRICES` in the Pages project (Settings → Functions →
KV bindings), with `sku` keys holding `{"amount": 4900, "currency": "USD"}`.
Or point at an upstream API instead:

```sh
wrangler pages secret put PRICE_API_URL
wrangler pages secret put PRICE_API_KEY
```
