// Password reset links (GO-134). A leading underscore keeps this file out of
// the Pages route table.
//
// Like an invitation: 32 random bytes shown once, only the SHA-256 stored,
// single use, short-lived (AUTH_RESET_MINUTES, default 60). A new link for an
// account retires the earlier unused ones. Using one sets the password, ends
// every session of the account and clears its login lock (reset.ts).

import { intVar, iso, randomToken, sha256hex } from "./_lib";
import type { Env } from "./_lib";

export const resetSeconds = (env: Env): number => intVar(env.AUTH_RESET_MINUTES, 60) * 60;

// createReset issues a link for userId; `by` is the administrator who asked,
// null for the account holder ("forgot password").
export async function createReset(env: Env, userId: string, by: string | null, now: number): Promise<{ token: string; expiresAt: string }> {
  const db = env.AUTH_DB;
  const token = randomToken();
  const expiresAt = iso(now + resetSeconds(env) * 1000);
  await db.batch([
    db.prepare("UPDATE password_resets SET used_at = ? WHERE user_id = ? AND used_at IS NULL").bind(iso(now), userId),
    db
      .prepare("INSERT INTO password_resets (id, user_id, token_hash, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)")
      .bind(crypto.randomUUID(), userId, await sha256hex(token), by, iso(now), expiresAt),
  ]);
  return { token, expiresAt };
}

// openReset: the account a token may still reset, or null (unknown, used,
// retired and expired tokens alike).
export async function openReset(env: Env, token: string, now: number): Promise<{ userId: string } | null> {
  const row = await env.AUTH_DB.prepare("SELECT user_id FROM password_resets WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?")
    .bind(await sha256hex(token), iso(now))
    .first<{ user_id: string }>();
  return row ? { userId: row.user_id } : null;
}

// consumeReset marks the token used, atomically: of two requests with the
// same token, one gets the account id and the other null. The caller checks
// and hashes the new password BEFORE this, so a refused password does not
// burn the link.
export async function consumeReset(env: Env, token: string, now: number): Promise<string | null> {
  const row = await env.AUTH_DB.prepare(
    "UPDATE password_resets SET used_at = ? WHERE token_hash = ? AND used_at IS NULL AND expires_at > ? RETURNING user_id",
  )
    .bind(iso(now), await sha256hex(token), iso(now))
    .first<{ user_id: string }>();
  return row?.user_id ?? null;
}
