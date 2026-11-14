// The gate in front of every permission-checked route, and the small readers
// those routes share. A leading underscore keeps this file out of the Pages
// route table.
//
// Order: configuration (503) → for anything but GET the CSRF defence (403,
// JSON only, never a plain form) → a valid access token whose session is live
// and whose account is active (401) → the permission, checked with `can`
// against the token's own `perms` (403 naming the missing permission). The
// token, not the database, is the authority: a grant that changed reaches this
// check at the caller's next refresh, like every other worker (README
// "Freshness"). Revoking the caller's sessions or blocking the account stops
// the caller here at once, because the session is looked up every time.

import { can } from "./_can";
import { prepare, preparePost, whoAmI } from "./_context";
import type { Ctx, Who } from "./_context";
import { permissionSources } from "./_grants";
import type { AccessClaims } from "./_jwt";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { isKnown, registryOf, unknownOf } from "./_registry";
import { readBody } from "./_request";
import type { Body } from "./_request";
import type { Role } from "./_roles";

export interface Caller extends Who {
  ctx: Ctx;
  perms: string[];
}

// authorize returns the caller, or the response to send. `required` null =
// any signed-in account.
export async function authorize(request: Request, env: Env, required: string | null): Promise<Caller | Response> {
  const ctx = request.method === "GET" ? await prepare(request, env) : await preparePost(request, env, false);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, false);
  if (!who) return json({ error: "not_authenticated" }, 401);
  // whoAmI without the refresh path finds a session only through the token.
  const perms = (who.claims as AccessClaims).perms;
  if (required && !can(perms, required)) return json({ error: "forbidden", permission: required }, 403);
  return { ...who, ctx, perms };
}

// param reads one route parameter ([id].ts) as a string.
export function param(params: Record<string, string | string[]> | undefined, name: string): string {
  const v = params?.[name];
  return typeof v === "string" ? v : "";
}

// body reads a JSON object, or answers 400.
export async function body(request: Request): Promise<Body | Response> {
  return (await readBody(request)) ?? json({ error: "invalid_request", detail: "expected a JSON object" }, 400);
}

// text reads an optional string field: undefined when absent, null when
// present but not a string or longer than max.
export function text(b: Body, field: string, max: number): string | undefined | null {
  if (!(field in b)) return undefined;
  const v = b[field];
  return typeof v === "string" && [...v].length <= max ? v.trim() : null;
}

export const invalid = (detail: string): Response => json({ error: "invalid_request", detail }, 400);
export const notFound = (what: string): Response => json({ error: `${what}_not_found` }, 404);
export const nameTaken = (): Response => json({ error: "name_taken" }, 409);

// ownerRequired: the database refused because no active owner would be left.
export const ownerRequired = (): Response =>
  json({ error: "owner_required", detail: "this change would leave no active account holding the owner role; grant owner to someone else first" }, 409);

// escalation: the caller tried to hand out permissions they do not hold.
export const escalation = (perms: string[]): Response =>
  json({ error: "escalation", detail: "you cannot grant permissions you do not hold yourself (unless you hold *)", permissions: perms }, 403);

// permissionsAnswer is one user's effective permissions with provenance, each
// flagged known or unknown against the registry — what both
// GET /api/auth/me/permissions and the admin view of a user answer.
export async function permissionsAnswer(env: Env, userId: string) {
  const reg = registryOf(env);
  const list = await permissionSources(env, userId);
  return { userId, permissions: list.map((p) => ({ name: p.name, known: isKnown(reg, p.name), sources: p.sources })) };
}

// roleAnswer is a role as the API shows it: its permissions and, flagged, the
// ones no module declares.
export const roleAnswer = (env: Env, role: Role) => ({ ...role, unknownPermissions: unknownOf(registryOf(env), role.permissions) });
