// What the shop can take money with: the payment providers, whether each is
// configured, and the webhook each one needs.
//
// Never a secret on this screen: the keys are not in the database at all, only
// in the deployment's secrets, so there is nothing to show.

import { api } from "./api.js";
import { el, notify, when, withCode } from "./dom.js";

export async function loadGateways() {
  const body = await api.get("/api/shop/admin/gateways");
  const host = document.getElementById("gateway-rows");

  host.replaceChildren(...(body.gateways ?? []).map((g) => gatewayCard(g, body.order)));
  document.getElementById("gateway-note").replaceChildren(...withCode(body.note));
}

function gatewayCard(g, order) {
  const position = order.indexOf(g.name);
  const state = !g.configured ? "needs_review" : g.enabled ? "on" : "off";
  const stateWord = !g.configured ? "not configured" : g.enabled ? "on" : "off";

  const facts = [
    ["Mode", g.mode === "unknown" ? "—" : g.mode === "test" ? "test keys" : "live keys"],
    ["Webhooks seen", String(g.eventsSeen)],
    ["Last event", g.lastEventAt ? `${g.lastEventType} · ${when(g.lastEventAt)}` : "none yet"],
    ["Payments taken", String(g.paymentsTaken)],
  ];

  return el("div", { class: "card" },
    el("header", {},
      el("div", {},
        el("h3", {}, g.label, " ", el("span", { class: "badge", "data-status": state, text: stateWord })),
        el("p", {}, ...withCode(g.configured
          ? position === 0
            ? "The first button a buyer sees."
            : g.enabled ? "Offered at checkout." : "Configured, but not offered."
          : `Set ${g.missing.map((name) => `\`${name}\``).join(" and ")} with \`wrangler pages secret put\`, then reload this screen.`)),
      ),
      g.configured
        ? el("div", { class: "actions" },
            el("button", {
              type: "button",
              class: g.enabled ? "secondary" : "",
              onClick: () => toggleGateway(g),
            }, g.enabled ? "Stop offering" : "Offer at checkout"),
            g.enabled && position > 0
              ? el("button", { type: "button", class: "ghost", onClick: () => promote(g) }, "Show first")
              : null,
          )
        : null,
    ),
    el("div", { class: "card-body" },
      el("dl", { class: "facts" },
        ...facts.map(([label, value]) => el("div", {}, el("dt", { text: label }), el("dd", { text: value }))),
      ),

      // The part sellers actually get stuck on: the exact URL, and the list of
      // events, in a form that can be copied rather than retyped.
      el("h4", { text: "Webhook" }),
      el("p", { class: "muted" },
        "Paste this into ",
        el("a", { href: g.dashboardUrl, target: "_blank", rel: "noreferrer noopener", text: `${g.label}'s dashboard` }),
        ". Without it the shop never hears that a payment succeeded, and a buyer pays without being sent their book.",
      ),
      el("p", {}, el("code", { class: "copyable", text: g.webhookUrl })),
      el("p", { class: "muted", text: "Events to send:" }),
      el("ul", { class: "list-plain" }, ...g.events.map((e) => el("li", {}, el("code", { text: e })))),

      g.eventsSeen === 0 && g.enabled
        ? el("p", { class: "muted", text: "Nothing has arrived here yet. That is expected until the first sale — and the first thing to check if a sale ever seems to go through without a delivery." })
        : null,
    ),
  );
}

async function toggleGateway(g) {
  const { gateways, order } = await api.get("/api/shop/admin/gateways");
  const next = g.enabled
    ? order.filter((name) => name !== g.name)
    : [...order.filter((name) => name !== g.name), g.name];

  // An order that names nothing means "whatever the environment says", which is
  // not the same as "offer none" — so the last one off is spelled out.
  const usable = gateways.filter((x) => x.configured).map((x) => x.name);
  const payload = next.length === 0 ? usable.filter((name) => name !== g.name) : next;

  try {
    await api.post("/api/shop/admin/gateways", { order: payload });
    notify("ok", g.enabled ? `${g.label} is no longer offered.` : `${g.label} is now offered at checkout.`);
    await loadGateways();
  } catch (err) {
    notify("error", err.message);
  }
}

async function promote(g) {
  const { order } = await api.get("/api/shop/admin/gateways");
  try {
    await api.post("/api/shop/admin/gateways", { order: [g.name, ...order.filter((n) => n !== g.name)] });
    notify("ok", `${g.label} is now the first button.`);
    await loadGateways();
  } catch (err) {
    notify("error", err.message);
  }
}
