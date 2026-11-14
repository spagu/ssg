// Roles: every role — built-in ones marked — with its permissions at a
// glance (wildcards and unknown names flagged), and a form for a new one.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { wildcardsIn } from "../models/perms.js";
import { act, chip, empty, field, loading, pageHead, tableRegion } from "./ui.js";

export default async function roles(app, view) {
  const { t } = app;
  mount(view, loading(app));
  const [list, grants] = await Promise.all([app.api.get("admin/roles"), app.api.get("admin/grants")]);
  const holders = (id) => grants.items.filter((g) => g.roleId === id).length;
  const rows = list.items.map((r) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("a", { href: href("role", { id: r.id }) }, r.name), r.description ? h("span", { class: "sub" }, r.description) : null),
      h("td", null, r.builtIn ? chip(t("roles.builtIn"), "info") : t("roles.custom")),
      h("td", null, h("span", { class: "perm-list" }, r.permissions.join(", ") || "—"), wildcardsIn(r.permissions).length ? chip(t("roles.wildcard"), "warning") : null, r.unknownPermissions.length ? chip(t("perm.unknown"), "warning") : null),
      h("td", null, app.fmt.number(holders(r.id))),
    ),
  );
  async function create(e) {
    e.preventDefault();
    const body = { name: e.target.name.value.trim(), description: e.target.description.value.trim(), permissions: [] };
    const r = await act(app, e.submitter, () => app.api.post("admin/roles", body));
    if (r) {
      app.notify(t("roles.created", { name: r.role.name }));
      app.go(href("role", { id: r.role.id }));
    }
  }
  mount(view, [
    pageHead(app, t("nav.roles"), { lead: t("roles.lead") }),
    rows.length ? tableRegion(app, t("roles.caption"), [t("roles.col.name"), t("roles.col.kind"), t("roles.col.perms"), t("roles.col.grants")], rows) : empty(t("roles.none")),
    h(
      "form",
      { class: "card form", "aria-labelledby": "new-role", on: { submit: (e) => void create(e) } },
      h("h2", { id: "new-role" }, t("roles.new")),
      field("r-name", t("roles.name"), h("input", { type: "text", name: "name", required: true, maxlength: "64", pattern: "[a-z][a-z0-9-]*", autocomplete: "off", spellcheck: "false" }), t("roles.nameHint")),
      field("r-desc", t("roles.description"), h("input", { type: "text", name: "description", maxlength: "500", autocomplete: "off" })),
      h("button", { type: "submit", class: "btn btn-primary" }, t("roles.create")),
    ),
  ]);
}
