// Status: how this worker is set up — signing keys and their age against the
// rotation advice, token lifetime and how long a permission change can take,
// password hashing, sign-in methods, the second factor and its policy, mail,
// the audit log's retention and the modules the registry knows. Never a
// secret's value.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { keyAdvice, staleness, warnings } from "../models/status.js";
import { chip, loading, pageHead, tableRegion } from "./ui.js";

const TONE = { ok: "ok", rotate: "warning", unknown: "info" };

export default async function status(app, view) {
  const { t, fmt } = app;
  mount(view, loading(app));
  const s = await app.api.get("admin/status");
  const facts = (pairs) => h("dl", { class: "facts" }, pairs.map(([k, v]) => h("div", null, h("dt", null, k), h("dd", null, v))));
  const section = (id, title, ...body) => h("section", { class: "block", "aria-labelledby": id }, h("h2", { id }, title), body);
  const stale = staleness(s.tokens.stalenessSeconds);
  const keys = s.keys.map((k) =>
    h("tr", null, h("th", { scope: "row" }, h("code", null, k.kid)), h("td", null, t(k.current ? "status.key.current" : "status.key.previous")), h("td", null, k.firstUsedAt ? fmt.date(k.firstUsedAt) : "—"), h("td", null, k.ageDays === null ? "—" : t("status.key.age", { count: k.ageDays })), h("td", null, chip(t(`status.key.${keyAdvice(k)}`), TONE[keyAdvice(k)]))),
  );
  const warn = warnings(s);
  mount(view, [
    pageHead(app, t("nav.status"), { lead: t("status.lead") }),
    warn.length ? h("div", { class: "callout warning", role: "status" }, h("p", { class: "callout-title" }, t("status.attention")), h("ul", null, warn.map((w) => h("li", null, t(`status.warn.${w}`, { count: s.mfa.accountsMissingFactor, days: s.rotationDays }))))) : h("div", { class: "callout ok" }, h("p", null, t("status.allGood"))),
    section("st-keys", t("status.keys"), h("p", { class: "hint" }, t("status.keysLead", { days: s.rotationDays })), tableRegion(app, t("status.keys"), [t("status.col.kid"), t("status.col.role"), t("status.col.first"), t("status.col.age"), t("status.col.advice")], keys)),
    section("st-tokens", t("status.tokens"), facts([
      [t("status.tokenLife"), t("status.minutes", { count: s.tokens.accessMinutes })],
      [t("status.staleness"), t("status.minSec", { minutes: stale.minutes, seconds: stale.seconds })],
      [t("status.sessionLife"), t("status.days", { count: s.tokens.sessionDays })],
      [t("status.serviceLife"), t("status.minutes", { count: s.tokens.serviceAccessMinutes })],
    ]), h("p", { class: "hint" }, t("status.stalenessHint"))),
    section("st-hash", t("status.hashing"), facts([
      [t("status.scheme"), s.hashing.scheme],
      [t("status.iterations"), fmt.number(s.hashing.iterations)],
      [t("status.passwordLength"), t("status.range", { min: s.hashing.passwordMin, max: s.hashing.passwordMax })],
    ])),
    section("st-signin", t("status.signIn"), facts([
      [t("status.methods"), fmt.list(s.signIn.methods.map((m) => t(`provider.${m}`)))],
      [t("status.access"), t(`status.access.${s.access}`)],
      [t("status.mfa"), t(`status.mfa.${s.mfa.state}`)],
      [t("status.mfaPolicy"), t(`status.policy.${s.mfa.policy}`)],
      [t("status.mfaRoles"), s.mfa.rolesNeedingFactor.join(", ") || "—"],
      [t("status.stepUp"), t("status.minutes", { count: s.mfa.stepUpMinutes })],
    ])),
    section("st-mail", t("status.mail"), facts([
      [t("status.mailState"), s.mail.state === "ready" ? t("status.mail.ready", { transport: s.mail.transport }) : t(`status.mail.${s.mail.state}`)],
      ...(s.mail.problem ? [[t("status.problem"), s.mail.problem]] : []),
      [t("status.pending"), h("a", { href: href("invitations") }, fmt.number(s.invitations.pending))],
      [t("status.retention"), t("status.days", { count: s.audit.retentionDays })],
    ])),
    section("st-modules", t("status.modules"), h("p", { class: "hint" }, t("status.modulesLead")), h("ul", { class: "plain" }, s.modules.map((m) => h("li", null, t("status.module", { id: m.id, count: m.permissions })))), s.registryProblems.length ? h("ul", { class: "problems" }, s.registryProblems.map((p) => h("li", null, p))) : null),
  ]);
}
