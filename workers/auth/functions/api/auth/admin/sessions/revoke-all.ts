// Cloudflare Pages Function: end every session of every account — the
// break-glass button (GO-138).
//   POST /api/auth/admin/sessions/revoke-all   {confirm: "revoke-all"} → 200 {ok: true, revoked}
//
// Requires `auth:admin` and a recent second factor (403 step_up_required).
// Everyone must sign in again; the caller's own session is kept, so the
// panel that pressed the button stays usable (sign out afterwards to end it
// too). Other workers accept already-issued tokens until they expire (at
// most AUTH_TOKEN_MINUTES + 30 s). The body must name the action (400
// invalid_request otherwise) — a stray request cannot press it. Audit:
// sessions_revoked {scope: "everyone", revoked}.

import { authorize, body, invalid } from "../../_admin";
import { adminEvent, stepUpWhen } from "../../_admin-sensitive";
import { revokeEveryone } from "../../_admin-sessions";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  if (b.confirm !== "revoke-all") return invalid('confirm: "revoke-all"');
  const stepUp = await stepUpWhen(caller, true);
  if (stepUp) return stepUp;
  const revoked = await revokeEveryone(env, caller.session.id, caller.ctx.now);
  await adminEvent(caller, "sessions_revoked", null, { scope: "everyone", revoked });
  return json({ ok: true, revoked });
};
