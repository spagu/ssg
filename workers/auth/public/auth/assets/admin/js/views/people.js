// People: every account, searchable and filterable (by status, role, group),
// a page at a time; a row opens the person. Invitations have their own view.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { allows } from "../models/access.js";
import { STATUSES, USER_FILTER, formData, userFilter } from "../models/filters.js";
import { act, btn, chip, empty, field, loading, pageHead, slot, tableRegion } from "./ui.js";

export default async function people(app, view, route) {
  const { t, fmt } = app;
  const filter = userFilter(route.query);
  mount(view, [pageHead(app, t("people.title"), { lead: t("people.lead") }), loading(app)]);
  const [roles, groups] = await Promise.all([app.api.get("admin/roles"), app.api.get("admin/groups")]);
  const list = slot("people-list");
  const rows = [];
  let cursor = null;

  const select = (name, label, options) =>
    field(`f-${name}`, label, h("select", { name }, h("option", { value: "" }, t("filter.any")), options.map(([v, l]) => h("option", { value: v, selected: filter[name] === v }, l))));
  const form = h(
    "form",
    { class: "filters", role: "search", "aria-label": t("people.filters"), on: { submit: (e) => (e.preventDefault(), app.go(href("people", {}, formData(new FormData(e.target), USER_FILTER)))) } },
    field("f-q", t("people.search"), h("input", { type: "search", name: "q", value: filter.q ?? "", autocomplete: "off" })),
    select("status", t("people.status"), STATUSES.map((s) => [s, t(`status.${s}`)])),
    select("roleId", t("people.role"), roles.items.map((r) => [r.id, r.name])),
    select("groupId", t("people.group"), groups.items.map((g) => [g.id, g.name])),
    h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("action.filter")), Object.keys(filter).length ? h("a", { class: "btn btn-quiet", href: href("people") }, t("action.clear")) : null),
  );

  const row = (u) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("a", { href: href("person", { id: u.id }) }, u.name || u.email), u.name ? h("span", { class: "sub" }, u.email) : null),
      h("td", null, chip(t(`status.${u.status}`), u.status === "blocked" ? "critical" : "ok"), u.owner ? chip(t("people.owner"), "info") : null),
      h("td", null, u.hasPassword ? t("people.password.yes") : t("people.password.no")),
      h("td", null, u.lastLoginAt ? fmt.relative(u.lastLoginAt) : t("people.never")),
      h("td", null, fmt.date(u.createdAt)),
    );

  function draw() {
    const head = [t("people.col.person"), t("people.col.status"), t("people.col.password"), t("people.col.lastLogin"), t("people.col.created")];
    list.set([
      rows.length ? tableRegion(app, t("people.caption", { count: rows.length }), head, rows.map(row)) : empty(Object.keys(filter).length ? t("people.noMatch") : t("people.none")),
      cursor ? btn(t("action.more"), (e) => void act(app, e.currentTarget, more)) : null,
    ]);
  }
  async function more() {
    const page = await app.api.get("admin/users", { ...filter, cursor });
    rows.push(...page.items);
    cursor = page.nextCursor;
    draw();
  }

  const actions = allows(app.me, "invite") ? h("a", { class: "btn btn-primary", href: href("invitations") }, t("people.invite")) : null;
  mount(view, [pageHead(app, t("people.title"), { lead: t("people.lead"), actions }), form, list.node]);
  await more();
}
