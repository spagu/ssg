// GET /auth/login — the sign-in page. A leading underscore keeps this file
// out of the Pages route table; functions/auth/login.ts serves it.
//
// The page renders the sign-in methods the site allows (signInMethods,
// _signin.ts): the password form, POSTed to /api/auth/login, which answers
// 303 to `next` (accepted only as a path on this host, _next.ts) or back here
// with ?error=<code>; and one button per enabled login provider (GO-151,
// _provider-pages.ts), each a plain form. "Forgot your password?" is shown
// only when the password is a method and mail works — without mail there is
// no self-service reset at all.
//
// ?reauth=1 (GO-151): a signed-in person sent here to sign in again before
// changing their sign-in methods — the page is shown, not skipped.
//
// Turnstile (GO-151 fix): with AUTH_TURNSTILE_SITE_KEY and the Turnstile
// secret both set, the password form carries Cloudflare's widget, and the
// page's CSP allows exactly its script and frame (_pages-csp.ts). Without
// them the page and its CSP are what they always were.
//
// Cloudflare Access (GO-139): when configured, "Sign in with Cloudflare
// Access" joins the provider buttons — a link to /auth/access (_access-pages.ts).

import { accessErrorText } from "./_access-i18n";
import { accessButton } from "./_access-pages";
import { ACCESS_METHOD } from "./_access-config";
import { whoAmI } from "./_context";
import type { Ctx } from "./_context";
import { STRINGS, fill, langOf } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import { redirect } from "./_lib";
import { mailState } from "./_mail";
import { safeNext } from "./_next";
import { errorText, okText } from "./_pages-errors";
import { TURNSTILE_SCRIPT, pageCsp, turnstileSiteKey } from "./_pages-csp";
import { esc, field, hidden, notice, page, rules } from "./_pages";
import { formOrigins, providerButtons, providersOf } from "./_provider-pages";
import { PASSWORD_METHOD } from "./_provider-policy";
import { signInMethods } from "./_signin";

interface MethodView {
  t: Strings;
  lang: Lang;
  next: string;
  errorRef: string | undefined;
  turnstileKey: string | null;
}

const passwordForm = (v: MethodView): string => `<form class="stack" method="post" action="/api/auth/login">
${hidden("next", v.next)}
${field({ id: "login-email", name: "email", label: v.t.email, type: "email", autocomplete: "username", errorRef: v.errorRef })}
${field({ id: "login-password", name: "password", label: v.t.password, type: "password", autocomplete: "current-password", errorRef: v.errorRef })}
${v.turnstileKey ? `<div class="cf-turnstile" data-sitekey="${esc(v.turnstileKey)}" data-language="${v.lang}" data-theme="auto"></div>` : ""}
<button class="btn btn-primary" type="submit">${esc(v.t.signIn)}</button>
</form>`;

export async function loginPage(ctx: Ctx): Promise<Response> {
  const q = new URL(ctx.request.url).searchParams;
  const lang: Lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const next = safeNext(q.get("next"), "");
  const error = q.get("error");
  const ok = q.get("ok");
  const reauth = q.get("reauth") === "1";
  // Already signed in and nothing to tell: go on. A page that sent the person
  // here after a failure (error), a sign-out (ok) or to sign in again
  // (reauth) is shown as it is.
  if (!error && !ok && !reauth && (await whoAmI(ctx, false))) return redirect(next || "/auth/");

  const site = new URL(ctx.request.url).host;
  const methods = signInMethods(ctx.env);
  const passwordOn = methods.some((m) => m.id === PASSWORD_METHOD);
  const turnstileKey = passwordOn ? turnstileSiteKey(ctx.env) : null;
  const message = error === "turnstile_required" && turnstileKey ? t.errTurnstileShown : accessErrorText(lang, error) || errorText(t, error, rules(ctx));
  const confirmation = okText(t, ok);
  // Without `next` the login goes to "Your panels" — unless the site set
  // AUTH_AFTER_LOGIN_URL, which login.ts uses when `next` is empty.
  const target = next || (ctx.env.AUTH_AFTER_LOGIN_URL ? "" : "/auth/");
  const view: MethodView = { t, lang, next: target, errorRef: error === "invalid_credentials" ? "notice" : undefined, turnstileKey };
  const forgot = passwordOn && mailState(ctx.env).kind === "ready" ? `<p><a href="/auth/forgot">${esc(t.forgotLink)}</a></p>` : "";
  // "Or use an account you already have" only under a password form; alone,
  // the buttons need no heading.
  const heading = passwordOn ? `<h2 id="providers-title" class="or">${esc(t.providersHeading)}</h2>\n` : "";
  const access = methods.some((m) => m.id === ACCESS_METHOD) ? accessButton(lang, target) : "";
  const buttons = providersOf(methods).length ? providerButtons(methods, t, { purpose: "signin", next: target }) : "";
  const providers =
    buttons || access ? `<section class="providers-block"${passwordOn ? ' aria-labelledby="providers-title"' : ""}>${heading}${buttons}${access}</section>` : "";
  const body = [
    message ? notice("error", t.errorHeading, message) : confirmation ? notice("ok", t.doneHeading, confirmation) : reauth ? notice("ok", t.reauthHeading, t.reauthText) : "",
    `<p class="lead">${esc(fill(passwordOn ? t.loginLead : t.loginLeadProviders, { site }))}</p>`,
    passwordOn ? passwordForm(view) : "",
    forgot,
    providers,
    `<p class="muted">${esc(t.inviteOnly)}</p>`,
  ].join("\n");
  const csp = pageCsp({ formAction: formOrigins(methods), turnstile: Boolean(turnstileKey) });
  return page({
    request: ctx.request,
    lang,
    title: t.loginTitle,
    description: fill(t.loginDescription, { site }),
    body,
    error: Boolean(message),
    csp,
    script: turnstileKey ? TURNSTILE_SCRIPT : undefined,
  });
}
