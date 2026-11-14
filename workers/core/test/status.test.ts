// GET /api/core/status: how each module's answer is classified, what leaves
// core (only the caller's credential, only to this host, no redirects
// followed), the timeout and the overall deadline on a fake clock, and a cache
// that is never shared between callers.
import { after, beforeEach, mock, test } from "node:test";
import assert from "node:assert/strict";
import * as status from "../functions/api/core/status.ts";
import { MODULE } from "../functions/api/core/_module.ts";
import { MAX_BYTES } from "../functions/api/core/_probe.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { CACHE_MAX_ENTRIES, clearStatusCache } from "../functions/api/core/_status.ts";
import { AUTH_ENV, ORIGIN, PASSWORD, PW_ENV, basic, call, entry, fakeFetch, manifest, registry, session, settle, settleUntil, slow } from "./harness.ts";

const caller = (cookie = "s=1") => ({
  authorization: basic(PASSWORD),
  cookie,
  "cf-connecting-ip": "198.51.100.7",
  "cf-access-jwt-assertion": "not-forwarded",
  "x-forwarded-for": "198.51.100.7",
  "user-agent": "test",
  "accept-language": "pl",
});
const mf = (id: string, endpoints: Record<string, string>) => manifest(id, { endpoints });
const url = (p: string) => `${ORIGIN}${p}`;

beforeEach(() => clearStatusCache());
after(() => useRegistry());

async function statusOf(env: Record<string, unknown> = PW_ENV, headers: Record<string, string> = caller()) {
  const r = await call(status.onRequest, { path: "/api/core/status", headers, env });
  assert.equal(r.status, 200, r.body);
  const body = r.json();
  return { body, byId: Object.fromEntries(body.items.map((i: { id: string }) => [i.id, i])) as Record<string, Record<string, unknown>> };
}

