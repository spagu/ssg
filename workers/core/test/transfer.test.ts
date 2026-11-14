// Export and import of module settings (GO-149): the file, a strict check of
// what comes back, a preview that lists every change, an apply bound to that
// preview by its fingerprint, and one atomic batch.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as exporter from "../functions/api/core/manage/export.ts";
import * as importer from "../functions/api/core/manage/import.ts";
import * as modulePatch from "../functions/api/core/manage/modules/[id].ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { checkImport } from "../functions/api/core/_settings-check.ts";
import { PW_ENV, call } from "./harness.ts";
import { fakeD1 } from "./d1.ts";
import { IDS, dbEnv, pw, send, useSite } from "./manage-fixture.ts";

beforeEach(useSite);
after(() => useRegistry());

const download = (env: Record<string, unknown>) => call(exporter.onRequest, { path: "/api/core/manage/export", env, headers: pw() });
const upload = (env: Record<string, unknown>, body: unknown, query = "") => send(importer.onRequest, "POST", `/api/core/manage/import${query}`, env, body);
const doc = (modules: unknown[], extra: Record<string, unknown> = {}) => ({ kind: "ssg-core-settings", format: 1, exportedAt: "2026-11-14T10:00:00.000Z", modules, ...extra });

test("export: every module in bar order, as an attachment; needs CORE_DB", async () => {
  const { env } = dbEnv();
  await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/shop", env, { hidden: true, labels: { en: "Shop" }, note: "Seasonal." });
  const r = await download(env);
  assert.equal(r.status, 200);
  assert.equal(r.headers["content-disposition"], 'attachment; filename="core-settings.json"');
  const d = r.json();
  assert.deepEqual([d.kind, d.format, typeof d.exportedAt], ["ssg-core-settings", 1, "string"]);
  assert.deepEqual(d.modules.map((m: { id: string }) => m.id), IDS);
  assert.deepEqual(d.modules[3], { id: "shop", hidden: true, position: null, labels: { en: "Shop" }, note: "Seasonal." });
  assert.deepEqual(checkImport(d, IDS), { ok: true, value: d.modules }, "an export is a valid import");
  assert.equal((await download(PW_ENV)).json().error, "no_database");
});

test("import: preview lists every change (resets included), apply needs the preview's fingerprint, then all at once", async () => {
  const from = dbEnv();
  await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/blog", from.env, { hidden: true, note: "Old blog." });
  const file = (await download(from.env)).json();

  const { env, db } = dbEnv();
  await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/comments", env, { labels: { pl: "Komentarze" } });
  const preview = await upload(env, file);
  assert.equal(preview.status, 200, preview.body);
  const p = preview.json();
  assert.equal(p.applied, false);
  assert.match(p.fingerprint, /^[0-9a-f]{64}$/);
  assert.deepEqual(p.changes, [
    { module: "blog", field: "hidden", from: false, to: true },
    { module: "blog", field: "note", from: "", to: "Old blog." },
    { module: "comments", field: "labels", from: { pl: "Komentarze" }, to: {} },
  ]);
  assert.equal(db.rows("SELECT COUNT(*) AS n FROM core_changes WHERE action = 'import'")[0].n, 0, "a preview writes nothing");

  const stale = await upload(env, file, "?apply=" + "0".repeat(64));
  assert.deepEqual([stale.status, stale.json().error], [409, "settings_changed"]);
  const applied = await upload(env, file, "?apply=" + p.fingerprint);
  assert.deepEqual([applied.status, applied.json().applied, applied.json().changes.length], [200, true, 3]);
  assert.deepEqual(db.rows("SELECT module_id, hidden, note, labels FROM core_module_settings WHERE hidden = 1 OR note <> '' OR labels <> '{}'"), [
    { module_id: "blog", hidden: 1, note: "Old blog.", labels: "{}" },
  ]);
  assert.deepEqual(JSON.parse(db.rows("SELECT detail FROM core_changes WHERE action = 'import'")[0].detail as string), { modules: 5, changes: 3 });

  const again = await upload(env, file);
  assert.deepEqual(again.json().changes, [], "the same file again changes nothing");
  const noop = await upload(env, file, "?apply=" + again.json().fingerprint);
  assert.deepEqual(noop.json(), { applied: true, changes: [] });
  assert.equal(db.rows("SELECT COUNT(*) AS n FROM core_changes WHERE action = 'import'")[0].n, 1, "a no-op import is not logged");
});

