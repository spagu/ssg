// The log screen: the outbox and the audit trail.

import { api } from "./api.js";
import { el, notify, td, when } from "./dom.js";

export async function loadOutbox() {
  const { messages, counts } = await api.get("/api/shop/admin/outbox");
  const rows = document.getElementById("outbox-rows");
  rows.replaceChildren(
    ...messages.map((m) =>
      el("tr", {},
        // The target is an address or a URL, never the message body: the body
        // holds download links and the buyer's details.
        td("Message",
          el("div", { class: "cell-title" },
            el("strong", { text: m.target }),
            el("small", { text: m.kind }),
          ),
        ),
        td("Tries", { class: "numeric" },
          el("div", { class: "cell-title" },
            el("strong", { text: String(m.attempts) }),
            el("small", { text: `next ${when(m.next_attempt)}` }),
          ),
        ),
        td("Last error", el("span", { class: "muted", text: m.last_error ?? "—" })),
        td("", { class: "actions" }, el("button", {
          type: "button", class: "secondary small",
          onClick: () => retry(m.id),
        }, "Retry now")),
      ),
    ),
  );
  if (messages.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "4", class: "muted", text:
      `Nothing waiting. ${counts.done} message${counts.done === 1 ? "" : "s"} sent so far.` })));
  }
}

async function retry(id) {
  try {
    await api.post("/api/shop/admin/outbox", { id });
    notify("ok", "Sent, or queued for another try.");
    await loadOutbox();
  } catch (err) {
    notify("error", err.message);
  }
}

export function bindOutbox() {
  document.getElementById("outbox-drain").addEventListener("click", async () => {
    try {
      const { sent } = await api.post("/api/shop/admin/outbox", { action: "drain" });
      notify("ok", `${sent} message(s) went out.`);
      await loadOutbox();
    } catch (err) {
      notify("error", err.message);
    }
  });
}

export async function loadAudit() {
  const { entries } = await api.get("/api/shop/admin/audit?limit=100");
  const rows = document.getElementById("audit-rows");
  rows.replaceChildren(
    ...entries.map((e) =>
      el("tr", {},
        td("When", el("span", { class: "muted", text: when(e.created_at) })),
        td("Who", e.actor),
        td("What", el("code", { text: e.action })),
        td("Detail", el("span", { class: "muted", text: e.detail ? JSON.stringify(e.detail) : "—" })),
      ),
    ),
  );
  if (entries.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "4", class: "muted", text: "Nothing logged yet." })));
  }
}
