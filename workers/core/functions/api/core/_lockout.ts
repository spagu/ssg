// The failed-login lockout for the shared-password mode. A leading underscore
// keeps this file out of the Pages route table.
//
// Without the optional CORE_DB database the lockout lives in the isolate's
// memory (this file): it is BEST EFFORT. Cloudflare runs many isolates and
// recycles them, so an attacker spread over isolates (or lucky with a
// recycle) gets more guesses than the limit. The optional rate-limit binding
// (see _password.ts) adds a ceiling across isolates. With CORE_DB bound the
// same LockoutStore is kept in the database (_lockout-d1.ts) and holds across
// isolates.

export interface LockoutStore {
  // lockedFor returns how many seconds the key is still locked, 0 when free.
  lockedFor(key: string, now: number): Promise<number>;
  // fail counts one failure and returns the count in the current window.
  fail(key: string, now: number, max: number, windowMs: number): Promise<number>;
  clear(key: string): Promise<void>;
}

interface Entry {
  failures: number;
  windowStart: number;
  lockedUntil: number;
}

// MAX_ENTRIES bounds the memory a flood of addresses can take; the oldest
// entry goes first.
export const MAX_ENTRIES = 5000;

// memoryLockout counts failures in a fixed window per key and locks the key
// for one window once the count reaches max.
export function memoryLockout(): LockoutStore & { size(): number } {
  const entries = new Map<string, Entry>();
  return {
    async lockedFor(key, now) {
      const e = entries.get(key);
      return e && e.lockedUntil > now ? Math.ceil((e.lockedUntil - now) / 1000) : 0;
    },
    async fail(key, now, max, windowMs) {
      const e = entries.get(key);
      const fresh = !e || e.windowStart + windowMs <= now;
      const next: Entry = fresh ? { failures: 1, windowStart: now, lockedUntil: 0 } : { ...e, failures: e.failures + 1 };
      if (next.failures >= max) next.lockedUntil = now + windowMs;
      entries.delete(key); // re-insert so the Map's order is "least recently failed first"
      entries.set(key, next);
      if (entries.size > MAX_ENTRIES) entries.delete(entries.keys().next().value as string);
      return next.failures;
    },
    async clear(key) {
      entries.delete(key);
    },
    size: () => entries.size,
  };
}
