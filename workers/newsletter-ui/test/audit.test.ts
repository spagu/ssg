// GO-137: the audit trail. One row per state-changing route that reached its
// handler, with the right actor in each mode, none for reads; targets are ids,
// never addresses (a GDPR erasure leaves nothing of the person); retention;
// the list endpoint and its filters; a failing write never fails the action.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { DEFAULT_RETENTION_DAYS, listAudit, recordAudit, retentionDays, targetOf } from "../src/audit.ts";
import type { Route } from "../src/router.ts";
import { ROUTES } from "../src/routes.ts";
import { ensureSchema } from "../src/schema.ts";
import { authEnv, session } from "./auth-fixtures.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

const B = "/newsletter-admin";
const rows = (d1: FakeD1) => d1.sqlite.prepare("SELECT * FROM admin_audit ORDER BY rowid").all() as Record<string, unknown>[];
const stub = (r: Route): Route => ({ ...r, handler: () => Response.json({ ok: true }) });
const pathOf = (r: Route) => B + r.path.replace(/:[a-z]+/gi, (p) => (p === ":hash" ? "a".repeat(64) : "x1"));
const send = async (env: Record<string, unknown>, r: Request, routes: Route[] = ROUTES) => handleFetch(r, env as never, fakeCtx() as never, routes);

test("auth mode: every state-changing route writes one row naming the account; reads write none", async () => {
  const env = authEnv();
  await ensureSchema(env as never);
  for (const r of ROUTES.filter((x) => x.path.startsWith("/api/"))) {
    const before = rows(env.NEWSLETTER_DB).length;
    const need = typeof r.permission === "string" ? [r.permission] : [...r.permission];
    const init = r.method === "GET" ? {} : { method: r.method, body: "{}" };
    const res = await send(env, await session(pathOf(r), need, init), [stub(r)]);
    assert.equal(res.status, 200, `${r.method} ${r.path}`);
    const added = rows(env.NEWSLETTER_DB).slice(before);
    if (typeof r.audit !== "string") {
      assert.equal(added.length, 0, `${r.method} ${r.path} must not be audited`);
      continue;
    }
    assert.equal(added.length, 1, `${r.method} ${r.path}`);
    assert.deepEqual({ ...added[0], id: "", at: "" }, {
      id: "", at: "", mode: "auth", actor_id: "u-ada", actor_email: "ada@example.com", action: r.audit,
      target: r.path.includes(":") ? added[0].target : null, status: 200,
    });
  }
});

test("password mode: the actor is the mode; a refused call is recorded with its status", async () => {
  const env = makeEnv();
  await send(env, req(B + "/api/campaigns/nope/pause", { method: "POST", auth: PASSWORD, body: "{}" }));
  const [row] = rows(env.NEWSLETTER_DB);
  assert.deepEqual({ mode: row.mode, actor_id: row.actor_id, actor_email: row.actor_email, action: row.action, target: row.target, status: row.status }, {
    mode: "password", actor_id: null, actor_email: null, action: "campaign.pause", target: "campaign:nope", status: 404,
  });
  // A request that never reached a handler (no credentials, no permission) is not an action.
  await send(env, req(B + "/api/campaigns/nope/pause", { method: "POST", body: "{}" }));
  assert.equal(rows(env.NEWSLETTER_DB).length, 1);
});

test("Access mode: the actor is the mode (this worker does not read Access's identity)", async () => {
  const env = makeEnv();
  await ensureSchema(env as never);
  const route = ROUTES.find((r) => r.audit === "campaign.cancel") as Route;
  await recordAudit(env as never, { mode: "access", user: null, perms: "all" }, route, { id: "c1" }, new Response("{}", { status: 409 }));
  const [row] = rows(env.NEWSLETTER_DB);
  assert.deepEqual([row.mode, row.actor_id, row.target, row.status], ["access", null, "campaign:c1", 409]);
});

