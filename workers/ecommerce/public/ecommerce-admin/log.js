// The log screen: the outbox and the audit trail.

import { api } from "./api.js";
import { el, notify, td, when } from "./dom.js";

export async function loadOutbox() {
  const { messages, counts } = await api.get("/api/shop/admin/outbox");
  const rows = document.getElementById("outbox-rows");
  rows.replaceChildren(
    ...messages.map((m) =>
      el("tr", {},
        td("Kind", m.kind),
        // The target is an address or a URL, never the message body: the body
        // holds download links and the buyer's details.
        td("To", m.target),
        td("Tries", `${m.attempts}, next ${when(m.next_attempt)}`),
        td("Last error", m.last_error ?? "—"),
        td("Retry", el("button", {
          type: "button", class: "quiet",
          onClick: () => retry(m.id),
        }, "Retry now")),
      ),
    ),
  );
  if (messages.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "5", text:
      `Nothing waiting. ${counts.done} message(s) sent so far.` })));
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
        td("When", when(e.created_at)),
        td("Who", e.actor),
        td("What", e.action),
        td("Detail", e.detail ? JSON.stringify(e.detail) : "—"),
      ),
    ),
  );
  if (entries.length === 0) {
    rows.replaceChildren(el("tr", {}, el("td", { colspan: "4", text: "Nothing logged yet." })));
  }
}
