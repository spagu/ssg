// GET /auth/account — the account page: who you are, how you sign in (GO-151:
// password and linked login providers, add / remove / set a password),
// change your password, where you are signed in. A leading underscore keeps this file out of the
// Pages route table; functions/auth/account.ts serves it.
//
// Every action is a plain form to an existing route: POST /api/auth/password,
// /api/auth/methods/:id, /api/auth/methods/set-password,
// /api/auth/oauth/<provider>/start (purpose link),
// POST /api/auth/sessions/:id (one of my sessions), /api/auth/logout and
// /api/auth/logout-all. They answer 303 back here (or to the login page) with
// ?ok= or ?error=, so the page works with JavaScript off. GO-140 adds the
// second factor's section (_mfa-pages-account.ts) when it is on.

import { accessErrorText } from "./_access-i18n";
import type { Ctx } from "./_context";
import { listOwnSessions } from "./_account";
import { STRINGS, fill, langOf, when } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import { PASSWORD_ERRORS, errorText, okText } from "./_pages-errors";
import { esc, field, hidden, notice, page, rules, signedIn, usernameFor } from "./_pages";
import type { OwnSession } from "./_account";
import { pageCsp } from "./_pages-csp";
import { methodsOf } from "./_provider-account";
import { formOrigins, methodsSection, policyNotice } from "./_provider-pages";
import { passwordEnabled, signInMethods } from "./_signin";
import { factorsSection } from "./_mfa-pages-account";

function sessionRow(s: OwnSession, lang: Lang, t: Strings): string {
  const started = when(lang, s.createdAt);
  const action = `<form method="post" action="/api/auth/sessions/${esc(s.id)}">${hidden("next", "/auth/account")}<button class="btn" type="submit" aria-label="${esc(fill(t.endSessionLabel, { date: started }))}">${esc(t.endSession)}</button></form>`;
  const mark = s.current ? ` <span class="chip chip-current">${esc(t.thisSession)}</span>` : "";
  const cell = (label: string, html: string) => `<td data-label="${esc(label)}">${html}</td>`;
  return `<tr>${cell(t.colStarted, esc(started) + mark)}${cell(t.colLastUsed, esc(when(lang, s.lastUsedAt)))}${cell(t.colEnds, esc(when(lang, s.expiresAt)))}<td>${action}</td></tr>`;
}

export async function accountPage(ctx: Ctx): Promise<Response> {
  const who = await signedIn(ctx);
  if (who instanceof Response) return who;
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const q = new URL(ctx.request.url).searchParams;
  const site = new URL(ctx.request.url).host;
  const error = q.get("error");
  const r = rules(ctx);
  // A wrong current password comes back as invalid_credentials: it belongs to
  // that field, the password rules to the new one, anything else on top.
  const current = error === "invalid_credentials" ? t.errCurrentPassword : undefined;
  const rule = error && PASSWORD_ERRORS.has(error) ? errorText(t, error, r) : undefined;
  const message = current ?? rule ?? (accessErrorText(lang, error) || errorText(t, error, r));
  const ok = okText(t, q.get("ok"));
  const sessions = await listOwnSessions(ctx.env, who.user.id, who.session.id, ctx.now);
  const u = who.user;
  // GO-151: how this account signs in, and what the method policy leaves out.
  const methods = signInMethods(ctx.env);
  const view = await methodsOf(ctx, who);

  const body = [
    message ? notice("error", t.errorHeading, message) : ok ? notice("ok", t.doneHeading, ok) : "",
    await policyNotice(ctx, who, t, view.sessionMethod),
    `<dl class="facts"><div><dt>${esc(t.name)}</dt><dd>${esc(u.display_name || "—")}</dd></div><div><dt>${esc(t.email)}</dt><dd>${esc(u.email)}</dd></div><div><dt>${esc(t.memberSince)}</dt><dd>${esc(when(lang, u.created_at))}</dd></div></dl>`,
    methodsSection(view, who, lang, t, r, methods, passwordEnabled(ctx.env)),
    // GO-140: the second factor, when it is on.
    await factorsSection(ctx, who, lang, t),
    !view.password ? "" : `<section class="card" aria-labelledby="pw-title"><h2 id="pw-title">${esc(t.passwordHeading)}</h2>
<form class="stack" method="post" action="/api/auth/password">
${hidden("next", "/auth/account")}${usernameFor(u.email)}
${field({ id: "pw-current", name: "current", label: t.currentPassword, type: "password", autocomplete: "current-password", error: current })}
${field({ id: "pw-new", name: "password", label: t.newPassword, type: "password", autocomplete: "new-password", hint: fill(t.newPasswordHint, { min: r.min }), error: rule, minlength: r.min, maxlength: r.max })}
<button class="btn btn-primary" type="submit">${esc(t.changePassword)}</button>
</form></section>`,
    `<section class="card" aria-labelledby="sess-title"><h2 id="sess-title">${esc(t.sessionsHeading)}</h2><p>${esc(t.sessionsLead)}</p>
<div class="table-scroll"><table class="data"><thead><tr><th scope="col">${esc(t.colStarted)}</th><th scope="col">${esc(t.colLastUsed)}</th><th scope="col">${esc(t.colEnds)}</th><th scope="col">${esc(t.colAction)}</th></tr></thead>
<tbody>${sessions.map((s) => sessionRow(s, lang, t)).join("")}</tbody></table></div>
<form class="inline-actions" method="post" action="/api/auth/logout-all">${hidden("next", "/auth/login?ok=signed_out_all")}<button class="btn btn-danger" type="submit" aria-describedby="all-hint">${esc(t.signOutEverywhere)}</button></form>
<p class="hint" id="all-hint">${esc(fill(t.signOutEverywhereHint, { minutes: ctx.s.tokenSeconds / 60 }))}</p></section>`,
  ].join("\n");
  const csp = pageCsp({ formAction: formOrigins(methods) });
  return page({ request: ctx.request, lang, title: t.accountTitle, description: fill(t.accountDescription, { site }), body, error: Boolean(error), account: { email: u.email }, csp });
}
