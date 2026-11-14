// One role: permissions picked from the registry, grouped by module with
// their descriptions in the viewer's language; a whole module (module:*) or
// everything (*) behind a warning; names no module declares kept and
// flagged; who holds the role; rename, describe, delete.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { byCodeUnit, modulesOf, prefixesOf, toggle, unknownOf, validPermission, wildcardsIn } from "../models/perms.js";
import { act, btn, chip, confirmDialog, empty, field, loading, pageHead, slot } from "./ui.js";

export default async function role(app, view, route) {
  const { t } = app;
  const id = route.params.id;
  mount(view, loading(app));
  const [d, reg, grants, people, groups] = await Promise.all([
    app.api.get(`admin/roles/${encodeURIComponent(id)}`),
    app.registry(),
    app.api.get("admin/grants", { roleId: id }),
    app.api.get("admin/users", { roleId: id, limit: 200 }),
    app.api.get("admin/groups"),
  ]);
  const r = d.role;
  const modules = modulesOf(reg.items, app.lang);
  let chosen = [...r.permissions].sort(byCodeUnit);
  const warn = slot();
  const extra = slot();
  const pick = slot("picker");

  // drawWarnings: what the current choice means, before it is saved. The
  // checkboxes are redrawn only when the list changed from outside them, so a
  // ticked box keeps the keyboard focus.
  function drawWarnings(redraw = false) {
    if (redraw) pick.set(picker());
    const wild = wildcardsIn(chosen);
    const unknown = unknownOf(chosen, modules);
    warn.set([
      wild.length ? h("div", { class: "callout warning", role: "status" }, h("p", { class: "callout-title" }, t("role.wildcard.title")), h("p", null, t(wild.includes("*") ? "role.wildcard.all" : "role.wildcard.module", { perms: wild.join(", ") }))) : null,
      unknown.length ? h("div", { class: "callout info" }, h("p", null, t("role.unknown", { perms: unknown.join(", ") }))) : null,
    ]);
    extra.set(unknown.length ? h("ul", { class: "plain rows" }, unknown.map((p) => h("li", null, h("code", null, p), chip(t("perm.unknown"), "warning"), btn(t("role.removePerm"), () => ((chosen = toggle(chosen, p, false)), drawWarnings(true)), { "aria-label": t("role.removePermLabel", { perm: p }) })))) : null);
  }
  const box = (name, label, description) =>
    h("label", { class: "check perm" }, h("input", { type: "checkbox", name: "perm", value: name, checked: chosen.includes(name), on: { change: (e) => ((chosen = toggle(chosen, name, e.target.checked)), drawWarnings()) } }), " ", h("span", null, h("code", null, label), description ? h("span", { class: "sub" }, description) : null));
  const picker = () => [
    h("fieldset", { class: "checks module" }, h("legend", null, t("role.everything")), box("*", "*", t("role.everythingHint"))),
    modules.map((m) => h("fieldset", { class: "checks module" }, h("legend", null, m.id), prefixesOf(m).map((w) => box(`${w}:*`, `${w}:*`, t("role.moduleAll", { module: m.id }))), m.permissions.map((p) => box(p.name, p.name, p.description)))),
  ];
  function addCustom(e) {
    e.preventDefault();
    const name = e.target.custom.value.trim();
    if (!validPermission(name)) return void app.notify(t("role.customInvalid", { perm: name }), "critical");
    chosen = toggle(chosen, name, true);
    e.target.reset();
    drawWarnings(true);
  }
  async function save(e) {
    e.preventDefault();
    const body = { description: e.target.description.value.trim(), permissions: chosen, ...(r.builtIn ? {} : { name: e.target.name.value.trim() }) };
    if ((await act(app, e.submitter, () => app.api.put(`admin/roles/${id}`, body))) !== undefined) {
      app.notify(t("role.saved", { minutes: app.me.tokenMinutes }));
      app.go(href("role", { id }));
    }
  }
  async function remove(e) {
    if (!(await confirmDialog(app, { title: t("role.delete.title", { role: r.name }), body: t("role.delete.body", { count: people.items.length }), confirm: t("role.delete.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/roles/${id}`))) !== undefined) {
      app.notify(t("role.delete.done", { role: r.name }));
      app.go(href("roles"));
    }
  }
  // Who holds it: the groups it is granted to, and every person it reaches
  // (directly or through one of those groups).
  const groupName = new Map(groups.items.map((g) => [g.id, g.name]));
  const viaGroups = grants.items.filter((g) => g.groupId);
  const who = [
    ...viaGroups.map((g) => h("li", null, t("role.holderGroup"), " ", h("a", { href: href("group", { id: g.groupId }) }, groupName.get(g.groupId) ?? g.groupId))),
    ...people.items.map((u) => h("li", null, h("a", { href: href("person", { id: u.id }) }, u.name || u.email), u.name ? h("span", { class: "sub" }, u.email) : null)),
  ];
  mount(view, [
    pageHead(app, r.name, { back: { href: href("roles"), label: t("nav.roles") }, lead: r.description || null, actions: r.name === "owner" ? null : btn(t("role.delete.button"), (e) => void remove(e), { variant: "danger" }) }),
    r.builtIn ? h("p", null, chip(t("roles.builtIn"), "info"), " ", t("role.builtInHint")) : null,
    h(
      "form",
      { class: "form", id: "role-form", on: { submit: (e) => void save(e) } },
      r.builtIn ? null : field("role-name", t("roles.name"), h("input", { type: "text", name: "name", value: r.name, required: true, pattern: "[a-z][a-z0-9-]*", maxlength: "64" }), t("roles.nameHint")),
      field("role-desc", t("roles.description"), h("input", { type: "text", name: "description", value: r.description, maxlength: "500" })),
      h("h2", null, t("role.perms")),
      h("p", { class: "hint" }, t("role.permsLead")),
      warn.node,
      pick.node,
      extra.node,
    ),
    // A name no module declares is added by its own small form; Save (tied to
    // the role's form with form=) comes after it, so it saves everything above.
    h("form", { class: "inline-form", on: { submit: (e) => addCustom(e) } }, field("role-custom", t("role.custom"), h("input", { type: "text", name: "custom", autocomplete: "off", spellcheck: "false" }), t("role.customHint")), h("button", { type: "submit", class: "btn" }, t("role.customAdd"))),
    h("p", null, h("button", { type: "submit", form: "role-form", class: "btn btn-primary" }, t("action.save"))),
    h("section", { class: "block", "aria-labelledby": "role-who" }, h("h2", { id: "role-who" }, t("role.who")), who.length ? h("ul", { class: "plain" }, who) : empty(t("role.nobody")), people.nextCursor ? h("p", null, h("a", { href: href("people", {}, { roleId: id }) }, t("role.whoAll"))) : null),
  ]);
  drawWarnings(true);
}
