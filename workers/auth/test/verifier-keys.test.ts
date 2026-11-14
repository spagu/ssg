// Where the portable verifier (GO-135) gets its keys: inline JWK / JWKS, or a JWKS URL with a
// per-isolate cache, one refetch per cool-down, bounded requests, and deny when keys are gone.
import { test } from "node:test";
import assert from "node:assert/strict";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import { onRequestGet as jwksRoute } from "../functions/api/auth/jwks.ts";
import { FETCH_TIMEOUT_MS, KEY_TTL_MS, MAX_JWKS_BYTES, REFETCH_COOLDOWN_MS, verifySession } from "../portable/session-verifier.ts";
import type { Deps } from "../portable/session-verifier.ts";
import { KEYS, makeEnv, newKeyPair } from "./fakes.ts";
import { KEYSET, NOW, NOW_MS, PAGE, config, keySet, part, sign, withCookie } from "./verifier-fixtures.ts";

const OLD = await newKeyPair();
const OLDSET = await keySet(OLD.priv);
const long = { exp: NOW + 86_400 };
const jwks = (...pubs: string[]) => JSON.stringify({ keys: pubs.map((p) => JSON.parse(p)) });
let urls = 0;
const fresh = () => config({ publicKey: "", jwksUrl: `/api/auth/jwks?n=${++urls}` }); // a cache entry of its own

// server: a fake fetch answering with `body()`, counting calls, on a clock the test moves.
function server(body: (url: string, init: RequestInit) => Response | Promise<Response>) {
  const s = { calls: 0, t: NOW_MS, inits: [] as RequestInit[] };
  const deps: Deps = { now: () => s.t, fetch: async (url, init) => (s.calls++, s.inits.push(init), body(url, init)) };
  return Object.assign(s, { deps });
}
const ok = (text: string) => () => new Response(text, { status: 200 });
const reason = async (token: string, c: ReturnType<typeof config>, deps: Deps, url = PAGE) => {
  const v = await verifySession(withCookie(token, url), c, deps);
  return v.ok ? "ok" : v.reason;
};

test("inline: one JWK, a JWKS with two keys (rotation), only usable signing keys", async () => {
  const deps = { now: () => NOW_MS };
  assert.equal(await reason(await sign(), config(), deps), "ok");
  const both = config({ publicKey: jwks(KEYS.pub, OLD.pub) });
  assert.equal(await reason(await sign(), both, deps), "ok");
  assert.equal(await reason(await sign({}, OLDSET), both, deps), "ok", "the retired key still verifies while listed");
  assert.equal(await reason(await sign({}, OLDSET), config(), deps), "unknown_key");
  const { x, y } = JSON.parse(KEYS.pub);
  const offCurve = { kty: "EC", crv: "P-256", x: b64urlEncode(new Uint8Array(32).fill(1)), y: b64urlEncode(new Uint8Array(32).fill(2)) };
  const noise = [null, 5, "k", { kty: "RSA", n: "a", e: "AQAB" }, { kty: "EC", crv: "P-384", x, y }, { kty: "EC", crv: "P-256", x, y, use: "enc" }];
  const more = [{ kty: "EC", crv: "P-256", x, y, alg: "ES384" }, { kty: "EC", crv: "P-256", x: 1, y }, offCurve];
  const usable = { ...JSON.parse(OLD.pub), kid: "ignored", alg: "ES256", use: "sig" };
  const mixed = config({ publicKey: JSON.stringify({ keys: [...noise, ...more, usable] }) });
  assert.equal(await reason(await sign({}, OLDSET), mixed, deps), "ok", "the one usable key, under its thumbprint, not its written kid");
  assert.equal(await reason(await sign(), mixed, deps), "unknown_key", "every unusable entry was skipped");
  for (const bad of [jwks(), JSON.stringify({ keys: noise }), "null", "[]", JSON.stringify(offCurve), KEYS.priv]) {
    assert.equal(await reason(await sign(), config({ publicKey: bad }), deps), "not_configured", bad.slice(0, 40));
  }
});

