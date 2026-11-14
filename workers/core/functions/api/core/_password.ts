// The shared-password sign-in (HTTP Basic, CORE_ADMIN_PASSWORD). A leading
// underscore keeps this file out of the Pages route table.
//
// Two guards, both checked BEFORE the password is compared — a locked address
// gets 429 even for the right password, otherwise a lockout would only change
// the error code of a wrong guess and slow nobody down. A burst of parallel
// guesses all pass that check before any failure is counted (security review
// SR-12), so the answer is decided after the comparison: a wrong guess whose
// count is beyond the limit gets 429 too (it learns nothing a locked attempt
// would not), and the right password is accepted only while the lock is still
// clear after the comparison. Nothing is written for a success but the clear,
// as before — every request in this mode carries the password:
//   1. CORE_LOGIN_LIMITER, a Workers rate-limit binding, when bound. It cannot
//      answer "is this key over?" without counting, so it counts every request
//      that carries a password — size it for normal use (README), it is a
//      ceiling across isolates, not a failure counter;
//   2. the failed-login lockout (_lockout.ts): CORE_LOGIN_MAX_FAILURES within
//      CORE_LOGIN_WINDOW_MINUTES (default 10 in 10) lock the address for one
//      window. With CORE_DB bound the count lives there and holds across
//      isolates (_lockout-d1.ts); without it, in this isolate's memory — best
//      effort (see _lockout.ts).

import { memoryLockout } from "./_lockout";
import { durableLockout } from "./_lockout-d1";
import type { LockoutStore } from "./_lockout";
import { clientIP, intVar, problem, timingSafeEqual } from "./_lib";
import type { Env } from "./_lib";

export const REALM = 'Basic realm="core-admin", charset="UTF-8"';

let store: LockoutStore = memoryLockout();

// useLockoutStore swaps the in-memory lockout's storage and returns the
// previous one (tests). With CORE_DB bound, the durable store is used instead,
// chosen per request from the environment (lockout below).
export function useLockoutStore(next: LockoutStore): LockoutStore {
  const previous = store;
  store = next;
  return previous;
}

export type PasswordResult = { ok: true } | { ok: false; response: Response };

// lockout: CORE_DB's store when bound, else the in-memory one.
const lockout = (env: Env): LockoutStore => (env.CORE_DB ? durableLockout(env.CORE_DB) : store);

const unauthorized = (): PasswordResult => ({ ok: false, response: problem("unauthorized", 401, undefined, { "www-authenticate": REALM }) });
const tooMany = (seconds: number): PasswordResult => ({
  ok: false,
  response: problem("too_many_failed_logins", 429, "try again later", { "retry-after": String(seconds) }),
});

// basicPassword extracts the password from a Basic header, or null.
function basicPassword(header: string): string | null {
  if (!header.startsWith("Basic ")) return null;
  try {
    const decoded = atob(header.slice(6));
    return decoded.slice(decoded.indexOf(":") + 1);
  } catch {
    return null;
  }
}

// verifyPassword checks the Basic credential against `expected` (non-empty).
export async function verifyPassword(request: Request, env: Env, expected: string): Promise<PasswordResult> {
  const header = request.headers.get("authorization") || "";
  // No credential yet: the browser's first request. Not an attempt, not counted.
  if (!header) return unauthorized();

  const key = `core-login:${clientIP(request)}`;
  if (env.CORE_LOGIN_LIMITER) {
    const { success } = await env.CORE_LOGIN_LIMITER.limit({ key });
    if (!success) return tooMany(60); // the binding's longest period
  }
  const now = Date.now();
  const guard = lockout(env);
  const wait = await guard.lockedFor(key, now);
  if (wait > 0) return tooMany(wait);

  const given = basicPassword(header);
  if (given !== null && timingSafeEqual(given, expected)) {
    const late = await guard.lockedFor(key, Date.now());
    if (late > 0) return tooMany(late);
    await guard.clear(key);
    return { ok: true };
  }
  const windowMs = intVar(env.CORE_LOGIN_WINDOW_MINUTES, 10) * 60_000;
  const max = intVar(env.CORE_LOGIN_MAX_FAILURES, 10);
  return (await guard.fail(key, now, max, windowMs)) > max ? tooMany(Math.ceil(windowMs / 1000)) : unauthorized();
}
