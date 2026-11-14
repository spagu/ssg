// Groups: every group with its members and the roles granted to it, and a
// form for a new one. A group hands its roles to every member.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { act, empty, field, loading, pageHead, tableRegion } from "./ui.js";

export default async function groups(app, view) {
  const { t } = app;
  mount(view, loading(app));
  const [list, grants] = await Promise.all([app.api.get("admin/groups"), app.api.get("admin/grants")]);
  const rolesOf = (id) => grants.items.filter((g) => g.groupId === id).map((g) => g.roleName);
  const rows = list.items.map((g) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("a", { href: href("group", { id: g.id }) }, g.name), g.description ? h("span", { class: "sub" }, g.description) : null),
      h("td", null, app.fmt.number(g.memberCount)),
      h("td", null, rolesOf(g.id).join(", ") || "—"),
    ),
  );
  async function create(e) {
    e.preventDefault();
    const body = { name: e.target.name.value.trim(), description: e.target.description.value.trim() };
    const r = await act(app, e.submitter, () => app.api.post("admin/groups", body));
    if (r) {
      app.notify(t("groups.created", { name: r.group.name }));
      app.go(href("group", { id: r.group.id }));
    }
  }
  mount(view, [
    pageHead(app, t("nav.groups"), { lead: t("groups.lead") }),
    rows.length ? tableRegion(app, t("groups.caption"), [t("groups.col.name"), t("groups.col.members"), t("groups.col.roles")], rows) : empty(t("groups.none")),
    h(
      "form",
      { class: "card form", "aria-labelledby": "new-group", on: { submit: (e) => void create(e) } },
      h("h2", { id: "new-group" }, t("groups.new")),
      field("g-name", t("groups.name"), h("input", { type: "text", name: "name", required: true, maxlength: "100", autocomplete: "off" })),
      field("g-desc", t("groups.description"), h("input", { type: "text", name: "description", maxlength: "500", autocomplete: "off" })),
      h("button", { type: "submit", class: "btn btn-primary" }, t("groups.create")),
    ),
  ]);
}
