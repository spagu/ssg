// Per-action permissions (GO-137). The names are module.json's — the single
// list the `auth` worker shows when an owner builds a role — and every route in
// routes.ts names what it needs. In password and Access modes the caller holds
// everything, as before; in `auth` mode the session token's `perms` decide,
// through the portable verifier's own `can` (wildcards `newsletter:*` and `*`
// included), so every worker that adopts `auth` answers the same way.

import manifest from "../module.json" with { type: "json" };
import type { Admin } from "./auth";
import { can } from "./session-verifier";

// SESSION: signed in is enough (reading one's own identity). PUBLIC: no admin
// at all — the /hooks/ routes, which verify a provider's signature instead.
// Neither is a valid permission name (no colon), so they can never collide.
export const SESSION = "session";
export const PUBLIC = "public";

// Need is what a route requires: one permission, several (all of them), or a
// marker above.
export type Need = string | readonly string[];

// DECLARED: the permissions module.json declares, in its order.
export const DECLARED: readonly string[] = manifest.permissions.map((p) => p.name);

// needed lists the concrete permissions behind a Need (none for a marker).
export function needed(need: Need): string[] {
  const list = typeof need === "string" ? [need] : [...need];
  return list.filter((p) => p !== SESSION && p !== PUBLIC);
}

// missing names the first permission `admin` lacks for `need`, or null when the
// call may go ahead. A route without a declared need is refused in `auth` mode
// ("undeclared"): fail closed, never open by omission.
export function missing(admin: Admin, need: Need | undefined): string | null {
  if (admin.perms === "all") return null;
  if (need === undefined) return "undeclared";
  const perms = admin.perms;
  return needed(need).find((p) => !can(perms, p)) ?? null;
}

// held is what /api/me reports: "all" in password and Access modes, otherwise
// the declared permissions the token grants (wildcards expanded, other
// workers' permissions left out).
export function held(admin: Admin): string[] | "all" {
  const perms = admin.perms;
  return perms === "all" ? "all" : DECLARED.filter((p) => can(perms, p));
}
