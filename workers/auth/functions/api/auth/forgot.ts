// Cloudflare Pages Function: "forgot password" — mail a reset link.
//   POST /api/auth/forgot   JSON {email} or the plain HTML form of /auth/forgot
//
// Exists only when mail works: without a transport it answers 404 not_found
// as if absent (and /auth/login shows no link to it); with a transport that
// can never send, 503 mail_misconfigured. Otherwise the answer is the same
// whether or not the address has an account — 200 {ok: true}, a form a 303
// to /auth/forgot?sent=1 — and so is its timing: the lookup, the link and the
// mail happen after the answer (waitUntil). A blocked account gets no mail.
// Rate limits, counted for every address (existing or not) through the
// login-lock machinery: per address FORGOT_PER_ACCOUNT requests, per IP
// FORGOT_PER_IP, then one per hour; a day without requests starts again.
// Over the limit: 429 too_many_requests (form: ?error=too_many_requests).
// 400 invalid_email.

import { preparePost } from "./_context";
import { langOf } from "./_i18n";
import { linkTo } from "./_delivery";
import { clientIP, json, looksLikeEmail, normEmail, redirect } from "./_lib";
import type { Env } from "./_lib";
import { lockKey, lockState, recordFailure } from "./_lockout";
import { mailState, resetMessage, send } from "./_mail";
import { isForm, readBody } from "./_request";
import { createReset, resetSeconds } from "./_resets";
import { findUserByEmail } from "./_users";

export const FORGOT_PER_ACCOUNT = 3;
export const FORGOT_PER_IP = 10;
const HOUR = { lockBaseSeconds: 3600, lockMaxSeconds: 3600 };

type Context = { request: Request; env: Env; waitUntil?: (p: Promise<unknown>) => void };

export const onRequestPost = async ({ request, env, waitUntil }: Context): Promise<Response> => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const state = mailState(env);
  if (state.kind === "none") return json({ error: "not_found" }, 404);
  if (state.kind === "broken") return json({ error: "mail_misconfigured", detail: state.problem }, 503);
  const form = isForm(request);
  const answer = (status: number, data: Record<string, unknown>, headers: Record<string, string> = {}): Response =>
    form ? redirect(status === 200 ? "/auth/forgot?sent=1" : `/auth/forgot?error=${data.error}`) : json(data, status, headers);

  const email = normEmail((await readBody(request))?.email);
  if (!looksLikeEmail(email)) return answer(400, { error: "invalid_email" });
  const now = ctx.now;
  const keys = [
    { key: await lockKey(env, "forgot_account", email), kind: "forgot_account" as const, threshold: FORGOT_PER_ACCOUNT },
    { key: await lockKey(env, "forgot_ip", clientIP(request)), kind: "forgot_ip" as const, threshold: FORGOT_PER_IP },
  ];
  const locked = await lockState(env, keys.map((k) => k.key), now);
  // The request is counted before anything is sent (SR-02): parallel requests
  // cannot all pass the read above.
  const wait = locked.retryAfter || (await recordFailure(env, keys, HOUR, now));
  if (wait > 0) return answer(429, { error: "too_many_requests" }, { "retry-after": String(wait) });

  const lang = langOf(request);
  const work = async () => {
    const user = await findUserByEmail(env, email);
    if (!user || user.status !== "active") return;
    const { token } = await createReset(env, user.id, null, now);
    const url = linkTo(request, "reset", token, lang);
    const sent = await send(env, resetMessage({ to: user.email, lang, site: new URL(request.url).host, url, minutes: resetSeconds(env) / 60 }));
    if (!sent.ok) console.warn(`auth: reset mail not sent: ${sent.detail}`);
  };
  const done = work().catch((e) => console.error(`auth: forgot-password: ${e}`));
  if (waitUntil) waitUntil(done);
  else await done;
  return answer(200, { ok: true });
};
