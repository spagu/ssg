// Wiring for the shop pages. One entry point; each page picks up only the
// parts it has markup for.
//
// The pages work without any of this: the product page has a real form that
// posts to /api/shop/checkout, and the cart page is the only screen that needs
// JavaScript at all (it is a client-side list by definition). Everything here
// is enhancement on top of markup that already works.

import { bindCartBadge, cart, formatMoney } from "./cart.js";
import { mountCart } from "./cart-page.js";
import { mountCheckout, mountThanks } from "./checkout.js";

/** Adds a product to the basket from a product card or a product page. */
function bindAddButtons() {
  for (const button of document.querySelectorAll("[data-add-to-cart]")) {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      const form = button.closest("form");
      const quantity = Number(form?.querySelector("[name=quantity]")?.value ?? 1) || 1;
      const result = cart.add(
        {
          sku: button.dataset.sku,
          name: button.dataset.name,
          priceMinor: Number(button.dataset.price ?? 0),
          currency: button.dataset.currency,
          url: button.dataset.url,
          image: button.dataset.image,
        },
        quantity,
      );

      // The confirmation replaces the button's own label region rather than
      // appearing somewhere else on the page: the eye is already here.
      const live = document.querySelector("[data-cart-live]");
      if (live) {
        live.textContent = result.ok
          ? `${button.dataset.name} added to your basket.`
          : "Your basket is full. Check out first, then start another order.";
      }
      button.dataset.added = result.ok ? "1" : "";
      if (result.ok) {
        const original = button.textContent;
        button.textContent = "Added ✓";
        setTimeout(() => {
          button.textContent = original;
        }, 2000);
      }
    });
  }
}

/** Prices on a static page can be stale — the shop's own catalogue is the
 *  authority. One request per page refreshes whatever is displayed, so a price
 *  change does not need a rebuild to be visible. */
async function refreshPrices() {
  const nodes = [...document.querySelectorAll("[data-price-for]")];
  if (nodes.length === 0) return;
  try {
    const res = await fetch("/api/shop/products", { headers: { accept: "application/json" } });
    if (!res.ok) return;
    const { products = [], shop = {} } = await res.json();
    const bySku = new Map(products.map((p) => [p.sku, p]));
    for (const node of nodes) {
      const product = bySku.get(node.dataset.priceFor);
      if (!product) continue;
      // A product may be priced in several currencies; the page shows the one
      // the shop sells in by default, falling back to whatever it does have.
      const currency = product.prices[shop.baseCurrency] !== undefined
        ? shop.baseCurrency
        : Object.keys(product.prices)[0];
      if (!currency) continue;
      const amount = product.prices[currency];
      node.textContent = formatMoney(amount, currency);
      const button = document.querySelector(`[data-add-to-cart][data-sku="${CSS.escape(product.sku)}"]`);
      if (button) {
        button.dataset.price = String(amount);
        button.dataset.currency = currency;
      }
    }
  } catch {
    // Offline, or the API is not deployed yet: the built-in price stands.
  }
}

document.addEventListener("DOMContentLoaded", () => {
  bindCartBadge();
  bindAddButtons();
  mountCart();
  mountCheckout();
  mountThanks();
  refreshPrices();
});
