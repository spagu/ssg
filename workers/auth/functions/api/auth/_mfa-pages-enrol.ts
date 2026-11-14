// GET /auth/mfa/enrol — set up a second factor (GO-140). A leading
// underscore keeps this file out of the Pages route table;
// functions/auth/mfa/enrol.ts serves it.
//
// Without ?factor: what a second factor is, which roles need one (?required=1
// after a sign-in that left them out), "Set up an authenticator app" (a plain
// form to /api/auth/mfa/totp/start) and "Add a passkey" (a form auth.js
// completes through WebAuthn). With
// ?factor=<id>, the account's own fresh enrolment: the QR code — drawn here
// (_mfa-qr.ts), the secret never leaves this worker for an outside service —
// the same key as text to type, and the form for the first code.
//
// Same freshness as the routes behind it (_mfa-gate.ts): an account with a
// factor is sent to the step-up page first, one without — when its sign-in is
// older than AUTH_RECENT_SIGNIN_MINUTES — to the login page (?reauth=1).

import type { Ctx } from "./_context";
import { STRINGS, fill, langOf } from "./_i18n";
import type { Strings } from "./_i18n";
import { redirect } from "./_lib";
import { mfaEnabled, open } from "./_mfa-crypto";
import { pendingTotp } from "./_mfa-factors";
import { codeField, rolesText } from "./_mfa-pages-parts";
import { qrSvg } from "./_mfa-qr";
import { freshness, stepUpRedirect } from "./_mfa-step-up";
import { safeNext, withParams } from "./_next";
import { errorText } from "./_pages-errors";
import { esc, field, hidden, notice, page, rules, signedIn } from "./_pages";
import { policyView, recentSignIn } from "./_provider-policy";
import { base32Encode, groupSecret, otpauthUri } from "./_totp";
import { registerOptions } from "./_webauthn-store";

// passkeyCreate: the passkey form; auth.js calls navigator.credentials.create()
// with `options` (a single-use challenge issued for this page) and puts the
// answer in the hidden `credential` field before sending it. The route
// answers like the app's confirmation: the backup-codes page for a first factor.
const passkeyCreate = (t: Strings, next: string, options: unknown): string => `<section class="card" aria-labelledby="pk-title" data-passkey="create" data-options="${esc(JSON.stringify(options))}" data-working="${esc(t.mfaPasskeyWorking)}" data-failed="${esc(t.errInvalidPasskey)}" hidden>
<h2 id="pk-title">${esc(t.mfaKindPasskey)}</h2><p>${esc(t.mfaPasskeyLead)}</p>
<form class="stack" method="post" action="/api/auth/mfa/passkey/register">${hidden("next", next)}${hidden("credential", "")}
${field({ id: "pk-name", name: "name", label: t.mfaNameLabel, type: "text", autocomplete: "off", hint: t.mfaNameHint, required: false, maxlength: 60 })}
<button class="btn btn-primary" type="submit">${esc(t.mfaPasskeyAdd)}</button></form><p class="hint" data-passkey-status role="status" aria-live="polite"></p></section>`;

export async function enrolPage(ctx: Ctx): Promise<Response> {
  const who = await signedIn(ctx);
  if (who instanceof Response) return who;
  const url = new URL(ctx.request.url);
  const q = url.searchParams;
  if (!mfaEnabled(ctx.env)) return redirect("/auth/account");
  const here = url.pathname + url.search;
  const fresh = await freshness(ctx, who);
  if (fresh === "step_up") return stepUpRedirect(here);
  if (fresh === "no_factor" && !recentSignIn(ctx.env, who.session, ctx.now)) return redirect(withParams(ctx.s.loginUrl, { reauth: "1", next: here }));
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const site = url.host;
  const next = safeNext(q.get("next"), "/auth/account");
  const error = q.get("error");
  const id = q.get("factor");
  const parts: string[] = [error ? notice("error", t.errorHeading, errorText(t, error, rules(ctx))) : ""];
  const row = id ? await pendingTotp(ctx.env, who.user.id, id, ctx.now) : null;
  const opened = row ? await open(ctx.env, row.secret as string, row.id, who.user.id) : null;
  if (row && opened) {
    const secret = base32Encode(opened.plain);
    const svg = qrSvg(otpauthUri(site, who.user.email, secret), t.mfaQrLabel) ?? "";
    parts.push(`<section class="card" aria-labelledby="scan-title"><h2 id="scan-title">${esc(t.mfaScanStep)}</h2><div class="qr-box">${svg}</div>
<p>${esc(t.mfaKeyStep)}</p><p><code class="secret">${esc(groupSecret(secret))}</code></p><p class="hint">${esc(t.mfaKeyHint)}</p></section>
<section class="card" aria-labelledby="confirm-title"><h2 id="confirm-title">${esc(t.mfaConfirmStep)}</h2>
<form class="stack" method="post" action="/api/auth/mfa/totp/confirm">${hidden("factorId", row.id)}${hidden("next", next)}
${codeField({ id: "mfa-code", name: "code", label: t.mfaCodeLabel, hint: t.mfaCodeHint, error: error === "invalid_code" })}
${field({ id: "mfa-name", name: "name", label: t.mfaNameLabel, type: "text", autocomplete: "off", hint: t.mfaNameHint, required: false, maxlength: 60 })}
<button class="btn btn-primary" type="submit">${esc(t.mfaConfirm)}</button></form></section>`);
  } else {
    if (id) parts.push(notice("error", t.errorHeading, row ? t.errMfaUnavailable : t.errEnrolmentExpired));
    const roles = (await policyView(ctx.env, who.user.id, "", false)).needsFactor;
    if (roles.length && fresh === "no_factor") parts.push(`<div class="callout warning" role="status"><p>${esc(fill(t.mfaEnrolRequired, { roles: rolesText(roles) }))}</p></div>`);
    parts.push(
      `<p class="lead">${esc(t.mfaEnrolLead)}</p>`,
      `<section class="card" aria-labelledby="app-title"><h2 id="app-title">${esc(t.mfaAppHeading)}</h2><p>${esc(t.mfaAppLead)}</p>
<form method="post" action="/api/auth/mfa/totp/start">${hidden("next", next)}<button class="btn btn-primary" type="submit">${esc(t.mfaAppStart)}</button></form></section>`,
      passkeyCreate(t, next, await registerOptions(ctx.env, ctx.request, who.user, who.session.id, ctx.now)),
    );
  }
  parts.push(`<p><a href="${esc(next)}">${esc(row && opened ? t.mfaBack : t.mfaSkip)}</a></p>`);
  return page({ request: ctx.request, lang, title: t.mfaEnrolTitle, description: fill(t.mfaEnrolDescription, { site }), body: parts.join("\n"), error: Boolean(error), account: { email: who.user.email } });
}