test("URL: fetched once, cached for KEY_TTL_MS, then fetched again", async () => {
  const s = server(ok(jwks(KEYS.pub)));
  const c = fresh();
  const token = await sign(long);
  assert.equal(await reason(token, c, s.deps), "ok");
  s.t += KEY_TTL_MS - 1;
  assert.equal(await reason(token, c, s.deps), "ok");
  assert.equal(s.calls, 1);
  s.t += 1;
  assert.equal(await reason(token, c, s.deps), "ok");
  assert.equal(s.calls, 2, "expired cache refetched");
  const init = s.inits[0];
  assert.equal(init.redirect, "manual");
  assert.equal(init.signal?.aborted, true, "the request's signal is released after a complete read");
});

test("URL: the real /api/auth/jwks answer, and concurrent cold requests share one fetch", async () => {
  const env = makeEnv();
  const s = server((url) => jwksRoute({ request: new Request(url), env } as never));
  const c = fresh();
  const token = await sign();
  const all = await Promise.all(Array.from({ length: 10 }, () => reason(token, c, s.deps)));
  assert.deepEqual(new Set(all), new Set(["ok"]));
  assert.equal(s.calls, 1);
});

test("URL: a flood of unknown kids costs one fetch per cool-down", async () => {
  const s = server(ok(jwks(KEYS.pub)));
  const c = fresh();
  const [h, p, sig] = (await sign(long)).split(".");
  const randomKid = () => `${part({ alg: "ES256", kid: crypto.randomUUID() })}.${p}.${sig}`;
  assert.equal(await reason(`${h}.${p}.${sig}`, c, s.deps), "ok");
  for (let i = 0; i < 200; i++) assert.equal(await reason(randomKid(), c, s.deps), "unknown_key");
  assert.equal(s.calls, 1, "inside the cool-down of the first fetch: no refetch at all");
  s.t += REFETCH_COOLDOWN_MS;
  for (let i = 0; i < 200; i++) assert.equal(await reason(randomKid(), c, s.deps), "unknown_key");
  assert.equal(s.calls, 2, "after the cool-down: ONE refetch for 200 unknown kids");
  s.t += REFETCH_COOLDOWN_MS - 1;
  await reason(randomKid(), c, s.deps);
  assert.equal(s.calls, 2, "still cooling down");
  assert.equal(await reason(`${h}.${p}.${sig}`, c, s.deps), "ok", "known keys unaffected");
});

test("URL: a rotated key is found by the refetch an unknown kid triggers", async () => {
  let body = jwks(OLD.pub);
  const s = server(() => new Response(body));
  const c = fresh();
  assert.equal(await reason(await sign(long, OLDSET), c, s.deps), "ok");
  body = jwks(KEYS.pub, OLD.pub);
  s.t += REFETCH_COOLDOWN_MS;
  assert.equal(await reason(await sign(long), c, s.deps), "ok");
  assert.equal(s.calls, 2);
});

test("URL: keys unavailable — deny cold, keep a warm cache until its TTL", async () => {
  let up = false;
  const s = server(() => (up ? new Response(jwks(KEYS.pub)) : new Response("down", { status: 503 })));
  const c = fresh();
  const token = await sign(long);
  assert.equal(await reason(token, c, s.deps), "keys_unavailable", "cold and down");
  assert.equal(await reason(token, c, s.deps), "keys_unavailable", "cool-down: no second fetch");
  assert.equal(s.calls, 1);
  s.t += REFETCH_COOLDOWN_MS;
  up = true;
  assert.equal(await reason(token, c, s.deps), "ok");
  up = false;
  s.t += REFETCH_COOLDOWN_MS;
  const stranger = await sign(long, await keySet((await newKeyPair()).priv));
  assert.equal(await reason(stranger, c, s.deps), "unknown_key", "refetch failed");
  assert.equal(await reason(token, c, s.deps), "ok", "the warm cache still serves");
  s.t += KEY_TTL_MS;
  assert.equal(await reason(token, c, s.deps), "keys_unavailable", "after the TTL a failed fetch denies");
  assert.equal(s.calls, 4);
});

