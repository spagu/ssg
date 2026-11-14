// Cloudflare Pages Function: one service account (GO-141).
//   GET    /api/auth/admin/service-accounts/:id   → 200 {serviceAccount, permissions: [{name, known, sources}], tokens: [token]}
//   PUT    /api/auth/admin/service-accounts/:id   {name?, description?, status?} → 200 {serviceAccount}
//   DELETE /api/auth/admin/service-accounts/:id   → 200 {ok: true} (its grants and tokens go with it)
//
// Requires `auth:admin`; PUT and DELETE never on an account holding more than
// the caller (403 escalation, SR-04). 404 service_account_not_found. `status` "disabled"
// stops every exchange of the account's tokens at once; access tokens already
// issued die within AUTH_SERVICE_ACCESS_MINUTES (+30 s leeway). `permissions`
// are the account's (from its grants) — what a token narrows; a token's own
// effective list is computed at each exchange.

import { authorize, body, invalid, nameTaken, notFound, ownerRequired, param, permissionsAnswer, text } from "../../../_admin";
import { mayActOn } from "../../../_admin-sensitive";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { SERVICE_NAME, deleteServiceAccount, getServiceAccount, updateServiceAccount } from "../../../_service-accounts";
import type { ServiceChange } from "../../../_service-accounts";
import { listTokens } from "../../../_service-tokens";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const account = await getServiceAccount(env, param(params, "id"));
  if (!account) return notFound("service_account");
  const { permissions } = await permissionsAnswer(env, account.id);
  return json({ serviceAccount: account, permissions, tokens: await listTokens(env, account.id, caller.ctx.now) });
};

export const onRequestPut: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  // SR-04: never an account holding more than the caller.
  const refused = await mayActOn(caller, param(params, "id"));
  if (refused) return refused;
  const patch: ServiceChange = {};
  const name = text(b, "name", 64);
  if (name !== undefined) {
    if (!name || !SERVICE_NAME.test(name)) return invalid("name: a lower-case slug, [a-z][a-z0-9-]*, at most 64 characters");
    patch.name = name;
  }
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  if (description !== undefined) patch.description = description;
  if ("status" in b) {
    if (b.status !== "active" && b.status !== "disabled") return invalid('status: "active" or "disabled"');
    patch.status = b.status;
  }
  const account = await updateServiceAccount(env, param(params, "id"), patch, caller.ctx.now);
  if (account === "not_found") return notFound("service_account");
  if (account === "name_taken") return nameTaken();
  return json({ serviceAccount: account });
};

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const refused = await mayActOn(caller, param(params, "id"));
  if (refused) return refused;
  const r = await deleteServiceAccount(env, param(params, "id"));
  if (r === "not_found") return notFound("service_account");
  if (r === "owner_required") return ownerRequired();
  return json({ ok: true });
};
