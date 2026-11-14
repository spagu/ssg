// What a service token may carry (GO-141): the intersection of its account's
// permissions with the token's own narrowing list, minus the (machine-)
// sensitive ones an owner did not allow. A leading underscore keeps this file
// out of the Pages route table. Pure functions — no database, no clock.

import { covers } from "./_perms";

// SENSITIVE is the one definition of what a machine never gets by default:
//   `*`             everything, in every worker;
//   `auth:…`        any permission of `auth` itself — who may do what;
//   `<worker>:erase` deleting people's data;
//   `<worker>:*`    a worker wildcard: it covers that worker's erase (and any
//                   action the worker adds later), so it is sensitive too —
//                   name the concrete permissions a script needs instead.
// This is the fixed part. What a machine never gets by default is wider —
// machineSensitive below adds the second-factor policy (GO-140) — but this
// list stays as it is: it is also the core of that policy's default
// (sensitiveForPeople, _mfa-policy.ts) and of Access provisioning's check.
export function isSensitive(perm: string): boolean {
  return perm === "*" || perm.startsWith("auth:") || perm.endsWith(":erase") || perm.endsWith(":*");
}

// machineSensitive: may a service token carry `perm` only when its
// allowSensitive covers it? Yes when isSensitive flags it, or when the account
// holds it only through roles that would need a second factor for a person
// (`free` = the account's permissions given by at least one role that does
// not; _service-policy.ts computes it). A machine can never present a second
// factor, so whatever people may use only with one, a machine may use only
// with an owner's explicit allowance.
export const machineSensitive = (perm: string, free: readonly string[]): boolean => isSensitive(perm) || !covers(free, perm);

// minimal drops an entry another entry already covers (newsletter:read next
// to newsletter:*), sorted: the smallest list that means the same.
function minimal(perms: Iterable<string>): string[] {
  const all = [...new Set(perms)];
  return all.filter((p) => !all.some((q) => q !== p && covers([q], p))).sort();
}

// both lists every entry of one list that the other covers — the
// intersection before tidying, so no concrete name is folded into a wildcard
// that the sensitive filter may remove afterwards.
function both(a: readonly string[], b: readonly string[]): string[] {
  return [...a.filter((p) => covers(b, p)), ...b.filter((p) => covers(a, p))];
}

// intersect is what both lists allow, wildcards honoured on both sides:
// [newsletter:*] ∩ [newsletter:read] = [newsletter:read]; [*] ∩ [x:*] = [x:*].
// Every entry of the result is an entry of one list that the other list
// covers, so the result never allows more than either.
export const intersect = (a: readonly string[], b: readonly string[]): string[] => minimal(both(a, b));

// tokenPermissions is what goes into a service access token, computed at each
// exchange from the account's permissions as they are then: narrowed by the
// token's list (null = no narrowing), then the machine-sensitive ones removed
// unless `allowSensitive` (set by an owner at creation) covers them. `free`
// defaults to the whole account — no second-factor policy, only isSensitive;
// the exchange always passes it. Tidied last: an account holding both `*` and
// `web-push:notify` keeps the latter.
export function tokenPermissions(account: readonly string[], list: readonly string[] | null, allowSensitive: readonly string[], free: readonly string[] = account): string[] {
  const narrowed = list === null ? account : both(account, list);
  return minimal(narrowed.filter((p) => !machineSensitive(p, free) || covers(allowSensitive, p)));
}
