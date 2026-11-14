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

## Module manifest

`module.json` describes this template as a module (see [docs/WORKER-MODULES.md](../../docs/WORKER-MODULES.md)): its name and description in five languages, and how to switch it off. It is a description only — no secrets, no configuration.

There is no admin surface to put a manifest endpoint behind, so the template ships the file only; the build reads it into the module registry when the site has the [`core`](../core/README.md) worker.
