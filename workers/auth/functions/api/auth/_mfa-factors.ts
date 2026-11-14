// An account's second factors and backup codes (GO-140): the data layer. A
// leading underscore keeps this file out of the Pages route table. Checking a
// code or an assertion is _mfa-verify.ts; this file only reads and writes.

import { iso, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import { seal } from "./_mfa-crypto";
import { newSecret } from "./_totp";

export type FactorKind = "totp" | "passkey";

export interface FactorRow {
  id: string;
  user_id: string;
  kind: FactorKind;
  name: string;
  secret: string | null;
  last_step: number;
  credential_id: string | null;
  public_key: string | null;
  algorithm: number | null;
  sign_count: number;
  transports: string;
  confirmed_at: string | null;
  created_at: string;
  last_used_at: string | null;
}

export interface FactorView {
  id: string;
  kind: FactorKind;
  name: string;
  createdAt: string;
  lastUsedAt: string | null;
}

export const view = (r: FactorRow): FactorView => ({ id: r.id, kind: r.kind, name: r.name, createdAt: r.confirmed_at as string, lastUsedAt: r.last_used_at });

// ENROL_MINUTES: how long an authenticator-app enrolment waits for its first code.
export const ENROL_MINUTES = 15;

// listFactors: the account's working factors (enrolments not yet confirmed
// are not factors), oldest first.
export async function listFactors(env: Env, userId: string): Promise<FactorRow[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT * FROM mfa_factors WHERE user_id = ? AND confirmed_at IS NOT NULL ORDER BY confirmed_at, id")
    .bind(userId)
    .all<FactorRow>();
  return results;
}

export const hasFactor = async (env: Env, userId: string): Promise<boolean> => (await listFactors(env, userId)).length > 0;

// startTotp makes an unconfirmed authenticator-app factor with a new secret,
// sealed under AUTH_MFA_KEY; an earlier unconfirmed one of the account goes.
// null when the key is unusable (nothing is written).
export async function startTotp(env: Env, userId: string, now: number): Promise<{ id: string; secret: Uint8Array } | null> {
  const id = crypto.randomUUID();
  const secret = newSecret();
  const sealed = await seal(env, secret, id, userId);
  if (!sealed) return null;
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("DELETE FROM mfa_factors WHERE user_id = ? AND confirmed_at IS NULL").bind(userId),
    env.AUTH_DB.prepare("INSERT INTO mfa_factors (id, user_id, kind, secret, created_at) VALUES (?, ?, 'totp', ?, ?)").bind(id, userId, sealed, iso(now)),
  ]);
  return { id, secret };
}

// pendingTotp: the account's own unconfirmed enrolment `id`, while it is fresh.
export function pendingTotp(env: Env, userId: string, id: string, now: number): Promise<FactorRow | null> {
  return env.AUTH_DB.prepare("SELECT * FROM mfa_factors WHERE id = ? AND user_id = ? AND kind = 'totp' AND confirmed_at IS NULL AND created_at > ?")
    .bind(id, userId, iso(now - ENROL_MINUTES * 60_000))
    .first<FactorRow>();
}

// confirmTotp turns the enrolment into a factor, remembering the step of the
// code that confirmed it (that code cannot be used again). False when it was
// confirmed meanwhile.
export async function confirmTotp(env: Env, userId: string, id: string, step: number, name: string, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare("UPDATE mfa_factors SET confirmed_at = ?, last_step = ?, name = ? WHERE id = ? AND user_id = ? AND confirmed_at IS NULL")
    .bind(iso(now), step, name, id, userId)
    .run();
  return r.meta.changes === 1;
}

// acceptStep records `step` as used — only if it is later than the last one,
// in the same statement, so two requests with one code cannot both pass.
export async function acceptStep(env: Env, id: string, step: number, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare("UPDATE mfa_factors SET last_step = ?, last_used_at = ? WHERE id = ? AND last_step < ?").bind(step, iso(now), id, step).run();
  return r.meta.changes === 1;
}

export async function reseal(env: Env, id: string, sealed: string): Promise<void> {
  await env.AUTH_DB.prepare("UPDATE mfa_factors SET secret = ? WHERE id = ?").bind(sealed, id).run();
}

export interface NewPasskey {
  credentialId: string;
  publicKey: string; // JWK JSON
  algorithm: number;
  signCount: number;
  transports: string[];
  name: string;
}

