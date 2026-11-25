// The campaign image store through the whole pipeline (#446): upload, list,
// delete, serving, the switch, and what happens when R2 and the table disagree.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { rendererFor } from "../src/click-track.ts";
import { notModified } from "../src/asset-serve.ts";
import { PASSWORD, fakeCtx, fakeR2, makeEnv, req } from "./fakes.ts";
import { gif, jpeg, png, svg, webpLossy } from "./asset-fixtures.ts";

const B = "/newsletter-admin";
const SITE = "https://example.com";
type Env = ReturnType<typeof makeEnv> & { NEWSLETTER_ASSETS?: ReturnType<typeof fakeR2> };

const assetEnv = (over: Record<string, unknown> = {}): Env =>
  makeEnv({ NEWSLETTER_ASSETS: fakeR2(), NEWSLETTER_SITE_URL: SITE, ...over }) as Env;
const call = (env: Env, path: string, init: Parameters<typeof req>[1] = {}) =>
  handleFetch(req(B + path, { auth: PASSWORD, ...init }), env as never, fakeCtx() as never);
const upload = (env: Env, body: BodyInit | null, headers: Record<string, string> = {}) => call(env, "/api/assets", { method: "POST", body, headers });
const hex = async (b: Uint8Array) => [...new Uint8Array(await crypto.subtle.digest("SHA-256", b))].map((x) => x.toString(16).padStart(2, "0")).join("");

test("upload: stored once under the hash of its bytes, with the header's facts", async () => {
  const env = assetEnv();
  const file = jpeg(800, 600, { exif: true });
  // The client's content-type is ignored: the bytes decide.
  const res = await upload(env, file, { "content-type": "image/png" });
  assert.equal(res.status, 201);
  const rec = await res.json();
  const key = `${await hex(file)}.jpg`;
  assert.deepEqual({ ...rec, createdAt: typeof rec.createdAt }, {
    key, url: `${SITE}${B}/hooks/a/${key}`, type: "image/jpeg", bytes: file.length, width: 800, height: 600, metadata: true, createdAt: "string",
  });
  const stored = env.NEWSLETTER_ASSETS!.objects.get(key)!;
  assert.deepEqual(stored.httpMetadata, { contentType: "image/jpeg", cacheControl: "public, max-age=31536000, immutable" });
  assert.deepEqual(stored.bytes, file, "the file is stored unchanged");

  const again = await upload(env, file);
  assert.equal(again.status, 200);
  assert.deepEqual(await again.json(), rec);
  assert.equal(env.NEWSLETTER_ASSETS!.calls.filter((c) => c.startsWith("put")).length, 1, "no second object");
  // A header that does not give the size: stored, with null dimensions.
  const sizeless = await (await upload(env, jpeg(1, 1).subarray(0, 20))).json();
  assert.deepEqual([sizeless.width, sizeless.height, sizeless.metadata], [null, null, false]);
  const listed = (await (await call(env, "/api/assets")).json()).items.find((i: { key: string }) => i.key === sizeless.key);
  assert.deepEqual([listed.width, listed.height], [null, null]);
  const audit = env.NEWSLETTER_DB.sqlite.prepare("SELECT action, status FROM admin_audit").all().map((r) => `${r.action} ${r.status}`);
  assert.deepEqual(audit, ["asset.upload 201", "asset.upload 200", "asset.upload 201"]);
});

