# picostore

A shop theme: a catalogue, a product page, a basket, a checkout and the four
pages either side of a payment. Built on [Pico CSS](https://picocss.com) v2.1.1
(MIT), vendored into `css/pico.min.css` — no build step, no CDN, no external
request except one font.

```text
templates/picostore/
├── index.html              # the books, then the latest posts
├── page.html               # an ordinary page
├── post.html               # a post
├── category.html           # archives
├── partials/chrome.html    # ps-head, ps-header, ps-footer
├── layouts/
│   ├── shop-index.html     # the catalogue as a page of its own
│   ├── shop-product.html   # one book: cover, price, buy, facts
│   ├── shop-cart.html      # the basket
│   ├── shop-checkout.html  # email, country, VAT number, pay
│   ├── shop-thanks.html    # waits for the webhook before saying "paid"
│   ├── shop-cancel.html    # nothing was charged
│   └── shop-resend.html    # lost the download email
├── css/
│   ├── pico.min.css        # Pico CSS v2.1.1, MIT, unmodified
│   ├── tokens.css          # the palette and the measurements
│   ├── style.css           # chrome, product grid, product page
│   └── shop.css            # basket, checkout, messages
└── js/
    ├── main.js             # colour scheme, current page
    ├── cart.js             # the basket itself
    ├── cart-page.js        # drawing the basket
    ├── checkout.js         # checkout and the thank-you page
    └── shop.js             # the entry point that wires them together
```

Pair it with the [ecommerce worker](../../workers/ecommerce/README.md), which
provides `/api/shop/*`. Without the worker the pages still build and render —
prices come from front matter — but nothing can be bought.

## A book is a Markdown file

```yaml
---
title: "The One-Person Shop"
slug: "the-one-person-shop"
status: "publish"
layout: "shop-product"     # this is what puts it in the catalogue
excerpt: "Tax, invoices and refunds for people selling their own work."
featured_image: "/images/cover-one-person-shop.svg"
sku: "EBOOK-SHOP"          # must match the product code in the shop
price_minor: 2400          # for the basket before the API answers
price_display: "€24.00"    # what is printed before JavaScript runs
currency: "EUR"
author: "Tomasz Wierzbicki"
format: "PDF and EPUB"
pages: 236
---
```

`layout: shop-product` is the only thing the catalogue looks at, so a book is in
the shop exactly when it says it is. The price in the front matter is what the
page shows on first paint; one request to `/api/shop/products` then replaces it
with the shop's own, so changing a price does not need a rebuild.

### Site variables

```yaml
variables:
  shop_name: "Paperless Press"
  shop_tagline: "Books, delivered the moment you pay for them"
  nav: [{ url: "/shop/", label: "Books" }]
  shop_gateways:
    - { id: "stripe", label: "Pay by card" }
    - { id: "paypal", label: "Pay with PayPal", secondary: true }
  shop_countries: [{ code: "PL", name: "Poland" }]
  turnstile_site_key: "0x4AAA…"   # optional
  gtm_id: "GTM-XXXXXXX"           # optional; nothing renders without it
```

## What works without JavaScript

The **Buy now** button on a product page is a real form that posts one line to
`/api/shop/checkout` and is answered with a redirect to the payment page. That
path needs nothing from the browser but a form submit.

The **basket** cannot work that way: it lives in `localStorage` and exists only
in the visitor's browser, so the cart and the checkout that reads it need
JavaScript. That is the deliberate division — one book without scripting,
several books with.

## Design

Colour comes from the Google Material palette, chosen for contrast rather than
for looks. Every pairing is measured against WCAG 2.2 AA: 4.5:1 for body text,
3:1 for borders and large text.

| Role | Light | Dark | Contrast |
|---|---|---|---|
| Text | `#263238` on white | `#ECEFF1` on `#263238` | 13.7:1 / 11.6:1 — AAA |
| Muted text | `#546E7A` | `#B0BEC5` | 5.6:1 / 7.4:1 — AA |
| Brand | `#303F9F` Indigo 700 | `#9FA8DA` Indigo 200 | 8.6:1 / 5.7:1 — AA |
| Price | `#00796B` Teal 700 | `#80CBC4` Teal 200 | 4.8:1 / 7.8:1 — AA |
| Error | `#C62828` Red 800 | `#EF9A9A` Red 200 | 6.4:1 / 6.2:1 — AA |

Dark is not a second palette: surfaces walk down the ink ramp and the brand
walks up the indigo ramp, because Indigo 700 on near-black is 2.1:1. Both
schemes are declared twice — once under `prefers-color-scheme` for the OS
setting, once under `[data-theme]` for the header toggle — so an explicit choice
wins in both directions.

Status is never colour alone. Every badge, note and message carries a word as
well, because a colour-blind buyer and a printed page both lose the colour.

Covers are drawn as SVG rather than photographed or picked from an icon set: a
typographic cover is readable at thumbnail size, weighs a kilobyte and says what
the book is.

### The one external request

`<link>` tags in `partials/chrome.html` fetch Inter from Google Fonts. Delete
the two tags and the stack falls back to `system-ui` — nothing else breaks.

## Accessibility

A skip link, a visible focus ring on everything focusable, tables with real
`<th scope>` and captions, and a live region that announces "added to your
basket" without moving focus. The basket table becomes a stack of labelled rows
below 40rem rather than scrolling sideways.
