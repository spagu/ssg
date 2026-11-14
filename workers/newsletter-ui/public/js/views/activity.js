// Activity (GO-137): the audit trail — who scheduled, paused, erased,
// imported — newest first, filtered by who and by action. Reads
// GET api/audit (permission newsletter:settings). An entry names the
// account in `auth` mode; with the shared password or Cloudflare Access
// the mode is all that is known, and the view says so.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { AUDIT_ACTIONS, auditWho } from "../models/access.js";
import { btn, busy, emptyState, errorBlock, loading, pageHead, slot } from "./ui.js";

export default async function activity(app, root, route) {
  const { t, fmt, api } = app;
  const actor = (route.query.actor ?? "").trim();
  const action = AUDIT_ACTIONS.includes(route.query.action) ? route.query.action : "";
  const list = slot();
  const more = slot();
  const reload = () => activity(app, root, route).catch((e) => mount(root, errorBlock(app, e, reload)));
  const filter = (e) => {
    e.preventDefault();
    const d = Object.fromEntries(new FormData(e.currentTarget));
    app.go(href("activity", {}, { actor: d.actor.trim() || null, action: d.action || null }));
  };
  mount(root, [
    pageHead(app, t("audit.title"), { lead: t("audit.lead") }),
    h(
      "form",
      { class: "filters", role: "search", "aria-label": t("audit.filter.go"), on: { submit: filter } },
      h("div", { class: "field" }, h("label", { for: "au-who" }, t("audit.filter.actor")), h("input", { id: "au-who", name: "actor", type: "search", value: actor, autocomplete: "off" })),
      h("div", { class: "field" }, h("label", { for: "au-act" }, t("audit.filter.action")), h("select", { id: "au-act", name: "action" }, h("option", { value: "" }, t("audit.filter.any")), AUDIT_ACTIONS.map((a) => h("option", { value: a, selected: a === action }, t(`audit.act.${a}`))))),
      h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("audit.filter.go"))),
    ),
    list.node,
    more.node,
  ]);
  list.set(loading(app));

  const items = [];
  let cursor = null;
  const cols = ["audit.col.at", "audit.col.who", "audit.col.action", "audit.col.target", "audit.col.status"];
  const row = (e) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, fmt.dateTime(e.at)),
      h("td", { "data-label": t("audit.col.who") }, auditWho(e) ?? t(`audit.mode.${e.mode}`)),
      h("td", { "data-label": t("audit.col.action") }, app.has(`audit.act.${e.action}`) ? t(`audit.act.${e.action}`) : e.action),
      h("td", { "data-label": t("audit.col.target") }, h("code", null, e.target ?? "—")),
      h("td", { "data-label": t("audit.col.status") }, e.status < 400 ? t("audit.status.ok") : t("audit.status.refused", { status: e.status })),
    );
  const table = () =>
    h(
      "div",
      { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "au-caption" },
      h("table", { class: "data list" }, h("caption", { id: "au-caption", class: "visually-hidden" }, t("audit.title")), h("thead", null, h("tr", null, cols.map((k) => h("th", { scope: "col" }, t(k))))), h("tbody", null, items.map(row))),
    );
  const page = async () => {
    const r = await api.get("audit", { actor, action, cursor, limit: 50 });
    items.push(...r.items);
    cursor = r.nextCursor;
    list.set(items.length ? table() : emptyState({ img: "all-clear", title: t("audit.empty.title"), body: t("audit.empty.body") }));
    more.set(cursor ? btn(t("action.loadMore"), (e) => busy(app, e.currentTarget, page)) : []);
  };
  try {
    await page();
  } catch (e) {
    list.set(errorBlock(app, e, reload));
  }
}
