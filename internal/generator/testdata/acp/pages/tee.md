---
title: Linen tee
slug: tee
status: publish
type: page
description: A linen T-shirt, cut straight.
schema:
  "@type": ProductGroup
  name: Linen tee
  productGroupID: TEE
  brand: { "@type": Brand, name: Northline }
  image: /images/tee.jpg
  variesBy: [https://schema.org/size]
  offers: { price: "39.00", priceCurrency: EUR, availability: https://schema.org/PreOrder }
  hasVariant:
    - { "@type": Product, sku: TEE-S, name: "Linen tee, S", size: S }
    - { "@type": Product, sku: TEE-M, name: "Linen tee, M", size: M, offers: { price: "41.00", priceCurrency: EUR, availability: https://schema.org/BackOrder } }
acp:
  material: Linen
  gender: unisex
  size_system: EU
---

The tee.
