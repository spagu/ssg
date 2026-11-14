// The module-management routes (GO-149): the view with and without CORE_DB,
// who may use them in each sign-in mode, one module's settings, the order —
// and that every change lands with its change-log row.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as manage from "../functions/api/core/manage/index.ts";
import * as modulePatch from "../functions/api/core/manage/modules/[id].ts";
import * as order from "../functions/api/core/manage/order.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { ACCESS_ENV, call, accessIssuer } from "./harness.ts";
import { fakeD1 } from "./d1.ts";
import { IDS, authEnv, authHeaders, dbEnv, pw, send, useSite } from "./manage-fixture.ts";

beforeEach(useSite);
after(() => useRegistry());

const view = async (env: Record<string, unknown>, headers = pw(), query = "") => call(manage.onRequest, { path: `/api/core/manage${query}`, headers, env });
const patch = (env: Record<string, unknown>, id: string, body: unknown, headers = pw()) => send(modulePatch.onRequest, "PATCH", `/api/core/manage/modules/${id}`, env, body, headers);
const put = (env: Record<string, unknown>, body: unknown, headers = pw()) => send(order.onRequest, "PUT", "/api/core/manage/order", env, body, headers);

test("without CORE_DB: read-only, the defaults, the available templates with their command and snippet", async () => {
  const r = await view({ CORE_ADMIN_PASSWORD: "core-pass" }, pw(), "?lang=pl");
  assert.equal(r.status, 200, r.body);
  const v = r.json();
  assert.deepEqual([v.writable, v.readOnly, v.permission, v.mode, v.lang], [false, "no_database", "core:manage", "password", "pl"]);
  assert.deepEqual(v.items.map((i: { id: string }) => i.id), IDS);
  const comments = v.items[1];
  assert.deepEqual(comments, {
    id: "comments", kind: "pages-function", title: "comments title (pl)", panel: "/comments-admin.html", hidden: false, position: null, labels: {}, note: "",
    updatedAt: null, updatedBy: null, disable: "Remove it. (pl)", status: null,
  });
  assert.deepEqual([v.items[3].title, v.items[3].disable, v.items[3].panel], ["shop", null, null], "a module without a manifest: its id, no disable text");
  assert.deepEqual(v.available, [
    { id: "contact-form", kind: "pages-function", title: "Contact form", command: "ssg new worker contact-form", snippet: "workers:\n  - name: contact-form\n    dir: workers/contact-form" },
    { id: "newsletter-ui", kind: "worker", title: "Newsletter", command: "ssg new worker newsletter-ui", snippet: "modules:\n  - id: newsletter-ui\n    url: /newsletter-ui\n    dir: workers/newsletter-ui" },
  ]);
  assert.deepEqual([v.history, v.changes, v.retention, v.limits], [[], [], { historyDays: 90, changesDays: 365 }, { label: 60, note: 500, importBytes: 65536 }]);
});

test("without CORE_DB every change is refused with what to do; nothing is kept in memory", async () => {
  const env = { CORE_ADMIN_PASSWORD: "core-pass" };
  for (const r of [await patch(env, "blog", { hidden: true }), await put(env, { order: IDS })]) {
    assert.equal(r.status, 503);
    assert.equal(r.json().error, "no_database");
    assert.match(r.json().detail, /CORE_DB/);
  }
});

test("a change needs the CSRF header; a GET does not", async () => {
  const { env } = dbEnv();
  const r = await call(modulePatch.onRequest, { method: "PATCH", path: "/api/core/manage/modules/blog", env, headers: pw(), body: '{"hidden":true}' });
  assert.equal(r.status, 403);
  assert.equal(r.json().error, "csrf_header_missing");
  assert.equal((await call(modulePatch.onRequest, { path: "/api/core/manage/modules/blog", env, headers: pw() })).status, 405);
});

