// Sessions: every live session — who, how they signed in, whether a second
// factor was passed, when — with End for one, All of a person, and the
// break-glass button that signs everyone out.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { act, btn, confirmDialog, empty, loading, pageHead, slot, tableRegion } from "./ui.js";

export default async function sessions(app, view, route) {
  const { t, fmt } = app;
  const userId = route.query.userId ?? "";
  const minutes = app.me.tokenMinutes;
  mount(view, loading(app));
  const list = slot();
  const items = [];
  let cursor = "";
  const reload = () => app.go(href("sessions", {}, { userId }));

  async function end(e, s) {
    if (!(await confirmDialog(app, { title: t("session.end.title", { email: s.email }), body: t("session.end.body", { started: fmt.dateTime(s.createdAt), minutes }), confirm: t("session.end.go") }))) return;
    const r = await act(app, e.currentTarget, () => app.api.del(`admin/sessions/${s.id}`));
    if (!r) return;
    if (r.current) return void location.assign(app.loginHref());
    app.notify(t("session.end.done"));
    await reload();
  }
  async function everyone(e) {
    const ok = await confirmDialog(app, {
      title: t("sessions.all.title"),
      body: t("sessions.all.body", { minutes }),
      confirm: t("sessions.all.go"),
      extra: h("p", { class: "check" }, h("input", { type: "checkbox", id: "all-ack", required: true }), " ", h("label", { for: "all-ack" }, t("sessions.all.ack"))),
    });
    if (!ok) return;
    const r = await act(app, e.currentTarget, () => app.api.post("admin/sessions/revoke-all", { confirm: "revoke-all" }));
    if (r) {
      app.notify(t("sessions.all.done", { count: r.revoked }));
      await reload();
    }
  }
  const row = (s) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, h("a", { href: href("person", { id: s.userId }) }, s.name || s.email), s.name ? h("span", { class: "sub" }, s.email) : null, s.userId === app.me.user?.id ? h("span", { class: "sub" }, t("sessions.you")) : null),
      h("td", null, s.method ? t(`provider.${s.method}`) : t("session.method.unknown")),
      h("td", null, s.secondFactor ? t("session.factor.yes", { factor: t(`factor.${s.secondFactor}`) }) : t("session.factor.no")),
      h("td", null, fmt.dateTime(s.createdAt)),
      h("td", null, fmt.relative(s.lastUsedAt)),
      h("td", null, btn(t("session.end.button"), (e) => void end(e, s), { "aria-label": t("session.end.labelWho", { email: s.email, started: fmt.dateTime(s.createdAt) }) })),
    );
  function draw() {
    list.set([
      items.length ? tableRegion(app, t("sessions.caption", { count: items.length }), [t("people.col.person"), t("session.col.method"), t("session.col.factor"), t("session.col.created"), t("session.col.used"), t("session.col.action")], items.map(row)) : empty(t("sessions.none")),
      cursor ? btn(t("action.more"), (e) => void act(app, e.currentTarget, more)) : null,
    ]);
  }
  async function more() {
    const page = await app.api.get("admin/sessions", { userId, cursor });
    items.push(...page.items);
    cursor = page.nextCursor ?? "";
    draw();
  }
  mount(view, [
    pageHead(app, t("nav.sessions"), { lead: t("sessions.lead", { minutes }) }),
    userId ? h("p", null, t("sessions.filtered"), " ", h("a", { href: href("sessions") }, t("action.clear"))) : null,
    list.node,
    h("section", { class: "block danger-zone", "aria-labelledby": "glass" }, h("h2", { id: "glass" }, t("sessions.all.heading")), h("p", null, t("sessions.all.lead")), btn(t("sessions.all.button"), (e) => void everyone(e), { variant: "danger" })),
  ]);
  await more();
}