test("URL: bounded requests — status, redirect, empty body, size cap, throw, not JSON", async () => {
  const token = await sign();
  const big = JSON.stringify({ keys: [JSON.parse(KEYS.pub)], pad: "x".repeat(MAX_JWKS_BYTES) });
  const chunked = () => {
    const bytes = new TextEncoder().encode(big);
    let sent = 0;
    return new Response(new ReadableStream({ pull: (ctl) => void (sent < bytes.length ? ctl.enqueue(bytes.slice(sent, (sent += 4096))) : ctl.close()) }));
  };
  const answers: [string, () => Response | Promise<Response>][] = [
    ["404", () => new Response("{}", { status: 404 })],
    ["redirect elsewhere", () => new Response(null, { status: 302, headers: { location: "https://evil.example.com/jwks" } })],
    ["200 without a body", () => new Response(null, { status: 200 })],
    ["over the size cap", () => new Response(big)],
    ["over the size cap, streamed", chunked],
    ["network error", () => Promise.reject(new Error("connection refused"))],
    ["not JSON", () => new Response("<html>")],
    ["no usable key", () => new Response(jwks())],
  ];
  for (const [name, answer] of answers) {
    const s = server(answer);
    assert.equal(await reason(token, fresh(), s.deps), "keys_unavailable", name);
    assert.equal(s.calls, 1, name);
  }
  assert.ok(big.length > MAX_JWKS_BYTES);
  assert.equal(await reason(token, fresh(), server(ok(jwks(KEYS.pub))).deps), "ok", "the same key under the cap");
});

test("URL: a slow answer or a slow body times out", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const token = await sign();
  const hang = (init: RequestInit) => new Promise<never>((_, reject) => init.signal?.addEventListener("abort", () => reject(new Error("aborted"))));
  const trickle = (init: RequestInit) =>
    new Response(new ReadableStream({ start: (ctl) => (ctl.enqueue(new TextEncoder().encode('{"keys":[')), init.signal?.addEventListener("abort", () => ctl.error(new Error("aborted")))) }));
  for (const answer of [hang, trickle]) {
    let called!: () => void;
    const fetched = new Promise<void>((r) => (called = r));
    const s = server((_, init) => (called(), answer(init)));
    const pending = reason(token, fresh(), s.deps);
    await fetched;
    await new Promise((r) => setImmediate(r));
    t.mock.timers.tick(FETCH_TIMEOUT_MS);
    assert.equal(await pending, "keys_unavailable");
  }
});

test("URL: same-origin https only — never fetched otherwise", async () => {
  const s = server(ok(jwks(KEYS.pub)));
  const token = await sign();
  for (const [jwksUrl, page] of [
    ["http://example.com/api/auth/jwks", PAGE],
    ["https://evil.example.com/api/auth/jwks", PAGE],
    ["https://example.com:8443/api/auth/jwks", PAGE],
    ["/api/auth/jwks", "http://example.com/admin/"],
  ]) {
    assert.equal(await reason(token, config({ publicKey: "", jwksUrl }), s.deps, page), "not_configured", `${jwksUrl} from ${page}`);
  }
  assert.equal(s.calls, 0);
  assert.equal(await reason(token, config({ publicKey: "", jwksUrl: "https://example.com/api/auth/jwks?abs" }), s.deps), "ok", "an absolute same-origin URL");
});

test("URL: without deps the global fetch is used", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response(jwks(KEYS.pub)));
  const live = await sign({ iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 60 });
  assert.equal((await verifySession(withCookie(live), fresh())).ok, true);
  assert.equal((globalThis.fetch as unknown as { mock: { callCount(): number } }).mock.callCount(), 1);
  assert.equal(KEYSET.signing.kid.length, 43);
});