test("auth mode: without core:manage no route reads or changes anything; with it, the change is logged under the account", async () => {
  const db = fakeD1();
  const env = authEnv(db);
  const reader = await authHeaders(["comments:read"]);
  for (const r of [await view(env, reader), await patch(env, "blog", { hidden: true }, reader), await put(env, { order: IDS }, reader)]) {
    assert.equal(r.status, 403);
    assert.deepEqual(r.json(), { error: "forbidden", permission: "core:manage" });
  }
  assert.equal(db.calls.filter((s) => /core_module_settings/.test(s) && !/CREATE/.test(s)).length, 0, "nothing was even read");
  const manager = await authHeaders(["core:manage"]);
  assert.equal((await patch(env, "blog", { hidden: true }, manager)).status, 200);
  assert.deepEqual(db.rows("SELECT actor, actor_id, module_id, action FROM core_changes"), [{ actor: "ada@example.com", actor_id: "u1", module_id: "blog", action: "hidden" }]);
  assert.equal((await view(env, await authHeaders(["*"]))).json().items.find((i: { id: string }) => i.id === "blog").updatedBy, "ada@example.com");
});

test("Cloudflare Access: every signed-in admin manages; the Access address is the actor", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const db = fakeD1();
    const headers = { "cf-access-jwt-assertion": await issuer.token() };
    const r = await patch({ ...ACCESS_ENV, CORE_DB: db }, "comments", { note: "Moderated by the kitchen team." }, headers);
    assert.equal(r.status, 200, r.body);
    assert.deepEqual(db.rows("SELECT actor, actor_id FROM core_changes"), [{ actor: "ada@example.com", actor_id: null }]);
  } finally {
    restore();
  }
});

test("PATCH: hidden, labels and note, each change logged with from/to; a request that changes nothing writes nothing", async () => {
  const { env, db } = dbEnv();
  let r = await patch(env, "comments", { hidden: true, labels: { en: "  Reader comments ", pl: "Komentarze czytelników", de: "" }, note: "Ask Ada before hiding." });
  assert.equal(r.status, 200, r.body);
  const item = r.json().item;
  assert.deepEqual([item.hidden, item.labels, item.note, item.updatedBy, r.json().changed], [true, { en: "Reader comments", pl: "Komentarze czytelników" }, "Ask Ada before hiding.", "password", 3]);
  assert.deepEqual(db.rows("SELECT actor, module_id, action, detail FROM core_changes ORDER BY action"), [
    { actor: "password", module_id: "comments", action: "hidden", detail: "{}" },
    { actor: "password", module_id: "comments", action: "labels", detail: JSON.stringify({ from: {}, to: { en: "Reader comments", pl: "Komentarze czytelników" } }) },
    { actor: "password", module_id: "comments", action: "note", detail: JSON.stringify({ from: "", to: "Ask Ada before hiding." }) },
  ]);
  const writes = db.calls.length;
  r = await patch(env, "comments", { hidden: true, labels: { pl: "Komentarze czytelników", en: "Reader comments" } });
  assert.deepEqual([r.status, r.json().changed], [200, 0]);
  assert.ok(!db.calls.slice(writes).some((s) => /^\s*INSERT/.test(s)), "nothing written");
  r = await patch(env, "comments", { hidden: false });
  assert.equal(db.rows("SELECT action FROM core_changes ORDER BY at DESC, rowid DESC LIMIT 1")[0].action, "shown");
  const v = (await view(env)).json();
  assert.deepEqual([v.writable, v.readOnly], [true, null]);
  assert.deepEqual(v.changes.map((c: { action: string }) => c.action), ["shown", "note", "labels", "hidden"], "newest first");
  assert.deepEqual(Object.keys(v.changes[0]).sort(), ["action", "actor", "at", "detail", "module"]);
});

