// What a person holds and how to change it: groups (join, leave) and roles
// granted directly (grant, take away). Each change says when it takes effect
// — at the account's next token refresh.

import { href } from "../core/router.js";
import { h } from "../core/vnode.js";
import { act, btn, confirmDialog, empty, field } from "./ui.js";

export async function accessSection(app, d, reload) {
  const { t } = app;
  const [roles, groups] = await Promise.all([app.api.get("admin/roles"), app.api.get("admin/groups")]);
  const who = d.user.email;
  const minutes = app.me.tokenMinutes;
  const after = async (button, fn, done) => {
    if ((await act(app, button, fn)) !== undefined) {
      app.notify(done);
      await reload();
    }
  };
  const leave = async (e, g) => {
    if (await confirmDialog(app, { title: t("access.leave.title", { email: who, group: g.name }), body: t("access.leave.body", { minutes }), confirm: t("access.leave.go") })) {
      await after(e.currentTarget, () => app.api.del(`admin/groups/${g.id}/members/${d.user.id}`), t("access.leave.done", { group: g.name }));
    }
  };
  const revoke = async (e, g) => {
    if (await confirmDialog(app, { title: t("access.revoke.title", { email: who, role: g.roleName }), body: t("access.revoke.body", { minutes }), confirm: t("access.revoke.go") })) {
      await after(e.currentTarget, () => app.api.del(`admin/grants/${g.id}`), t("access.revoke.done", { role: g.roleName }));
    }
  };
  const inGroups = new Set(d.groups.map((g) => g.id));
  const granted = new Set(d.grants.map((g) => g.roleId));
  const groupForm = h(
    "form",
    { class: "inline-form", on: { submit: (e) => (e.preventDefault(), void after(e.submitter, () => app.api.post(`admin/groups/${e.target.group.value}/members`, { userId: d.user.id }), t("access.join.done", { minutes }))) } },
    field("join-group", t("access.join.label"), h("select", { name: "group", required: true }, h("option", { value: "" }, t("filter.choose")), groups.items.filter((g) => !inGroups.has(g.id)).map((g) => h("option", { value: g.id }, g.name)))),
    h("button", { type: "submit", class: "btn" }, t("access.join.go")),
  );
  const roleForm = h(
    "form",
    { class: "inline-form", on: { submit: (e) => (e.preventDefault(), void after(e.submitter, () => app.api.post("admin/grants", { roleId: e.target.role.value, userId: d.user.id }), t("access.grant.done", { minutes }))) } },
    field("grant-role", t("access.grant.label"), h("select", { name: "role", required: true }, h("option", { value: "" }, t("filter.choose")), roles.items.filter((r) => !granted.has(r.id)).map((r) => h("option", { value: r.id }, r.name)))),
    h("button", { type: "submit", class: "btn" }, t("access.grant.go")),
  );
  return h(
    "section",
    { class: "block", "aria-labelledby": "sec-access" },
    h("h2", { id: "sec-access" }, t("access.title")),
    h("h3", null, t("access.groups")),
    d.groups.length
      ? h("ul", { class: "plain rows" }, d.groups.map((g) => h("li", null, h("a", { href: href("group", { id: g.id }) }, g.name), btn(t("access.leave.button"), (e) => void leave(e, g), { "aria-label": t("access.leave.label", { group: g.name }) }))))
      : empty(t("access.groups.none")),
    groupForm,
    h("h3", null, t("access.roles")),
    d.grants.length
      ? h("ul", { class: "plain rows" }, d.grants.map((g) => h("li", null, h("a", { href: href("role", { id: g.roleId }) }, g.roleName), btn(t("access.revoke.button"), (e) => void revoke(e, g), { "aria-label": t("access.revoke.label", { role: g.roleName }) }))))
      : empty(t("access.roles.none")),
    roleForm,
  );
}
