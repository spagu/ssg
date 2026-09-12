---
title: "Privacy"
status: "publish"
slug: "shop/privacy"
excerpt: "What this shop stores about you, and for how long."
---

A shop cannot sell you anything without knowing where to send it. This is the
whole list of what is kept, and why.

## What is stored

**Your email address**, because the download link and the invoice go there, and
because you need to be able to ask for the link again.

**Your name and country**, because an invoice needs them and because the country
decides the VAT rate. A VAT number if you gave one.

**What you bought, when, for how much, and in what currency.** This is the
invoice, and tax law requires it to be kept for years — in Poland, five from the
end of the accounting year. That obligation outranks a request to delete it.

**A hash of your IP address**, salted, as one of the two pieces of evidence the
VAT rules require for deciding which country's rate applies. It is a one-way
hash, and if the shop is deployed without a salt configured, no IP is recorded at
all.

**Your browser's user agent string**, alongside the order, for the same
evidence requirement.

## What is not stored

Your card number, your bank details or anything else you type on the payment
page. That page belongs to Stripe or PayPal and this shop never sees it — what
comes back is a reference and a yes or no.

No third-party tracking script runs on these pages. If the shop owner turns on
analytics, it happens server-side after a sale, without a cookie and without
your address leaving the building unhashed.

## Who else sees it

The payment provider you choose (Stripe or PayPal), the service that sends the
email, and the accountant who files the VAT. Nobody else, and nothing is sold.

## What you can ask for

A copy of everything above, a correction, or deletion of anything not held
under the invoicing obligation. Write to help@example.com; you will get an
answer within a month. You can also complain to your data protection authority.
