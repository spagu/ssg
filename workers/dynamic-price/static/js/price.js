// Client snippet for the dynamic-price worker. Copy into your site's static/js
// and include it on pricing pages. Any element with data-price-sku is filled
// with the live amount from /api/price/:sku.
//
//   <span data-price-sku="premium-annual">…</span>
(function () {
  "use strict";
  const fmt = function (amount, currency) {
    try {
      return new Intl.NumberFormat(undefined, { style: "currency", currency: currency || "USD" }).format(
        amount / 100,
      );
    } catch (e) {
      return (amount / 100).toFixed(2) + " " + (currency || "USD");
    }
  };
  document.querySelectorAll("[data-price-sku]").forEach(function (el) {
    const sku = el.dataset.priceSku;
    fetch("/api/price/" + encodeURIComponent(sku))
      .then(function (r) {
        return r.ok ? r.json() : Promise.reject(new Error("price lookup failed: " + r.status));
      })
      .then(function (p) {
        if (typeof p.amount === "number") el.textContent = fmt(p.amount, p.currency);
      })
      .catch(function () {
        /* leave the server-rendered fallback price in place */
      });
  });
})();
