// Cloudflare Pages Function: reset another account's second factors (GO-140).
//   POST /api/auth/mfa/reset   JSON {userId}; header x-ssg-auth: 1 → 200 {ok: true, removed, revoked}
//
// For someone who lost their phone or key: every factor, enrolment and backup
// code of that account goes, its sessions end, and it enrols again at its
// next sign-in (_mfa-reset.ts). Requires `*` (an owner) and a recent second
// factor (step-up). Only ANOTHER account: 400 own_account (remove your own
// factors on the account page). 403 forbidden {permission: "*"} |
// step_up_required; 404 user_not_found; 401 not_authenticated; 404 not_found
// while the second factor is off. JSON only — the admin panel (GO-138) calls it.

import { authorize, body, invalid } from "../_admin";
import { json } from "../_lib";
import type { Env } from "../_lib";
import { mfaEnabled } from "../_mfa-crypto";
import { notFound } from "../_mfa-gate";
import { resetAccount } from "../_mfa-reset";
import { requireStepUp } from "../_mfa-step-up";
import { findUserById } from "../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  if (!mfaEnabled(env)) return notFound();
  if (!caller.perms.includes("*")) return json({ error: "forbidden", permission: "*" }, 403);
  const stepUp = await requireStepUp(caller.ctx, caller);
  if (stepUp) return stepUp;
  const b = await body(request);
  if (b instanceof Response) return b;
  if (typeof b.userId !== "string" || !b.userId) return invalid("userId: required");
  if (b.userId === caller.user.id) return json({ error: "own_account", detail: "remove your own factors on the account page" }, 400);
  const user = await findUserById(env, b.userId);
  if (!user) return json({ error: "user_not_found" }, 404);
  const done = await resetAccount(caller.ctx, user.id, { via: "owner", by: caller.user.id });
  return json({ ok: true, ...done });
};
