// The audit log: sign-ins, refusals, second-factor events and every
// administrative change — who, what, when — filtered by kind, account and
// dates, a page at a time, and exported as CSV with the same filter.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { AUDIT_FILTER, auditFilter, formData, lastDays, toApi } from "../models/filters.js";
import { EVENT_GROUPS } from "../models/events.js";
import { detailText, kindLabel } from "./audit-format.js";
import { act, btn, empty, field, loading, pageHead, slot, tableRegion } from "./ui.js";

export default async function audit(app, view, route) {
  const { t, fmt } = app;
  const f = auditFilter(route.query);
  const apiFilter = toApi(f);
  mount(view, loading(app));
  const list = slot();
  const items = [];
  let cursor = "";
  let retention = 365;

  const kinds = h(
    "select",
    { name: "kind" },
    h("option", { value: "" }, t("filter.any")),
    Object.entries(EVENT_GROUPS).map(([group, ks]) => h("optgroup", { label: t(`audit.group.${group}`) }, ks.map((k) => h("option", { value: k, selected: f.kind === k }, kindLabel(app, k))))),
  );
  const form = h(
    "form",
    { class: "filters", role: "search", "aria-label": t("audit.filters"), on: { submit: (e) => (e.preventDefault(), app.go(href("audit", {}, { ...formData(new FormData(e.target), AUDIT_FILTER), user: f.user }))) } },
    field("a-kind", t("audit.kind"), kinds),
    field("a-from", t("audit.from"), h("input", { type: "date", name: "from", value: f.from ?? "" })),
    field("a-to", t("audit.to"), h("input", { type: "date", name: "to", value: f.to ?? "" })),
    h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("action.filter")), Object.keys(f).length ? h("a", { class: "btn btn-quiet", href: href("audit") }, t("action.clear")) : null),
  );
  const quick = h("p", { class: "quick" }, t("audit.quick"), " ", [7, 30, 90].map((n) => [h("a", { href: href("audit", {}, { ...f, ...lastDays(n) }) }, t("audit.lastDays", { count: n })), " "]));

  const row = (e) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("time", { datetime: e.at }, fmt.dateTime(e.at))),
      h("td", null, kindLabel(app, e.kind)),
      h("td", null, e.userId ? h("a", { href: href("audit", {}, { ...f, user: e.userId }) }, e.email ?? t("audit.deleted")) : "—"),
      h("td", null, e.by ? h("a", { href: href("audit", {}, { ...f, user: e.by }) }, e.byEmail ?? t("audit.deleted")) : "—"),
      h("td", null, e.method ? t(`provider.${e.method}`) : "—"),
      h("td", { class: "detail" }, detailText(e) || "—"),
    );
  function draw() {
    list.set([
      items.length ? tableRegion(app, t("audit.caption", { count: items.length }), [t("audit.col.at"), t("audit.col.kind"), t("audit.col.account"), t("audit.col.by"), t("audit.col.method"), t("audit.col.detail")], items.map(row)) : empty(t("audit.none")),
      cursor ? btn(t("action.more"), (e) => void act(app, e.currentTarget, more)) : null,
    ]);
  }
  async function more() {
    const page = await app.api.get("admin/events", { ...apiFilter, cursor });
    items.push(...page.items);
    cursor = page.nextCursor ?? "";
    retention = page.retentionDays;
    draw();
  }
  await more();
  const exportHref = app.api.url("admin/events/export", apiFilter);
  mount(view, [
    pageHead(app, t("nav.audit"), { lead: t("audit.lead"), actions: h("a", { class: "btn", href: exportHref, download: "" }, t("audit.export")) }),
    h("p", { class: "hint" }, t("audit.retention", { days: retention })),
    f.user ? h("p", null, t("audit.forAccount"), " ", h("a", { href: href("audit", {}, { ...f, user: "" }) }, t("action.clear"))) : null,
    form,
    quick,
    list.node,
  ]);
  draw(); // the first page arrived before the list's place existed
}