test("upload: refusals — type, size (declared and actual), empty, CSRF", async () => {
  const env = assetEnv({ NEWSLETTER_ASSETS_MAX_BYTES: "100" });
  for (const bad of [svg(), gif(), new TextEncoder().encode("hello")]) {
    const res = await upload(env, bad, { "content-type": "image/jpeg" });
    assert.equal(res.status, 415);
    const body = await res.json();
    assert.equal(body.error, "unsupported_type");
    assert.ok(!JSON.stringify(body).includes("svg") && !JSON.stringify(body).includes("GIF"), "no request bytes echoed");
  }
  const big = new Uint8Array(101);
  big.set(png(1, 1));
  assert.equal((await upload(env, big)).status, 413, "refused after reading");
  assert.equal((await upload(env, png(1, 1), { "content-length": "5000000" })).status, 413, "refused by content-length");
  // A body that lies about its length is still cut off at the cap.
  let pulled = 0;
  const stream = new ReadableStream<Uint8Array>({
    pull(c) {
      pulled++;
      c.enqueue(new Uint8Array(60));
    },
  });
  const lying = await handleFetch(
    new Request(SITE + B + "/api/assets", { method: "POST", body: stream, duplex: "half", headers: { authorization: req("/", { auth: PASSWORD }).headers.get("authorization")!, "x-newsletter-admin": "1", "content-length": "10" } } as RequestInit),
    env as never,
    fakeCtx() as never,
  );
  assert.equal(lying.status, 413);
  assert.ok(pulled < 5, "reading stopped at the cap");
  assert.equal((await (await upload(env, null)).json()).error, "empty_body");
  assert.equal((await upload(env, new Uint8Array(0))).status, 400);
  assert.equal((await call(env, "/api/assets", { method: "POST", body: png(1, 1), csrf: false })).status, 403);
  assert.equal((await call(env, "/api/assets", { method: "POST", body: png(1, 1), auth: "wrong" })).status, 401);
  assert.equal(env.NEWSLETTER_ASSETS!.objects.size, 0);
});

test("without the binding: every asset route is 404 assets_disabled, mail renders as before", async () => {
  const env = makeEnv({ NEWSLETTER_SITE_URL: SITE }) as Env;
  for (const [method, path] of [["GET", "/api/assets"], ["POST", "/api/assets"], ["DELETE", `/api/assets/${"a".repeat(64)}.png`], ["GET", `/hooks/a/${"a".repeat(64)}.png`]]) {
    const res = await call(env, path, { method, body: method === "POST" ? png(1, 1) : undefined });
    assert.equal(res.status, 404, path);
    assert.equal((await res.json()).error, "assets_disabled");
  }
  const c = { id: "c1", subject: "Hi", body: "![x](https://example.com/a.png) {{unsubscribe_url}}", bodyFormat: "markdown" as const, textOverride: null };
  const site = { url: SITE, host: "example.com" };
  const r = { email: "a@example.com", language: "en", token: "t".repeat(43) };
  const without = (await rendererFor(env as never, c, site, [r], false))(r);
  const withStore = (await rendererFor(assetEnv() as never, c, site, [r], false))(r);
  assert.deepEqual(withStore, without);
});

test("a base that cannot work answers 503 on the API; the summary and settings say why", async () => {
  const env = assetEnv({ NEWSLETTER_ASSETS_URL: "http://img.example.com" });
  for (const [method, path] of [["GET", "/api/assets"], ["POST", "/api/assets"], ["DELETE", `/api/assets/${"a".repeat(64)}.png`]]) {
    const res = await call(env, path, { method, body: method === "POST" ? png(1, 1) : undefined });
    assert.equal(res.status, 503, path);
    assert.equal((await res.json()).error, "assets_misconfigured");
  }
  const summary = await (await call(env, "/api/module/summary")).json();
  assert.ok(summary.problems.some((p: { code: string }) => p.code === "assets_unusable"));
  const settings = await (await call(env, "/api/settings")).json();
  assert.equal(settings.assets.enabled, true);
  assert.equal(settings.assets.baseUrl, null);
  assert.match(settings.assets.problem, /NEWSLETTER_ASSETS_URL/);
  const fine = await (await call(assetEnv({ NEWSLETTER_ASSETS_URL: "https://img.example.com" }), "/api/settings")).json();
  assert.deepEqual(fine.assets, { enabled: true, baseUrl: "https://img.example.com", maxBytes: 2097152, maxWidth: null, problem: null });
});

