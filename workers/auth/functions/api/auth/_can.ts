// Permission names and the one check every worker makes. A leading underscore
// keeps this file out of the Pages route table.
//
// Self-contained on purpose: no imports. GO-135 copies `can` byte for byte
// into the portable verifier that other workers carry, so it must not grow a
// dependency.
//
// A permission is `worker:action`, lower case — `newsletter:send`. A role may
// also hold a trailing wildcard: `newsletter:*` (every action of one worker)
// or `*` (everything). Nothing else is a pattern: `*:erase`, `newsletter:*x`,
// `news*`, spaces, upper case and empty parts are not permissions at all.

const PERMISSION = /^(?:\*|[a-z][a-z0-9-]*:(?:\*|[a-z][a-z0-9-]*))$/;

// validPermission: a well-formed name or wildcard of at most 64 characters
// (the manifest format's limit for a name).
export function validPermission(s: unknown): s is string {
  return typeof s === "string" && s.length <= 64 && PERMISSION.test(s);
}

// can tells whether a holder of `perms` may do `required`. `required` is one
// concrete permission (never a wildcard — asking "may they do everything?"
// is a different question, and answers false here). Only three entries can
// allow it: the exact name, its worker's `worker:*`, or `*`; every other
// string, however close, compares unequal.
export function can(perms: readonly string[], required: string): boolean {
  if (!Array.isArray(perms) || !validPermission(required) || required.endsWith("*")) return false;
  const all = required.slice(0, required.indexOf(":") + 1) + "*";
  return perms.some((p) => p === required || p === all || p === "*");
}
