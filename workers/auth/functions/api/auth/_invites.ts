// Invitations — the only way to an account (GO-134). A leading underscore
// keeps this file out of the Pages route table.
//
// The token is 32 random bytes shown once (in the mail, or to the inviter);
// only its SHA-256 is stored. An invitation is open while it is neither
// accepted nor revoked and has not expired. Inviting an address again, and
// sending a new link, both make the earlier token useless at once.
// Accepting is _invites-accept.ts.

import { intVar, iso, randomToken, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import type { Lang } from "./_i18n";

export interface Invitation {
  id: string;
  email: string;
  name: string;
  roleIds: string[];
  groupIds: string[];
  lang: Lang;
  invitedBy: string | null;
  createdAt: string;
  expiresAt: string;
  acceptedAt: string | null;
  acceptedUserId: string | null;
  revokedAt: string | null;
  revokedBy: string | null;
  revokeReason: string | null;
}

export type InviteStatus = "pending" | "accepted" | "expired" | "revoked";

interface Row {
  id: string;
  email: string;
  display_name: string;
  role_ids: string;
  group_ids: string;
  lang: string;
  invited_by: string | null;
  created_at: string;
  expires_at: string;
  accepted_at: string | null;
  accepted_user_id: string | null;
  revoked_at: string | null;
  revoked_by: string | null;
  revoke_reason: string | null;
}

const shape = (r: Row): Invitation => ({
  id: r.id,
  email: r.email,
  name: r.display_name,
  roleIds: JSON.parse(r.role_ids) as string[],
  groupIds: JSON.parse(r.group_ids) as string[],
  lang: r.lang as Lang,
  invitedBy: r.invited_by,
  createdAt: r.created_at,
  expiresAt: r.expires_at,
  acceptedAt: r.accepted_at,
  acceptedUserId: r.accepted_user_id,
  revokedAt: r.revoked_at,
  revokedBy: r.revoked_by,
  revokeReason: r.revoke_reason,
});

export function statusOf(inv: Invitation, now: number): InviteStatus {
  if (inv.revokedAt) return "revoked";
  if (inv.acceptedAt) return "accepted";
  return Date.parse(inv.expiresAt) > now ? "pending" : "expired";
}

// AUTH_INVITE_HOURS, default 72: long enough for a weekend, short enough that
// a forgotten mail in someone's inbox stops being a key.
export const inviteSeconds = (env: Env): number => intVar(env.AUTH_INVITE_HOURS, 72) * 3600;

export interface NewInvitation {
  email: string;
  name: string;
  roleIds: string[];
  groupIds: string[];
  lang: Lang;
  invitedBy: string;
}

// createInvitation revokes any open invitation for the address ("replaced")
// and inserts the new one, in one batch. Returns it with its token.
export async function createInvitation(env: Env, n: NewInvitation, now: number): Promise<{ invitation: Invitation; token: string }> {
  const db = env.AUTH_DB;
  const id = crypto.randomUUID();
  const token = randomToken();
  await db.batch([
    db
      .prepare("UPDATE invitations SET revoked_at = ?, revoked_by = ?, revoke_reason = 'replaced' WHERE email = ? AND accepted_at IS NULL AND revoked_at IS NULL")
      .bind(iso(now), n.invitedBy, n.email),
    db
      .prepare(
        `INSERT INTO invitations (id, email, display_name, token_hash, role_ids, group_ids, lang, invited_by, created_at, expires_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .bind(id, n.email, n.name, await sha256hex(token), JSON.stringify(n.roleIds), JSON.stringify(n.groupIds), n.lang, n.invitedBy, iso(now), iso(now + inviteSeconds(env) * 1000)),
  ]);
  return { invitation: (await getInvitation(env, id)) as Invitation, token };
}

export async function getInvitation(env: Env, id: string): Promise<Invitation | null> {
  const row = await env.AUTH_DB.prepare("SELECT * FROM invitations WHERE id = ?").bind(id).first<Row>();
  return row ? shape(row) : null;
}

// openByToken finds the invitation a token opens: unknown, used, revoked and
// expired tokens alike are null.
export async function openByToken(env: Env, token: string, now: number): Promise<Invitation | null> {
  return openWhere(env, "token_hash", await sha256hex(token), now);
}

// openByHash is openByToken for a holder of the token's SHA-256 only (a
// login-provider flow, GO-151); openByEmail finds the open invitation for an
// address — used ONLY with an address a login provider asserts it verified.
export const openByHash = (env: Env, tokenHash: string, now: number): Promise<Invitation | null> => openWhere(env, "token_hash", tokenHash, now);
export const openByEmail = (env: Env, email: string, now: number): Promise<Invitation | null> => openWhere(env, "email", email, now);

async function openWhere(env: Env, column: "token_hash" | "email", value: string, now: number): Promise<Invitation | null> {
  const row = await env.AUTH_DB.prepare(
    `SELECT * FROM invitations WHERE ${column} = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ? ORDER BY created_at DESC LIMIT 1`,
  )
    .bind(value, iso(now))
    .first<Row>();
  return row ? shape(row) : null;
}

export interface Listed extends Invitation {
  status: InviteStatus;
  invitedByEmail: string | null;
}

// listInvitations: every invitation (or the one with `id`), newest first,
// with its status and the inviter's address (null once that account is gone).
export async function listInvitations(env: Env, now: number, id = ""): Promise<Listed[]> {
  const { results } = await env.AUTH_DB.prepare(
    `SELECT i.*, u.email AS inviter_email FROM invitations i LEFT JOIN users u ON u.id = i.invited_by
     WHERE ? = '' OR i.id = ? ORDER BY i.created_at DESC, i.id`,
  )
    .bind(id, id)
    .all<Row & { inviter_email: string | null }>();
  return results.map((r) => {
    const inv = shape(r);
    return { ...inv, status: statusOf(inv, now), invitedByEmail: r.inviter_email };
  });
}

// revokeInvitation withdraws an invitation that was not accepted. Revoking a
// revoked one again is not an error.
export async function revokeInvitation(env: Env, id: string, by: string, now: number): Promise<"ok" | "not_found" | "accepted"> {
  const inv = await getInvitation(env, id);
  if (!inv) return "not_found";
  if (inv.acceptedAt) return "accepted";
  await env.AUTH_DB.prepare("UPDATE invitations SET revoked_at = ?, revoked_by = ?, revoke_reason = 'revoked' WHERE id = ? AND revoked_at IS NULL")
    .bind(iso(now), by, id)
    .run();
  return "ok";
}

// renewToken gives a pending or expired invitation a new token and a new
// expiry; the old token stops working in the same statement.
export async function renewToken(env: Env, id: string, now: number): Promise<{ invitation: Invitation; token: string } | null> {
  const token = randomToken();
  const res = await env.AUTH_DB.prepare("UPDATE invitations SET token_hash = ?, expires_at = ? WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL")
    .bind(await sha256hex(token), iso(now + inviteSeconds(env) * 1000), id)
    .run();
  return res.meta.changes === 1 ? { invitation: (await getInvitation(env, id)) as Invitation, token } : null;
}
