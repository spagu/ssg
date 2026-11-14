// The read-mostly sections of a person's page: effective permissions with
// where each comes from, sign-in methods, second factor, sessions, events.

import { href } from "../core/router.js";
import { h } from "../core/vnode.js";
import { eventLine } from "./audit-format.js";
import { act, btn, chip, confirmDialog, empty, tableRegion } from "./ui.js";

const section = (title, id, ...body) => h("section", { class: "block", "aria-labelledby": id }, h("h2", { id }, title), body);

// sourceText: "role X, given directly" / "role X, through group Y".
export const sourceText = (app, s) => (s.via === "group" ? app.t("perm.viaGroup", { role: s.roleName, group: s.groupName }) : app.t("perm.direct", { role: s.roleName }));

export function permissionsSection(app, d) {
  const { t } = app;
  if (!d.permissions.length) return section(t("person.perms.title"), "sec-perms", empty(t("person.perms.none")));
  const rows = d.permissions.map((p) =>
    h("tr", null, h("th", { scope: "row" }, h("code", null, p.name), p.known ? null : chip(t("perm.unknown"), "warning")), h("td", null, h("ul", { class: "plain" }, p.sources.map((s) => h("li", null, sourceText(app, s)))))),
  );
  return section(t("person.perms.title"), "sec-perms", h("p", { class: "hint" }, t("person.perms.lead", { minutes: app.me.tokenMinutes })), tableRegion(app, t("person.perms.title"), [t("perm.col.name"), t("perm.col.from")], rows));
}

export function methodsSection(app, d) {
  const { t, fmt } = app;
  const items = [
    h("li", null, d.methods.password ? t("person.methods.password") : t("person.methods.noPassword")),
    ...d.methods.identities.map((i) => h("li", null, t("person.methods.identity", { provider: t(`provider.${i.provider}`), email: i.email || "—", linked: fmt.date(i.linkedAt), used: i.lastUsedAt ? fmt.relative(i.lastUsedAt) : t("people.never") }))),
  ];
  return section(t("person.methods.title"), "sec-methods", h("ul", { class: "plain" }, items));
}

export function factorSection(app, d) {
  const { t, fmt } = app;
  const f = d.secondFactor;
  if (!f.enabled) return section(t("person.factor.title"), "sec-factor", h("p", null, t("person.factor.off")));
  if (!f.factors.length) return section(t("person.factor.title"), "sec-factor", h("p", null, t("person.factor.none")));
  const rows = f.factors.map((x) => h("tr", null, h("th", { scope: "row" }, x.name || t(`factor.${x.kind}`)), h("td", null, t(`factor.${x.kind}`)), h("td", null, fmt.date(x.createdAt)), h("td", null, x.lastUsedAt ? fmt.relative(x.lastUsedAt) : t("people.never"))));
  return section(
    t("person.factor.title"),
    "sec-factor",
    tableRegion(app, t("person.factor.title"), [t("factor.col.name"), t("factor.col.kind"), t("factor.col.added"), t("factor.col.used")], rows),
    h("p", null, t("person.factor.backup", { count: f.backupCodesLeft })),
  );
}

export function sessionsSection(app, d, reload) {
  const { t, fmt } = app;
  if (!d.sessions.length) return section(t("person.sessions.title"), "sec-sessions", empty(t("person.sessions.none")));
  const end = async (e, s) => {
    if (!(await confirmDialog(app, { title: t("session.end.title", { email: d.user.email }), body: t("session.end.body", { started: fmt.dateTime(s.createdAt), minutes: app.me.tokenMinutes }), confirm: t("session.end.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/sessions/${s.id}`))) !== undefined) {
      app.notify(t("session.end.done"));
      await reload();
    }
  };
  const rows = d.sessions.map((s) =>
    h(
      "tr",
      null,
      h("th", { scope: "row" }, s.method ? t(`provider.${s.method}`) : t("session.method.unknown")),
      h("td", null, s.secondFactor ? t(`factor.${s.secondFactor}`) : t("session.factor.no")),
      h("td", null, fmt.dateTime(s.createdAt)),
      h("td", null, fmt.relative(s.lastUsedAt)),
      h("td", null, btn(t("session.end.button"), (e) => void end(e, s), { "aria-label": t("session.end.label", { started: fmt.dateTime(s.createdAt) }) })),
    ),
  );
  return section(t("person.sessions.title"), "sec-sessions", tableRegion(app, t("person.sessions.title"), [t("session.col.method"), t("session.col.factor"), t("session.col.created"), t("session.col.used"), t("session.col.action")], rows));
}

export function eventsSection(app, d) {
  const { t } = app;
  const more = h("p", null, h("a", { href: href("audit", {}, { user: d.user.id }) }, t("person.events.all")));
  if (!d.events.length) return section(t("person.events.title"), "sec-events", empty(t("person.events.none")), more);
  return section(t("person.events.title"), "sec-events", h("ul", { class: "events" }, d.events.map((e) => h("li", null, eventLine(app, e)))), more);
}
