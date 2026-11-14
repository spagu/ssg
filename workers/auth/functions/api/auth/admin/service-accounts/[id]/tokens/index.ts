// Cloudflare Pages Function: the tokens of one service account (GO-141).
//   GET  /api/auth/admin/service-accounts/:id/tokens   → 200 {items: [token], nextCursor: null}
//   POST /api/auth/admin/service-accounts/:id/tokens
//        {name?, expiresInDays?, permissions?, allowSensitive?} → 201 {token, secret}
//
// Requires `auth:admin`. `secret` (`ssgsvc_<id>_<random>`) is in this one
// answer and nowhere else — not in the database, not in a list, not in a log.
// A token is {id, serviceAccountId, name, display, permissions (null = the
// account's), allowSensitive, createdBy, createdAt, expiresAt, lastUsedAt,
// revokedAt, status}.
//
//   expiresInDays   1 … AUTH_SERVICE_TOKEN_MAX_DAYS (default 365); default 90
//   permissions     narrows the account's: the token gets the intersection
//   allowSensitive  sensitive permissions (_service-perms.ts: *, auth:…,
//                   …:erase, worker:*) the token may carry; only a holder of
//                   `*` — an owner — may set it (403 sensitive_needs_owner),
//                   with a recent second factor when the owner has one
//                   (GO-140: 403 step_up_required)
//
// 403 escalation {permissions} when what the token could carry — its list, or
// without one the account's current permissions, plus allowSensitive — goes
// beyond the caller's own (unless the caller holds *). 404
// service_account_not_found; 400 invalid_request {detail}.

import { authorize, body, escalation, invalid, notFound, param, text } from "../../../../_admin";
import { adminEvent } from "../../../../_admin-sensitive";
import { effectivePermissions } from "../../../../_grants";
import { json } from "../../../../_lib";
import type { Env } from "../../../../_lib";
import { beyond, parsePermissions } from "../../../../_perms";
import { requireStepUp } from "../../../../_mfa-step-up";
import { getServiceAccount } from "../../../../_service-accounts";
import { createToken, listTokens, serviceSettings } from "../../../../_service-tokens";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const account = await getServiceAccount(env, param(params, "id"));
  if (!account) return notFound("service_account");
  return json({ items: await listTokens(env, account.id, caller.ctx.now), nextCursor: null });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const account = await getServiceAccount(env, param(params, "id"));
  if (!account) return notFound("service_account");
  const name = text(b, "name", 100);
  if (name === null) return invalid("name: a string of at most 100 characters");
  const ss = serviceSettings(env);
  const days = b.expiresInDays ?? ss.defaultDays;
  if (!Number.isInteger(days) || (days as number) < 1 || (days as number) > ss.maxDays) {
    return invalid(`expiresInDays: a whole number of days from 1 to ${ss.maxDays}`);
  }
  let list: string[] | null = null;
  if (b.permissions !== undefined && b.permissions !== null) {
    const p = parsePermissions(b.permissions);
    if (!p.ok) return invalid(`permissions: ${p.detail}`);
    list = p.perms;
  }
  const sensitive = parsePermissions(b.allowSensitive ?? []);
  if (!sensitive.ok) return invalid(`allowSensitive: ${sensitive.detail}`);
  if (sensitive.perms.length && !caller.perms.includes("*")) {
    return json({ error: "sensitive_needs_owner", detail: "only an owner (a holder of *) may let a token carry sensitive permissions" }, 403);
  }
  // GO-140: and a recent second factor, when the owner has one.
  const stepUp = sensitive.perms.length ? await requireStepUp(caller.ctx, caller) : null;
  if (stepUp) return stepUp;
  const reach = list ?? (await effectivePermissions(env, account.id));
  const over = beyond(caller.perms, [...new Set([...reach, ...sensitive.perms])]);
  if (over.length) return escalation(over);
  const created = await createToken(env, account.id, { name: name ?? "", days: days as number, permissions: list, allowSensitive: sensitive.perms }, caller.user.id, caller.ctx.now);
  // GO-138: audit service_token_created — its public id, never the secret.
  await adminEvent(caller, "service_token_created", null, { serviceAccount: account.name, token: created.token.display });
  return json(created, 201);
};
