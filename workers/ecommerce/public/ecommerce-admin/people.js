// Who can get in, and what the shop can take money with.
//
// Two screens that look unrelated and are the same subject: the credentials a
// shop runs on. One is people, the other is providers, and neither screen ever
// shows a secret — not a password hash, not an API key. There is nothing to
// show: the hashes are never sent to the browser and the keys are not in the
// database at all.

import { api } from "./api.js";
import { badge, confirmDestructive, el, notify, td, when } from "./dom.js";

let you = "";
let canManage = false;
let minPassword = 12;

// ── Users ───────────────────────────────────────────────────────────────────

export async function loadUsers() {
  const body = await api.get("/api/shop/admin/users");
  you = body.you;
  canManage = body.canManage;
  minPassword = body.minPasswordLength ?? 12;

  const rows = document.getElementById("user-rows");
  rows.replaceChildren(...(body.users ?? []).map((u) => userRow(u, body.owners)));

  document.getElementById("user-add").hidden = !canManage;
  document.getElementById("user-new-card").hidden = true;

  // An owner with nobody else is one forgotten password away from a shop they
  // cannot get into. Worth saying before that happens rather than after.
  const alone = document.getElementById("user-alone");
  alone.hidden = body.owners !== 1 || (body.users ?? []).length !== 1;

  renderPasswordCard();
}

function userRow(user, owners) {
  const isYou = user.id === you;
  const lastOwner = user.role === "owner" && owners === 1 && !user.disabled;

  const actions = el("div", { class: "rowactions" });
  if (canManage) {
    actions.append(
      el("button", { type: "button", class: "secondary small", onClick: () => editUser(user, lastOwner) }, "Edit"),
      el("button", {
        type: "button",
        class: "ghost",
        disabled: lastOwner,
        title: lastOwner ? "The only owner who can sign in" : "",
        onClick: () => setDisabled(user, !user.disabled),
      }, user.disabled ? "Restore" : "Suspend"),
      el("button", {
        type: "button",
        class: "ghost danger",
        disabled: lastOwner,
        onClick: () => removeUser(user),
      }, "Delete"),
    );
  }

  return el("tr", {},
    td("Person",
      el("div", { class: "cell-title" },
        el("strong", { text: user.name || user.email }),
        el("small", { text: user.name ? user.email : isYou ? "this is you" : "" }),
      ),
    ),
    td("Role",
      el("span", {},
        badge(user.role === "owner" ? "on" : "draft"),
        " ",
        el("span", { class: "muted", text: user.role }),
      ),
    ),
    td("State", user.disabled
      ? el("span", { class: "badge", "data-status": "cancelled", text: "suspended" })
      : el("span", { class: "badge", "data-status": "active", text: "active" })),
    td("Last signed in", el("span", { class: "muted", text: user.lastLoginAt ? when(user.lastLoginAt) : "never" })),
    td("", { class: "actions" }, actions),
  );
}

export function bindUsers() {
  const card = document.getElementById("user-new-card");
  const form = document.getElementById("user-new");

  document.getElementById("user-add").addEventListener("click", () => {
    card.hidden = false;
    document.getElementById("new-user-email").focus();
  });
  document.getElementById("user-new-cancel").addEventListener("click", () => {
    form.reset();
    card.hidden = true;
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      await api.post("/api/shop/admin/users", {
        email: data.get("email"),
        name: data.get("name"),
        password: data.get("password"),
        role: data.get("role"),
      });
      form.reset();
      notify("ok", "Account created. Tell them their password in person or through something that is not email.");
      await loadUsers();
    } catch (err) {
      notify("error", err.message);
    }
  });
}

async function setDisabled(user, disabled) {
  const question = disabled
    ? `Suspend ${user.email}? They are signed out immediately and cannot sign in again until you restore them.`
    : `Let ${user.email} sign in again?`;
  if (!confirmDestructive(question)) return;
  try {
    await api.patch(`/api/shop/admin/users/${encodeURIComponent(user.id)}`, { disabled });
    notify("ok", disabled ? "Suspended, and signed out." : "Restored.");
    await loadUsers();
  } catch (err) {
    notify("error", err.message);
  }
}

async function removeUser(user) {
  if (!confirmDestructive(
    `Delete ${user.email}? What they did stays in the log with their name on it; only the account goes.`,
  )) return;
  try {
    await api.del(`/api/shop/admin/users/${encodeURIComponent(user.id)}`);
    notify("ok", "Account deleted.");
    await loadUsers();
  } catch (err) {
    notify("error", err.message);
  }
}

