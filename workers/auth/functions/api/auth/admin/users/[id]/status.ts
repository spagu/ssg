// Cloudflare Pages Function: block or unblock an account (GO-138).
//   POST /api/auth/admin/users/:id/status   {status: "blocked" | "active"} → 200 {ok: true, status, revoked}
//
// Requires `auth:admin`. Blocking ends every session of the account at once
// (`revoked` says how many) and refuses every way in until it is unblocked;
// other workers accept an already-issued token until it expires (at most
// AUTH_TOKEN_MINUTES + 30 s). Never one's own account (400 own_account),
// never an account holding more than the caller (403 escalation), never the
// last active owner (409 owner_required, nothing changed). Blocking an
// account that holds `owner` is sensitive: a recent second factor (403
// step_up_required). 404 user_not_found; 400 invalid_request. Audit:
// account_blocked / account_unblocked.

import { authorize, body, escalation, invalid, notFound, ownerRequired, param } from "../../../_admin";
import { adminEvent, stepUpWhen } from "../../../_admin-sensitive";
import { effectivePermissions } from "../../../_grants";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { holdsOwner } from "../../../_mfa-reset";
import { beyond } from "../../../_perms";
import { revokeUserSessions } from "../../../_sessions";
import { findUserById, setUserStatus } from "../../../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const status = b.status;
  if (status !== "blocked" && status !== "active") return invalid("status: blocked or active");
  const user = await findUserById(env, param(params, "id"));
  if (!user) return notFound("user");
  if (user.id === caller.user.id) return json({ error: "own_account", detail: "you cannot block the account you are signed in with" }, 400);
  const over = beyond(caller.perms, await effectivePermissions(env, user.id));
  if (over.length) return escalation(over);
  const stepUp = await stepUpWhen(caller, status === "blocked" && (await holdsOwner(env, user.id)));
  if (stepUp) return stepUp;
  const now = caller.ctx.now;
  const done = await setUserStatus(env, user.id, status, now);
  if (done === "owner_required") return ownerRequired();
  if (done === "not_found") return notFound("user");
  const revoked = status === "blocked" ? await revokeUserSessions(env, user.id, "account_blocked", now) : 0;
  await adminEvent(caller, status === "blocked" ? "account_blocked" : "account_unblocked", user.id, { revoked });
  return json({ ok: true, status, revoked });
};