// addPasskey stores a registered credential; false when that credential id is
// already registered (to any account).
export async function addPasskey(env: Env, userId: string, p: NewPasskey, now: number): Promise<string | null> {
  const id = crypto.randomUUID();
  const r = await env.AUTH_DB.prepare(
    `INSERT OR IGNORE INTO mfa_factors (id, user_id, kind, name, credential_id, public_key, algorithm, sign_count, transports, confirmed_at, created_at)
     VALUES (?, ?, 'passkey', ?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(id, userId, p.name, p.credentialId, p.publicKey, p.algorithm, p.signCount, JSON.stringify(p.transports), iso(now), iso(now))
    .run();
  return r.meta.changes === 1 ? id : null;
}

// moveCounter stores the new signature counter only if the stored one is still
// what was checked (two assertions at once cannot both move it).
export async function moveCounter(env: Env, id: string, from: number, to: number, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare("UPDATE mfa_factors SET sign_count = ?, last_used_at = ? WHERE id = ? AND sign_count = ?").bind(to, iso(now), id, from).run();
  return r.meta.changes === 1;
}

// removeFactor deletes one of the account's factors; with the last one go the
// backup codes (they belong to the second factor, not on their own).
// `keepOne`: a role of the account needs a factor, so the delete happens only
// while another confirmed factor remains — in the same statement, so two
// parallel removals cannot both pass a count read before either (SR-13).
export async function removeFactor(env: Env, userId: string, id: string, keepOne = false): Promise<boolean> {
  const db = env.AUTH_DB;
  const others = keepOne ? " AND EXISTS (SELECT 1 FROM mfa_factors o WHERE o.user_id = mfa_factors.user_id AND o.confirmed_at IS NOT NULL AND o.id <> mfa_factors.id)" : "";
  const [r] = await db.batch([
    db.prepare(`DELETE FROM mfa_factors WHERE id = ? AND user_id = ? AND confirmed_at IS NOT NULL${others}`).bind(id, userId),
    db.prepare("DELETE FROM mfa_backup_codes WHERE user_id = ? AND NOT EXISTS (SELECT 1 FROM mfa_factors WHERE user_id = ? AND confirmed_at IS NOT NULL)").bind(userId, userId),
  ]);
  return r.meta.changes === 1;
}

// resetFactors removes every factor, enrolment and backup code of an account
// and forgets the second factor of its sessions (recovery, _mfa-reset.ts).
export async function resetFactors(env: Env, userId: string): Promise<number> {
  const db = env.AUTH_DB;
  const [r] = await db.batch([
    db.prepare("DELETE FROM mfa_factors WHERE user_id = ?").bind(userId),
    db.prepare("DELETE FROM mfa_backup_codes WHERE user_id = ?").bind(userId),
    db.prepare("DELETE FROM session_factors WHERE session_id IN (SELECT id FROM sessions WHERE user_id = ?)").bind(userId),
  ]);
  return r.meta.changes;
}

// Backup codes: 16 characters of Crockford's base32 (no I, L, O, U), 80
// random bits, shown as XXXX-XXXX-XXXX-XXXX.
const CODE_ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
export const BACKUP_CODES = 10;

export function normBackupCode(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length > 64) return null;
  const s = raw.toUpperCase().replace(/[\s-]/g, "").replace(/O/g, "0").replace(/[IL]/g, "1");
  return /^[0-9A-HJKMNP-TV-Z]{16}$/.test(s) ? s : null;
}

const codeHash = (userId: string, code: string): Promise<string> => sha256hex(`ssg-auth-backup\0${userId}\0${code}`);

// newBackupCodes replaces the account's set with ten new codes and returns
// them, formatted — the only time they exist outside the person's hands.
export async function newBackupCodes(env: Env, userId: string, now: number): Promise<string[]> {
  const codes = Array.from({ length: BACKUP_CODES }, () => [...crypto.getRandomValues(new Uint8Array(16))].map((b) => CODE_ALPHABET[b & 31]).join(""));
  const db = env.AUTH_DB;
  const inserts = await Promise.all(
    codes.map(async (c) => db.prepare("INSERT INTO mfa_backup_codes (code_hash, user_id, created_at) VALUES (?, ?, ?)").bind(await codeHash(userId, c), userId, iso(now))),
  );
  await db.batch([db.prepare("DELETE FROM mfa_backup_codes WHERE user_id = ?").bind(userId), ...inserts]);
  return codes.map((c) => c.replace(/(.{4})(?=.)/g, "$1-"));
}

// useBackupCode spends one unused code of the account; true once per code.
export async function useBackupCode(env: Env, userId: string, raw: unknown, now: number): Promise<boolean> {
  const code = normBackupCode(raw);
  if (!code) return false;
  const r = await env.AUTH_DB.prepare("UPDATE mfa_backup_codes SET used_at = ? WHERE code_hash = ? AND user_id = ? AND used_at IS NULL")
    .bind(iso(now), await codeHash(userId, code), userId)
    .run();
  return r.meta.changes === 1;
}

export async function backupCodesLeft(env: Env, userId: string): Promise<number> {
  const row = await env.AUTH_DB.prepare("SELECT COUNT(*) AS n FROM mfa_backup_codes WHERE user_id = ? AND used_at IS NULL").bind(userId).first<{ n: number }>();
  return (row as { n: number }).n;
}
