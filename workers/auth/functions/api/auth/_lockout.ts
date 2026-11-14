// Failed-login lockout, per account AND per IP, with a growing delay. A
// leading underscore keeps this file out of the Pages route table.
//
// Checked BEFORE any password is hashed: a locked key gets 429 even for the
// right password (otherwise the lock would only change the error code of a
// wrong guess), and a locked request costs no hashing CPU.
//
// Counted BEFORE the secret is checked, too (security review SR-02): reading
// the lock, checking, then writing the failure let a burst of parallel
// requests all pass the read before any of them wrote, so every guess of the
// burst was checked. recordFailure is therefore called first, as a
// reservation: it answers how long to wait when this attempt is beyond the
// threshold, and the caller refuses without checking anything. A success
// gives its reservation back (clearFailures for the account's own key,
// releaseAttempt for a shared one such as the IP's).
//
// After `threshold` failures a key is locked for base seconds, doubled for
// every further failure, up to max. A failure more than a day after the last
// one starts the count again. The account counter exists for addresses that
// have no account too, so a lock never reveals whether an account exists.
// The price is that someone can keep a known address locked out; the cap
// (AUTH_LOCK_MAX_SECONDS) bounds that, and the IP counter (higher threshold,
// many people may share an address) bounds one source's guessing overall.
//
// Keys are salted hashes, never the address or the IP: with AUTH_HASH_SALT
// when set, otherwise with a secret derived from the signing key (always
// present when the worker runs). Rotating that key resets the counters.

import { iso, sha256hex } from "./_lib";
import type { Env } from "./_lib";

// forgot_*: the request counters of POST /api/auth/forgot (GO-134), kept
// apart from the login counters so asking for mail never locks a login.
// service_*: the failure counters of POST /api/auth/token (GO-141), per token
// id and per IP, apart from the login's so a broken CI job never locks people out.
// mfa_*: wrong second-factor codes (GO-140), per account and per IP, apart
// from the password's; mfa_reset_ip / mfa_reset_account: wrong break-glass
// tokens, per IP and per account (SR-03). service_token is no longer
// counted (SR-08): a token's id is public and its secret unguessable, so a
// per-id lock only let anyone lock a working token out; kept in the type so
// rows written before stay readable.
export type LockKind =
  | "account"
  | "ip"
  | "bootstrap"
  | "forgot_account"
  | "forgot_ip"
  | "service_token"
  | "service_ip"
  | "mfa_account"
  | "mfa_ip"
  | "mfa_reset_ip"
  | "mfa_reset_account";
const RESET_MS = 24 * 3600 * 1000;

export interface LockPolicy {
  lockBaseSeconds: number;
  lockMaxSeconds: number;
}

async function salt(env: Env): Promise<string> {
  if (env.AUTH_HASH_SALT) return env.AUTH_HASH_SALT;
  // The caller only gets here with a valid AUTH_SIGNING_KEY (JSON with `d`).
  const d = (JSON.parse(env.AUTH_SIGNING_KEY as string) as { d: string }).d;
  return "key:" + (await sha256hex("ssg-auth-lockout\0" + d));
}

export async function lockKey(env: Env, kind: LockKind, value: string): Promise<string> {
  return sha256hex(`${await salt(env)}\0${kind}\0${value}`);
}

export interface LockState {
  retryAfter: number; // seconds until every key is unlocked; 0 = not locked
  failures: number; // the highest recent failure count among the keys
}

export async function lockState(env: Env, keys: string[], now: number): Promise<LockState> {
  let retryAfter = 0;
  let failures = 0;
  for (const key of keys) {
    const row = await env.AUTH_DB.prepare("SELECT failures, last_failure_at, locked_until FROM login_attempts WHERE key = ?")
      .bind(key)
      .first<{ failures: number; last_failure_at: string; locked_until: string | null }>();
    if (!row) continue;
    if (Date.parse(row.last_failure_at) + RESET_MS > now) failures = Math.max(failures, row.failures);
    const until = row.locked_until ? Date.parse(row.locked_until) : 0;
    if (until > now) retryAfter = Math.max(retryAfter, Math.ceil((until - now) / 1000));
  }
  return { retryAfter, failures };
}

