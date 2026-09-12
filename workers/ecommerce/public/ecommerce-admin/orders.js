// The orders screen: a filtered, paged list, and one order in full.

import { api } from "./api.js";
import { badge, confirmDestructive, el, money, notify, td, when } from "./dom.js";

let cursor = null;
let query = { q: "", status: "" };

export function bindOrders() {
  const form = document.getElementById("order-filter");
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const data = new FormData(form);
    query = { q: String(data.get("q") ?? ""), status: String(data.get("status") ?? "") };
    document.getElementById("export-orders").href =
      `/api/shop/admin/export?type=orders`;
    loadOrders(true);
  });
  document.getElementById("orders-more").addEventListener("click", () => loadOrders(false));
}

export async function loadOrders(reset = true) {
  if (reset) cursor = null;
  const params = new URLSearchParams({ limit: "25" });
  if (query.q) params.set("q", query.q);
  if (query.status) params.set("status", query.status);
  if (cursor) params.set("cursor", cursor);

  const body = await api.get(`/api/shop/admin/orders?${params}`);
  const rows = document.getElementById("order-rows");
  const made = (body.orders ?? []).map(orderRow);
  if (reset) rows.replaceChildren(...made);
  else rows.append(...made);

  cursor = body.nextCursor;
  document.getElementById("orders-more").hidden = !cursor;
  if (rows.childElementCount === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "6", text: "No orders match that." })));
  }
}

function orderRow(order) {
  return el(
    "tr",
    {},
    td("Number", el("code", { text: order.number })),
    td("When", when(order.createdAt)),
    td("Customer", order.email ?? "—"),
    td("Total", money(order.totalMinor, order.currency)),
    td("Status", badge(order.status)),
    td("Open", el("button", { class: "quiet", type: "button", onClick: () => openOrder(order.id) }, "Open")),
  );
}

