// One group: its name and description, members (add, remove), the roles it
// grants (grant, take away), and deleting it. Every change reaches the
// members at their next token refresh.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { act, btn, confirmDialog, empty, field, loading, pageHead, tableRegion } from "./ui.js";

export default async function group(app, view, route) {
  const { t, fmt } = app;
  const id = route.params.id;
  mount(view, loading(app));
  const [d, roles] = await Promise.all([app.api.get(`admin/groups/${encodeURIComponent(id)}`), app.api.get("admin/roles")]);
  const g = d.group;
  const minutes = app.me.tokenMinutes;
  const reload = () => app.go(href("group", { id }));
  const after = async (button, fn, done) => {
    if ((await act(app, button, fn)) !== undefined) {
      app.notify(done);
      await reload();
    }
  };
  const confirmed = (dialog) => confirmDialog(app, dialog);

  async function addMember(e) {
    e.preventDefault();
    const email = e.target.email.value.trim().toLowerCase();
    const button = e.submitter;
    await after(button, async () => {
      const found = (await app.api.get("admin/users", { q: email, limit: 5 })).items.find((u) => u.email === email);
      if (!found) throw Object.assign(new Error(email), { kind: "notfound", code: "user_not_found" });
      return app.api.post(`admin/groups/${id}/members`, { userId: found.id });
    }, t("group.added", { email, minutes }));
  }
  const removeMember = async (e, m) => {
    if (await confirmed({ title: t("access.leave.title", { email: m.email, group: g.name }), body: t("access.leave.body", { minutes }), confirm: t("access.leave.go") })) {
      await after(e.currentTarget, () => app.api.del(`admin/groups/${id}/members/${m.userId}`), t("access.leave.done", { group: g.name }));
    }
  };
  const revoke = async (e, gr) => {
    if (await confirmed({ title: t("group.revoke.title", { group: g.name, role: gr.roleName }), body: t("group.revoke.body", { count: g.memberCount, minutes }), confirm: t("access.revoke.go") })) {
      await after(e.currentTarget, () => app.api.del(`admin/grants/${gr.id}`), t("access.revoke.done", { role: gr.roleName }));
    }
  };
  const remove = async (e) => {
    if (!(await confirmed({ title: t("group.delete.title", { group: g.name }), body: t("group.delete.body", { count: g.memberCount }), confirm: t("group.delete.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/groups/${id}`))) !== undefined) {
      app.notify(t("group.delete.done", { group: g.name }));
      app.go(href("groups"));
    }
  };
  async function save(e) {
    e.preventDefault();
    await after(e.submitter, () => app.api.put(`admin/groups/${id}`, { name: e.target.name.value.trim(), description: e.target.description.value.trim() }), t("group.saved"));
  }

  const granted = new Set(d.grants.map((x) => x.roleId));
  const members = d.members.map((m) =>
    h("tr", null, h("th", { scope: "row" }, h("a", { href: href("person", { id: m.userId }) }, m.name || m.email), m.name ? h("span", { class: "sub" }, m.email) : null), h("td", null, t(`status.${m.status}`)), h("td", null, fmt.date(m.addedAt)), h("td", null, btn(t("access.leave.button"), (e) => void removeMember(e, m), { "aria-label": t("group.remove.label", { email: m.email }) }))),
  );
  mount(view, [
    pageHead(app, g.name, { back: { href: href("groups"), label: t("nav.groups") }, lead: g.description || null, actions: btn(t("group.delete.button"), (e) => void remove(e), { variant: "danger" }) }),
    h("section", { class: "block", "aria-labelledby": "grp-members" },
      h("h2", { id: "grp-members" }, t("group.members", { count: d.members.length })),
      members.length ? tableRegion(app, t("group.members", { count: d.members.length }), [t("people.col.person"), t("people.col.status"), t("group.col.added"), t("session.col.action")], members) : empty(t("group.noMembers")),
      h("form", { class: "inline-form", on: { submit: (e) => void addMember(e) } }, field("grp-add", t("group.add.label"), h("input", { type: "email", name: "email", required: true, autocomplete: "off", spellcheck: "false" }), t("group.add.hint")), h("button", { type: "submit", class: "btn" }, t("group.add.go")))),
    h("section", { class: "block", "aria-labelledby": "grp-roles" },
      h("h2", { id: "grp-roles" }, t("group.roles")),
      d.grants.length ? h("ul", { class: "plain rows" }, d.grants.map((gr) => h("li", null, h("a", { href: href("role", { id: gr.roleId }) }, gr.roleName), btn(t("access.revoke.button"), (e) => void revoke(e, gr), { "aria-label": t("access.revoke.label", { role: gr.roleName }) })))) : empty(t("group.noRoles")),
      h("form", { class: "inline-form", on: { submit: (e) => (e.preventDefault(), void after(e.submitter, () => app.api.post("admin/grants", { roleId: e.target.role.value, groupId: id }), t("access.grant.done", { minutes }))) } },
        field("grp-role", t("access.grant.label"), h("select", { name: "role", required: true }, h("option", { value: "" }, t("filter.choose")), roles.items.filter((r) => !granted.has(r.id)).map((r) => h("option", { value: r.id }, r.name)))),
        h("button", { type: "submit", class: "btn" }, t("access.grant.go")))),
    h("form", { class: "card form", "aria-labelledby": "grp-edit", on: { submit: (e) => void save(e) } },
      h("h2", { id: "grp-edit" }, t("group.edit")),
      field("grp-name", t("groups.name"), h("input", { type: "text", name: "name", required: true, maxlength: "100", value: g.name })),
      field("grp-desc", t("groups.description"), h("input", { type: "text", name: "description", maxlength: "500", value: g.description })),
      h("button", { type: "submit", class: "btn btn-primary" }, t("action.save"))),
  ]);
}
