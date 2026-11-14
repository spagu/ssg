// The pages a one-time link opens — GET /auth/invite?token=…, GET
// /auth/reset?token=… — and GET /auth/forgot. A leading underscore keeps
// this file out of the Pages route table; functions/auth/*.ts serve them.
//
// Opening a link changes nothing: mail scanners and link previews fetch every
// URL in a message, so the GET only shows a form, and the change happens on
// its POST (/api/auth/invite, /api/auth/reset). The token is in the URL, so
// these pages, like all of them, send no Referer and are never cached.

import type { Ctx } from "./_context";
import { STRINGS, fill, langOf, when } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import { openByToken } from "./_invites";
import { isToken } from "./_lib";
import { mailState } from "./_mail";
import { PASSWORD_ERRORS, errorText } from "./_pages-errors";
import type { Rules } from "./_pages-errors";
import { esc, field, hidden, notice, page, rules, usernameFor } from "./_pages";
import { openReset } from "./_resets";
import { pageCsp } from "./_pages-csp";
import { formOrigins, providerButtons, providersOf } from "./_provider-pages";
import { signInMethods } from "./_signin";
import type { SignInMethod } from "./_signin";
import { findUserById } from "./_users";

interface Ask {
  ctx: Ctx;
  lang: Lang;
  t: Strings;
  site: string;
  error: string | null;
  r: Rules;
}

function ask(ctx: Ctx): Ask {
  const lang = langOf(ctx.request);
  return { ctx, lang, t: STRINGS[lang], site: new URL(ctx.request.url).host, error: new URL(ctx.request.url).searchParams.get("error"), r: rules(ctx) };
}

const back = (t: Strings): string => `<p><a href="/auth/login">${esc(t.backToSignIn)}</a></p>`;

// gone is the answer for a link that does not work: one page for unknown,
// used, withdrawn and expired alike.
const gone = (a: Ask, description: string, text: string, extra = ""): Response =>
  page({ request: a.ctx.request, lang: a.lang, title: a.t.invalidLinkTitle, description, body: `<p>${esc(text)}</p>${extra}${back(a.t)}`, status: 410 });

// The password fields of a form that sets a new password.
const newPassword = (a: Ask, rule: string | undefined): string =>
  field({ id: "new-password", name: "password", label: a.t.newPassword, type: "password", autocomplete: "new-password", hint: fill(a.t.newPasswordHint, { min: a.r.min }), error: rule, minlength: a.r.min, maxlength: a.r.max });

// The blocks the invitee chooses from: the password (when it is a method
// here) and, together, the enabled login providers (GO-151) — each a form that
// carries the invitation token to POST /api/auth/oauth/<id>/start.
const passwordBlock = (a: Ask, token: string, email: string, name: string, rule: string | undefined): string =>
  `<section class="card" aria-labelledby="m-password"><h2 id="m-password">${esc(a.t.methodPassword)}</h2>
<p>${esc(a.t.methodPasswordLead)}</p>
<form class="stack" method="post" action="/api/auth/invite">
${hidden("token", token)}${hidden("method", "password")}${usernameFor(email)}
${field({ id: "invite-name", name: "name", label: a.t.name, type: "text", autocomplete: "name", value: name, hint: a.t.nameHint, required: false, maxlength: 200 })}
${newPassword(a, rule)}
<button class="btn btn-primary" type="submit">${esc(a.t.createAccount)}</button>
</form></section>`;

const providersBlock = (a: Ask, token: string, methods: SignInMethod[]): string =>
  `<section class="card" aria-labelledby="m-providers"><h2 id="m-providers">${esc(a.t.methodProvidersHeading)}</h2>
<p>${esc(a.t.methodProvidersLead)}</p>
${providerButtons(methods, a.t, { purpose: "invite", token })}</section>`;

