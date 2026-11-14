// One service account: on or off, the roles granted to it, what its tokens
// can reach, its tokens (expiry, last use, revoke) and a new token — whose
// secret is shown once. Only an owner (`*`) may let a token carry sensitive
// permissions.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { allows } from "../models/access.js";
import { act, btn, chip, confirmDialog, copyButton, empty, field, loading, pageHead, slot, tableRegion } from "./ui.js";

const TONE = { active: "ok", expired: "warning", revoked: "critical" };
const names = (raw) => String(raw ?? "").split(/[\s,]+/).filter(Boolean);

export default async function service(app, view, route) {
  const { t, fmt } = app;
  const id = route.params.id;
  mount(view, loading(app));
  const [d, roles, grants] = await Promise.all([app.api.get(`admin/service-accounts/${encodeURIComponent(id)}`), app.api.get("admin/roles"), app.api.get("admin/grants", { userId: id })]);
  const a = d.serviceAccount;
  const once = slot();
  const reload = () => app.go(href("service", { id }));
  const after = async (button, fn, done) => {
    if ((await act(app, button, fn)) !== undefined) {
      app.notify(done);
      await reload();
    }
  };
  const toggle = (e) => after(e.currentTarget, () => app.api.put(`admin/service-accounts/${id}`, { status: a.status === "active" ? "disabled" : "active" }), t(a.status === "active" ? "service.disabled" : "service.enabled", { name: a.name }));
  async function remove(e) {
    if (!(await confirmDialog(app, { title: t("service.delete.title", { name: a.name }), body: t("service.delete.body", { count: d.tokens.length }), confirm: t("service.delete.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/service-accounts/${id}`))) !== undefined) {
      app.notify(t("service.delete.done", { name: a.name }));
      app.go(href("services"));
    }
  }
  async function revoke(e, tok) {
    if (await confirmDialog(app, { title: t("token.revoke.title", { token: tok.name || tok.display }), body: t("token.revoke.body", { minutes: app.me.tokenMinutes }), confirm: t("token.revoke.go") })) {
      await after(e.currentTarget, () => app.api.del(`admin/service-accounts/${id}/tokens/${tok.id}`), t("token.revoke.done"));
    }
  }
  async function create(e) {
    e.preventDefault();
    const f = new FormData(e.target);
    const list = names(f.get("permissions"));
    const sensitive = names(f.get("allowSensitive"));
    const body = { name: String(f.get("name")).trim(), expiresInDays: Number(f.get("days")), ...(list.length ? { permissions: list } : {}), ...(sensitive.length ? { allowSensitive: sensitive } : {}) };
    const r = await act(app, e.submitter, () => app.api.post(`admin/service-accounts/${id}/tokens`, body));
    if (!r) return;
    e.target.reset();
    once.set(h("div", { class: "callout warning once", role: "alert" }, h("p", { class: "callout-title" }, t("once.title")), h("p", null, t("once.token")), h("label", { for: "once-secret" }, t("once.secret")), h("input", { id: "once-secret", type: "text", readonly: true, value: r.secret, class: "mono" }), copyButton(app, r.secret, t("action.copy"))));
    document.getElementById("once-secret")?.focus();
  }
  const tokenRow = (k) =>
    h("tr", null, h("th", { scope: "row" }, k.name || "—", h("span", { class: "sub mono" }, k.display)), h("td", null, chip(t(`token.status.${k.status}`), TONE[k.status])), h("td", null, k.permissions ? k.permissions.join(", ") : t("token.allOfAccount")), h("td", null, fmt.date(k.expiresAt)), h("td", null, k.lastUsedAt ? fmt.relative(k.lastUsedAt) : t("people.never")), h("td", null, k.status === "active" ? btn(t("token.revoke.button"), (e) => void revoke(e, k), { "aria-label": t("token.revoke.label", { token: k.name || k.display }) }) : null));
  mount(view, [
    pageHead(app, a.name, { back: { href: href("services"), label: t("nav.services") }, lead: a.description || null, actions: [btn(t(a.status === "active" ? "service.disable" : "service.enable"), (e) => void toggle(e)), btn(t("service.delete.button"), (e) => void remove(e), { variant: "danger" })] }),
    h("p", { class: "chips" }, chip(t(`service.status.${a.status}`), a.status === "active" ? "ok" : "critical")),
    h("section", { class: "block", "aria-labelledby": "sa-roles" },
      h("h2", { id: "sa-roles" }, t("access.roles")),
      grants.items.length ? h("ul", { class: "plain rows" }, grants.items.map((g) => h("li", null, h("a", { href: href("role", { id: g.roleId }) }, g.roleName), btn(t("access.revoke.button"), (e) => void after(e.currentTarget, () => app.api.del(`admin/grants/${g.id}`), t("access.revoke.done", { role: g.roleName })), { "aria-label": t("access.revoke.label", { role: g.roleName }) })))) : empty(t("access.roles.none")),
      h("form", { class: "inline-form", on: { submit: (e) => (e.preventDefault(), void after(e.submitter, () => app.api.post("admin/grants", { roleId: e.target.role.value, userId: id }), t("service.granted"))) } },
        field("sa-role", t("access.grant.label"), h("select", { name: "role", required: true }, h("option", { value: "" }, t("filter.choose")), roles.items.map((r) => h("option", { value: r.id }, r.name)))),
        h("button", { type: "submit", class: "btn" }, t("access.grant.go"))),
      h("p", { class: "hint" }, t("service.reach", { perms: d.permissions.map((p) => p.name).join(", ") || "—" }))),
    h("section", { class: "block", "aria-labelledby": "sa-tokens" },
      h("h2", { id: "sa-tokens" }, t("service.tokens")),
      once.node,
      d.tokens.length ? tableRegion(app, t("service.tokens"), [t("token.col.name"), t("token.col.status"), t("token.col.perms"), t("token.col.expires"), t("token.col.used"), t("session.col.action")], d.tokens.map(tokenRow)) : empty(t("service.noTokens")),
      h("form", { class: "card form", "aria-labelledby": "sa-new-token", on: { submit: (e) => void create(e) } },
        h("h3", { id: "sa-new-token" }, t("token.new")),
        field("tk-name", t("token.name"), h("input", { type: "text", name: "name", maxlength: "100", autocomplete: "off" })),
        field("tk-days", t("token.days"), h("input", { type: "number", name: "days", min: "1", max: "3650", value: "90", required: true, inputmode: "numeric" }), t("token.daysHint")),
        field("tk-perms", t("token.perms"), h("textarea", { name: "permissions", rows: "2", spellcheck: "false" }), t("token.permsHint")),
        allows(app.me, "allowSensitive") ? field("tk-sensitive", t("token.sensitive"), h("textarea", { name: "allowSensitive", rows: "2", spellcheck: "false" }), t("token.sensitiveHint")) : null,
        h("button", { type: "submit", class: "btn btn-primary" }, t("token.create")))),
  ]);
}
