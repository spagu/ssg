// Cloudflare Pages Function: service accounts (GO-141) — identities for CI,
// cron and scripts, which never sign in through the login page.
//   GET  /api/auth/admin/service-accounts   → 200 {items: [serviceAccount], nextCursor: null}
//   POST /api/auth/admin/service-accounts   {name, description?} → 201 {serviceAccount}
//
// Requires `auth:admin`. A service account is {id, name, description, status
// ("active" | "disabled"), createdBy, createdAt, updatedAt}; `name` a
// lower-case slug ([a-z][a-z0-9-]*, at most 64), `description` at most 500
// characters. Roles reach it through POST /api/auth/admin/grants with its id
// as `userId`. 400 invalid_request {detail}; 409 name_taken.

import { authorize, body, invalid, nameTaken, text } from "../../_admin";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { SERVICE_NAME, createServiceAccount, listServiceAccounts } from "../../_service-accounts";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json({ items: await listServiceAccounts(env), nextCursor: null });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const name = text(b, "name", 64);
  if (!name || !SERVICE_NAME.test(name)) return invalid("name: a lower-case slug, [a-z][a-z0-9-]*, at most 64 characters");
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  const account = await createServiceAccount(env, name, description ?? "", caller.user.id, caller.ctx.now);
  if (account === "name_taken") return nameTaken();
  return json({ serviceAccount: account }, 201);
};