test("each answer is classified; only cookie and authorization ever leave core, never a redirect followed", async () => {
  const streamed = () =>
    new Response(
      new ReadableStream({
        pull(c) {
          c.enqueue(new Uint8Array(64 * 1024));
        },
      }),
    );
  useRegistry(
    registry(
      entry("alive", mf("alive", { manifest: "/api/alive/module", health: "/api/alive/health" })),
      entry("plain", mf("plain", { manifest: "/api/plain/module" })),
      entry("locked", mf("locked", { manifest: "/api/locked/module" })),
      entry("forbid", mf("forbid", { manifest: "/api/forbid/module" })),
      entry("broken", mf("broken", { manifest: "/api/broken/module" })),
      entry("gone", mf("gone", { manifest: "/api/gone/module" })),
      entry("moved", mf("moved", { health: "/api/moved/health" })),
      entry("notjson", mf("notjson", { manifest: "/api/notjson/module" })),
      entry("wrongid", mf("wrongid", { manifest: "/api/wrongid/module" })),
      entry("big", mf("big", { manifest: "/api/big/module" })),
      entry("stream", mf("stream", { manifest: "/api/stream/module" })),
      entry("silent", mf("silent", {})),
      entry("core", MODULE),
      entry("offsite", mf("offsite", { health: "/h" }), "https://evil.example.com"),
      entry("slashes", mf("slashes", { health: "/h" }), "//evil.example.com"),
      entry("ext", null, "/ext/"),
      entry("bad", { id: "bad" }),
    ),
  );
  const f = fakeFetch({
    [url("/api/alive/health")]: new Response("ok"),
    [url("/api/plain/module")]: Response.json(manifest("plain", { version: "2.0.0" })),
    [url("/api/locked/module")]: new Response("", { status: 401 }),
    [url("/api/forbid/module")]: new Response("", { status: 403 }),
    [url("/api/broken/module")]: new Response("", { status: 500 }),
    [url("/api/moved/health")]: new Response(null, { status: 302, headers: { location: "https://evil.example.com/steal" } }),
    [url("/api/notjson/module")]: new Response("<html>"),
    [url("/api/wrongid/module")]: Response.json(manifest("plain")),
    [url("/api/big/module")]: new Response("{}", { headers: { "content-length": String(MAX_BYTES + 1) } }),
    [url("/api/stream/module")]: streamed,
    [url("/ext/api/module")]: Response.json(manifest("ext", { version: "3.1.4" })),
  });
  try {
    const { body, byId } = await statusOf();
    const brief = Object.fromEntries(Object.entries(byId).map(([id, i]) => [id, [i.status, i.reason ?? null, i.httpStatus ?? null, i.version]]));
    assert.deepEqual(brief, {
      alive: ["reachable", null, 200, "1.2.3"],
      plain: ["reachable", null, 200, "2.0.0"],
      locked: ["denied", null, 401, "1.2.3"],
      forbid: ["denied", null, 403, "1.2.3"],
      broken: ["unreachable", "http_status", 500, "1.2.3"],
      gone: ["unreachable", "network", null, "1.2.3"],
      moved: ["unreachable", "redirect", 302, "1.2.3"],
      notjson: ["invalid_manifest", "not_json", 200, "1.2.3"],
      wrongid: ["invalid_manifest", "rejected", 200, "1.2.3"],
      big: ["invalid_manifest", "too_large", 200, "1.2.3"],
      stream: ["invalid_manifest", "too_large", 200, "1.2.3"],
      silent: ["unknown", "no_endpoint", null, "1.2.3"],
      core: ["reachable", null, null, "1.0.0"],
      offsite: ["unreachable", "refused_target", null, "1.2.3"],
      slashes: ["unreachable", "refused_target", null, "1.2.3"],
      ext: ["reachable", null, 200, "3.1.4"],
      bad: ["invalid_manifest", "registry", null, null],
    });
    assert.match(String(byId.wrongid.detail), /describes "plain", not "wrongid"/);
    assert.match(String(byId.bad.detail), /is required/);
    assert.equal(byId.core.ms, null);
    assert.ok(!("manifest" in byId.plain), "a live manifest is not echoed");
    assert.ok(!Number.isNaN(Date.parse(body.checkedAt)));
    // What left core: this host only, GET, redirect manual, two headers.
    for (const s of f.sent) {
      assert.ok(s.url.startsWith(`${ORIGIN}/`), s.url);
      assert.deepEqual([s.method, s.redirect], ["GET", "manual"]);
      assert.deepEqual(s.headers, { authorization: basic(PASSWORD), cookie: "s=1" }, s.url);
    }
    assert.equal(f.sent.length, 12, "offsite, slashes, silent, core and bad are never asked");
    assert.ok(!f.sent.some((s) => s.url.includes("evil")));
  } finally {
    f.restore();
  }
});

test("a caller without a cookie sends only authorization — nothing is invented", async () => {
  useRegistry(registry(entry("alive", mf("alive", { health: "/api/alive/health" }))));
  const f = fakeFetch({ [url("/api/alive/health")]: new Response("ok") });
  try {
    await statusOf(PW_ENV, { authorization: basic(PASSWORD) });
    assert.deepEqual(f.sent[0].headers, { authorization: basic(PASSWORD) });
  } finally {
    f.restore();
  }
});

// run starts a status request on the mocked clock, advances it by ms, and
// returns the answer and how long it took on that clock.
async function run(env: Record<string, unknown>, ms: number, sent: () => number, expected: number) {
  const started = Date.now();
  let done = false;
  const pending = statusOf(env).then((r) => ((done = true), r));
  await settleUntil(() => sent() >= expected); // every timeout is set before its request goes out
  mock.timers.tick(ms - 1);
  await settle();
  assert.equal(done, false, "not answered before the limit");
  mock.timers.tick(1);
  await settle();
  const got = await pending;
  return { ...got, took: Date.now() - started };
}

test("a module that does not answer is unreachable after its timeout; the others are not held up", async () => {
  mock.timers.enable({ apis: ["setTimeout", "Date"], now: 1_000_000 });
  useRegistry(registry(entry("alive", mf("alive", { health: "/a" })), entry("sleepy", mf("sleepy", { health: "/s" }))));
  const f = fakeFetch({ [url("/a")]: new Response("ok"), [url("/s")]: slow(60_000, () => new Response("late")) });
  try {
    const { byId, took } = await run(PW_ENV, 2000, () => f.sent.length, 2);
    assert.equal(took, 2000);
    assert.deepEqual([byId.sleepy.status, byId.sleepy.reason, byId.sleepy.ms], ["unreachable", "timeout", 2000]);
    assert.deepEqual([byId.alive.status, byId.alive.ms], ["reachable", 0]);
  } finally {
    f.restore();
    mock.timers.reset();
  }
});

