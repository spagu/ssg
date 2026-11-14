// Cloudflare Pages Function: the break-glass reset for an owner who lost every
// second factor (GO-140).
//   POST /api/auth/mfa/break-glass   {token} — the challenge page's plain form or JSON with x-ssg-auth: 1
//
// Works only from the pending step of a sign-in (the account's own first
// factor passed: the cookie of _mfa-pending.ts), only for an account holding
// the owner role, only while AUTH_MFA_RESET_TOKEN is set, and only once per
// value (_mfa-reset.ts). The token is compared in constant time; wrong ones
// count towards a per-IP and a per-account lock (5, then the growing delay),
// and a token shorter than 32 characters counts as unset (SR-03). Then the account's
// factors are reset, the pending step is used up and a session starts — with
// the roles that need a second factor left out until it enrols again, which a
// form is sent to do at once (/auth/mfa/enrol?required=1). JSON: 200 {user,
// expiresAt, sessionExpiresAt, enrol: true}. Refusals: 404 not_found (no
// token configured, or this value already used — as if the route did not
// exist); 401 mfa_pending_invalid | invalid_token; 403 not_owner; 429
// too_many_attempts. Delete the secret afterwards.

import { preparePost } from "../_context";
import { clientIP, json, redirect, secretEqual } from "../_lib";
import type { Env } from "../_lib";
import { clearFailures, lockKey, lockState, recordFailure } from "../_lockout";
import type { LockEntry } from "../_lockout";
import { mfaEnabled } from "../_mfa-crypto";
import { notFound } from "../_mfa-gate";
import { clearPendingCookie, consumePending, readPending } from "../_mfa-pending";
import { holdsOwner, resetAccount, resetTokenUsed, spendResetToken } from "../_mfa-reset";
import { CHALLENGE_PAGE } from "../_mfa-signin";
import { withParams } from "../_next";
import { isForm, readBody, str } from "../_request";
import { openSession, signInRedirect } from "../_signin";
import { publicUser } from "../_users";

const FAILURES = 5;
// MIN_TOKEN: a shorter AUTH_MFA_RESET_TOKEN counts as unset (SR-03): it is
// the only thing between an owner's password and an owner session without
// a second factor, so it must not be guessable.
export const MIN_TOKEN = 32;

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const expected = env.AUTH_MFA_RESET_TOKEN;
  if (!mfaEnabled(env) || !expected) return notFound();
  if (expected.length < MIN_TOKEN) {
    console.error(`auth: AUTH_MFA_RESET_TOKEN is shorter than ${MIN_TOKEN} characters and is ignored; set a longer one`);
    return notFound();
  }
  const form = isForm(request);
  const fail = (error: string, status: number, headers: Record<string, string> = {}): Response =>
    form ? redirect(withParams(CHALLENGE_PAGE, { error, use: "recover" })) : json({ error }, status, headers);
  const pending = await readPending(env, request, ctx.now);
  if (!pending) return form ? redirect(withParams(ctx.s.loginUrl, { error: "mfa_expired" })) : json({ error: "mfa_pending_invalid" }, 401);

  // Per IP and per account (SR-03), counted before the token is compared (SR-02).
  const entries: LockEntry[] = [
    { key: await lockKey(env, "mfa_reset_ip", clientIP(request)), kind: "mfa_reset_ip", threshold: FAILURES },
    { key: await lockKey(env, "mfa_reset_account", pending.user.id), kind: "mfa_reset_account", threshold: FAILURES },
  ];
  const state = await lockState(env, entries.map((e) => e.key), ctx.now);
  const wait = state.retryAfter || (await recordFailure(env, entries, ctx.s, ctx.now));
  if (wait > 0) return fail("too_many_attempts", 429, { "retry-after": String(wait) });
  const token = str((await readBody(request))?.token);
  if (!(await secretEqual(token, expected))) return fail("invalid_token", 401);
  // The right token: the reservation is given back, whatever follows.
  for (const e of entries) await clearFailures(env, e.key);
  if (await resetTokenUsed(env, token)) return notFound();
  // Checked before the token is spent, so a non-owner cannot burn it.
  if (!(await holdsOwner(env, pending.user.id))) return fail("not_owner", 403);
  if (!(await spendResetToken(env, token, pending.user.id, ctx.now))) return notFound();
  await consumePending(env, pending.hash, ctx.now);
  await resetAccount(ctx, pending.user.id, { via: "break_glass" });
  const started = await openSession(ctx, pending.user, pending.method, null);
  if (form) return signInRedirect(ctx, started, pending.next || "/auth/", [clearPendingCookie()]);
  if (!started.ok) return json({ error: "permissions_too_large", detail: started.tooLarge }, 500, {}, [clearPendingCookie()]);
  const { session, issued } = started;
  return json({ user: publicUser(pending.user), expiresAt: issued.exp, sessionExpiresAt: session.expires_at, enrol: started.enrol }, 200, {}, [clearPendingCookie(), ...issued.cookies]);
};