function editUser(user, lastOwner) {
  const panel = document.getElementById("user-editor");

  const form = el("form", {
    onSubmit: async (event) => {
      event.preventDefault();
      const data = new FormData(form);
      const password = String(data.get("password") ?? "");
      try {
        await api.patch(`/api/shop/admin/users/${encodeURIComponent(user.id)}`, {
          name: data.get("name"),
          role: data.get("role"),
          ...(password ? { password } : {}),
        });
        notify("ok", password ? "Saved. Their sessions have been signed out." : "Saved.");
        panel.hidden = true;
        await loadUsers();
      } catch (err) {
        notify("error", err.message);
      }
    },
  },
    el("div", { class: "fields two" },
      field("Name", el("input", { name: "name", value: user.name ?? "", maxlength: "120" }),
        "Shown here and beside what they do in the log."),
      field("Email", el("input", { value: user.email, disabled: true }),
        "What they sign in with. Create a new account to change it."),
      field("Role",
        el("select", { name: "role", disabled: lastOwner },
          el("option", { value: "staff", selected: user.role === "staff" }, "Staff — the day's work"),
          el("option", { value: "owner", selected: user.role === "owner" }, "Owner — everything, including money"),
        ),
        lastOwner
          ? "The only owner who can sign in. Make somebody else an owner first."
          : "Staff can read orders, resend a download and take a note. Only an owner can change prices, VAT, settings or these accounts.",
      ),
      field("Set a new password",
        el("input", { name: "password", type: "password", autocomplete: "new-password", minlength: String(minPassword) }),
        `Leave empty to keep the current one. At least ${minPassword} characters; setting it signs them out everywhere.`,
      ),
    ),
    el("div", { class: "savebar" },
      el("span", { class: "spacer" }),
      el("button", { type: "button", class: "secondary", onClick: () => (panel.hidden = true) }, "Cancel"),
      el("button", { type: "submit" }, "Save"),
    ),
  );

  panel.replaceChildren(
    el("div", { class: "card" },
      el("header", {}, el("div", {},
        el("h3", { text: user.name || user.email }),
        el("p", { text: user.id === you ? "This is you." : "" }),
      )),
      el("div", { class: "card-body" }, form),
    ),
  );
  panel.hidden = false;
  panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

/** Changing your own password, which asks for the current one — the owner-driven
 *  reset above does not, because an owner does not know a colleague's. */
function renderPasswordCard() {
  const host = document.getElementById("own-password");

  const form = el("form", {
    onSubmit: async (event) => {
      event.preventDefault();
      const data = new FormData(form);
      try {
        const result = await api.post("/api/shop/admin/password", {
          currentPassword: data.get("currentPassword"),
          newPassword: data.get("newPassword"),
        });
        form.reset();
        notify("ok", result.message ?? "Password changed.");
      } catch (err) {
        notify("error", err.message);
      }
    },
  },
    el("div", { class: "fields two" },
      field("Current password", el("input", { name: "currentPassword", type: "password", autocomplete: "current-password", required: true })),
      field("New password",
        el("input", { name: "newPassword", type: "password", autocomplete: "new-password", minlength: String(minPassword), required: true }),
        `At least ${minPassword} characters. A short sentence beats a word with symbols in it.`),
    ),
    el("div", { class: "savebar" },
      el("p", { text: "Every session is signed out, including this one." }),
      el("span", { class: "spacer" }),
      el("button", { type: "submit" }, "Change password"),
    ),
  );

  host.replaceChildren(
    el("header", {}, el("div", {},
      el("h3", { text: "Your own password" }),
      el("p", { text: "Asked for the current one first, so a borrowed session cannot become a permanent one." }),
    )),
    el("div", { class: "card-body" }, form),
  );
}

// ── Payments ────────────────────────────────────────────────────────────────

export async function loadGateways() {
  const body = await api.get("/api/shop/admin/gateways");
  const host = document.getElementById("gateway-rows");

  host.replaceChildren(...(body.gateways ?? []).map((g) => gatewayCard(g, body.order)));
  document.getElementById("gateway-note").textContent = body.note ?? "";
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
        el("p", { text: g.configured
          ? position === 0
            ? "The first button a buyer sees."
            : g.enabled ? "Offered at checkout." : "Configured, but not offered."
          : `Set ${g.missing.join(" and ")} with \`wrangler pages secret put\`, then reload this screen.` }),
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

// ── Shared ──────────────────────────────────────────────────────────────────

let seq = 0;

function field(label, control, help) {
  control.id = control.id || `p-${++seq}`;
  return el("div", { class: "field" },
    el("label", { for: control.id, text: label }),
    control,
    help ? el("small", { text: help }) : null,
  );
}
