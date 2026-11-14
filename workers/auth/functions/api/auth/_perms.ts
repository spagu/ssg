// Permission sets as the admin API handles them: reading a list a client sent,
// and the escalation rule. A leading underscore keeps this file out of the
// Pages route table. The check itself (`can`) lives in _can.ts.

import { can, validPermission } from "./_can";

// MAX_ROLE_PERMISSIONS: more than this in one role is a mistake, not a model.
export const MAX_ROLE_PERMISSIONS = 64;

// covers tells whether someone holding `held` already holds everything `perm`
// allows: `*` only through `*`, `worker:*` through itself or `*`, a single
// permission through `can`.
export function covers(held: readonly string[], perm: string): boolean {
  if (held.includes("*")) return true;
  if (perm.endsWith("*")) return held.includes(perm);
  return can(held, perm);
}

// beyond lists the permissions in `perms` the holder of `held` does not
// cover — what an administrator would hand out without having it. Empty means
// the grant is allowed.
export const beyond = (held: readonly string[], perms: readonly string[]): string[] => perms.filter((p) => !covers(held, p));

export type Parsed = { ok: true; perms: string[] } | { ok: false; detail: string };

// parsePermissions reads a role's permission list from a request: an array of
// valid names or wildcards, returned de-duplicated and sorted.
export function parsePermissions(raw: unknown): Parsed {
  if (!Array.isArray(raw)) return { ok: false, detail: "permissions must be an array of strings" };
  const bad = raw.find((p) => !validPermission(p));
  if (bad !== undefined) return { ok: false, detail: `${JSON.stringify(bad)} is not a permission: use worker:action, worker:* or *` };
  const perms = [...new Set(raw as string[])].sort(byCodeUnit);
  if (perms.length > MAX_ROLE_PERMISSIONS) return { ok: false, detail: `at most ${MAX_ROLE_PERMISSIONS} permissions per role` };
  return { ok: true, perms };
}

// byCodeUnit orders strings by UTF-16 code units — what a bare .sort() does,
// made explicit (and locale-independent, unlike localeCompare): permission
// names, role names and ids must sort the same everywhere, every time.
export const byCodeUnit = (a: string, b: string): number => Number(a > b) - Number(a < b);
