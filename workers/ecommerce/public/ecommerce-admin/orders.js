// The orders screen: a filtered, paged list, and one order in full.

import { api } from "./api.js";
import { badge, confirmDestructive, el, money, notify, td, when } from "./dom.js";
import { bindSortableHeaders, Pager, renderPager } from "./pager.js";

/** One page of orders at a time, sorted by whichever column was asked for. */
const pager = new Pager({
  sort: "date",
  dir: "desc",
  fetchPage: async (params) => {
    const body = await api.get(`/api/shop/admin/orders?${params}`);
    drawRows(body.orders ?? []);
    return { total: body.total, nextCursor: body.nextCursor, count: (body.orders ?? []).length };
  },
});

function filtersFrom(form) {
  const data = new FormData(form);
  return {
    q: String(data.get("q") ?? ""),
    status: String(data.get("status") ?? ""),
    gateway: String(data.get("gateway") ?? ""),
    from: String(data.get("from") ?? ""),
    to: String(data.get("to") ?? ""),
  };
}

export function bindOrders() {
  const form = document.getElementById("order-filter");
  const redraw = () => afterLoad();

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    await pager.reset(filtersFrom(form));
    redraw();
  });
  // A reset button empties the fields after this handler, so the filters are
  // read on the next frame rather than from the values still on screen.
  form.addEventListener("reset", () => {
    setTimeout(async () => {
      await pager.reset({});
      redraw();
    });
  });

  bindSortableHeaders(document.getElementById("order-table"), pager, redraw);
}

function afterLoad() {
  bindSortableHeaders(document.getElementById("order-table"), pager, afterLoad);
  renderPager(document.getElementById("order-pager"), pager, afterLoad);
}

export async function loadOrders(reset = true) {
  if (reset) await pager.reset(pager.filters);
  else await pager.load();
  afterLoad();
}

function drawRows(orders) {
  const rows = document.getElementById("order-rows");
  rows.replaceChildren(...orders.map(orderRow));

  if (orders.length === 0) {
    const filtered = Object.values(pager.filters).some(Boolean);
    rows.replaceChildren(
      el("tr", {},
        el("td", { colspan: "6" },
          el("div", { class: "empty" },
            el("h3", { text: filtered ? "Nothing matches that" : "No orders yet" }),
            el("p", { text: filtered
              ? "Try a different search, or clear the filter to see everything."
              : "An order appears here the moment someone reaches the payment page — paid or not." }),
          ),
        ),
      ),
    );
  }
}

function orderRow(order) {
  return el(
    "tr",
    {},
    td("Order", el("strong", {}, el("code", { text: order.number }))),
    td("Customer",
      el("div", { class: "cell-title" },
        el("strong", { text: order.email ?? "—" }),
        el("small", { text: [order.name, order.country].filter(Boolean).join(" · ") || "—" }),
      ),
    ),
    td("Status", badge(order.status)),
    td("Total", { class: "numeric" }, money(order.totalMinor, order.currency)),
    td("Placed", { class: "numeric" }, el("span", { class: "muted", text: when(order.createdAt) })),
    td("", { class: "actions" },
      el("button", { type: "button", class: "secondary small", onClick: () => openOrder(order.id) }, "Open"),
    ),
  );
}

/** Back to the list, from wherever the order was opened. */
function closeOrder() {
  document.getElementById("order-detail").hidden = true;
  document.getElementById("orders-list").hidden = false;
  document.getElementById("crumb").textContent = "Orders";
  window.scrollTo({ top: 0 });
}

async function openOrder(id) {
  const panel = document.getElementById("order-detail");
  const data = await api.get(`/api/shop/admin/orders/${encodeURIComponent(id)}`);
  const { order, customer, items, payments, invoices, tokens } = data;

  panel.replaceChildren(
    el("div", { class: "page-head" },
      el("div", {},
        el("button", { type: "button", class: "ghost back", onClick: closeOrder },
          el("span", { "aria-hidden": "true", text: "←" }), " Orders"),
        el("h2", {}, "Order ", el("code", { text: order.number })),
        el("p", { text: `Placed ${when(order.created_at)}${order.paid_at ? ` · paid ${when(order.paid_at)}` : ""}` }),
      ),
      el("div", { class: "actions" }, badge(order.status)),
    ),

    el("div", { class: "card" },
      el("header", {}, el("div", {}, el("h3", { text: "Summary" }))),
      el("div", { class: "card-body" }, facts(order, customer)),
    ),

    el("div", { class: "card" },
      el("header", {}, el("div", {}, el("h3", { text: "Lines" }))),
      lineTable(items, order.currency),
    ),

    el("div", { class: "card" },
      el("header", {}, el("div", {},
        el("h3", { text: "Payments and invoices" }),
        el("p", { text: "What the provider confirmed, and the documents that followed." }),
      )),
      el("div", { class: "card-body" },
        paymentList(payments),
        invoiceList(invoices),
      ),
    ),

    el("div", { class: "card" },
      el("header", {}, el("div", {},
        el("h3", { text: "Download links" }),
        el("p", { text: "The links themselves were never stored, only their hashes — so this shows how a download is going, and can never hand one out." }),
      )),
      el("div", { class: "card-body" }, tokenList(tokens)),
    ),

    actionCard(order),

    el("div", { class: "card" },
      el("header", {}, el("div", {}, el("h3", { text: "Private note" }))),
      el("div", { class: "card-body" }, noteForm(order)),
    ),
  );

  document.getElementById("orders-list").hidden = true;
  panel.hidden = false;
  document.getElementById("crumb").textContent = `Order ${order.number}`;
  window.scrollTo({ top: 0 });
}

