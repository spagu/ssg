// Uncertain sends: messages that were in flight when a consumer died. The
// engine never re-sends them by itself (a duplicate is worse than one visibly
// missing message); the admin decides per row or for all: re-send once more,
// or dismiss. `safeResend` says whether the transport itself would drop a
// second copy (Resend within 24 hours) — otherwise a re-send may deliver twice.

import { h } from "../core/vnode.js";
import { busy, confirmDialog, errorBlock } from "./ui.js";

export async function uncertainSection(app, c, after) {
  const { t, fmt, api } = app;
  const base = `campaigns/${encodeURIComponent(c.id)}/uncertain`;
  let r;
  try {
    r = await api.get(base, { limit: 200 });
  } catch (e) {
    return errorBlock(app, e);
  }
  if (!r.items.length) return null;
  const picked = new Set();
  const act = (kind) => async (e) => {
    const ids = picked.size ? [...picked] : null;
    const count = ids ? ids.length : r.items.length;
    const vars = { count, subject: c.subject };
    const ok = await confirmDialog(app, {
      title: t(`unc.${kind}.title`, vars),
      body: kind === "resend" ? t(r.safeResend ? "unc.resend.safe" : "unc.resend.unsafe", vars) : t("unc.dismiss.body", vars),
      confirm: t(`unc.${kind}.go`, vars),
      danger: kind === "resend" && !r.safeResend,
    });
    if (!ok) return;
    const res = await busy(app, e.currentTarget, () => api.post(`${base}/${kind}`, ids ? { ids } : {}));
    if (res) {
      app.notify(t(`unc.${kind}.done`, { count: kind === "resend" ? res.resent : res.dismissed }));
      app.go(location.hash);
      after();
    }
  };
  return h(
    "section",
    { class: "card", "aria-labelledby": "unc-title" },
    h("h2", { id: "unc-title", tabindex: "-1" }, t("unc.title", { count: r.items.length })),
    h("p", null, t("unc.lead")),
    h("p", { class: `callout ${r.safeResend ? "info" : "warning"}` }, t(r.safeResend ? "unc.hint.safe" : "unc.hint.unsafe", { transport: r.transport ?? "—" })),
    h(
      "div",
      { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "unc-caption" },
      h(
        "table",
        { class: "data list" },
        h("caption", { id: "unc-caption" }, t("unc.caption")),
        h("thead", null, h("tr", null, h("th", { scope: "col" }, t("col.pick")), h("th", { scope: "col" }, t("col.email")), h("th", { scope: "col" }, t("col.claimedAt")), h("th", { scope: "col" }, t("col.safe")))),
        h(
          "tbody",
          null,
          r.items.map((u) => {
            const label = u.email ?? t("unc.erased");
            return h(
              "tr",
              null,
              h("td", { "data-label": t("col.pick") }, h("input", { type: "checkbox", "aria-label": t("unc.pick", { email: label }), on: { change: (e) => (e.target.checked ? picked.add(u.subscriberId) : picked.delete(u.subscriberId)) } })),
              h("th", { scope: "row" }, label),
              h("td", { "data-label": t("col.claimedAt") }, fmt.dateTime(u.claimedAt)),
              h("td", { "data-label": t("col.safe") }, t(u.safeResend ? "unc.safe.yes" : "unc.safe.no")),
            );
          }),
        ),
      ),
    ),
    h("p", { class: "hint" }, t("unc.pickHint")),
    h(
      "div",
      { class: "form-actions" },
      h("button", { type: "button", class: "btn btn-primary", on: { click: act("resend") } }, t("unc.resend")),
      h("button", { type: "button", class: "btn", on: { click: act("dismiss") } }, t("unc.dismiss")),
    ),
  );
}