test("list: newest first, paged, with inUse; delete refuses an image any campaign mentions", async () => {
  const env = assetEnv({ NEWSLETTER_ASSETS_URL: "https://img.example.com/n/" });
  const keys: string[] = [];
  for (const file of [png(1, 1), png(2, 2), webpLossy(3, 3)]) keys.push((await (await upload(env, file)).json()).key);
  // Same second: the key breaks the tie, still a stable order.
  env.NEWSLETTER_DB.sqlite.exec("UPDATE assets SET created_at = '2026-01-01T00:00:00.000Z'");
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE assets SET created_at = '2026-02-01T00:00:00.000Z' WHERE key = ?").run(keys[0]);
  const used = keys[1];
  for (const [status, body, text] of [["sent", `![a](https://img.example.com/n/${used})`, null], ["draft", "nothing", `see ${keys[2]}`]]) {
    env.NEWSLETTER_DB.sqlite
      .prepare("INSERT INTO campaigns (id, subject, body, text_override, status, created_at, updated_at) VALUES (?, 's', ?, ?, ?, 'x', 'x')")
      .run(crypto.randomUUID(), body, text, status);
  }
  const first = await (await call(env, "/api/assets?limit=2")).json();
  assert.equal(first.items.length, 2);
  assert.equal(first.items[0].key, keys[0]);
  assert.equal(first.items[0].url, `https://img.example.com/n/${keys[0]}`);
  assert.equal(first.items[0].inUse, false);
  const second = await (await call(env, `/api/assets?limit=2&cursor=${first.nextCursor}`)).json();
  assert.equal(second.nextCursor, null);
  const all = [...first.items, ...second.items];
  assert.deepEqual(all.map((i) => i.key).sort(), [...keys].sort());
  assert.deepEqual(all.filter((i) => i.inUse).map((i) => i.key).sort(), [keys[1], keys[2]].sort());
  assert.equal((await call(env, "/api/assets?limit=0")).status, 400);
  assert.equal((await call(env, "/api/assets?cursor=nope")).status, 400);
  assert.equal((await (await call(env, "/api/assets")).json()).items.length, 3);

  const del = (k: string) => call(env, `/api/assets/${k}`, { method: "DELETE" });
  const busy = await del(used);
  assert.equal(busy.status, 409);
  assert.equal((await busy.json()).error, "asset_in_use");
  assert.equal((await del(keys[2])).status, 409, "a draft's text version counts too");
  assert.equal((await del(`${"f".repeat(64)}.png`)).status, 404);
  assert.equal((await del("not-a-key")).status, 404);
  assert.equal((await del(keys[0])).status, 204);
  assert.equal(env.NEWSLETTER_ASSETS!.objects.has(keys[0]), false);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM assets").get()!.n, 2);
});

test("R2 and the table disagreeing: every step is safely repeatable", async () => {
  const env = assetEnv();
  const r2 = env.NEWSLETTER_ASSETS!;
  const file = png(9, 9);
  const quiet = console.error;
  console.error = () => {};
  try {
    // R2 put fails: nothing listed, nothing stored; the repeat succeeds.
    r2.state.failOn = /^put$/;
    assert.equal((await upload(env, file)).status, 500);
    r2.state.failOn = null;
    // The row insert fails after the put: the object is unlisted; the repeat lists it.
    env.NEWSLETTER_DB.state.failOn = /INSERT OR IGNORE INTO assets/;
    assert.equal((await upload(env, file)).status, 500);
    assert.equal(r2.objects.size, 1);
    env.NEWSLETTER_DB.state.failOn = null;
    const rec = await (await upload(env, file)).json();
    assert.equal(rec.width, 9);
    // The row delete fails after the object is gone: still listed; a repeat upload puts the bytes back…
    env.NEWSLETTER_DB.state.failOn = /DELETE FROM assets/;
    assert.equal((await call(env, `/api/assets/${rec.key}`, { method: "DELETE" })).status, 500);
    env.NEWSLETTER_DB.state.failOn = null;
    assert.equal(r2.objects.has(rec.key), false);
    assert.equal((await upload(env, file)).status, 200);
    assert.equal(r2.objects.has(rec.key), true, "healed");
    // …and a repeat delete finishes the job.
    assert.equal((await call(env, `/api/assets/${rec.key}`, { method: "DELETE" })).status, 204);
    assert.equal((await call(env, `/api/assets/${rec.key}`, { method: "DELETE" })).status, 404);
  } finally {
    console.error = quiet;
  }
});

