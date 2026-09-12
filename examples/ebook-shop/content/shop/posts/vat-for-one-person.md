---
title: "The two pieces of evidence"
status: "publish"
slug: "the-two-pieces-of-evidence"
date: 2026-08-14
excerpt: "Why a shop selling ebooks in the EU asks where you live, and what it does with the answer."
tags: ["vat", "selling"]
---

A shop that sells you a physical book charges the VAT of the country it posts
from. A shop that sells you a file charges the VAT of the country you are in.
That single difference is where most of the complexity in this shop lives.

The rule is that the place of supply for a digital service is the customer's
place, and that the seller has to hold two non-contradictory pieces of evidence
for where that is. Not one — two. A billing address alone is a claim; a billing
address that agrees with the country the payment came from is evidence.

So at checkout we ask for a country, and we keep a salted hash of the IP address
Cloudflare resolved. When the payment clears, the provider tells us the country
of the card, and that becomes a third. If the billing address and the card
disagree, the order is not quietly charged at whichever rate is convenient — it
is flagged for a human, because the difference between 5% and 23% on a €19 book
is not the problem. The problem is doing it a thousand times and being asked
about it three years later.

The part people find unfair is that a customer who mistypes their country is not
doing anything wrong, and the seller is the one who answers for it. That is why
the field is a list and not a text box, and why the shop would rather refuse a
sale than guess.
