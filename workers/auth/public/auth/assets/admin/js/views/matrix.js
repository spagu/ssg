// The access matrix: people × modules — for each, none, some or all of the
// module's permissions — as a real table (row and column headers, the
// person's column sticky), with a sentence per module under it for anyone
// who would rather read than scan. Filters: a person, a status (server
// side), and "who holds this module" (here).

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { STATUSES, pick } from "../models/filters.js";
import { LEVELS, filterRows, rowsOf, tally } from "../models/matrix.js";
import { modulesOf } from "../models/perms.js";
import { empty, field, loading, pageHead, tableRegion } from "./ui.js";

const MAX_PAGES = 10;

export default async function matrix(app, view, route) {
  const { t } = app;
  const f = pick(route.query, ["q", "status", "module", "level"]);
  mount(view, loading(app));
  const reg = await app.registry();
  const modules = modulesOf(reg.items, app.lang);
  const users = [];
  let cursor = "";
  for (let i = 0; i < MAX_PAGES; i++) {
    const page = await app.api.get("admin/matrix", { q: f.q, status: f.status, limit: 200, cursor });
    users.push(...page.items);
    cursor = page.nextCursor;
    if (!cursor) break;
  }
  const all = rowsOf(users, modules);
  const rows = filterRows(all, modules, f);
  const cell = (c) => h("td", { class: `lvl lvl-${c.level}` }, c.level === "some" ? t("matrix.some", { held: c.held, total: c.total }) : t(`matrix.${c.level}`));
  const body = rows.map((r) => h("tr", null, h("th", { scope: "row" }, h("a", { href: href("person", { id: r.user.id }) }, r.user.name || r.user.email), r.user.status === "blocked" ? h("span", { class: "sub" }, t("status.blocked")) : null), r.cells.map(cell)));
  const sel = (name, label, options) => field(`m-${name}`, label, h("select", { name }, h("option", { value: "" }, t("filter.any")), options.map(([v, l]) => h("option", { value: v, selected: f[name] === v }, l))));
  const form = h(
    "form",
    { class: "filters", role: "search", "aria-label": t("matrix.filters"), on: { submit: (e) => (e.preventDefault(), app.go(href("matrix", {}, pick(Object.fromEntries(new FormData(e.target)), ["q", "status", "module", "level"])))) } },
    field("m-q", t("people.search"), h("input", { type: "search", name: "q", value: f.q ?? "", autocomplete: "off" })),
    sel("status", t("people.status"), STATUSES.map((s) => [s, t(`status.${s}`)])),
    sel("module", t("matrix.module"), modules.map((m) => [m.id, m.id])),
    sel("level", t("matrix.level"), [["any", t("matrix.any")], ...LEVELS.map((l) => [l, t(`matrix.${l}`)])]),
    h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t("action.filter")), Object.keys(f).length ? h("a", { class: "btn btn-quiet", href: href("matrix") }, t("action.clear")) : null),
  );
  const summary = tally(all, modules).map((s) => h("li", null, t("matrix.summary", { module: s.module, all: s.all, some: s.some, none: s.none })));
  mount(view, [
    pageHead(app, t("nav.matrix"), { lead: t("matrix.lead", { minutes: app.me.tokenMinutes }) }),
    form,
    !modules.length
      ? empty(t("matrix.noModules"))
      : rows.length
        ? tableRegion(app, t("matrix.caption", { count: rows.length }), [h("th", { scope: "col", class: "sticky" }, t("people.col.person")), ...modules.map((m) => h("th", { scope: "col" }, m.id))], body, { className: "matrix" })
        : empty(t("people.noMatch")),
    cursor ? h("p", { class: "hint" }, t("matrix.truncated", { count: users.length })) : null,
    h("section", { class: "block", "aria-labelledby": "m-sum" }, h("h2", { id: "m-sum" }, t("matrix.summaryTitle")), h("ul", { class: "plain" }, summary)),
  ]);
}