async function openOrder(id) {
  const panel = document.getElementById("order-detail");
  const data = await api.get(`/api/shop/admin/orders/${encodeURIComponent(id)}`);
  const { order, customer, items, payments, invoices, tokens } = data;

  panel.replaceChildren(
    el("div", { class: "editor" },
      el("h3", {}, "Order ", el("code", { text: order.number }), " ", badge(order.status)),
      summary(order, customer),
      el("h3", { text: "Lines" }),
      lineTable(items, order.currency),
      el("h3", { text: "Payments" }),
      paymentList(payments),
      el("h3", { text: "Invoices" }),
      invoiceList(invoices),
      el("h3", { text: "Download links" }),
      tokenList(tokens),
      el("h3", { text: "Actions" }),
      actions(order, panel),
      noteForm(order),
    ),
  );
  panel.hidden = false;
  panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

function summary(order, customer) {
  const facts = [
    ["Placed", when(order.created_at)],
    ["Paid", when(order.paid_at)],
    ["Customer", customer?.email ?? "—"],
    ["Name", customer?.name ?? "—"],
    ["VAT number", customer?.vat_id ?? "—"],
    ["Tax country", order.tax_country ?? "—"],
    ["Reverse charge", order.reverse_charge ? "yes" : "no"],
    ["Provider", order.gateway ?? "—"],
    ["Net", money(order.subtotal_minor, order.currency)],
    ["VAT", money(order.tax_minor, order.currency)],
    ["Total", money(order.total_minor, order.currency)],
  ];
  // The evidence the tax decision rested on, shown because it is the thing an
  // auditor asks about and the thing an owner otherwise never sees.
  const evidence = order.taxEvidence
    ? Object.entries(order.taxEvidence).map(([k, v]) => `${k}: ${v}`).join(", ")
    : "none recorded";
  facts.push(["Tax evidence", evidence]);

  return el("div", { class: "cards" },
    ...facts.map(([label, value]) =>
      el("dl", { class: "card" }, el("dt", { text: label }), el("dd", { text: String(value) })),
    ),
  );
}

function lineTable(items, currency) {
  return el("table", { class: "grid" },
    el("thead", {}, el("tr", {},
      el("th", { scope: "col", text: "Product" }),
      el("th", { scope: "col", text: "Qty" }),
      el("th", { scope: "col", text: "Unit" }),
      el("th", { scope: "col", text: "VAT" }),
      el("th", { scope: "col", text: "Total" }),
    )),
    el("tbody", {}, ...items.map((i) =>
      el("tr", {},
        td("Product", i.name),
        td("Qty", String(i.quantity)),
        td("Unit", money(i.unit_minor, currency)),
        td("VAT", `${(i.tax_rate_bp / 100).toFixed(2)}% · ${money(i.tax_minor, currency)}`),
        td("Total", money(i.total_minor, currency)),
      ),
    )),
  );
}

function paymentList(payments) {
  if (payments.length === 0) return el("p", { class: "hint", text: "Nothing recorded yet." });
  return el("ul", {}, ...payments.map((p) =>
    el("li", {}, `${p.kind} ${p.status} · ${money(p.amount_minor, p.currency)} · ${p.gateway} ${p.gateway_ref} · ${when(p.created_at)}`),
  ));
}

function invoiceList(invoices) {
  if (invoices.length === 0) return el("p", { class: "hint", text: "None issued yet." });
  return el("ul", {}, ...invoices.map((inv) =>
    el("li", {}, `${inv.kind === "credit_note" ? "Credit note" : "Invoice"} ${inv.number} · ${when(inv.issued_at)} · ${money(inv.total_minor, inv.currency)}`),
  ));
}

function tokenList(tokens) {
  if (tokens.length === 0) return el("p", { class: "hint", text: "No links issued." });
  return el("ul", {},
    ...tokens.map((t) =>
      el("li", {}, `${t.uses} of ${t.max_uses} downloads used · expires ${when(t.expires_at)}${t.revoked ? " · revoked" : ""}`),
    ),
    // Said plainly, because the first thing an owner tries is to copy the link
    // out of the panel and paste it into an email.
    el("li", { class: "hint", text: "The links themselves were never stored — only their hashes. Use “Send the links again” to issue fresh ones." }),
  );
}

function actions(order, panel) {
  const row = el("div", { class: "row" });
  const refresh = async () => {
    panel.hidden = true;
    await loadOrders(true);
  };

  if (["paid", "fulfilled"].includes(order.status)) {
    row.append(
      el("button", { type: "button", class: "quiet", onClick: () => act(`resend`, order, refresh, "Links sent.") }, "Send the links again"),
      el("button", { type: "button", class: "danger", onClick: () => refund(order, refresh) }, "Refund"),
    );
  }
  if (["pending", "needs_review"].includes(order.status)) {
    row.append(
      el("button", { type: "button", onClick: () => markPaid(order, refresh) }, "Mark as paid"),
    );
  }
  if (["pending", "failed"].includes(order.status)) {
    row.append(
      el("button", { type: "button", class: "quiet", onClick: () => act("cancel", order, refresh, "Cancelled.") }, "Cancel"),
    );
  }
  if (row.childElementCount === 0) {
    row.append(el("p", { class: "hint", text: "Nothing to do from here for an order in this state." }));
  }
  return row;
}

async function act(action, order, after, okMessage, body = {}) {
  try {
    await api.post(`/api/shop/admin/orders/${encodeURIComponent(order.id)}/${action}`, body);
    notify("ok", okMessage);
    await after();
  } catch (err) {
    notify("error", err.message);
  }
}

function refund(order, after) {
  const typed = window.prompt(
    `Refund how much, in ${order.currency}? Leave as it is for the whole order.`,
    (order.total_minor / 100).toFixed(2),
  );
  if (typed === null) return;
  const amountMinor = Math.round(Number(typed.replace(",", ".")) * 100);
  if (!Number.isFinite(amountMinor) || amountMinor <= 0) {
    notify("error", "That is not an amount.");
    return;
  }
  // The provider is asked; the order changes when the provider's webhook says
  // it did. The message says so, because otherwise the unchanged status reads
  // as a failure.
  act("refund", order, after, "The refund was sent to the provider. The order updates when it confirms.", { amountMinor });
}

function markPaid(order, after) {
  if (!confirmDestructive(`Mark ${order.number} as paid? This issues the invoice and emails the links.`)) return;
  const reference = window.prompt("Reference for your records (a bank transfer id, say):", "") ?? "";
  act("mark-paid", order, after, "Marked as paid. Invoice issued and links queued.", { reference });
}

function noteForm(order) {
  const input = el("textarea", { name: "notes", maxlength: "4000" }, order.notes ?? "");
  return el(
    "form",
    {
      onSubmit: async (event) => {
        event.preventDefault();
        try {
          await api.patch(`/api/shop/admin/orders/${encodeURIComponent(order.id)}`, { notes: input.value });
          notify("ok", "Note saved.");
        } catch (err) {
          notify("error", err.message);
        }
      },
    },
    el("label", { for: "order-note", text: "Private note (never shown to the buyer)" }),
    Object.assign(input, { id: "order-note" }),
    el("button", { type: "submit" }, "Save note"),
  );
}
