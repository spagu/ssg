// Checkout and the thank-you page.
//
// The checkout form posts to the shop's own endpoint, which prices the basket
// again from its database and hands back a payment URL. Nothing here decides
// what anything costs; the summary on screen is a courtesy, and the server is
// the authority (the same rule the API documents as "price from the DB").

import { cart, formatMoney } from "./cart.js";

function note(root, kind, title, message) {
  const box = root.querySelector("[data-checkout-note]");
  if (!box) return;
  box.className = `ps-note ps-note-${kind}`;
  box.replaceChildren();
  const strong = document.createElement("strong");
  strong.textContent = title;
  box.append(strong, document.createTextNode(message));
  box.hidden = false;
  box.setAttribute("role", kind === "error" ? "alert" : "status");
}

/** The basket, drawn into the order summary beside the form. */
function renderSummary(root) {
  const list = root.querySelector("[data-summary-lines]");
  const totalOut = root.querySelector("[data-summary-total]");
  if (!list) return cart.lines();

  const lines = cart.lines();
  list.replaceChildren(
    ...lines.map((line) => {
      const li = document.createElement("li");
      const name = document.createElement("span");
      name.textContent = line.quantity > 1 ? `${line.name} × ${line.quantity}` : line.name;
      const price = document.createElement("span");
      price.textContent = formatMoney(line.priceMinor * line.quantity, line.currency);
      li.append(name, price);
      return li;
    }),
  );

  const total = cart.total();
  if (totalOut) {
    totalOut.textContent = total.mixed ? "—" : formatMoney(total.amountMinor, total.currency);
  }
  return lines;
}

export function mountCheckout() {
  const root = document.querySelector("[data-checkout-page]");
  if (!root) return;

  const form = root.querySelector("form");
  const lines = renderSummary(root);
  document.addEventListener("picostore:cart", () => renderSummary(root));

  // Arriving at checkout with nothing in the basket is a wrong turn, not an
  // error: say where the products are.
  const emptyBox = root.querySelector("[data-checkout-empty]");
  if (lines.length === 0) {
    if (emptyBox) emptyBox.hidden = false;
    form?.setAttribute("hidden", "hidden");
    return;
  }

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const button = event.submitter;
    const gateway = button?.value || data.get("gateway") || "";

    // form.elements, not querySelectorAll: the payment buttons live in the
    // summary beside the form and are tied to it by their form attribute.
    const buttons = [...form.elements].filter((el) => el.type === "submit");
    for (const b of buttons) b.disabled = true;
    const spinner = root.querySelector("[data-checkout-busy]");
    if (spinner) spinner.hidden = false;

    try {
      const res = await fetch("/api/shop/checkout", {
        method: "POST",
        headers: { "content-type": "application/json", accept: "application/json" },
        body: JSON.stringify({
          items: cart.asItems(),
          gateway,
          email: data.get("email"),
          name: data.get("name"),
          country: data.get("country"),
          vatId: data.get("vatId"),
          locale: document.documentElement.lang || "en",
          consentWaiver: data.get("consentWaiver") ? "1" : "",
          consentMarketing: data.get("consentMarketing") ? "1" : "",
          turnstileToken: data.get("cf-turnstile-response"),
        }),
      });
      const body = await res.json();

      if (!res.ok) {
        note(root, "error", "We could not start the payment. ", body.message ?? "Please try again.");
        return;
      }

      // The basket is cleared only once the order exists on the server. A
      // failed payment then still has something to go back to.
      cart.clear();
      try {
        sessionStorage.setItem("picostore.lastOrder", JSON.stringify({ id: body.orderId, key: body.orderKey }));
      } catch {
        /* the link in the redirect carries the same two values */
      }
      window.location.assign(body.redirectUrl);
    } catch {
      note(root, "error", "We could not reach the shop. ", "Check your connection and try again.");
    } finally {
      for (const b of buttons) b.disabled = false;
      if (spinner) spinner.hidden = true;
    }
  });
}

/** The thank-you page waits for the webhook, because the redirect back from a
 *  provider proves nothing about whether the money arrived. */
export function mountThanks() {
  const root = document.querySelector("[data-thanks-page]");
  if (!root) return;

  const params = new URLSearchParams(window.location.search);
  const id = params.get("order") ?? "";
  const key = params.get("k") ?? "";
  const statusOut = root.querySelector("[data-order-status]");
  const numberOut = root.querySelector("[data-order-number]");
  const listOut = root.querySelector("[data-order-downloads]");
  if (!id || !key) {
    note(root, "error", "This link is incomplete. ", "Use the link from your confirmation email.");
    return;
  }

  let attempt = 0;
  const poll = async () => {
    attempt += 1;
    try {
      const res = await fetch(`/api/shop/orders/${encodeURIComponent(id)}/status?k=${encodeURIComponent(key)}`, {
        headers: { accept: "application/json" },
      });
      if (!res.ok) {
        note(root, "error", "We cannot find that order. ", "Use the link from your confirmation email.");
        return;
      }
      const order = await res.json();
      if (numberOut) numberOut.textContent = order.number;

      if (order.status === "paid" || order.status === "fulfilled") {
        if (statusOut) statusOut.textContent = "Paid. Your download links are on their way by email.";
        renderDownloads(listOut, order.downloads ?? []);
        note(root, "ok", "Thank you. ", "A receipt and your links have been emailed to you.");
        return;
      }
      if (order.status === "failed" || order.status === "cancelled") {
        note(root, "error", "The payment did not go through. ", "Nothing was charged. You can try again.");
        if (statusOut) statusOut.textContent = "Not paid.";
        return;
      }

      // Still pending: providers confirm in a second or two, occasionally in
      // a minute. Back off rather than hammer.
      if (statusOut) statusOut.textContent = "Waiting for the payment provider to confirm…";
      if (attempt < 20) setTimeout(poll, Math.min(5000, 500 * attempt));
      else note(root, "error", "This is taking longer than usual. ", "Your email will arrive when it clears.");
    } catch {
      if (attempt < 5) setTimeout(poll, 3000);
    }
  };
  poll();
}

function renderDownloads(list, downloads) {
  if (!list) return;
  list.replaceChildren(
    ...downloads.map((d) => {
      const li = document.createElement("li");
      const name = document.createElement("span");
      name.textContent = d.name;
      const meta = document.createElement("small");
      meta.textContent = `${d.remaining} download${d.remaining === 1 ? "" : "s"} left · available until ${new Date(d.expiresAt).toLocaleDateString()}`;
      name.append(meta);
      li.append(name);
      return li;
    }),
  );
  list.hidden = downloads.length === 0;
}
