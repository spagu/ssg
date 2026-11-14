// One person: the account, what it may do and why, how it signs in, its
// second factor, sessions and recent events — and every action on it, each
// confirmed with the object and the consequence named.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { allows } from "../models/access.js";
import { accessSection } from "./person-access.js";
import { eventsSection, factorSection, methodsSection, permissionsSection, sessionsSection } from "./person-parts.js";
import { act, btn, chip, confirmDialog, copyButton, loading, pageHead } from "./ui.js";

export default async function person(app, view, route) {
  const { t, fmt } = app;
  const id = route.params.id;
  mount(view, loading(app));
  const d = await app.api.get(`admin/users/${encodeURIComponent(id)}`);
  const u = d.user;
  const who = u.email;
  const reload = () => app.go(href("person", { id }));
  const self = u.id === app.me.user?.id;

  async function confirmThen(button, dialog, fn, done) {
    if (!(await confirmDialog(app, dialog))) return;
    const r = await act(app, button, fn);
    if (r !== undefined) {
      app.notify(done(r));
      await reload();
    }
  }

  const block = (e) =>
    u.status === "active"
      ? confirmThen(e.currentTarget, { title: t("person.block.title", { email: who }), body: t("person.block.body", { count: d.sessions.length, minutes: app.me.tokenMinutes }), confirm: t("person.block.go") }, () => app.api.post(`admin/users/${id}/status`, { status: "blocked" }), () => t("person.block.done", { email: who }))
      : confirmThen(e.currentTarget, { title: t("person.unblock.title", { email: who }), body: t("person.unblock.body"), confirm: t("person.unblock.go"), danger: false }, () => app.api.post(`admin/users/${id}/status`, { status: "active" }), () => t("person.unblock.done", { email: who }));
  const signOut = (e) =>
    confirmThen(e.currentTarget, { title: t("person.revoke.title", { email: who }), body: t("person.revoke.body", { minutes: app.me.tokenMinutes }), confirm: t("person.revoke.go") }, () => app.api.post(`admin/users/${id}/revoke-sessions`), (r) => t("person.revoke.done", { count: r.revoked }));
  const resetFactor = (e) =>
    confirmThen(e.currentTarget, { title: t("person.mfaReset.title", { email: who }), body: t("person.mfaReset.body"), confirm: t("person.mfaReset.go") }, () => app.api.post("mfa/reset", { userId: id }), () => t("person.mfaReset.done", { email: who }));
  const remove = async (e) => {
    if (!(await confirmDialog(app, { title: t("person.delete.title", { email: who }), body: t("person.delete.body"), confirm: t("person.delete.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/users/${id}`))) !== undefined) {
      app.notify(t("person.delete.done", { email: who }));
      app.go(href("people"));
    }
  };
  const resetLink = async (e) => {
    const byLink = app.me.mail !== "ready";
    const ok = await confirmDialog(app, { title: t("person.resetLink.title", { email: who }), body: t(byLink ? "person.resetLink.bodyLink" : "person.resetLink.bodyMail"), confirm: t("person.resetLink.go"), danger: false });
    if (!ok) return;
    const r = await act(app, e.currentTarget, () => app.api.post(`admin/users/${id}/reset-link`, byLink ? { delivery: "link" } : {}));
    if (!r) return;
    if (r.link) oneTime.set(r.link);
    else app.notify(t("person.resetLink.mailed", { email: who }));
  };

  // A link shown once, with a copy button and the warning that it will not be shown again.
  const oneTime = {
    el: null,
    set(link) {
      mount(this.el, h("div", { class: "callout warning once", role: "alert" }, h("p", { class: "callout-title" }, t("once.title")), h("p", null, t("once.reset", { email: who })), h("label", { for: "once-link" }, t("once.label")), h("input", { id: "once-link", type: "text", readonly: true, value: link, class: "mono" }), copyButton(app, link, t("action.copy"))));
    },
  };

  const actions = self
    ? [h("p", { class: "hint" }, t("person.self"))]
    : [
        btn(u.status === "active" ? t("person.block.button") : t("person.unblock.button"), (e) => void block(e), { variant: u.status === "active" ? "" : "primary" }),
        btn(t("person.resetLink.button"), (e) => void resetLink(e)),
        btn(t("person.revoke.button"), (e) => void signOut(e), { disabled: d.sessions.length === 0 }),
        app.me.mfa !== "off" && d.secondFactor.factors.length && allows(app.me, "resetFactor") ? btn(t("person.mfaReset.button"), (e) => void resetFactor(e)) : null,
        btn(t("person.delete.button"), (e) => void remove(e), { variant: "danger" }),
      ];

  mount(view, [
    pageHead(app, u.name || u.email, { back: { href: href("people"), label: t("nav.people") }, lead: u.name ? u.email : null }),
    h("p", { class: "chips" }, chip(t(`status.${u.status}`), u.status === "blocked" ? "critical" : "ok"), u.owner ? chip(t("people.owner"), "info") : null, u.mustChangePassword ? chip(t("person.mustChange"), "warning") : null),
    h("div", { class: "toolbar", role: "group", "aria-label": t("person.actions") }, actions),
    h("div", { ref: (el) => (oneTime.el = el) }),
    h("dl", { class: "facts" }, [
      [t("person.created"), fmt.dateTime(u.createdAt)],
      [t("person.lastLogin"), u.lastLoginAt ? fmt.dateTime(u.lastLoginAt) : t("people.never")],
      [t("person.updated"), fmt.dateTime(u.updatedAt)],
    ].map(([k, v]) => h("div", null, h("dt", null, k), h("dd", null, v)))),
    permissionsSection(app, d),
    await accessSection(app, d, reload),
    methodsSection(app, d),
    factorSection(app, d),
    sessionsSection(app, d, reload),
    eventsSection(app, d),
  ]);
}
