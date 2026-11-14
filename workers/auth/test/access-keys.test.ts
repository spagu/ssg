// GO-139: the team's keys — cached, refetched on rotation at most once per
// cool-down, kept through a short outage, refused when none are usable, and
// fetched from the configured team host only.
import { test } from "node:test";
import assert from "node:assert/strict";
import { accessKey, accessKeyCache, accessKeyTimes } from "../functions/api/auth/_access-keys.ts";
import { verifyAccessToken } from "../functions/api/auth/_access-jwt.ts";
import { accessLookup } from "../functions/api/auth/_access-config.ts";
import type { AccessSetup } from "../functions/api/auth/_access-config.ts";
import { T0 } from "./fakes.ts";
import { ACCESS_ENV, FakeAccess, HOST, KEY, OTHER_KEY, PERSON, mint } from "./access-fake.ts";

const SETUP = (accessLookup(ACCESS_ENV as never) as { setup: AccessSetup }).setup;
const MIN = 60_000;
const ask = async (token: string, now: number, extra: Record<string, string> = {}) => {
  const v = await verifyAccessToken(new Request("https://example.com/auth/access", { headers: { "cf-access-jwt-assertion": token, ...extra } }), SETUP, now);
  return v.ok ? "ok" : `${v.reason}${v.unavailable ? " (unavailable)" : ""}`;
};

test("cached for an hour; a rotated key is fetched once; unknown key ids cannot make the worker hammer the team", async () => {
  const fake = new FakeAccess().install();
  try {
    const t1 = await mint();
    assert.equal(await ask(t1, T0), "ok");
    assert.equal(await ask(t1, T0 + 30 * MIN), "ok");
    assert.equal(fake.requests.length, 1);
    // Access rotates: the new kid is unknown → one refetch, then known.
    fake.keys = [KEY, OTHER_KEY];
    const t2 = await mint(PERSON, {}, { key: OTHER_KEY, now: T0 + 31 * MIN });
    assert.equal(await ask(t2, T0 + 31 * MIN), "ok");
    assert.equal(fake.requests.length, 2);
    // Made-up kids: at most one fetch per cool-down.
    for (let i = 0; i < 5; i++) assert.equal(await ask(await mint(PERSON, {}, { header: { kid: `x${i}` }, now: T0 + 31 * MIN }), T0 + 31 * MIN + i), "unknown key id");
    assert.equal(fake.requests.length, 2);
    assert.equal(await ask(await mint(PERSON, {}, { header: { kid: "x9" }, now: T0 + 33 * MIN }), T0 + 33 * MIN), "unknown key id");
    assert.equal(fake.requests.length, 3, "after the cool-down it may ask again");
    // Past the hour: refreshed on the next use.
    assert.equal(await ask(await mint(PERSON, {}, { now: T0 + 95 * MIN }), T0 + 95 * MIN), "ok");
    assert.equal(fake.requests.length, 4);
  } finally {
    fake.restore();
  }
});

test("keys unavailable: refused (never accepted unchecked); cached keys keep working through an outage, up to a day", async () => {
  const fake = new FakeAccess().install();
  // A key the runtime refuses to import is skipped (Node accepts nearly any
  // RSA JWK, the Workers runtime does not: the refusal is simulated).
  const subtle = crypto.subtle as unknown as { importKey: (...a: unknown[]) => Promise<CryptoKey> };
  const realImport = subtle.importKey.bind(crypto.subtle);
  subtle.importKey = (...a: unknown[]) => ((a[1] as { n?: string }).n === "REFUSED" ? Promise.reject(new Error("refused")) : realImport(...a));
  try {
    for (const mode of ["500", "throw", "empty", "odd"] as const) {
      accessKeyCache.clear();
      fake.mode = mode;
      assert.equal(await ask(await mint(), T0), "keys unavailable (unavailable)", mode);
    }
    // Within the cool-down a failing team is not asked again.
    const n = fake.requests.length;
    assert.equal(await ask(await mint(), T0 + 1000), "keys unavailable (unavailable)");
    assert.equal(fake.requests.length, n);

    accessKeyCache.clear();
    fake.mode = "ok";
    assert.equal(await ask(await mint(), T0), "ok");
    fake.mode = "500";
    const later = T0 + 2 * 60 * MIN;
    assert.equal(await ask(await mint(PERSON, {}, { now: later }), later), "ok", "stale keys still verify while the team is down");
    const dayLater = T0 + accessKeyTimes.maxStaleMs + 2 * MIN;
    assert.equal(await ask(await mint(PERSON, {}, { now: dayLater }), dayLater), "keys unavailable (unavailable)", "not forever");
  } finally {
    subtle.importKey = realImport;
    fake.restore();
  }
});

test("concurrent first requests share one fetch; the host comes from configuration, never the request", async () => {
  const fake = new FakeAccess().install();
  try {
    const token = await mint();
    const results = await Promise.all([1, 2, 3].map(() => ask(token, T0, { host: "evil.example.com", "x-forwarded-host": "evil.example.com" })));
    assert.deepEqual(results, ["ok", "ok", "ok"]);
    assert.deepEqual(fake.requests, [`https://${HOST}/cdn-cgi/access/certs`]);
    assert.equal(await accessKey("otherteam.cloudflareaccess.com", KEY.kid, T0).catch((e: Error) => e.message), "unavailable", "another host has its own cache; nothing it answers is mixed with this team's");
  } finally {
    fake.restore();
  }
});
