// A per-isolate cache of someone else's public keys (GO-139, #435): the
// Cloudflare Access team's certs (_access-keys.ts) and an external issuer's
// JWKS (_external-keys.ts). A leading underscore keeps this file out of the
// Pages route table.
//
// Freshness without hammering the key host, and without failing open:
//   - keys younger than ttlMs that hold what the token asks for are used as
//     they are;
//   - an unknown key id (the issuer rotated its keys) or an older cache asks
//     again, but at most once per coolDownMs — a stream of tokens with made-up
//     key ids cannot turn into a stream of requests to the key host;
//   - when asking fails, the keys already held keep working for up to
//     maxStaleMs (a short outage of the key endpoint does not lock everyone
//     out); with none usable the answer is "unavailable" and the sign-in is
//     refused — never accepted unchecked.
// Concurrent requests of one isolate share a single fetch.

export interface KeyTimes {
  ttlMs: number;
  coolDownMs: number;
  maxStaleMs: number;
}

export interface KeyEntry<T> {
  keys: T | null; // null = never fetched successfully
  fetchedAt: number;
  triedAt: number;
  pending: Promise<void> | null;
}

export type KeyCache<T> = Map<string, KeyEntry<T>>;

export interface KeySource<T> {
  cache: KeyCache<T>;
  id: string; // the cache key: a host, a URL
  times: KeyTimes;
  // fetch asks the key host; null when that failed or gave nothing usable
  // (the fetcher logs why — never a token).
  fetch: () => Promise<T | null>;
  // has: do these keys hold what the token at hand asks for (its key id)?
  has: (keys: T) => boolean;
}

function entryOf<T>(cache: KeyCache<T>, id: string): KeyEntry<T> {
  let e = cache.get(id);
  if (!e) {
    e = { keys: null, fetchedAt: 0, triedAt: Number.NEGATIVE_INFINITY, pending: null };
    cache.set(id, e);
  }
  return e;
}

// cachedKeys: the keys to look the token's key up in, or "unavailable".
export async function cachedKeys<T>(src: KeySource<T>, now: number): Promise<T | "unavailable"> {
  const e = entryOf(src.cache, src.id);
  if (e.keys !== null && now - e.fetchedAt < src.times.ttlMs && src.has(e.keys)) return e.keys;
  if (e.pending === null && now - e.triedAt >= src.times.coolDownMs) {
    e.triedAt = now;
    e.pending = src.fetch().then((fresh) => {
      if (fresh !== null) {
        e.keys = fresh;
        e.fetchedAt = now;
      }
      e.pending = null;
    });
  }
  if (e.pending !== null) await e.pending;
  if (e.keys === null || now - e.fetchedAt >= src.times.maxStaleMs) return "unavailable";
  return e.keys;
}
