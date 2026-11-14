// Checking a second factor (GO-140): an authenticator-app code, a backup
// code or a passkey answer, for one account — at sign-in (the pending step)
// or for a step-up — with the lockout and the audit trail. A leading
// underscore keeps this file out of the Pages route table.
//
// Lockout: the password's mechanism (_lockout.ts) with its own counters,
// mfa_account (per account, AUTH_LOCK_ACCOUNT_FAILURES) and mfa_ip (per IP,
// AUTH_LOCK_IP_FAILURES), checked and counted before any code is: a locked
// request gets 429 even with the right code, and parallel guesses cannot
// outrun the count (SR-02). A 6-digit code has a million values; five
// tries, then a doubling pause, keep guessing hopeless.

import { emitEvent } from "./_audit";
import type { Ctx } from "./_context";
import { clientIP } from "./_lib";
import type { Env } from "./_lib";
import { clearFailures, lockKey, lockState, recordFailure, releaseAttempt } from "./_lockout";
import { mfaState, open, seal } from "./_mfa-crypto";
import { acceptStep, backupCodesLeft, listFactors, reseal, useBackupCode } from "./_mfa-factors";
import type { Body } from "./_request";
import { matchStep, normCode } from "./_totp";
import type { User } from "./_users";
import { checkAssertion } from "./_webauthn-store";

export type SecondFactor = "totp" | "backup" | "passkey";
export type CheckPurpose = "signin" | "step-up";

export type Checked =
  | { ok: true; factor: SecondFactor }
  | { ok: false; error: "invalid_code"; status: 401 }
  | { ok: false; error: "too_many_attempts"; status: 429; retryAfter: number }
  | { ok: false; error: "mfa_unavailable"; status: 503 };

// checkTotp: does `raw` match one of the account's authenticator apps, at a
// step later than the last accepted one? The step is recorded at once.
export async function checkTotp(env: Env, userId: string, raw: unknown, now: number): Promise<boolean> {
  const code = normCode(raw);
  if (!code) return false;
  for (const f of await listFactors(env, userId)) {
    if (f.kind !== "totp") continue;
    const opened = await open(env, f.secret as string, f.id, userId);
    if (!opened) {
      console.error(`auth: the secret of a second factor (${f.id}) cannot be decrypted; check AUTH_MFA_KEY / AUTH_MFA_KEY_PREVIOUS`);
      continue;
    }
    const step = await matchStep(opened.plain, code, now);
    if (step === null || step <= f.last_step || !(await acceptStep(env, f.id, step, now))) continue;
    if (opened.stale) await reseal(env, f.id, (await seal(env, opened.plain, f.id, userId)) as string);
    return true;
  }
  return false;
}

// checkSecondFactor: body.passkey (an assertion), else body.backupCode, else
// body.code. `boundTo` is what a passkey challenge was issued to.
export async function checkSecondFactor(ctx: Ctx, user: User, body: Body, purpose: CheckPurpose, boundTo: string): Promise<Checked> {
  const { env, s, now } = ctx;
  const usesCode = body.passkey === undefined && !body.backupCode;
  if (usesCode && mfaState(env) !== "ready") return { ok: false, error: "mfa_unavailable", status: 503 };
  const accountKey = await lockKey(env, "mfa_account", user.id);
  const ipKey = await lockKey(env, "mfa_ip", clientIP(ctx.request));
  const state = await lockState(env, [accountKey, ipKey], now);
  if (state.retryAfter > 0) return { ok: false, error: "too_many_attempts", status: 429, retryAfter: state.retryAfter };
  // Counted before the code is checked (SR-02): a burst of parallel guesses
  // cannot all pass the read above before any failure is written.
  const ipEntry = { key: ipKey, kind: "mfa_ip" as const, threshold: s.lockIP };
  const over = await recordFailure(env, [{ key: accountKey, kind: "mfa_account", threshold: s.lockAccount }, ipEntry], s, now);
  if (over > 0) return { ok: false, error: "too_many_attempts", status: 429, retryAfter: over };

  let factor: SecondFactor | null;
  if (body.passkey !== undefined) factor = (await checkAssertion(env, ctx.request, user.id, purpose, boundTo, body.passkey, now)) ? "passkey" : null;
  else if (body.backupCode) factor = (await useBackupCode(env, user.id, body.backupCode, now)) ? "backup" : null;
  else factor = (await checkTotp(env, user.id, body.code, now)) ? "totp" : null;

  if (!factor) {
    const tried = body.passkey !== undefined ? "passkey" : body.backupCode ? "backup" : "totp";
    await emitEvent(env, { kind: "mfa_failed", userId: user.id, detail: { purpose, factor: tried } }, now);
    return { ok: false, error: "invalid_code", status: 401 };
  }
  // As with the password: the IP counter only gets this attempt back.
  await clearFailures(env, accountKey);
  await releaseAttempt(env, [ipEntry]);
  await emitEvent(env, { kind: "mfa_passed", userId: user.id, detail: { purpose, factor } }, now);
  if (factor === "backup") await emitEvent(env, { kind: "mfa_backup_code_used", userId: user.id, detail: { left: await backupCodesLeft(env, user.id) } }, now);
  return { ok: true, factor };
}