function facts(order, customer) {
  const rows = [
    ["Customer", customer?.email ?? "—"],
    ["Name", customer?.name ?? "—"],
    ["VAT number", customer?.vat_id ?? "—"],
    ["Tax country", order.tax_country ?? "—"],
    ["Reverse charge", order.reverse_charge ? "yes" : "no"],
    ["Provider", order.gateway ?? "—"],
    ["Net", money(order.subtotal_minor, order.currency)],
    ["VAT", money(order.tax_minor, order.currency)],
    ["Total", money(order.total_minor, order.currency)],
    // The evidence the tax decision rested on: the thing an auditor asks about
    // and the thing an owner otherwise never sees.
    ["Tax evidence", order.taxEvidence
      ? Object.entries(order.taxEvidence).map(([k, v]) => `${k}: ${v}`).join(", ")
      : "none recorded"],
  ];
  return el("dl", { class: "facts" },
    ...rows.map(([label, value]) =>
      el("div", {}, el("dt", { text: label }), el("dd", { text: String(value) })),
    ),
  );
}

function lineTable(items, currency) {
  return el("div", { class: "table-wrap" },
    el("table", { class: "grid" },
      el("thead", {}, el("tr", {},
        el("th", { scope: "col", text: "Product" }),
        el("th", { scope: "col", class: "numeric", text: "Qty" }),
        el("th", { scope: "col", class: "numeric", text: "Unit" }),
        el("th", { scope: "col", text: "VAT" }),
        el("th", { scope: "col", class: "numeric", text: "Total" }),
      )),
      el("tbody", {}, ...items.map((i) =>
        el("tr", {},
          td("Product",
            el("div", { class: "cell-title" },
              el("strong", { text: i.name }),
              el("small", {}, el("code", { text: i.sku })),
            ),
          ),
          td("Qty", { class: "numeric" }, String(i.quantity)),
          td("Unit", { class: "numeric" }, money(i.unit_minor, currency)),
          td("VAT", `${(i.tax_rate_bp / 100).toFixed(2)}% · ${money(i.tax_minor, currency)}`),
          td("Total", { class: "numeric" }, money(i.total_minor, currency)),
        ),
      )),
    ),
  );
}

function paymentList(payments) {
  if (payments.length === 0) return el("p", { class: "muted", text: "Nothing recorded yet." });
  return el("ul", { class: "list-plain" }, ...payments.map((p) =>
    el("li", {}, `${p.kind} ${p.status} · ${money(p.amount_minor, p.currency)} · ${p.gateway} ${p.gateway_ref} · ${when(p.created_at)}`),
  ));
}

function invoiceList(invoices) {
  if (invoices.length === 0) return el("p", { class: "muted", text: "None issued." });
  return el("ul", { class: "list-plain" }, ...invoices.map((inv) =>
    el("li", {}, `${inv.kind === "credit_note" ? "Credit note" : "Invoice"} ${inv.number} · ${when(inv.issued_at)} · ${money(inv.total_minor, inv.currency)}`),
  ));
}

function tokenList(tokens) {
  if (tokens.length === 0) return el("p", { class: "muted", text: "No links issued." });
  return el("ul", { class: "list-plain" },
    ...tokens.map((t) =>
      el("li", {}, `${t.uses} of ${t.max_uses} downloads used · expires ${when(t.expires_at)}${t.revoked ? " · revoked" : ""}`),
    ),
    // Said plainly, because the first thing an owner tries is to copy a link
    // out of the panel and paste it into an email.
    el("li", { class: "muted", text: "Use “Send the links again” to issue fresh ones." }),
  );
}

function actionCard(order) {
  const row = el("div", { class: "savebar" });
  const refresh = async () => {
    await loadOrders(false); // stay on the page the reader was looking at
    closeOrder();
  };

  if (["paid", "fulfilled"].includes(order.status)) {
    row.append(
      el("button", { type: "button", class: "secondary", onClick: () => act("resend", order, refresh, "Links sent.") }, "Send the links again"),
      el("button", { type: "button", class: "danger", onClick: () => refund(order, refresh) }, "Refund"),
    );
  }
  if (["pending", "needs_review"].includes(order.status)) {
    row.append(el("button", { type: "button", onClick: () => markPaid(order, refresh) }, "Mark as paid"));
  }
  if (["pending", "failed"].includes(order.status)) {
    row.append(el("button", { type: "button", class: "secondary", onClick: () => act("cancel", order, refresh, "Cancelled.") }, "Cancel"));
  }

  if (row.childElementCount === 0) {
    return el("div", { class: "card" },
      el("div", { class: "card-body" },
        el("p", { class: "muted", text: `Nothing to do from here for an order that is ${order.status}.` }),
      ),
    );
  }
  row.prepend(el("span", { class: "spacer" }));
  return el("div", { class: "card" },
    el("header", {}, el("div", {}, el("h3", { text: "Actions" }))),
    el("div", { class: "card-body" }, row),
  );
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
  const input = el("textarea", { id: "order-note", name: "notes", maxlength: "4000" }, order.notes ?? "");
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
    el("div", { class: "field" },
      el("label", { for: "order-note", text: "Private note" }),
      input,
      el("small", { text: "Never shown to the buyer, and never sent anywhere." }),
    ),
    el("div", { class: "savebar" },
      el("span", { class: "spacer" }),
      el("button", { type: "submit", class: "secondary" }, "Save note"),
    ),
  );
}