export async function invitePage(ctx: Ctx): Promise<Response> {
  const a = ask(ctx);
  const token = new URL(ctx.request.url).searchParams.get("token") ?? "";
  const inv = isToken(token) ? await openByToken(ctx.env, token, ctx.now) : null;
  const description = fill(a.t.inviteDescription, { site: a.site });
  if (!inv) return gone(a, description, a.t.invalidInvite);
  const rule = a.error && PASSWORD_ERRORS.has(a.error) ? errorText(a.t, a.error, a.r) : undefined;
  const message = rule ?? errorText(a.t, a.error, a.r);
  const methods = signInMethods(ctx.env);
  const body = [
    message ? notice("error", a.t.errorHeading, message) : "",
    `<p class="lead">${esc(fill(a.t.inviteLead, { site: a.site, email: inv.email }))}</p>`,
    `<p class="muted">${esc(fill(a.t.inviteExpires, { date: when(a.lang, inv.expiresAt) }))}</p>`,
    `<p class="step">${esc(a.t.inviteMethods)}</p>`,
    methods.some((m) => m.id === "password") ? passwordBlock(a, token, inv.email, inv.name, rule) : "",
    providersOf(methods).length ? providersBlock(a, token, methods) : "",
  ].join("\n");
  const csp = pageCsp({ formAction: formOrigins(methods) });
  return page({ request: ctx.request, lang: a.lang, title: a.t.inviteTitle, description, body, error: Boolean(message), csp });
}

export async function resetPage(ctx: Ctx): Promise<Response> {
  const a = ask(ctx);
  const token = new URL(ctx.request.url).searchParams.get("token") ?? "";
  const open = isToken(token) ? await openReset(ctx.env, token, ctx.now) : null;
  const user = open ? await findUserById(ctx.env, open.userId) : null;
  const description = fill(a.t.resetDescription, { site: a.site });
  if (!user) {
    const forgot = mailState(ctx.env).kind === "ready" ? `<p><a href="/auth/forgot">${esc(a.t.forgotLink)}</a></p>` : "";
    return gone(a, description, fill(a.t.invalidReset, { minutes: a.r.minutes }), forgot);
  }
  const rule = a.error && PASSWORD_ERRORS.has(a.error) ? errorText(a.t, a.error, a.r) : undefined;
  const message = rule ?? errorText(a.t, a.error, a.r);
  const body = [
    message ? notice("error", a.t.errorHeading, message) : "",
    `<p class="lead">${esc(fill(a.t.resetLead, { email: user.email }))}</p>`,
    `<form class="stack" method="post" action="/api/auth/reset">
${hidden("token", token)}${usernameFor(user.email)}
${newPassword(a, rule)}
<button class="btn btn-primary" type="submit">${esc(a.t.setPassword)}</button>
</form>`,
  ].join("\n");
  return page({ request: ctx.request, lang: a.lang, title: a.t.resetTitle, description, body, error: Boolean(message) });
}

export async function forgotPage(ctx: Ctx): Promise<Response> {
  const a = ask(ctx);
  const description = fill(a.t.forgotDescription, { site: a.site });
  const state = mailState(ctx.env).kind;
  if (state !== "ready") {
    const body = `<p>${esc(a.t.forgotUnavailable)}</p>${back(a.t)}`;
    return page({ request: ctx.request, lang: a.lang, title: a.t.notAvailableTitle, description, body, status: state === "none" ? 404 : 503 });
  }
  if (new URL(ctx.request.url).searchParams.get("sent") === "1") {
    const body = `${notice("ok", a.t.doneHeading, a.t.forgotSent)}${back(a.t)}`;
    return page({ request: ctx.request, lang: a.lang, title: a.t.forgotTitle, description, body });
  }
  const fieldError = a.error === "invalid_email" ? errorText(a.t, a.error, a.r) : undefined;
  const message = errorText(a.t, a.error, a.r);
  const body = [
    message ? notice("error", a.t.errorHeading, message) : "",
    `<p class="lead">${esc(fill(a.t.forgotLead, { minutes: a.r.minutes }))}</p>`,
    `<form class="stack" method="post" action="/api/auth/forgot">
${field({ id: "forgot-email", name: "email", label: a.t.email, type: "email", autocomplete: "username", error: fieldError })}
<button class="btn btn-primary" type="submit">${esc(a.t.sendLink)}</button>
</form>`,
    back(a.t),
  ].join("\n");
  return page({ request: ctx.request, lang: a.lang, title: a.t.forgotTitle, description, body, error: Boolean(message) });
}
