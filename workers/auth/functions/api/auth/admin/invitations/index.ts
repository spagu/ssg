// Cloudflare Pages Function: invitations — the only way to a new account.
//   GET  /api/auth/admin/invitations   → 200 {items: [invitation], nextCursor: null}
//   POST /api/auth/admin/invitations   {email, name?, roleIds?, groupIds?, lang?, delivery?}
//        → 201 {invitation, delivery: "mail"} or {invitation, delivery: "link", link}
//
// Requires `auth:invite`. The inviter cannot hand out more than they hold: the
// union of the roles' permissions and the groups' permissions must be covered
// by the caller's (403 escalation {permissions}). 409 account_exists when the
// address has an account; an open invitation for the address is replaced.
// `lang` is the invitee's language (default: the inviter's page language).
// `delivery` — see _delivery.ts; the link is in the answer only for "link",
// once. 404 role_not_found / group_not_found; 400 invalid_request; 502
// mail_failed (the invitation stays, its link unsent: resend or revoke it).
// An invitation is {id, email, name, roleIds, groupIds, lang, status,
// invitedBy, invitedByEmail, createdAt, expiresAt, acceptedAt,
// acceptedUserId, revokedAt, revokedBy, revokeReason}; status is pending,
// accepted, expired or revoked. Roles or groups carrying `*` (owner) or
// `auth:*` need a recent second factor when the caller has one (GO-140,
// GO-138): 403 step_up_required. Audit: invitation_sent.

import { authorize, body, escalation, invalid, notFound, text } from "../../_admin";
import { adminEvent, handsOutAdmin, stepUpWhen } from "../../_admin-sensitive";
import { chooseDelivery, deliverInvitation } from "../../_delivery";
import { groupPermissions } from "../../_grants";
import { getGroup } from "../../_groups";
import { asLang, langOf } from "../../_i18n";
import { createInvitation, listInvitations } from "../../_invites";
import { json, looksLikeEmail, normEmail } from "../../_lib";
import type { Env } from "../../_lib";
import { beyond } from "../../_perms";
import { getRole } from "../../_roles";
import { findUserByEmail } from "../../_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:invite");
  if (caller instanceof Response) return caller;
  return json({ items: await listInvitations(env, caller.ctx.now), nextCursor: null });
};

const MAX_IDS = 20;

// ids reads an optional list of ids: absent = [], otherwise strings, de-duplicated.
function ids(raw: unknown): string[] | null {
  if (raw === undefined) return [];
  if (!Array.isArray(raw) || raw.length > MAX_IDS || raw.some((v) => typeof v !== "string" || !v)) return null;
  return [...new Set(raw as string[])];
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:invite");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const email = normEmail(b.email);
  if (!looksLikeEmail(email)) return invalid("email: an e-mail address");
  const name = text(b, "name", 200);
  if (name === null) return invalid("name: at most 200 characters");
  const roleIds = ids(b.roleIds);
  const groupIds = ids(b.groupIds);
  if (!roleIds || !groupIds) return invalid(`roleIds, groupIds: arrays of at most ${MAX_IDS} ids`);
  const lang = b.lang === undefined ? langOf(request) : asLang(b.lang);
  if (!lang) return invalid("lang: en, pl, hi, de or fr");
  const mode = chooseDelivery(env, b.delivery);
  if (mode instanceof Response) return mode;
  if (await findUserByEmail(env, email)) return json({ error: "account_exists" }, 409);

  const perms: string[] = [];
  for (const id of roleIds) {
    const role = await getRole(env, id);
    if (!role) return notFound("role");
    perms.push(...role.permissions);
  }
  for (const id of groupIds) {
    if (!(await getGroup(env, id))) return notFound("group");
    perms.push(...(await groupPermissions(env, id)));
  }
  const over = beyond(caller.perms, [...new Set(perms)].sort());
  if (over.length) return escalation(over);
  // GO-140, GO-138: an invitation that hands out `*` (owner) or `auth:*` needs a recent second factor.
  const stepUp = await stepUpWhen(caller, handsOutAdmin(perms));
  if (stepUp) return stepUp;

  const now = caller.ctx.now;
  const { invitation, token } = await createInvitation(env, { email, name: name ?? "", roleIds, groupIds, lang, invitedBy: caller.user.id }, now);
  const [view] = await listInvitations(env, now, invitation.id);
  const sent = await deliverInvitation(env, request, mode, invitation, token, caller.user);
  if (sent instanceof Response) return sent;
  await adminEvent(caller, "invitation_sent", null, { email, delivery: sent.delivery });
  return json({ invitation: view, ...sent }, 201);
};
