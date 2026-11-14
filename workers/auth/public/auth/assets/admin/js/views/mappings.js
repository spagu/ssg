// Cloudflare Access group mappings: a group as the identity provider names
// it in the Access token → an `auth` group. Shown only while Access is
// configured. A mapping changes only the memberships it added.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { act, btn, confirmDialog, empty, field, loading, pageHead, tableRegion } from "./ui.js";

export default async function mappings(app, view) {
  const { t, fmt } = app;
  mount(view, loading(app));
  const [list, groups] = await Promise.all([app.api.get("admin/access-groups"), app.api.get("admin/groups")]);
  const reload = () => app.go(href("mappings"));
  async function add(e) {
    e.preventDefault();
    const r = await act(app, e.submitter, () => app.api.post("admin/access-groups", { accessGroup: e.target.accessGroup.value.trim(), groupId: e.target.groupId.value }));
    if (r) {
      app.notify(t("mappings.added", { count: r.resynced }));
      await reload();
    }
  }
  async function remove(e, m) {
    if (!(await confirmDialog(app, { title: t("mappings.remove.title", { access: m.accessGroup, group: m.groupName ?? "—" }), body: t("mappings.remove.body"), confirm: t("mappings.remove.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/access-groups/${m.id}`))) !== undefined) {
      app.notify(t("mappings.removed"));
      await reload();
    }
  }
  const rows = list.items.map((m) =>
    h("tr", null, h("th", { scope: "row" }, h("code", null, m.accessGroup)), h("td", null, m.groupName ? h("a", { href: href("group", { id: m.groupId }) }, m.groupName) : t("mappings.groupGone")), h("td", null, fmt.date(m.createdAt)), h("td", null, btn(t("mappings.remove.button"), (e) => void remove(e, m), { "aria-label": t("mappings.remove.label", { access: m.accessGroup }) }))),
  );
  mount(view, [
    pageHead(app, t("nav.mappings"), { lead: t("mappings.lead") }),
    app.me.access === "misconfigured" ? h("div", { class: "callout warning" }, h("p", null, t("status.warn.accessMisconfigured"))) : null,
    rows.length ? tableRegion(app, t("mappings.caption"), [t("mappings.col.access"), t("mappings.col.group"), t("mappings.col.created"), t("session.col.action")], rows) : empty(t("mappings.none")),
    h("form", { class: "card form", "aria-labelledby": "new-map", on: { submit: (e) => void add(e) } },
      h("h2", { id: "new-map" }, t("mappings.new")),
      field("map-access", t("mappings.access"), h("input", { type: "text", name: "accessGroup", required: true, maxlength: "256", autocomplete: "off", spellcheck: "false" }), t("mappings.accessHint")),
      field("map-group", t("mappings.group"), h("select", { name: "groupId", required: true }, h("option", { value: "" }, t("filter.choose")), groups.items.map((g) => h("option", { value: g.id }, g.name)))),
      h("button", { type: "submit", class: "btn btn-primary" }, t("mappings.add"))),
  ]);
}
