// The optional width cap on upload (#446): with the Images binding a wider
// image is stored as a capped copy of the same type, found again by the
// upload's hash; without it, or when the transform fails, the file is stored
// as uploaded.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { assetMaxWidth, capWidth } from "../src/asset-resize.ts";
import { sniffImage } from "../src/asset-image.ts";
import { MIGRATIONS, migrate } from "../src/schema.ts";
import { PASSWORD, fakeCtx, fakeD1, fakeImages, fakeR2, makeEnv, req } from "./fakes.ts";
import { jpeg, png, webpExtended } from "./asset-fixtures.ts";

const B = "/newsletter-admin";
const SITE = "https://example.com";
type Env = ReturnType<typeof makeEnv> & { NEWSLETTER_ASSETS: ReturnType<typeof fakeR2> };

const hex = async (b: Uint8Array) => [...new Uint8Array(await crypto.subtle.digest("SHA-256", b))].map((x) => x.toString(16).padStart(2, "0")).join("");
const env = (over: Record<string, unknown> = {}) => makeEnv({ NEWSLETTER_ASSETS: fakeR2(), NEWSLETTER_SITE_URL: SITE, ...over }) as Env;
const upload = async (e: Env, body: Uint8Array) => {
  const res = await handleFetch(req(B + "/api/assets", { method: "POST", body, auth: PASSWORD }), e as never, fakeCtx() as never);
  return { status: res.status, body: await res.json() };
};
const sourceOf = (e: Env, key: string) => e.NEWSLETTER_DB.sqlite.prepare("SELECT source_sha256 FROM assets WHERE key = ?").get(key)?.source_sha256;
const puts = (e: Env) => e.NEWSLETTER_ASSETS.calls.filter((c) => c.startsWith("put")).length;

// scaled answers the way the binding would: a header of the requested width,
// in the requested format, height kept in proportion to the input's.
const scaled = (input: Uint8Array, o: { width: number; format: string }) => {
  const i = sniffImage(input)!;
  const h = Math.round((i.height! * o.width) / i.width!);
  return o.format === "image/png" ? png(o.width, h) : o.format === "image/webp" ? webpExtended(o.width, h) : jpeg(o.width, h);
};

test("without the binding: stored as uploaded, the answer exactly the wave-1 record", async () => {
  const e = env();
  const file = jpeg(3000, 2000, { exif: true });
  const { status, body } = await upload(e, file);
  assert.equal(status, 201);
  assert.deepEqual(Object.keys(body).sort(), ["bytes", "createdAt", "height", "key", "metadata", "type", "url", "width"]);
  assert.equal(body.key, `${await hex(file)}.jpg`);
  assert.deepEqual(e.NEWSLETTER_ASSETS.objects.get(body.key)!.bytes, file, "byte for byte");
  assert.equal(sourceOf(e, body.key), null);
  assert.deepEqual(Object.keys((await upload(e, file)).body).sort(), Object.keys(body).sort());
});

test("with the binding: a wider JPEG is stored capped, found again by the upload's hash", async () => {
  let n = 0;
  // Not deterministic: each transform gives different bytes.
  const images = fakeImages((input, o) => jpeg(o.width, 800 + n++));
  const e = env({ NEWSLETTER_IMAGES: images });
  const file = jpeg(2400, 1600, { exif: true });
  const first = await upload(e, file);
  assert.equal(first.status, 201);
  const stored = e.NEWSLETTER_ASSETS.objects.get(first.body.key)!.bytes;
  assert.equal(first.body.key, `${await hex(stored)}.jpg`, "the key is the hash of the bytes stored");
  assert.notEqual(first.body.key, `${await hex(file)}.jpg`);
  assert.deepEqual([first.body.width, first.body.height, first.body.bytes, first.body.metadata], [1200, 800, stored.length, false]);
  assert.deepEqual(first.body.resize, { resized: true, maxWidth: 1200, original: { width: 2400, height: 1600, bytes: file.length }, error: null });
  assert.deepEqual(images.calls, [{ width: 1200, format: "image/jpeg", quality: 80, inputBytes: file.length }]);
  assert.equal(sourceOf(e, first.body.key), await hex(file));

  const again = await upload(e, file);
  assert.equal(again.status, 200);
  assert.equal(again.body.key, first.body.key, "stored once although a new transform would differ");
  assert.equal(again.body.resize.resized, true);
  assert.equal(images.calls.length, 1, "not transformed again");
  assert.equal(puts(e), 1);

  // The copy's object gone (a half-finished delete): made again, listed anew.
  e.NEWSLETTER_ASSETS.objects.delete(first.body.key);
  const healed = await upload(e, file);
  assert.equal(healed.status, 201);
  assert.ok(e.NEWSLETTER_ASSETS.objects.has(healed.body.key));
  const third = await upload(e, file);
  assert.equal(third.body.key, healed.body.key, "the newest copy with bytes answers");
  // Without the binding later, the same file still finds its copy.
  const off = { ...e, NEWSLETTER_IMAGES: undefined } as Env;
  const plain = await upload(off, file);
  assert.deepEqual([plain.status, plain.body.key, plain.body.resize], [200, healed.body.key, undefined]);
});

test("a deterministic transform whose object is gone: the row stays, the bytes come back", async () => {
  const e = env({ NEWSLETTER_IMAGES: fakeImages(scaled) });
  const file = png(2000, 1000);
  const first = await upload(e, file);
  e.NEWSLETTER_ASSETS.objects.delete(first.body.key);
  const again = await upload(e, file);
  assert.equal(again.status, 200);
  assert.equal(again.body.key, first.body.key);
  assert.equal(again.body.resize.resized, true);
  assert.ok(e.NEWSLETTER_ASSETS.objects.has(first.body.key));
});

