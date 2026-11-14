// Cloudflare Pages Function: my second factors (GO-140).
//   GET /api/auth/mfa/factors → 200 {enabled, state, factors: [{id, kind, name,
//       createdAt, lastUsedAt}], backupCodesLeft, requiredBy: [role], sessionFactor:
//       {factor, verifiedAt} | null, stepUpMinutes}
//
// `state` is off | ready | misconfigured (AUTH_MFA_KEY); `requiredBy` the
// roles the account holds that need a second factor; `sessionFactor` what
// this session passed. Never a secret, a key or a code. 401
// not_authenticated without a valid access token.

import { authorize } from "../../_admin";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { mfaState } from "../../_mfa-crypto";
import { backupCodesLeft, listFactors, view } from "../../_mfa-factors";
import { sessionFactor, stepUpMinutes } from "../../_mfa-policy";
import { policyView } from "../../_provider-policy";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  const state = mfaState(env);
  const f = await sessionFactor(env, caller.session.id);
  return json({
    enabled: state !== "off",
    state,
    factors: (await listFactors(env, caller.user.id)).map(view),
    backupCodesLeft: await backupCodesLeft(env, caller.user.id),
    requiredBy: (await policyView(env, caller.user.id, "", false)).needsFactor,
    sessionFactor: f ? { factor: f.factor, verifiedAt: f.verified_at } : null,
    stepUpMinutes: stepUpMinutes(env),
  });
};
