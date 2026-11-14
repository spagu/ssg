// Cloudflare Pages Function: how this worker is set up (GO-138).
//   GET /api/auth/admin/status   → 200 {keys, rotationDays, tokens, hashing, signIn, access, mfa, mail, audit, invitations, modules, registryProblems}
//
// Requires `auth:admin`. Never a secret's value — only whether one is set
// and what follows from it:
//   keys        [{kid, current, firstUsedAt, ageDays, rotate}] — the published
//               key ids; age counted from the first token this worker signed
//               with the key; `rotate` past rotationDays (365)
//   tokens      {accessMinutes, stalenessSeconds (lifetime + 30 s leeway: the
//               longest a permission change waits), sessionDays, serviceAccessMinutes}
//   hashing     {scheme, iterations, passwordMin, passwordMax}
//   signIn      {methods: ["password", "github", "access", …], password}
//   access      absent | enabled | misconfigured
//   mfa         {state: off | ready | misconfigured, stepUpMinutes, policy:
//               default | custom | invalid, rolesNeedingFactor, accountsMissingFactor}
//   mail        {state: none | ready | broken, transport?, problem?}
//   audit       {retentionDays}; invitations {pending}
//   modules     [{id, permissions}] from the registry; registryProblems [string]

import { authorize } from "../_admin";
import { adminStatus } from "../_admin-status";
import { json } from "../_lib";
import type { Env } from "../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json(await adminStatus(caller.ctx));
};
