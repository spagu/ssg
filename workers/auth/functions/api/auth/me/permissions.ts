// Cloudflare Pages Function: my own effective permissions, and where each
// comes from.
//   GET /api/auth/me/permissions   any signed-in account
//
// 200 {userId, permissions: [{name, known, sources: [{roleId, roleName, via:
// "user"} | {roleId, roleName, via: "group", groupId, groupName}]}], token:
// [string]} — `permissions` is what the grants say now (what the next refresh
// puts in the token), `token` is what the current access token carries. They
// differ for at most AUTH_TOKEN_MINUTES after a change. 401 without a session.
// GO-140: when roles are left out because this session has not passed a
// second factor, `secondFactor: {roles, hasFactor, url}` says which and where
// to get it (/auth/mfa/enrol without a factor, /auth/mfa/step-up with one);
// absent otherwise.

import { authorize, permissionsAnswer } from "../_admin";
import { json } from "../_lib";
import { hasFactor } from "../_mfa-factors";
import { sessionFactor } from "../_mfa-policy";
import { ENROL_PAGE, STEP_UP_PAGE } from "../_mfa-signin";
import { policyView } from "../_provider-policy";
import type { Env } from "../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  const passed = (await sessionFactor(env, caller.session.id)) !== null;
  const roles = (await policyView(env, caller.user.id, "", passed)).needsFactor;
  const has = roles.length ? await hasFactor(env, caller.user.id) : false;
  const secondFactor = roles.length ? { secondFactor: { roles, hasFactor: has, url: has ? STEP_UP_PAGE : ENROL_PAGE } } : {};
  return json({ ...(await permissionsAnswer(env, caller.user.id)), token: caller.perms, ...secondFactor });
};
