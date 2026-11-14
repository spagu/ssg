// The registry of known permissions (GET /api/auth/permissions) as the role
// editor and the matrix use it: grouped by module, described in the viewer's
// language, wildcards and unknown names flagged. Pure functions.

import { can } from "./access.js";

// PERMISSION is the worker's rule (functions/api/auth/_can.ts validPermission):
// "*", "worker:*" or "worker:action", lower case.
const PERMISSION = /^(\*|[a-z][a-z0-9-]*:(\*|[a-z][a-z0-9-]*))$/;
export const validPermission = (p) => typeof p === "string" && p.length <= 64 && PERMISSION.test(p);

export const isWildcard = (p) => p === "*" || p.endsWith(":*");

// describe picks a permission's description in `lang`, then English, else null.
export function describe(item, lang) {
  const d = item && item.description;
  if (!d || typeof d !== "object") return null;
  return d[lang] || d.en || null;
}

// modulesOf groups the known permissions by module, modules and names
// sorted: [{id, permissions: [{name, description}]}].
export function modulesOf(items, lang) {
  const by = new Map();
  for (const item of items) {
    if (!item.known || !item.module) continue;
    if (!by.has(item.module)) by.set(item.module, []);
    by.get(item.module).push({ name: item.name, description: describe(item, lang) });
  }
  return [...by.keys()].sort().map((id) => ({ id, permissions: by.get(id).sort((a, b) => (a.name < b.name ? -1 : 1)) }));
}

// unknownOf: the names in `perms` no module declares (wildcards count as
// known when the module has any permission; "*" always).
export function unknownOf(perms, modules) {
  const names = modules.flatMap((m) => m.permissions.map((p) => p.name));
  return perms.filter((p) => p !== "*" && !(isWildcard(p) ? names.some((n) => n.startsWith(p.slice(0, -1))) : names.includes(p)));
}

// wildcardsIn: the wildcards a set holds — each one deserves a warning.
export const wildcardsIn = (perms) => perms.filter(isWildcard);

// toggle adds or removes one name, keeping the list sorted and unique.
export function toggle(perms, name, on) {
  const set = new Set(perms);
  if (on) set.add(name);
  else set.delete(name);
  return [...set].sort();
}

// covered: does `perms` reach `name` only through a wildcard it holds?
export const coveredByWildcard = (perms, name) => !perms.includes(name) && can(perms, name);

// prefixesOf: the worker names a module's permissions use ("newsletter" for
// the newsletter-ui module) — each gets a "<worker>:*" choice.
export const prefixesOf = (module) => [...new Set(module.permissions.map((p) => p.name.slice(0, p.name.indexOf(":"))))].sort();