test("a created campaign is recorded by the id it was given", async () => {
  const env = authEnv();
  const res = await send(env, await session(B + "/api/campaigns", ["newsletter:edit"], { method: "POST", body: JSON.stringify({ subject: "Hi" }) }));
  assert.equal(res.status, 201);
  const { id } = await res.json();
  assert.equal(rows(env.NEWSLETTER_DB)[0].target, `campaign:${id}`);
});

test("GDPR erasure: the erased address is nowhere in the audit trail", async () => {
  const env = authEnv();
  const email = "erase.me@example.com";
  env.NEWSLETTER_DB.sqlite.exec(`INSERT INTO subscribers (id, email, status, tags, consent_text, consent_at, token, created_at, updated_at)
    VALUES ('s-erase', '${email}', 'confirmed', '', 'yes', '2026-01-01T00:00:00.000Z', '${"t".repeat(43)}', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`);
  const all = ["*"];
  const steps: [string, string, string?][] = [
    ["PATCH", "/api/subscribers/s-erase", JSON.stringify({ tags: ["vip"] })],
    ["POST", "/api/suppressions", JSON.stringify({ email, reason: "manual" })],
    ["POST", "/api/subscribers/s-erase/unsubscribe", "{}"],
    ["DELETE", "/api/subscribers/s-erase"],
    ["DELETE", `/api/subscribers/${encodeURIComponent(email)}`], // an address where an id belongs
  ];
  for (const [method, path, body] of steps) await send(env, await session(B + path, all, { method, body }));
  const trail = rows(env.NEWSLETTER_DB);
  assert.deepEqual(trail.map((r) => r.action), ["subscriber.edit", "suppression.add", "subscriber.unsubscribe", "subscriber.erase", "subscriber.erase"]);
  assert.equal(trail[3].target, "subscriber:s-erase");
  assert.equal(trail[4].target, null, "an address in the path is left out");
  const dump = JSON.stringify(env.NEWSLETTER_DB.sqlite.prepare("SELECT * FROM admin_audit").all());
  assert.doesNotMatch(dump, /erase\.me/);
  assert.deepEqual([...new Set(dump.match(/[\w.+-]+@[\w.-]+/g))], ["ada@example.com"], "no address but the actor's");
});

test("retention: default 365 days, 0 keeps forever, rows past it purged on write", async () => {
  assert.equal(retentionDays({} as never), DEFAULT_RETENTION_DAYS);
  assert.equal(retentionDays({ NEWSLETTER_AUDIT_RETENTION_DAYS: " 30 " } as never), 30);
  assert.equal(retentionDays({ NEWSLETTER_AUDIT_RETENTION_DAYS: "0" } as never), 0);
  assert.equal(retentionDays({ NEWSLETTER_AUDIT_RETENTION_DAYS: "-3" } as never), DEFAULT_RETENTION_DAYS);
  const route = ROUTES.find((r) => r.audit === "campaign.pause") as Route;
  const admin = { mode: "password" as const, user: null, perms: "all" as const };
  for (const [days, kept] of [["30", 1], ["0", 2]] as const) {
    const env = makeEnv({ NEWSLETTER_AUDIT_RETENTION_DAYS: days });
    await ensureSchema(env as never);
    env.NEWSLETTER_DB.sqlite.exec("INSERT INTO admin_audit (id, at, mode, action, status) VALUES ('old', '2000-01-01T00:00:00.000Z', 'password', 'campaign.pause', 200)");
    await recordAudit(env as never, admin, route, { id: "c" }, new Response(null, { status: 204 }));
    assert.equal(rows(env.NEWSLETTER_DB).length, kept, `retention ${days}`);
  }
});

test("a failing audit write is logged, and the action's answer stands", async () => {
  const env = makeEnv();
  env.NEWSLETTER_DB.state.failOn = /INSERT INTO admin_audit/;
  const logged: unknown[] = [];
  const quiet = console.error;
  console.error = (...a: unknown[]) => void logged.push(a);
  try {
    const res = await send(env, req(B + "/api/campaigns/nope/pause", { method: "POST", auth: PASSWORD, body: "{}" }));
    assert.equal(res.status, 404);
  } finally {
    console.error = quiet;
  }
  assert.match(String((logged[0] as unknown[])[0]), /audit/);
});

