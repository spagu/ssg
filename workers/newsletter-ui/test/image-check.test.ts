// The pre-send image check (#446): which images the rendered mail has, and
// for each whether it is stored, has alt text and answers a mail image
// proxy's kind of request. Never a real network call: fetch is injected.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { MIGRATIONS, migrate } from "../src/schema.ts";
import { MAX_PROBED_IMAGES, checkImages, imagesOf } from "../src/image-check.ts";
import { MAX_REDIRECTS, PROBE_AGENTS, pool, probeFetch, probeStore } from "../src/image-probe.ts";
import type { Fetcher } from "../src/image-probe.ts";
import { PASSWORD, fakeCtx, fakeR2, makeEnv, req } from "./fakes.ts";

const B = "/newsletter-admin";
const SITE = "https://example.com";
const KEY = "a".repeat(64) + ".png";
type Env = ReturnType<typeof makeEnv>;

async function insertCampaign(env: Env, id: string, body: string, opts: { format?: string; subject?: string } = {}) {
  await migrate(env.NEWSLETTER_DB as never, MIGRATIONS);
  const now = "2026-11-01T00:00:00.000Z";
  env.NEWSLETTER_DB.sqlite
    .prepare("INSERT INTO campaigns (id, subject, body, body_format, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
    .run(id, opts.subject ?? "Hi", body, opts.format ?? "markdown", now, now);
}

// A fetcher answering by URL; records every request.
type Answer = { status?: number; type?: string | null; location?: string } | Error;
function fetcher(answers: Record<string, Answer | ((agent: string) => Answer)>) {
  const calls: { url: string; init: RequestInit }[] = [];
  let cancelled = 0;
  const f: Fetcher = async (url, init) => {
    calls.push({ url, init });
    const ua = new Headers(init.headers).get("user-agent") ?? "";
    const raw = answers[url] ?? { status: 404, type: "text/html" };
    const a = typeof raw === "function" ? raw(ua) : raw;
    if (a instanceof Error) throw a;
    const headers = new Headers();
    if (a.type !== null) headers.set("content-type", a.type ?? "image/png");
    if (a.location) headers.set("location", a.location);
    const body = new ReadableStream({
      pull(c) {
        c.enqueue(new Uint8Array(1024));
      },
      cancel() {
        cancelled++;
      },
    });
    return new Response(a.status === 304 ? null : body, { status: a.status ?? 200, headers });
  };
  return { f, calls, cancelled: () => cancelled };
}

async function withFetch<T>(f: Fetcher, run: () => Promise<T>): Promise<T> {
  const original = globalThis.fetch;
  globalThis.fetch = f as typeof fetch;
  try {
    return await run();
  } finally {
    globalThis.fetch = original;
  }
}

const check = (env: Env, id: string, query = "") =>
  handleFetch(req(`${B}/api/campaigns/${id}/images${query}`, { auth: PASSWORD }), env as never, fakeCtx() as never);

test("route: images as the rendered mail has them, store off — alt and reachability only", async () => {
  const env = makeEnv({ NEWSLETTER_SITE_URL: SITE });
  await insertCampaign(env, "c1", [
    "![A cat](https://img.example.net/cat.png)",
    "![](https://img.example.net/blocked.jpg)",
    "![x](http://img.example.net/plain.png)", // dropped by the sanitizer: not in the mail
    '<img src="javascript:alert(1)">',
    "![lang](https://img.example.net/{{language}}.png)",
  ].join("\n\n"));
  const net = fetcher({
    "https://img.example.net/cat.png": {},
    "https://img.example.net/blocked.jpg": { status: 403, type: "text/html" },
    "https://img.example.net/pl.png": {},
  });
  const res = await withFetch(net.f, () => check(env, "c1", "?language=pl"));
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.deepEqual(body.items.map((i: { url: string }) => i.url), [
    "https://img.example.net/cat.png", "https://img.example.net/blocked.jpg", "https://img.example.net/pl.png",
  ]);
  assert.deepEqual(body.items[0], {
    url: "https://img.example.net/cat.png", alt: "A cat", stored: false, problems: [],
    probe: { via: "fetch", ok: true, status: 200, contentType: "image/png", error: null, agent: null },
  });
  assert.deepEqual(body.items[1].problems, ["no_alt", "unreachable"]);
  assert.deepEqual(body.items[1].probe, { via: "fetch", ok: false, status: 403, contentType: "text/html", error: null, agent: "GoogleImageProxy" });
  assert.equal(body.checked, 3);
  assert.equal(body.skipped, 0);
  // Each passing image is asked once per agent; the failing one stops at the first.
  assert.equal(net.calls.length, 2 + 1 + 2);
  const agents = net.calls.filter((c) => c.url.endsWith("cat.png")).map((c) => new Headers(c.init.headers).get("user-agent"));
  assert.deepEqual(agents, PROBE_AGENTS.map((a) => a.userAgent));
  for (const a of agents) assert.match(a!, /^ssg-newsletter-image-check\/1\.0 \(mail image proxy test; (GoogleImageProxy|YahooMailProxy)\)$/);
  assert.ok(net.calls.every((c) => c.init.redirect === "manual" && c.init.method === "GET" && c.init.signal instanceof AbortSignal));
  assert.equal(net.cancelled(), net.calls.length, "every body cancelled unread");
  // Nothing changed, nothing stored.
  const row = env.NEWSLETTER_DB.sqlite.prepare("SELECT status, updated_at FROM campaigns WHERE id = 'c1'").get();
  assert.deepEqual({ ...row }, { status: "draft", updated_at: "2026-11-01T00:00:00.000Z" });
});

test("route: store on — not_stored, own images judged from R2, a custom domain fetched", async () => {
  const bucket = fakeR2();
  await bucket.put(KEY, new Uint8Array([1]), { httpMetadata: { contentType: "image/png" } });
  const own = `${SITE}${B}/hooks/a/${KEY}`;
  const gone = `${SITE}${B}/hooks/a/${"b".repeat(64)}.jpg`;
  const env = makeEnv({ NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: bucket });
  await insertCampaign(env, "c2", `<p><img src="${own}" alt="Logo"><img src="${gone}" alt=" "><img src="https://img.example.net/x.png" alt="x"></p>`, { format: "html" });
  const net = fetcher({ "https://img.example.net/x.png": {} });
  const body = await (await withFetch(net.f, () => check(env, "c2"))).json();
  assert.deepEqual(body.items.map((i: { problems: string[] }) => i.problems), [[], ["no_alt", "unreachable"], ["not_stored"]]);
  assert.deepEqual(body.items[0].probe, { via: "store", ok: true, status: 200, contentType: "image/png", error: null, agent: null });
  assert.deepEqual(body.items[1].probe, { via: "store", ok: false, status: 404, contentType: null, error: null, agent: null });
  assert.deepEqual(body.items.map((i: { stored: boolean }) => i.stored), [true, true, false]);
  assert.deepEqual(net.calls.map((c) => c.url), ["https://img.example.net/x.png", "https://img.example.net/x.png"], "own route never fetched");

  // With NEWSLETTER_ASSETS_URL the base is the custom domain, which is fetched.
  const cdn = makeEnv({ NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: fakeR2(), NEWSLETTER_ASSETS_URL: "https://cdn.example.com/n" });
  await insertCampaign(cdn, "c3", `![a](https://cdn.example.com/n/${KEY})`);
  const net2 = fetcher({ [`https://cdn.example.com/n/${KEY}`]: {} });
  const b2 = await (await withFetch(net2.f, () => check(cdn, "c3"))).json();
  assert.deepEqual([b2.items[0].stored, b2.items[0].problems, b2.items[0].probe.via], [true, [], "fetch"]);
  // A misconfigured store gives no base: nothing counts as stored, and no
  // image is reported not_stored for what is a configuration fault.
  const bad = makeEnv({ NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: fakeR2(), NEWSLETTER_ASSETS_URL: "http://cdn.example.com" });
  await insertCampaign(bad, "c4", "![a](http://cdn.example.com/a.png) ![b](https://cdn.example.com/b.png)");
  const b3 = await (await withFetch(fetcher({}).f, () => check(bad, "c4"))).json();
  assert.deepEqual(b3.items.map((i: { problems: string[] }) => i.problems), [["unreachable"]]);
  assert.equal(b3.items[0].stored, false);
});

test("route: at most MAX_PROBED_IMAGES distinct images probed; the rest still judged", async () => {
  const env = makeEnv({ NEWSLETTER_SITE_URL: SITE });
  const urls = Array.from({ length: MAX_PROBED_IMAGES + 2 }, (_, i) => `https://img.example.net/${i}.png`);
  await insertCampaign(env, "c5", [...urls, urls[0]].map((u, i) => (i === urls.length - 1 ? `![](${u})` : `![i${i}](${u})`)).join("\n\n"));
  const ok =fetcher(Object.fromEntries(urls.map((u) => [u, {}])));
  const body = await (await withFetch(ok.f, () => check(env, "c5"))).json();
  assert.equal(body.items.length, urls.length + 1, "one item per <img>, the repeat included");
  assert.deepEqual([body.checked, body.skipped], [MAX_PROBED_IMAGES, 2]);
  assert.equal(new Set(ok.calls.map((c) => c.url)).size, MAX_PROBED_IMAGES);
  assert.ok(ok.calls.length <= MAX_PROBED_IMAGES * PROBE_AGENTS.length * (1 + MAX_REDIRECTS), "within the subrequest budget");
  const skipped = body.items.slice(MAX_PROBED_IMAGES, urls.length);
  assert.ok(skipped.every((i: { probe: unknown }) => i.probe === null));
  assert.deepEqual(skipped[1].problems, ["no_alt"], "alt still judged without a probe");
  assert.equal(body.items[urls.length].probe.ok, true, "a repeated URL shares its probe");
});

test("route: refusals — language, unknown campaign, no site, unknown variable, auth", async () => {
  const env = makeEnv({ NEWSLETTER_SITE_URL: SITE });
  await insertCampaign(env, "c6", "![a](https://img.example.net/a.png) {{nope}}");
  assert.equal((await check(env, "c6", "?language=x_y")).status, 400);
  assert.equal((await (await check(env, "missing")).json()).error, "not_found");
  const unknown = await (await check(env, "c6")).json();
  assert.deepEqual(unknown, { error: "unknown_variable", detail: "{{nope}}" });
  const noSite = makeEnv();
  await insertCampaign(noSite, "c7", "x");
  assert.equal((await (await check(noSite, "c7")).json()).error, "no_site_url");
  const anon = await handleFetch(req(`${B}/api/campaigns/c6/images`), env as never, fakeCtx() as never);
  assert.equal(anon.status, 401);
  // An empty body: no images, nothing fetched.
  await insertCampaign(env, "c8", "just text");
  assert.deepEqual(await (await check(env, "c8")).json(), { items: [], checked: 0, skipped: 0 });
});

test("probeFetch: redirects, timeouts, failures, content types, the second agent", async () => {
  const U = "https://img.example.net/a.png";
  const one = fetcher({ [U]: { status: 301, location: "/b.png" }, "https://img.example.net/b.png": {} });
  assert.deepEqual(await probeFetch(U, one.f), { via: "fetch", ok: true, status: 200, contentType: "image/png", error: null, agent: null });
  assert.deepEqual(one.calls.map((c) => c.url), [U, "https://img.example.net/b.png", U, "https://img.example.net/b.png"]);
  const two = fetcher({ [U]: { status: 302, location: "/b.png" }, "https://img.example.net/b.png": { status: 302, location: "/c.png" } });
  assert.deepEqual(await probeFetch(U, two.f), { via: "fetch", ok: false, status: 302, contentType: null, error: "too_many_redirects", agent: "GoogleImageProxy" });
  assert.equal((await probeFetch(U, fetcher({ [U]: { status: 301, location: "http://img.example.net/a.png" } }).f)).error, "not_https");
  assert.equal((await probeFetch(U, fetcher({ [U]: { status: 301, location: "https://[" } }).f)).error, "fetch_failed");
  assert.deepEqual(await probeFetch(U, fetcher({ [U]: { status: 304 } }).f), { via: "fetch", ok: false, status: 304, contentType: "image/png", error: null, agent: "GoogleImageProxy" }, "a 3xx without location is the answer");
  assert.equal((await probeFetch("http://img.example.net/a.png", fetcher({}).f)).error, "not_https");
  const timeout = Object.assign(new Error("t"), { name: "TimeoutError" });
  const abort = Object.assign(new Error("a"), { name: "AbortError" });
  assert.equal((await probeFetch(U, fetcher({ [U]: timeout }).f)).error, "timeout");
  assert.equal((await probeFetch(U, fetcher({ [U]: abort }).f)).error, "timeout");
  assert.equal((await probeFetch(U, fetcher({ [U]: new TypeError("dns") }).f)).error, "fetch_failed");
  const thrown: Fetcher = async () => {
    throw "not an error";
  };
  assert.equal((await probeFetch(U, thrown)).error, "fetch_failed");
  assert.deepEqual(await probeFetch(U, fetcher({ [U]: { type: "text/html" } }).f), { via: "fetch", ok: false, status: 200, contentType: "text/html", error: "not_image", agent: "GoogleImageProxy" });
  assert.equal((await probeFetch(U, fetcher({ [U]: { type: null } }).f)).error, "not_image");
  // Only the Yahoo token is refused: reported with that agent.
  const yahoo = fetcher({ [U]: (ua) => (ua.includes("YahooMailProxy") ? { status: 403, type: "text/html" } : {}) });
  assert.deepEqual(await probeFetch(U, yahoo.f), { via: "fetch", ok: false, status: 403, contentType: "text/html", error: null, agent: "YahooMailProxy" });
  // A real timeout signal reaches the fetcher.
  const slow: Fetcher = (_url, init) =>
    new Promise((_resolve, reject) => init.signal!.addEventListener("abort", () => reject(init.signal!.reason)));
  assert.equal((await probeFetch(U, slow, 5)).error, "timeout");
  // A body whose cancel fails is no problem.
  const badCancel: Fetcher = async () => ({ status: 200, headers: new Headers({ "content-type": "image/png" }), body: { cancel: () => Promise.reject(new Error("x")) } }) as unknown as Response;
  assert.equal((await probeFetch(U, badCancel)).ok, true);
  const noBody: Fetcher = async () => new Response(null, { status: 404 });
  assert.equal((await probeFetch(U, noBody)).status, 404);
});

test("probeStore and pool", async () => {
  const bucket = fakeR2();
  await bucket.put("k1", new Uint8Array([1]), { httpMetadata: { contentType: "text/plain" } });
  await bucket.put("k2", new Uint8Array([1]));
  assert.deepEqual(await probeStore(bucket as never, "k1"), { via: "store", ok: false, status: 200, contentType: "text/plain", error: "not_image", agent: null });
  assert.equal((await probeStore(bucket as never, "k2")).error, "not_image");
  assert.equal((await probeStore({ head: async () => ({ size: 1 }) } as never, "k3")).contentType, null);
  bucket.state.failOn = /head/;
  assert.deepEqual(await probeStore(bucket as never, "k1"), { via: "store", ok: false, status: null, contentType: null, error: "fetch_failed", agent: null });
  let running = 0;
  let peak = 0;
  const out = await pool([5, 1, 3, 2, 4], 2, async (n) => {
    peak = Math.max(peak, ++running);
    await new Promise((r) => setTimeout(r, n));
    running--;
    return n * 10;
  });
  assert.deepEqual(out, [50, 10, 30, 20, 40]);
  assert.equal(peak, 2);
  assert.deepEqual(await pool([], 4, async () => 1), []);
});

test("checkImages: an invalid base path never treats a URL as the worker's own", async () => {
  const bucket = fakeR2();
  const env = { NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: bucket, NEWSLETTER_UI_BASE: "/../x" } as never;
  const html = `<img src="${SITE}/../x/hooks/a/${KEY}" alt="a"><img src="${SITE}/newsletter-admin/hooks/a/not-a-key.png" alt="b">`;
  const net = fetcher({});
  const r = await checkImages(env, { url: SITE, host: "example.com" }, html, net.f);
  assert.ok(r.items.every((i) => i.probe!.via === "fetch"));
  assert.equal(bucket.calls.length, 0);
  assert.deepEqual(imagesOf('<p><img src="https://example.com/a.png"></p>'), [{ url: "https://example.com/a.png", alt: null }]);
  const own = { NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: bucket } as never;
  const r2 = await checkImages(own, { url: SITE, host: "example.com" }, `<img src="${SITE}/newsletter-admin/hooks/a/not-a-key.png" alt="b">`, net.f);
  assert.equal(r2.items[0].probe!.via, "fetch", "a name the store never writes is fetched, not looked up");
});
