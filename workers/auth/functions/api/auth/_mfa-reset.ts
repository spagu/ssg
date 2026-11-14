// Losing a second factor (GO-140): the two ways back, and what they do. A
// leading underscore keeps this file out of the Pages route table.
//
//   by another owner  POST /api/auth/mfa/reset {userId}: a holder of `*` with a
//                     recent second factor resets ANOTHER account (never its
//                     own — that is removing a factor, with a step-up).
//   break-glass       POST /api/auth/mfa/break-glass {token}: for an owner who
//                     lost every factor and has no other owner to ask. The
//                     token is the wrangler secret AUTH_MFA_RESET_TOKEN, works
//                     ONCE (its hash is kept in mfa_reset_tokens, so the same
//                     value never works again), and only from the pending step
//                     of a sign-in — after the account's own first factor — so
//                     it resets the account that just proved its password and
//                     no other, and only an account holding the owner role.
//
// A reset removes every factor, enrolment and backup code of the account,
// forgets the second factor of its sessions and ends those sessions; the
// account must enrol again at its next sign-in (its roles that need a second
// factor stay out of the token until then). Each is an audit event.

import { emitEvent } from "./_audit";
import type { Ctx } from "./_context";
import { iso, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import { resetFactors } from "./_mfa-factors";
import { revokeUserSessions } from "./_sessions";

// holdsOwner: does the owner role reach this account (directly or by a group)?
export async function holdsOwner(env: Env, userId: string): Promise<boolean> {
  return (await env.AUTH_DB.prepare("SELECT 1 AS x FROM owner_holders WHERE user_id = ?").bind(userId).first()) !== null;
}

// spendResetToken records a break-glass value as used; false when it was.
export async function spendResetToken(env: Env, token: string, userId: string, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare("INSERT OR IGNORE INTO mfa_reset_tokens (token_hash, user_id, used_at) VALUES (?, ?, ?)")
    .bind(await sha256hex("ssg-auth-mfa-reset\0" + token), userId, iso(now))
    .run();
  return r.meta.changes === 1;
}

export async function resetTokenUsed(env: Env, token: string): Promise<boolean> {
  return (await env.AUTH_DB.prepare("SELECT 1 AS x FROM mfa_reset_tokens WHERE token_hash = ?").bind(await sha256hex("ssg-auth-mfa-reset\0" + token)).first()) !== null;
}

// resetAccount removes the account's factors and ends its sessions.
export async function resetAccount(ctx: Ctx, userId: string, detail: Record<string, string>): Promise<{ removed: number; revoked: number }> {
  const removed = await resetFactors(ctx.env, userId);
  const revoked = await revokeUserSessions(ctx.env, userId, "mfa_reset", ctx.now);
  await emitEvent(ctx.env, { kind: "mfa_reset", userId, detail: { ...detail, removed } }, ctx.now);
  return { removed, revoked };
}