test("formats are kept: PNG without quality (no palette), WebP stays WebP", async () => {
  const images = fakeImages(scaled);
  const e = env({ NEWSLETTER_IMAGES: images, NEWSLETTER_ASSETS_MAX_WIDTH: "800" });
  const p = await upload(e, png(1600, 400, { exif: true }));
  assert.deepEqual([p.body.type, p.body.width, p.body.height, p.body.metadata], ["image/png", 800, 200, false]);
  const w = await upload(e, webpExtended(1000, 1000));
  assert.deepEqual([w.body.type, w.body.width], ["image/webp", 800]);
  assert.deepEqual(images.calls.map(({ inputBytes, ...o }) => (void inputBytes, o)), [
    { width: 800, format: "image/png" },
    { width: 800, format: "image/webp", quality: 80 },
  ]);
  const settings = await handleFetch(req(B + "/api/settings", { auth: PASSWORD }), e as never, fakeCtx() as never);
  assert.equal((await settings.json()).assets.maxWidth, 800);
});

test("not wider than the cap, or no width: stored as uploaded, never enlarged", async () => {
  const images = fakeImages(scaled);
  const e = env({ NEWSLETTER_IMAGES: images });
  for (const file of [jpeg(1200, 100), png(10, 10), jpeg(1, 1).subarray(0, 20)]) {
    const { status, body } = await upload(e, file);
    assert.equal(status, 201);
    assert.equal(body.key, `${await hex(file)}.${body.key.split(".")[1]}`);
    assert.deepEqual(e.NEWSLETTER_ASSETS.objects.get(body.key)!.bytes, file);
    assert.equal(body.resize.resized, false);
    assert.equal(body.resize.error, null);
  }
  assert.equal(images.calls.length, 0);
});

test("a failing transform never loses the upload: the original is stored, the answer says why", async () => {
  const images = fakeImages(scaled);
  images.state.fail = true;
  const e = env({ NEWSLETTER_IMAGES: images });
  const file = jpeg(5000, 100, { exif: true });
  const { status, body } = await upload(e, file);
  assert.equal(status, 201);
  assert.equal(body.key, `${await hex(file)}.jpg`);
  assert.deepEqual([body.width, body.metadata], [5000, true]);
  assert.deepEqual(body.resize, { resized: false, maxWidth: 1200, original: { width: 5000, height: 100, bytes: file.length }, error: "transform_failed" });
  assert.equal(sourceOf(e, body.key), null);
  // The same file again: the stored original answers; no retry is made.
  images.state.fail = false;
  const again = await upload(e, file);
  assert.deepEqual([again.status, again.body.key, again.body.resize.resized, again.body.resize.error], [200, body.key, false, null]);
});

test("unexpected output — another type, still too wide, no width, not an image — stores the original", async () => {
  const outputs = [png(1200, 10), jpeg(1300, 10), jpeg(1, 1).subarray(0, 20), new Uint8Array([1, 2, 3])];
  for (const out of outputs) {
    const e = env({ NEWSLETTER_IMAGES: fakeImages(() => out) });
    const file = jpeg(2000, 10);
    const { body } = await upload(e, file);
    assert.equal(body.key, `${await hex(file)}.jpg`);
    assert.equal(body.resize.error, "unexpected_output");
  }
});

test("the race: a capped copy someone else just listed answers 200 with its row", async () => {
  const e = env({ NEWSLETTER_IMAGES: fakeImages(scaled) });
  const file = jpeg(2400, 1200);
  const out = scaled(file, { width: 1200, format: "image/jpeg" });
  const key = `${await hex(out)}.jpg`;
  // Listed by another upload between this one's lookup and its insert.
  const db = e.NEWSLETTER_DB;
  const prepare = db.prepare.bind(db);
  db.prepare = ((sql: string) => {
    if (sql.startsWith("INSERT OR IGNORE INTO assets")) {
      db.sqlite.prepare("INSERT INTO assets (key, type, bytes, created_at) VALUES (?, 'image/jpeg', 1, '2026-01-01')").run(key);
    }
    return prepare(sql);
  }) as typeof db.prepare;
  const { status, body } = await upload(e, file);
  assert.deepEqual([status, body.key, body.bytes, body.resize.resized], [200, key, 1, true]);
});

test("assetMaxWidth clamps; capWidth without the binding returns the input", async () => {
  assert.equal(assetMaxWidth({} as never), 1200);
  assert.equal(assetMaxWidth({ NEWSLETTER_ASSETS_MAX_WIDTH: "10" } as never), 320);
  assert.equal(assetMaxWidth({ NEWSLETTER_ASSETS_MAX_WIDTH: "99999" } as never), 4000);
  assert.equal(assetMaxWidth({ NEWSLETTER_ASSETS_MAX_WIDTH: "wide" } as never), 1200);
  const file = jpeg(5000, 10);
  const r = await capWidth({} as never, file, sniffImage(file)!);
  assert.equal(r.bytes, file);
  assert.equal(r.report, null);
});

test("migration 8 adds source_sha256 to a version-7 assets table", async () => {
  const d1 = fakeD1();
  await migrate(d1 as never, MIGRATIONS.filter((m) => m.version < 8));
  d1.sqlite.exec("DROP TABLE assets");
  d1.sqlite.exec("CREATE TABLE assets (key TEXT PRIMARY KEY, type TEXT NOT NULL, bytes INTEGER NOT NULL, width INTEGER, height INTEGER, metadata INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)");
  await migrate(d1 as never, MIGRATIONS);
  const cols = d1.sqlite.prepare("PRAGMA table_info(assets)").all().map((c) => c.name);
  assert.ok(cols.includes("source_sha256"));
  assert.equal(d1.sqlite.prepare("SELECT version FROM schema_meta").get()!.version, 8);
});