test("targetOf: path ids by collection, a create's id, never an address", async () => {
  const route = (path: string) => ({ method: "POST", path }) as Route;
  const ok = (body: unknown, type = "application/json") => new Response(JSON.stringify(body), { headers: { "content-type": type } });
  assert.equal(await targetOf(route("/api/suppressions/:hash"), { hash: "ab" }, ok({})), "suppression:ab");
  assert.equal(await targetOf(route("/api/x/:a/y/:b"), { a: "1", b: "" }, ok({})), "x:1");
  assert.equal(await targetOf(route("/api/campaigns/from-post"), {}, ok({ id: "c9" })), "campaign:c9");
  assert.equal(await targetOf(route("/api/campaigns"), {}, ok({ id: "a@example.com" })), null);
  assert.equal(await targetOf(route("/api/campaigns"), {}, ok({ id: 7 })), null);
  assert.equal(await targetOf(route("/api/campaigns"), {}, ok({ id: "c" }, "text/csv")), null);
  assert.equal(await targetOf(route("/api/campaigns"), {}, new Response("{", { headers: { "content-type": "application/json" } })), null);
  assert.equal(await targetOf(route("/api/campaigns"), {}, new Response(null, { status: 204 })), null);
  assert.equal(await targetOf(route("/api/campaigns"), {}, new Response(JSON.stringify({ id: "c" }), { status: 400, headers: { "content-type": "application/json" } })), null);
});

test("GET /api/audit: newest first, cursor pages, filters by actor and action", async () => {
  const env = authEnv();
  await ensureSchema(env as never);
  const ins = env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO admin_audit (id, at, mode, actor_id, actor_email, action, target, status) VALUES (?, ?, ?, ?, ?, ?, ?, 200)");
  ins.run("a", "2026-11-01T00:00:00.000Z", "password", null, null, "campaign.pause", "campaign:1");
  ins.run("b", "2026-11-02T00:00:00.000Z", "auth", "u-ada", "ada@example.com", "campaign.schedule", "campaign:1");
  ins.run("c", "2026-11-03T00:00:00.000Z", "auth", "u-bob", "bob@example.com", "subscriber.erase", "subscriber:9");
  const list = async (q: string, perms = ["newsletter:settings"]) => send(env, await session(B + "/api/audit" + q, perms));
  const first = await (await list("?limit=2")).json();
  assert.deepEqual(first.items.map((i: { id: string }) => i.id), ["c", "b"]);
  assert.deepEqual(first.items[0], { id: "c", at: "2026-11-03T00:00:00.000Z", mode: "auth", actor: { id: "u-bob", email: "bob@example.com" }, action: "subscriber.erase", target: "subscriber:9", status: 200 });
  const second = await (await list(`?limit=2&cursor=${first.nextCursor}`)).json();
  assert.deepEqual([second.items.map((i: { id: string }) => i.id), second.nextCursor, second.items[0].actor], [["a"], null, null]);
  const ids = async (q: string) => (await (await list(q)).json()).items.map((i: { id: string }) => i.id);
  assert.deepEqual(await ids("?actor=u-ada"), ["b"]);
  assert.deepEqual(await ids("?actor=BOB@example.com"), ["c"]);
  assert.deepEqual(await ids("?actor=password"), ["a"]);
  assert.deepEqual(await ids("?action=campaign.pause"), ["a"]);
  for (const q of ["?limit=0", "?limit=x", "?cursor=%%%"]) assert.equal((await list(q)).status, 400, q);
  assert.equal((await list("", ["newsletter:read"])).status, 403, "newsletter:settings only");
  const direct = await listAudit({ env, url: new URL("https://example.com/x?limit=500") } as never);
  assert.equal((await direct.json()).items.length, 3);
});