test("upload race: the second insert of the same file answers with the first row", async () => {
  const env = assetEnv();
  const file = png(4, 4);
  const key = `${await hex(file)}.png`;
  // As if another request inserted the row between this one's SELECT and INSERT.
  const db = env.NEWSLETTER_DB;
  const prepare = db.prepare;
  db.prepare = (sql: string) => {
    if (sql.startsWith("INSERT OR IGNORE INTO assets")) {
      db.sqlite.prepare("INSERT INTO assets (key, type, bytes, width, height, metadata, created_at) VALUES (?, 'image/png', 1, 4, 4, 0, '2026-01-01T00:00:00.000Z')").run(key);
    }
    return prepare(sql);
  };
  const res = await upload(env, file);
  assert.equal(res.status, 200);
  assert.equal((await res.json()).createdAt, "2026-01-01T00:00:00.000Z");
});

test("serve: public, immutable, nosniff, etag/304; only store keys reach R2; nothing recorded", async () => {
  const env = assetEnv();
  const file = webpLossy(5, 5);
  const { key } = await (await upload(env, file)).json();
  const r2 = env.NEWSLETTER_ASSETS!;
  const auditBefore = env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM admin_audit").get()!.n;
  const get = (path: string, init: RequestInit = {}) => handleFetch(new Request(SITE + B + path, init), env as never, fakeCtx() as never);

  const res = await get(`/hooks/a/${key}`);
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "image/webp");
  assert.equal(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  assert.equal(res.headers.get("content-length"), String(file.length));
  assert.deepEqual(new Uint8Array(await res.arrayBuffer()), file);
  const etag = res.headers.get("etag")!;
  assert.ok(etag.startsWith('"'));

  const head = await get(`/hooks/a/${key}`, { method: "HEAD" });
  assert.equal(head.status, 200);
  assert.equal(head.headers.get("etag"), etag);
  assert.equal(await head.text(), "");
  for (const method of ["GET", "HEAD"]) {
    const cached = await get(`/hooks/a/${key}`, { method, headers: { "if-none-match": etag } });
    assert.equal(cached.status, 304, method);
    assert.equal(cached.headers.get("cache-control"), "public, max-age=31536000, immutable");
  }

  const calls = r2.calls.length;
  for (const path of [`/hooks/a/${key.toUpperCase()}`, `/hooks/a/..%2F${key}`, "/hooks/a/x.webp", `/hooks/a/${key.replace(".webp", ".svg")}`]) {
    assert.equal((await get(path)).status, 404, path);
  }
  assert.equal(r2.calls.length, calls, "a name the store never writes is not looked up");
  assert.equal((await get(`/hooks/a/${"0".repeat(64)}.png`)).status, 404);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM admin_audit").get()!.n, auditBefore, "nothing recorded");

  // An object stored without a content type (by hand) is not sniffed into one.
  r2.objects.set(`${"1".repeat(64)}.png`, { bytes: png(1, 1), httpMetadata: {}, uploaded: new Date() });
  assert.equal((await get(`/hooks/a/${"1".repeat(64)}.png`)).headers.get("content-type"), "application/octet-stream");
});

test("notModified: lists, weak tags and *", () => {
  assert.equal(notModified(null, '"a"'), false);
  assert.equal(notModified('"b", W/"a"', '"a"'), true);
  assert.equal(notModified("*", '"a"'), true);
  assert.equal(notModified('"b"', '"a"'), false);
});