// lockSeconds is the lock after the n-th failure for a threshold.
export function lockSeconds(n: number, threshold: number, policy: LockPolicy): number {
  if (n < threshold) return 0;
  return Math.min(policy.lockMaxSeconds, policy.lockBaseSeconds * 2 ** Math.min(n - threshold, 20));
}

export interface LockEntry {
  key: string;
  kind: LockKind;
  threshold: number;
}

// recordFailure counts one failure for each key atomically (two requests at
// once both count) and sets the lock when a threshold is reached. Called
// before the secret is checked, it is the attempt's reservation: it returns
// the seconds to wait when this attempt must NOT be checked, 0 when it may.
//
// Past the threshold, an attempt may be checked only once its key's lock has
// run out, and then only one: the attempt that moves the expired lock on
// (compare-and-swap on locked_until) is checked; a parallel one that loses the
// swap, or that arrives while the lock is set — or before the request that
// reached the threshold has written it (NULL) — is refused. A refused attempt
// gives its counts back, so waiting out a lock behaves as before (the delay
// grows per checked failure, not per refusal).
export async function recordFailure(env: Env, entries: LockEntry[], policy: LockPolicy, now: number): Promise<number> {
  const db = env.AUTH_DB;
  const stale = iso(now - RESET_MS);
  const counted: { e: LockEntry; failures: number; lock: string | null }[] = [];
  const refuse = async (seconds: number): Promise<number> => {
    for (const c of counted) await db.prepare("UPDATE login_attempts SET failures = MAX(failures - 1, 0) WHERE key = ?").bind(c.e.key).run();
    return Math.max(1, seconds);
  };
  for (const e of entries) {
    const row = (await db
      .prepare(
        `INSERT INTO login_attempts (key, kind, failures, last_failure_at) VALUES (?, ?, 1, ?)
         ON CONFLICT (key) DO UPDATE SET
           failures = CASE WHEN login_attempts.last_failure_at < ? THEN 1 ELSE login_attempts.failures + 1 END,
           last_failure_at = excluded.last_failure_at
         RETURNING failures, locked_until`,
      )
      .bind(e.key, e.kind, iso(now), stale)
      .first<{ failures: number; locked_until: string | null }>()) as { failures: number; locked_until: string | null };
    counted.push({ e, failures: row.failures, lock: row.locked_until });
    const until = row.locked_until ? Date.parse(row.locked_until) : 0;
    if (row.failures > e.threshold && (row.locked_until === null || until > now)) {
      return refuse(until > now ? Math.ceil((until - now) / 1000) : lockSeconds(e.threshold, e.threshold, policy));
    }
  }
  for (const c of counted) {
    const seconds = lockSeconds(c.failures, c.e.threshold, policy);
    if (seconds === 0) continue;
    const at = iso(now + seconds * 1000);
    if (c.failures === c.e.threshold) {
      await db.prepare("UPDATE login_attempts SET locked_until = ? WHERE key = ?").bind(at, c.e.key).run();
      continue;
    }
    // Past the threshold with an expired lock: only the attempt that moves it on is checked.
    const moved = await db.prepare("UPDATE login_attempts SET locked_until = ? WHERE key = ? AND locked_until = ?").bind(at, c.e.key, c.lock).run();
    if (moved.meta.changes !== 1) return refuse(seconds);
  }
  return 0;
}

// releaseAttempt gives back the reservation of an attempt that succeeded, on
// keys a success must not clear (an IP is shared): one failure fewer — the
// row gone when none is left — and the lock lifted when that leaves the key
// under its threshold.
export async function releaseAttempt(env: Env, entries: LockEntry[]): Promise<void> {
  for (const e of entries) {
    await env.AUTH_DB.batch([
      env.AUTH_DB.prepare("DELETE FROM login_attempts WHERE key = ? AND failures <= 1").bind(e.key),
      env.AUTH_DB.prepare(
        `UPDATE login_attempts SET failures = failures - 1,
           locked_until = CASE WHEN failures - 1 < ? THEN NULL ELSE locked_until END
         WHERE key = ?`,
      ).bind(e.threshold, e.key),
    ]);
  }
}

export async function clearFailures(env: Env, key: string): Promise<void> {
  await env.AUTH_DB.prepare("DELETE FROM login_attempts WHERE key = ?").bind(key).run();
}
