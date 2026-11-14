// Cloudflare Pages Function: send an invitation again, with a new link.
//   POST /api/auth/admin/invitations/:id/resend   {delivery?} (or no body)
//        → 200 {invitation, delivery: "mail"} or {invitation, delivery: "link", link}
//
// Requires `auth:invite`. A pending or expired invitation gets a new token
// and a new expiry; the old link is dead from the same statement on. The
// caller must cover the invitation's roles and groups as they are now (403
// escalation — the one resending may hold less than the one who invited).
// 409 invitation_closed (accepted or withdrawn), 409 account_exists (the
// address got an account meanwhile), 404 invitation_not_found; delivery and
// 502 mail_failed as for POST /api/auth/admin/invitations. An invitation
// carrying `*` or `auth:*` needs a recent second factor, as when it was sent
// (403 step_up_required, SR-05).

import { authorize, escalation, notFound, param } from "../../../_admin";
import { handsOutAdmin, stepUpWhen } from "../../../_admin-sensitive";
import { chooseDelivery, deliverInvitation } from "../../../_delivery";
import { groupPermissions } from "../../../_grants";
import { getInvitation, listInvitations, renewToken } from "../../../_invites";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { beyond } from "../../../_perms";
import { readBody } from "../../../_request";
import { getRole } from "../../../_roles";
import { findUserByEmail } from "../../../_users";
import { byCodeUnit } from "../../../_perms";

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:invite");
  if (caller instanceof Response) return caller;
  const b = (await readBody(request)) ?? {}; // the body is optional
  const inv = await getInvitation(env, param(params, "id"));
  if (!inv) return notFound("invitation");
  if (inv.acceptedAt || inv.revokedAt) return json({ error: "invitation_closed" }, 409);
  const mode = chooseDelivery(env, b.delivery);
  if (mode instanceof Response) return mode;
  if (await findUserByEmail(env, inv.email)) return json({ error: "account_exists" }, 409);
  const perms: string[] = [];
  for (const id of inv.roleIds) perms.push(...((await getRole(env, id))?.permissions ?? []));
  for (const id of inv.groupIds) perms.push(...(await groupPermissions(env, id)));
  const over = beyond(caller.perms, [...new Set(perms)].sort(byCodeUnit));
  if (over.length) return escalation(over);
  const stepUp = await stepUpWhen(caller, handsOutAdmin(perms));
  if (stepUp) return stepUp;

  const renewed = await renewToken(env, inv.id, caller.ctx.now);
  if (!renewed) return json({ error: "invitation_closed" }, 409);
  const [view] = await listInvitations(env, caller.ctx.now, inv.id);
  const sent = await deliverInvitation(env, request, mode, renewed.invitation, renewed.token, caller.user);
  if (sent instanceof Response) return sent;
  return json({ invitation: view, ...sent });
};
