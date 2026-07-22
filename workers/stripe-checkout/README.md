# stripe-checkout worker

Cloudflare Pages Functions for Stripe payments beside a static SSG site:

- `POST /api/checkout` — creates a Checkout Session (one-time or subscription)
  and returns its `url` for a client-side redirect.
- `POST /api/stripe-webhook` — verifies the `Stripe-Signature` with WebCrypto
  HMAC-SHA256 and dispatches on the event type.

Raw `fetch` against the Stripe REST API — no npm dependencies, Direct-Upload
friendly.

## Config

```yaml
worker:
  dir: workers/stripe-checkout
  mode: functions
  routes_include:
    - /api/*
```

## Secrets

```sh
wrangler pages secret put STRIPE_SECRET_KEY
wrangler pages secret put CHECKOUT_SUCCESS_URL
wrangler pages secret put CHECKOUT_CANCEL_URL
wrangler pages secret put STRIPE_WEBHOOK_SECRET
```

Point a Stripe webhook endpoint at `https://<your-site>/api/stripe-webhook`
and fill in the fulfilment TODOs in `stripe-webhook.ts`.
