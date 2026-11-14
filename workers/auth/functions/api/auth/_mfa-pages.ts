// GET /auth/mfa (the second step of signing in) and GET /auth/mfa/step-up
// (the second factor again, before a sensitive change) — GO-140. A leading
// underscore keeps this file out of the Pages route table; functions/auth/mfa/
// serves them.
//
// /auth/mfa needs the pending-step cookie (_mfa-pending.ts): without a live
// one the browser goes back to the login page (?error=mfa_expired). It names
// the account the first factor proved — nothing the person did not just type
// or choose — and offers what that account can answer with: the app's code
// (a plain form), a passkey (a form auth.js completes through WebAuthn), a
// backup code, and how to get help.
// ?error= and ?use=backup|recover come back from mfa/verify and break-glass.

import { prepare } from "./_context";
import type { Ctx } from "./_context";
import { STRINGS, fill, langOf } from "./_i18n";
import type { Strings } from "./_i18n";
import { redirect } from "./_lib";
import { listFactors } from "./_mfa-factors";
import { readPending } from "./_mfa-pending";
import { stepUpMinutes } from "./_mfa-policy";
import { backupBlock, codeField, codeForm, passkeyBlock } from "./_mfa-pages-parts";
import { safeNext, withParams } from "./_next";
import { errorText } from "./_pages-errors";
import { esc, notice, page, rules, signedIn } from "./_pages";
import { methodLabel } from "./_provider-pages";
import type { Env } from "./_lib";
import { mfaEnabled } from "./_mfa-crypto";
import { assertOptions } from "./_webauthn-store";

interface Answer {
  t: Strings;
  kinds: Set<string>;
  action: string;
  fields: Record<string, string>;
  error: string | null;
  use: string | null;
  passkey: unknown; // the request options of a challenge issued for this page, or null
}

// answers: the ways to answer, in the order a person most likely has them.
function answers(a: Answer): string[] {
  const backupError = a.use === "backup" && Boolean(a.error);
  return [
    a.kinds.has("totp") ? `<section class="card" aria-labelledby="app-title"><h2 id="app-title">${esc(a.t.mfaAppHeading)}</h2>${codeForm(a.t, a.action, a.fields, Boolean(a.error) && !backupError)}</section>` : "",
    a.passkey ? passkeyBlock(a.t, a.action, a.fields, a.passkey) : "",
    a.kinds.has("passkey") && !a.kinds.has("totp") ? `<noscript><p>${esc(a.t.mfaPasskeyNoScript)}</p></noscript>` : "",
    backupBlock(a.t, a.action, a.fields, a.use === "backup", backupError),
  ];
}

export async function challengePage(ctx: Ctx): Promise<Response> {
  const pending = mfaEnabled(ctx.env) ? await readPending(ctx.env, ctx.request, ctx.now) : null;
  if (!pending) return redirect(withParams(ctx.s.loginUrl, { error: "mfa_expired" }));
  const q = new URL(ctx.request.url).searchParams;
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const site = new URL(ctx.request.url).host;
  const error = q.get("error");
  const use = q.get("use");
  const kinds = new Set((await listFactors(ctx.env, pending.user.id)).map((f) => f.kind as string));
  const recover = ctx.env.AUTH_MFA_RESET_TOKEN
    ? `<p>${esc(t.mfaBreakGlassLead)}</p><form class="stack" method="post" action="/api/auth/mfa/break-glass">
${codeField({ id: "mfa-token", name: "token", label: t.mfaBreakGlassLabel, hint: t.mfaBreakGlassHint, error: use === "recover" && Boolean(error) })}
<button class="btn btn-danger" type="submit">${esc(t.mfaBreakGlassButton)}</button></form>`
    : "";
  const body = [
    error ? notice("error", t.errorHeading, errorText(t, error, rules(ctx))) : "",
    `<p class="lead">${esc(fill(t.mfaLead, { email: pending.user.email, method: methodLabel(t, pending.method) }))}</p>`,
    ...answers({ t, kinds, action: "/api/auth/mfa/verify", fields: {}, error, use, passkey: await assertOptions(ctx.env, ctx.request, pending.user.id, "signin", pending.hash, ctx.now) }),
    `<details class="card fold"${use === "recover" ? " open" : ""}><summary>${esc(t.mfaLostHeading)}</summary><p>${esc(t.mfaLostText)}</p>${recover}</details>`,
    `<p><a href="${esc(ctx.s.loginUrl)}">${esc(t.mfaCancel)}</a></p>`,
  ].join("\n");
  return page({ request: ctx.request, lang, title: t.mfaTitle, description: fill(t.mfaDescription, { site }), body, error: Boolean(error) });
}

export async function stepUpPage(ctx: Ctx): Promise<Response> {
  const who = await signedIn(ctx);
  if (who instanceof Response) return who;
  const q = new URL(ctx.request.url).searchParams;
  const next = safeNext(q.get("next"), "/auth/account");
  const kinds = new Set((await listFactors(ctx.env, who.user.id)).map((f) => f.kind as string));
  // Nothing to step up with (the second factor is off, or no factor yet): go on.
  if (!mfaEnabled(ctx.env) || !kinds.size) return redirect(next);
  const lang = langOf(ctx.request);
  const t = STRINGS[lang];
  const site = new URL(ctx.request.url).host;
  const error = q.get("error");
  const body = [
    error ? notice("error", t.errorHeading, errorText(t, error, rules(ctx))) : "",
    `<p class="lead">${esc(fill(t.mfaStepUpLead, { minutes: stepUpMinutes(ctx.env) }))}</p>`,
    ...answers({ t, kinds, action: "/api/auth/mfa/step-up", fields: { next }, error, use: q.get("use"), passkey: await assertOptions(ctx.env, ctx.request, who.user.id, "step-up", who.session.id, ctx.now) }),
    `<p><a href="${esc(next)}">${esc(t.mfaBack)}</a></p>`,
  ].join("\n");
  return page({ request: ctx.request, lang, title: t.mfaStepUpTitle, description: fill(t.mfaStepUpDescription, { site }), body, error: Boolean(error), account: { email: who.user.email } });
}

// route serves one of the pages for functions/auth/mfa/*.
export const route =
  (render: (ctx: Ctx) => Promise<Response>): PagesFunction<Env> =>
  async ({ request, env }) => {
    const ctx = await prepare(request, env);
    return ctx instanceof Response ? ctx : render(ctx);
  };