test("the overall deadline caps the answer even when the per-module timeout is longer", async () => {
  mock.timers.enable({ apis: ["setTimeout", "Date"], now: 1_000_000 });
  useRegistry(registry(entry("a", mf("a", { health: "/a" })), entry("b", mf("b", { manifest: "/b" }))));
  const f = fakeFetch({ [url("/a")]: slow(9000, () => new Response("late")), [url("/b")]: slow(9000, () => Response.json(manifest("b"))) });
  try {
    const env = { ...PW_ENV, CORE_PROBE_TIMEOUT_MS: "5000", CORE_STATUS_DEADLINE_MS: "3000" };
    const { byId, took } = await run(env, 3000, () => f.sent.length, 2);
    assert.equal(took, 3000);
    for (const id of ["a", "b"]) assert.deepEqual([byId[id].status, byId[id].reason], ["unreachable", "deadline"]);
  } finally {
    f.restore();
    mock.timers.reset();
  }
});

test("answers are cached per caller: the same credential hits the cache, another one never sees it", async () => {
  mock.timers.enable({ apis: ["Date"], now: 1_000_000 });
  useRegistry(registry(entry("m", mf("m", { manifest: "/api/m/module" }))));
  const f = fakeFetch({ [url("/api/m/module")]: Response.json(manifest("m")) });
  try {
    assert.equal((await statusOf()).byId.m.cached, undefined);
    assert.equal((await statusOf()).byId.m.cached, true);
    assert.equal(f.sent.length, 1);
    assert.equal((await statusOf(PW_ENV, caller("s=2"))).byId.m.cached, undefined, "another caller is asked for afresh");
    assert.deepEqual(f.sent.map((s) => s.headers.cookie), ["s=1", "s=2"]);
    mock.timers.tick(15_000);
    assert.equal((await statusOf()).byId.m.cached, undefined, "the entry expired");
    assert.equal(f.sent.length, 3);
  } finally {
    f.restore();
    mock.timers.reset();
  }
});

test("the cache stays bounded, dropping the oldest entry", async () => {
  const many = Array.from({ length: CACHE_MAX_ENTRIES + 1 }, (_, i) => entry(`m${i}`, null, `/m${i}`));
  useRegistry(registry(...many));
  const f = fakeFetch({});
  try {
    await statusOf();
    assert.equal(f.sent.length, CACHE_MAX_ENTRIES + 1);
    useRegistry(registry(many[0], many[CACHE_MAX_ENTRIES]));
    const { byId } = await statusOf();
    assert.deepEqual([byId.m0.cached, byId[`m${CACHE_MAX_ENTRIES}`].cached], [undefined, true]);
  } finally {
    f.restore();
  }
});

test("auth mode: status lists only the modules the caller may see", async () => {
  useRegistry(
    registry(
      entry("comments", manifest("comments", { nav: [["/c.html", "comments:read"]], endpoints: {} })),
      entry("blog", manifest("blog", { nav: [["/b.html", "blog:read"]], endpoints: {} })),
      entry("ext", null, "/ext"),
    ),
  );
  const f = fakeFetch({ [url("/ext/api/module")]: new Response("", { status: 403 }) });
  try {
    const { body } = await statusOf(AUTH_ENV, { cookie: await session(["comments:read"]) });
    assert.deepEqual(body.items.map((i: { id: string }) => i.id), ["comments"]);
    // blog's manifest is known at build time and the caller may not see it:
    // no request goes out with this caller's credential. ext is unknown to
    // the build, so it has to be asked (and is then filtered).
    assert.ok(!f.sent.some((r: { url: string }) => r.url.includes("/b.html") || r.url.includes("blog")), "an invisible module is never asked");
    assert.ok(f.sent.some((r: { url: string }) => r.url === url("/ext/api/module")), "a module the build does not know is asked");
  } finally {
    f.restore();
  }
});
