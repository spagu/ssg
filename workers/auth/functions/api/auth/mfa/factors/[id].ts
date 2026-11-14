// Cloudflare Pages Function: remove one of my second factors (GO-140).
//   DELETE /api/auth/mfa/factors/:id   header x-ssg-auth: 1 → 200 {ok: true}
//   POST   /api/auth/mfa/factors/:id   the same from the account page's plain form
//
// Needs a recent second factor (step-up). The LAST factor can go only when no
// role the account holds needs one: otherwise 409 factor_required {roles}
// (remove the role first, or add another factor). With the last factor go
// the backup codes. Someone else's factor, an unknown id and a removed one
// are all 404 factor_not_found. A form gets a 303 to /auth/account with
// ?ok=factor_removed or ?error=<code>. 401; 403 step_up_required; 404
// not_found while the second factor is off.

import { param } from "../../_admin";
import { emitEvent } from "../../_audit";
import type { Env } from "../../_lib";
import { listFactors, removeFactor } from "../../_mfa-factors";
import { mfaGate } from "../../_mfa-gate";
import { policyView } from "../../_provider-policy";
import { json, redirect } from "../../_lib";
import { withParams } from "../../_next";

const remove: PagesFunction<Env> = async ({ request, env, params }) => {
  const g = await mfaGate(request, env, true);
  if (g instanceof Response) return g;
  const { ctx, who } = g;
  const id = param(params, "id");
  const factors = await listFactors(env, who.user.id);
  const factor = factors.find((f) => f.id === id);
  if (!factor) return g.fail("factor_not_found", 404);
  const roles = (await policyView(env, who.user.id, "", false)).needsFactor;
  if (factors.length === 1 && roles.length) return g.fail("factor_required", 409, { roles });
  // The "keep one" rule is checked again by the delete itself (SR-13): a
  // parallel removal may have taken the other factor since the list above.
  if (!(await removeFactor(env, who.user.id, id, roles.length > 0))) {
    return (await listFactors(env, who.user.id)).some((f) => f.id === id) ? g.fail("factor_required", 409, { roles }) : g.fail("factor_not_found", 404);
  }
  await emitEvent(env, { kind: "mfa_removed", userId: who.user.id, detail: { factor: factor.kind, factorId: id, last: factors.length === 1 } }, ctx.now);
  return g.form ? redirect(withParams(g.back, { ok: "factor_removed" })) : json({ ok: true });
};

export const onRequestDelete = remove;
export const onRequestPost = remove;
