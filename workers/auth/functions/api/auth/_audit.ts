// The audit trail of sign-ins and sign-in methods (GO-151), the table GO-138's
// log reads. A leading underscore keeps this file out of the Pages route
// table. _schema.ts lists AUDIT_TABLES in TABLES; schema.sql mirrors them.
//
// emitEvent is the ONE function every sign-in event goes through: a later
// ticket that wants more (another sink, a filter, retention) changes it here
// and every caller follows. It never throws: a lost audit row must not turn a
// sign-in into an error page, so a failed write is logged (kind only) and the
// request goes on.
//
//   auth_events  one row per event: when, what (kind), whose account (NULL
//                when there is none — a refused provider sign-in), the sign-in
//                method, and a small JSON detail. Never a token, a code, a
//                secret or a raw IP: details are reasons and addresses.
//
// Kinds today: signin, signin_refused, method_linked, method_unlinked,
// password_set, invitation_accepted; account_created and groups_synced
// (GO-139, Cloudflare Access); mfa_* (GO-140, below). The password login's failures are
// counted by the lockout (login_attempts), not logged here: an attacker
// should not be able to grow this table one guess at a time.

import { intVar, iso } from "./_lib";
import type { Env } from "./_lib";

export const AUDIT_TABLES = [
  `CREATE TABLE IF NOT EXISTS auth_events (
     id      TEXT PRIMARY KEY,
     at      TEXT NOT NULL,
     kind    TEXT NOT NULL,
     user_id TEXT,
     method  TEXT,
     detail  TEXT NOT NULL DEFAULT '{}'
   )`,
  `CREATE INDEX IF NOT EXISTS idx_auth_events_at ON auth_events (at)`,
  `CREATE INDEX IF NOT EXISTS idx_auth_events_user ON auth_events (user_id, at)`,
];

export type EventKind =
  | "signin"
  | "signin_refused"
  | "method_linked"
  | "method_unlinked"
  | "password_set"
  | "invitation_accepted"
  // GO-139: an account made at a first Access sign-in (AUTH_ACCESS_AUTO_CREATE),
  // and group memberships the Access group mapping added or removed.
  | "account_created"
  | "groups_synced"
  // GO-140: the second factor — enrolled, removed, a challenge passed or
  // failed (sign-in or step-up; only after a first factor or within a session,
  // so the lockout bounds how fast they grow), a backup code spent, backup
  // codes regenerated, and an account's factors reset (by an owner or with the
  // break-glass secret).
  | "mfa_enrolled"
  | "mfa_removed"
  | "mfa_passed"
  | "mfa_failed"
  | "mfa_backup_code_used"
  | "mfa_backup_codes_regenerated"
  | "mfa_reset"
  // GO-138: what administrators change — accounts blocked, unblocked and
  // deleted, sessions ended, grants and memberships, roles, invitations,
  // reset links, service tokens — and a signing key first used (a rotation).
  // `user_id` is the account changed (null when none: a role, everyone);
  // `detail.by` is the administrator.
  | "account_blocked"
  | "account_unblocked"
  | "account_deleted"
  | "sessions_revoked"
  | "session_revoked"
  | "grant_added"
  | "grant_removed"
  | "member_added"
  | "member_removed"
  | "role_created"
  | "role_changed"
  | "role_deleted"
  | "group_deleted"
  | "access_mapping_added"
  | "access_mapping_removed"
  | "invitation_sent"
  | "invitation_revoked"
  | "reset_link_issued"
  | "service_token_created"
  | "service_token_revoked"
  | "signing_key_seen";

export interface AuthEvent {
  kind: EventKind;
  userId?: string | null;
  method?: string | null;
  detail?: Record<string, string | number | boolean | null>;
}

// auditRetentionDays: how long an event is kept (AUTH_AUDIT_RETENTION_DAYS,
// default 365). GO-138: the purge runs with every write, in the same batch —
// no cron, and an idle site keeps its last year until something happens.
export const auditRetentionDays = (env: Env): number => intVar(env.AUTH_AUDIT_RETENTION_DAYS, 365);

export async function emitEvent(env: Env, e: AuthEvent, now: number): Promise<void> {
  const db = env.AUTH_DB;
  try {
    await db.batch([
      db
        .prepare("INSERT INTO auth_events (id, at, kind, user_id, method, detail) VALUES (?, ?, ?, ?, ?, ?)")
        .bind(crypto.randomUUID(), iso(now), e.kind, e.userId ?? null, e.method ?? null, JSON.stringify(e.detail ?? {})),
      db.prepare("DELETE FROM auth_events WHERE at < ?").bind(iso(now - auditRetentionDays(env) * 86_400_000)),
    ]);
  } catch {
    console.error(`auth: audit event ${e.kind} not recorded`);
  }
}