test("import is atomic: a failure in the batch leaves the settings as they were", async () => {
  const { env, db } = dbEnv();
  await send(modulePatch.onRequest, "PATCH", "/api/core/manage/modules/blog", env, { note: "Keep me." });
  const file = doc([{ id: "comments", hidden: true }]);
  const fp = (await upload(env, file)).json().fingerprint;
  db.state.failOn = /INSERT INTO core_changes/;
  assert.equal((await upload(env, file, "?apply=" + fp)).status, 500);
  assert.deepEqual(db.rows("SELECT module_id, note, hidden FROM core_module_settings"), [{ module_id: "blog", note: "Keep me.", hidden: 0 }]);
});

test("import refuses what is not exactly a settings file for this site", async () => {
  const { env } = dbEnv();
  const cases: [unknown, number, string, RegExp][] = [
    ["{", 400, "invalid_json", /not JSON/],
    [JSON.stringify(doc([])) + " ".repeat(70_000), 413, "too_large", /65536/],
    [[], 422, "invalid_settings", /not a JSON object/],
    [doc([], { kind: "other" }), 422, "invalid_settings", /kind/],
    [doc([], { format: 2 }), 422, "invalid_settings", /format 2/],
    [doc([], { exportedAt: 5 }), 422, "invalid_settings", /exportedAt/],
    [doc([], { extra: true }), 422, "invalid_settings", /unknown field "extra"/],
    [{ ...doc([]), modules: {} }, 422, "invalid_settings", /must be a list/],
    [doc(Array.from({ length: 501 }, () => ({ id: "blog" }))), 422, "invalid_settings", /more than 500/],
    [doc(["blog"]), 422, "invalid_settings", /modules\[0\] must be an object/],
    [doc([{ id: "Blog" }]), 422, "invalid_settings", /modules\[0\]\.id/],
    [doc([{ id: "blog", extra: 1 }]), 422, "invalid_settings", /unknown field/],
    [doc([{ id: "blog", hidden: 1 }]), 422, "invalid_settings", /hidden/],
    [doc([{ id: "blog", position: -1 }]), 422, "invalid_settings", /position/],
    [doc([{ id: "blog", position: 1.5 }]), 422, "invalid_settings", /position/],
    [doc([{ id: "blog", position: 10_000 }]), 422, "invalid_settings", /position/],
    [doc([{ id: "blog", labels: { xx_y: "a" } }]), 422, "invalid_settings", /language code/],
    [doc([{ id: "blog", note: 7 }]), 422, "invalid_settings", /note/],
    [doc([{ id: "nope" }, { id: "gone" }, { id: "nope" }]), 422, "unknown_modules", /nope, gone$/],
    [doc([{ id: "blog" }, { id: "blog" }]), 422, "invalid_settings", /twice/],
  ];
  for (const [body, status, error, detail] of cases) {
    const r = await upload(env, body);
    assert.deepEqual([r.status, r.json().error], [status, error], JSON.stringify(body).slice(0, 80));
    assert.match(r.json().detail, detail);
  }
  const minimal = await upload(env, doc([{ id: "blog" }]).modules ? { kind: "ssg-core-settings", format: 1, modules: [{ id: "blog", position: null }] } : null);
  assert.equal(minimal.status, 200, "exportedAt and every field but id are optional");
  assert.equal((await upload({ ...PW_ENV, CORE_DB: fakeD1() }, doc([]), "")).json().changes.length, 0);
});
