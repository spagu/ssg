// Roles and groups from an external issuer's roles claim (#435). A leading
// underscore keeps this file out of the Pages route table.
//
// Only with AUTH_JWT_ROLE_MAP. At each sign-in the person's SYNCHRONISED
// grants and memberships are made to match the token's roles claim
// (AUTH_JWT_ROLES_CLAIM) through the mapping: a mapped external role gives
// its roles and groups; one the token no longer carries takes back what the
// synchronisation gave (_synced.ts — never a grant or membership an
// administrator made by hand). A token without the claim, or with nothing
// that matches, gets nothing from the mapping. Nothing is ever given that the
// mapping does not name:
//   - the owner role cannot be mapped (_external-config.ts refuses it), and a
//     mapped group that holds the owner role is skipped (logged): a token
//     never makes an owner;
//   - a name that is not a role or a group here is skipped (logged);
//   - a mapped role that holds administrative permissions is given — the
//     mapping says so explicitly (docs/EXTERNAL-JWT.md warns about it).
// Without a mapping, roles are left to the administrator, as for any account.

import { emitEvent } from "./_audit";
import type { Claims } from "./_external-jwt";
import type { ExternalSetup, RoleTargets } from "./_external-config";
import { rolesClaim } from "./_external-identity";
import type { Env } from "./_lib";
import { OWNER_ROLE_NAME } from "./_rbac-schema";
import { syncMarked } from "./_synced";
import type { Marks } from "./_synced";

const ROLE_MARKS: Marks = { table: "jwt_synced_grants", col: "role_id", label: "external role sync" };
const GROUP_MARKS: Marks = { table: "jwt_synced_members", col: "group_id", label: "external role sync" };

type Named = { id: string; name: string };

async function named(env: Env, table: "roles" | "groups", names: Set<string>): Promise<Named[]> {
  const { results } = await env.AUTH_DB.prepare(`SELECT id, name FROM ${table} WHERE name IN (SELECT value FROM json_each(?)) ORDER BY name`)
    .bind(JSON.stringify([...names]))
    .all<Named>();
  const found = new Set(results.map((r) => r.name));
  for (const n of names) if (!found.has(n)) console.warn(`auth: AUTH_JWT_ROLE_MAP names ${table === "roles" ? "a role" : "a group"} "${n.slice(0, 64)}" that does not exist here; skipped`);
  return results;
}

// parseRoleMap reads AUTH_JWT_ROLE_MAP: null when unset, the problem as a
// string when it is not usable (the owner role included).
export function parseRoleMap(raw: string): Map<string, RoleTargets> | null | string {
  if (!raw) return null;
  const bad = 'AUTH_JWT_ROLE_MAP must be a JSON object {"<external role>": {"roles": ["<role>"], "groups": ["<group>"]}}';
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return bad;
  }
  if (!v || typeof v !== "object" || Array.isArray(v)) return bad;
  const out = new Map<string, RoleTargets>();
  const names = (x: unknown): string[] | null => (x === undefined ? [] : Array.isArray(x) && x.every((n) => typeof n === "string" && n) ? (x as string[]) : null);
  for (const [external, targets] of Object.entries(v)) {
    if (!targets || typeof targets !== "object" || Array.isArray(targets)) return bad;
    const t = targets as Record<string, unknown>;
    const roles = names(t.roles);
    const groups = names(t.groups);
    if (!roles || !groups || Object.keys(t).some((k) => k !== "roles" && k !== "groups")) return bad;
    if (roles.includes(OWNER_ROLE_NAME)) return `AUTH_JWT_ROLE_MAP maps "${external.slice(0, 64)}" to the owner role; the owner role is never given by a token`;
    out.set(external, { roles, groups });
  }
  return out;
}

// wanted: the role and group ids the token's roles earn through the mapping.
async function wanted(env: Env, map: NonNullable<ExternalSetup["roleMap"]>, external: string[]): Promise<{ roles: Map<string, string>; groups: Map<string, string> }> {
  const roleNames = new Set<string>();
  const groupNames = new Set<string>();
  for (const e of external) {
    const t = map.get(e);
    t?.roles.forEach((r) => roleNames.add(r));
    t?.groups.forEach((g) => groupNames.add(g));
  }
  const roles = await named(env, "roles", roleNames);
  const groups = await named(env, "groups", groupNames);
  const { results: owners } = await env.AUTH_DB.prepare(
    "SELECT DISTINCT g.group_id FROM grants g JOIN roles r ON r.id = g.role_id WHERE r.name = ? AND g.group_id IS NOT NULL",
  )
    .bind(OWNER_ROLE_NAME)
    .all<{ group_id: string }>();
  const ownerGroups = new Set(owners.map((o) => o.group_id));
  const safe = groups.filter((g) => {
    if (ownerGroups.has(g.id)) console.warn(`auth: AUTH_JWT_ROLE_MAP names the group "${g.name.slice(0, 64)}", which holds the owner role; skipped`);
    return !ownerGroups.has(g.id);
  });
  return { roles: new Map(roles.map((r) => [r.id, r.name])), groups: new Map(safe.map((g) => [g.id, g.name])) };
}

// applyExternalRoles synchronises one person; an audit event when anything
// changed (or was kept by the owner invariant).
export async function applyExternalRoles(env: Env, s: ExternalSetup, userId: string, claims: Claims, now: number): Promise<void> {
  if (!s.roleMap) return;
  const w = await wanted(env, s.roleMap, rolesClaim(claims, s.rolesClaim));
  const roles = await syncMarked(env, ROLE_MARKS, userId, w.roles, now);
  const groups = await syncMarked(env, GROUP_MARKS, userId, w.groups, now);
  const changed = [roles.added, roles.removed, groups.added, groups.removed, groups.kept].some((l) => l.length);
  if (!changed) return;
  const detail = {
    rolesAdded: roles.added.join(", "),
    rolesRemoved: roles.removed.join(", "),
    groupsAdded: groups.added.join(", "),
    groupsRemoved: groups.removed.join(", "),
    keptForOwner: groups.kept.join(", "),
  };
  await emitEvent(env, { kind: "roles_synced", userId, method: s.method, detail }, now);
}
