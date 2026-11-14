// Helpers for the GO-141 tests: a service account with a role, a token for it
// through the admin API, and the exchange.
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { onRequestPost as createAccount } from "../functions/api/auth/admin/service-accounts/index.ts";
import { onRequestPost as createTokenRoute } from "../functions/api/auth/admin/service-accounts/[id]/tokens/index.ts";
import { onRequestPost as exchange } from "../functions/api/auth/token.ts";
import { Browser, T0, call } from "./fakes.ts";

export { exchange };

let roles = 0;

// account creates a service account as `browser` and grants it a role
// holding `perms` (none when empty); returns its id.
export async function account(env: Env, browser: Browser, perms: string[], name = `svc-${++roles}`): Promise<string> {
  const res = await call(createAccount, env, "/api/auth/admin/service-accounts", { json: { name }, browser });
  if (res.status !== 201) throw new Error(`service account: ${res.status} ${await res.text()}`);
  const id = ((await res.json()) as { serviceAccount: { id: string } }).serviceAccount.id;
  if (perms.length) {
    const role = (await createRole(env, { name: `svc-role-${++roles}`, description: "", permissions: perms }, T0)) as Role;
    await addGrant(env, role.id, { userId: id }, "test", T0);
  }
  return id;
}

// newToken posts a token for `id` as `browser`; returns status and body.
export async function newToken(env: Env, browser: Browser, id: string, json: Record<string, unknown> = {}) {
  const res = await call(createTokenRoute, env, `/api/auth/admin/service-accounts/${id}/tokens`, { json, browser, params: { id } });
  return { status: res.status, body: (await res.json()) as Record<string, any> };
}

// secretFor: a token's secret, or a failure thrown.
export async function secretFor(env: Env, browser: Browser, id: string, json: Record<string, unknown> = {}): Promise<string> {
  const r = await newToken(env, browser, id, json);
  if (r.status !== 201) throw new Error(`token: ${r.status} ${JSON.stringify(r.body)}`);
  return r.body.secret as string;
}

// swap posts to /api/auth/token with `authorization` (and any extra headers)
// from `ip`, with no CSRF headers at all — a script, not a page.
export function swap(env: Env, authorization: string | null, headers: Record<string, string> = {}, ip = "198.51.100.9"): Promise<Response> {
  const h: Record<string, string> = { "cf-connecting-ip": ip, ...headers };
  if (authorization !== null) h.authorization = authorization;
  return call(exchange, env, "/api/auth/token", { method: "POST", csrf: false, headers: h });
}

// claimsOf decodes a JWT's payload (no verification — the tests verify separately).
export const claimsOf = (jwt: string): Record<string, any> => JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString());