test("PATCH refuses an unknown module, a body too large or not JSON, and every malformed field", async () => {
  const { env } = dbEnv();
  const cases: [string, unknown, number, string][] = [
    ["nope", { hidden: true }, 404, "unknown_module"],
    ["blog", "{", 400, "invalid_json"],
    ["blog", "x".repeat(20_000), 413, "too_large"],
    ["blog", [], 422, "invalid_settings"],
    ["blog", {}, 422, "invalid_settings"],
    ["blog", { hidden: "yes" }, 422, "invalid_settings"],
    ["blog", { colour: "red" }, 422, "invalid_settings"],
    ["blog", { labels: { english: "x" } }, 422, "invalid_settings"],
    ["blog", { labels: ["x"] }, 422, "invalid_settings"],
    ["blog", { labels: { en: 3 } }, 422, "invalid_settings"],
    ["blog", { labels: { en: "x".repeat(61) } }, 422, "invalid_settings"],
    ["blog", { labels: { en: "two\nlines" } }, 422, "invalid_settings"],
    ["blog", { note: 1 }, 422, "invalid_settings"],
    ["blog", { note: "x".repeat(501) }, 422, "invalid_settings"],
    ["blog", { note: "bell\u0007" }, 422, "invalid_settings"],
  ];
  for (const [id, body, status, error] of cases) {
    const r = await patch(env, id, body);
    assert.deepEqual([r.status, r.json().error], [status, error], `${id} ${JSON.stringify(body)}`);
    assert.ok(r.json().detail, "every refusal says why");
  }
  const big = await call(modulePatch.onRequest, { method: "PATCH", path: "/api/core/manage/modules/blog", env, headers: { ...pw(), "x-core-admin": "1", "content-length": "999999" }, body: "{}" });
  assert.equal(big.status, 413, "a declared length over the cap is refused before reading");
  assert.equal((await patch(env, "blog", { note: "line one\nline two" })).status, 200, "a note may have line breaks");
});

test("PUT order: every module once; the view and the stored positions follow; unchanged writes nothing", async () => {
  const { env, db } = dbEnv();
  const next = ["web-push", "comments", "blog", "core", "shop"];
  let r = await put(env, { order: next });
  assert.deepEqual([r.status, r.json()], [200, { order: next, changed: 1 }]);
  assert.deepEqual((await view(env)).json().items.map((i: { id: string; position: number }) => `${i.id}:${i.position}`), ["web-push:0", "comments:1", "blog:2", "core:3", "shop:4"]);
  assert.deepEqual(JSON.parse(db.rows("SELECT detail FROM core_changes WHERE action = 'order'")[0].detail as string), { from: IDS, to: next });
  r = await put(env, { order: next });
  assert.deepEqual(r.json(), { order: next, changed: 0 });
  const refusals: [unknown, string][] = [
    [{ order: "blog" }, "invalid_settings"],
    [{ order: IDS, extra: 1 }, "invalid_settings"],
    [{ order: [...IDS.slice(1), 3] }, "invalid_settings"],
    [{ order: [...IDS, "nope"] }, "unknown_modules"],
    [{ order: [...IDS, "blog"] }, "invalid_settings"],
    [{ order: IDS.slice(1) }, "invalid_settings"],
  ];
  for (const [body, error] of refusals) assert.deepEqual([(await put(env, body)).status, (await put(env, body)).json().error], [422, error], JSON.stringify(body));
  assert.equal((await put(env, "[")).status, 400);
});

test("a database failure: the view stays usable read-only; a change is a plain 500", async () => {
  const { env, db } = dbEnv();
  await patch(env, "blog", { hidden: true }); // the schema exists
  db.state.failOn = /FROM core_changes/;
  const v = (await view(env)).json();
  assert.deepEqual([v.writable, v.readOnly, v.items.length, v.changes], [false, "database_error", 5, []]);
  db.state.failOn = /INSERT INTO core_module_settings/;
  const r = await patch(env, "comments", { hidden: true });
  assert.deepEqual([r.status, r.json()], [500, { error: "internal_error" }]);
});

test("a registry missing or retention variables set: the view says so", async () => {
  const { env } = dbEnv(undefined, { CORE_HISTORY_DAYS: "7", CORE_CHANGES_DAYS: "30" });
  assert.deepEqual((await view(env)).json().retention, { historyDays: 7, changesDays: 30 });
  useRegistry({ format: 99, modules: [] });
  assert.equal((await view(env)).json().error, "registry_unsupported");
  assert.equal((await patch(env, "blog", { hidden: true })).json().error, "registry_unsupported");
});
