// The bar that says how much is edited and not yet published, with
// "Review and publish" and "Discard all". Empty when nothing is pending, so
// it takes no space. It lives outside the views: edits survive switching
// between Options, Appearance and Files.

import { h } from "../core/vnode.js";
import { openDialog } from "../panel/dialog.js";

export function pendingBar(app) {
  const { t } = app;
  if (!app.t) return null;
  const settings = app.changes.size;
  const files = app.themeDrafts.size;
  const problems = app.problems.size;
  if (!settings && !files && !problems) return null;
  const parts = [];
  if (settings) parts.push(t("pending.settings", { count: settings }));
  if (files) parts.push(t("pending.files", { count: files }));
  if (problems) parts.push(t("pending.problems", { count: problems }));
  const publish = async () => (await import("./publish.js")).publishAll(app);
  const discard = async () => {
    const ok = await openDialog({
      title: t("pending.discardTitle"),
      content: h("p", null, t("pending.discardBody")),
      actions: [{ label: t("action.cancel"), value: null }, { label: t("pending.discard"), value: "go", variant: "danger", submit: true }],
    });
    if (!ok) return;
    app.discardSettings();
    app.themeDrafts.clear();
    app.notify(t("pending.discarded"));
    app.refresh();
  };
  return h(
    "div",
    { class: "pending-bar" },
    h("p", null, h("strong", null, t("pending.title")), " ", parts.join(" \u00b7 ")),
    h(
      "div",
      { class: "pending-actions" },
      h("button", { type: "button", class: "btn", on: { click: () => void discard().catch((e) => app.notify(String(e), "critical")) } }, t("pending.discard")),
      app.caps.settings || app.caps.theme
        ? h("button", { type: "button", class: "btn btn-primary", on: { click: () => void publish().catch((e) => app.notify(String(e?.message ?? e), "critical")) } }, t("pending.publish"))
        : null,
    ),
  );
}
