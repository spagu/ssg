// GO-138: the account routes the admin panel stands on — listing and
// filtering, one person's page, block / unblock, delete — with the owner
// invariant, the escalation rule, one's own account, and step-up where the
// change is sensitive.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as list } from "../functions/api/auth/admin/users/index.ts";
import { onRequestDelete as del, onRequestGet as detail } from "../functions/api/auth/admin/users/[id]/index.ts";
import { onRequestPost as setStatus } from "../functions/api/auth/admin/users/[id]/status.ts";
import { onRequestGet as matrix } from "../functions/api/auth/admin/matrix.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { T0, call, makeEnv, useClock } from "./fakes.ts";
import { code, enrolTotp, mfaEnv, signIn, stepUp } from "./mfa.ts";
import { roleId, seedUser, signedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const get = async (fn: unknown, env: Env, path: string, browser: never, params = {}) => {
  const r = await call(fn, env, path, { browser, params });
  return { status: r.status, body: await r.json() };
};
const errorOf = async (r: Response) => [r.status, (await r.json()).error];

test("users: filtered by text, status, role and group, paged by address", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin"]);
  const ids: string[] = [];
  for (const n of ["anna", "bert", "cora", "dan", "eve"]) ids.push(await seedUser(env, `${n}@example.com`));
  const role = (await createRole(env, { name: "reader", description: "", permissions: ["newsletter:read"] }, T0)) as Role;
  const group = (await createGroup(env, "Team", "", T0)) as Group;
  await addGrant(env, role.id, { groupId: group.id }, "t", T0);
  await addMember(env, group.id, ids[1], "t", T0);
  await addGrant(env, role.id, { userId: ids[2] }, "t", T0);
  env.AUTH_DB.sqlite.prepare("UPDATE users SET status = 'blocked' WHERE id = ?").run(ids[4]);
  const q = async (s: string) => (await get(list, env, `/api/auth/admin/users${s}`, admin.browser as never)).body;
  const page1 = await q("?limit=3");
  assert.deepEqual(page1.items.map((u: { email: string }) => u.email), ["admin@example.com", "anna@example.com", "bert@example.com"]);
  assert.equal(page1.nextCursor, "bert@example.com");
  assert.deepEqual((await q(`?limit=3&cursor=${page1.nextCursor}`)).items.map((u: { email: string }) => u.email), ["cora@example.com", "dan@example.com", "eve@example.com"]);
  assert.equal((await q("?limit=3&cursor=dan@example.com")).nextCursor, null);
  assert.deepEqual((await q("?q=COR")).items.map((u: { email: string }) => u.email), ["cora@example.com"]);
  assert.deepEqual((await q("?status=blocked")).items.map((u: { email: string }) => u.email), ["eve@example.com"]);
  assert.deepEqual((await q(`?roleId=${role.id}`)).items.map((u: { email: string }) => u.email), ["bert@example.com", "cora@example.com"]);
  assert.deepEqual((await q(`?groupId=${group.id}`)).items.map((u: { email: string }) => u.email), ["bert@example.com"]);
  const first = page1.items[0];
  assert.deepEqual(Object.keys(first).sort(), ["createdAt", "email", "hasPassword", "id", "lastLoginAt", "mustChangePassword", "name", "owner", "status"]);
  for (const bad of ["?status=gone", "?limit=0", "?limit=201", "?limit=x", `?q=${"x".repeat(201)}`]) assert.equal((await get(list, env, `/api/auth/admin/users${bad}`, admin.browser as never)).status, 400, bad);
  const m = (await get(matrix, env, "/api/auth/admin/matrix?q=cora", admin.browser as never)).body;
  assert.deepEqual(m.items, [{ id: ids[2], email: "cora@example.com", name: "Ada", status: "active", permissions: ["newsletter:read"] }]);
  assert.equal((await get(matrix, env, "/api/auth/admin/matrix?status=x", admin.browser as never)).status, 400);
  assert.deepEqual((await get(matrix, env, "/api/auth/admin/matrix?q=nobody", admin.browser as never)).body.items, []);
});

test("one person: account, methods, factor without secrets, groups, grants, provenance, sessions, events", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const admin = await signedIn({ ...env, AUTH_MFA_KEY: undefined }, "admin@example.com", ["auth:admin", "newsletter:read"]);
  const eva = await signedIn(env, "eva@example.com", ["newsletter:read"]);
  const { secret } = await enrolTotp(env, eva.browser, clock.now);
  const group = (await createGroup(env, "Team", "", T0)) as Group;
  await addMember(env, group.id, eva.id, "t", T0);
  // The admin's own role would need a factor while it is on: read with it off
  // (same database) — the factors are listed either way.
  const off = { ...env, AUTH_MFA_KEY: undefined };
  const r = await get(detail, off, `/api/auth/admin/users/${eva.id}`, admin.browser as never, { id: eva.id });
  assert.equal(r.status, 200);
  const d = r.body;
  assert.deepEqual([d.user.email, d.user.owner, d.methods.password, d.secondFactor.enabled, d.secondFactor.factors.length], ["eva@example.com", false, true, false, 1]);
  assert.equal(d.secondFactor.backupCodesLeft, 10);
  assert.deepEqual(d.groups.map((g: { name: string }) => g.name), ["Team"]);
  assert.equal(d.grants.length, 1);
  assert.deepEqual(d.permissions[0].sources[0].via, "user");
  assert.equal(d.sessions.length, 1);
  assert.equal(d.sessions[0].method, "password");
  assert.ok(Array.isArray(d.events));
  const text = JSON.stringify(d);
  for (const leak of [secret, "pbkdf2", "refresh_hash", "ip_hash"]) assert.ok(!text.includes(leak), leak);
  assert.equal((await get(detail, off, "/api/auth/admin/users/nobody", admin.browser as never, { id: "nobody" })).status, 404);
});

test("block, unblock: sessions end, the owner invariant holds, one's own account and escalation refused", async () => {
  useClock();
  const env = makeEnv();
  const boss = await signedIn(env, "boss@example.com", ["*"]); // `*` through a custom role: not an owner
  const owner = await signedIn(env, "owner@example.com", "owner");
  const helper = await signedIn(env, "helper@example.com", ["auth:admin"]);
  const post = (b: unknown, id: string, status: unknown) => call(setStatus, env, `/api/auth/admin/users/${id}/status`, { json: { status }, browser: b as never, params: { id } });
  assert.deepEqual(await errorOf(await post(boss.browser, owner.id, "blocked")), [409, "owner_required"]);
  assert.deepEqual(await errorOf(await post(helper.browser, owner.id, "blocked")), [403, "escalation"]);
  assert.deepEqual(await errorOf(await post(boss.browser, boss.id, "blocked")), [400, "own_account"]);
  assert.deepEqual(await errorOf(await post(boss.browser, helper.id, "gone")), [400, "invalid_request"]);
  assert.deepEqual(await errorOf(await post(boss.browser, "nobody", "blocked")), [404, "user_not_found"]);
  const blocked = await post(boss.browser, helper.id, "blocked");
  assert.deepEqual(await blocked.json(), { ok: true, status: "blocked", revoked: 1 });
  assert.equal((await call(list, env, "/api/auth/admin/users", { browser: helper.browser })).status, 401, "a blocked account is out at once");
  assert.deepEqual(await (await post(boss.browser, helper.id, "active")).json(), { ok: true, status: "active", revoked: 0 });
  const kinds = env.AUTH_DB.sqlite.prepare("SELECT kind FROM auth_events WHERE user_id = ? ORDER BY at, rowid").all(helper.id).map((r) => (r as { kind: string }).kind);
  assert.deepEqual(kinds.filter((k) => k.startsWith("account_")), ["account_blocked", "account_unblocked"]);
  // A row gone between the lookup and the update (another administrator).
  const real = env.AUTH_DB.prepare;
  env.AUTH_DB.prepare = ((sql: string) => (sql.startsWith("UPDATE users SET status") ? real("UPDATE users SET status = ?, updated_at = ? WHERE id = ? AND 0") : real(sql))) as never;
  assert.deepEqual(await errorOf(await post(boss.browser, helper.id, "blocked")), [404, "user_not_found"]);
  env.AUTH_DB.prepare = real;
});

test("delete: the account and its ways in go, audit stays; owner invariant, own account, escalation", async () => {
  useClock();
  const env = makeEnv();
  const boss = await signedIn(env, "boss@example.com", ["*"]);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const helper = await signedIn(env, "helper@example.com", ["auth:admin", "newsletter:read"]);
  const eva = await signedIn(env, "eva@example.com", ["newsletter:read"]);
  const remove = (b: unknown, id: string) => call(del, env, `/api/auth/admin/users/${id}`, { method: "DELETE", browser: b as never, params: { id } });
  assert.deepEqual(await errorOf(await remove(boss.browser, owner.id)), [409, "owner_required"]);
  assert.deepEqual(await errorOf(await remove(helper.browser, owner.id)), [403, "escalation"]);
  assert.deepEqual(await errorOf(await remove(boss.browser, boss.id)), [400, "own_account"]);
  assert.deepEqual(await errorOf(await remove(boss.browser, "nobody")), [404, "user_not_found"]);
  assert.deepEqual(await (await remove(helper.browser, eva.id)).json(), { ok: true });
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM sessions WHERE user_id = ?").get(eva.id)?.n, 0);
  const ev = env.AUTH_DB.sqlite.prepare("SELECT detail FROM auth_events WHERE kind = 'account_deleted'").get() as { detail: string };
  assert.deepEqual(JSON.parse(ev.detail), { by: helper.id, email: "eva@example.com" });
  const real = env.AUTH_DB.batch.bind(env.AUTH_DB);
  env.AUTH_DB.batch = (async (l: never[]) => (env.AUTH_DB.sqlite.prepare("DELETE FROM users WHERE id = ?").run(helper.id), real(l))) as never;
  assert.deepEqual(await errorOf(await remove(boss.browser, helper.id)), [404, "user_not_found"]);
});

test("step-up: delete, blocking an owner, granting auth:* and editing a role to hold it — refused, then done", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "boss@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "boss@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(11 * 60_000);
  const { onRequestPost: refresh } = await import("../functions/api/auth/refresh.ts");
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  const other = await seedUser(env, "other@example.com");
  const owner2 = await seedUser(env, "owner2@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: owner2 }, "t", T0);
  const admins = (await createRole(env, { name: "admins", description: "", permissions: ["auth:*"] }, T0)) as Role;
  const plain = (await createRole(env, { name: "plain", description: "", permissions: ["newsletter:read"] }, T0)) as Role;
  const { onRequestPost: grant } = await import("../functions/api/auth/admin/grants/index.ts");
  const { onRequestPut: putRole } = await import("../functions/api/auth/admin/roles/[id].ts");
  const actions: [string, () => Promise<Response>][] = [
    ["delete", () => call(del, env, `/api/auth/admin/users/${other}`, { method: "DELETE", browser, params: { id: other } })],
    ["block an owner", () => call(setStatus, env, `/api/auth/admin/users/${owner2}/status`, { json: { status: "blocked" }, browser, params: { id: owner2 } })],
    ["grant auth:*", () => call(grant, env, "/api/auth/admin/grants", { json: { roleId: admins.id, userId: owner2 }, browser })],
    ["role gains auth:*", () => call(putRole, env, `/api/auth/admin/roles/${plain.id}`, { method: "PUT", json: { permissions: ["auth:*"] }, browser, params: { id: plain.id } })],
  ];
  for (const [name, run] of actions) assert.deepEqual(await errorOf(await run()), [403, "step_up_required"], name);
  const ok = await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser });
  assert.equal(ok.status, 200);
  for (const [name, run] of actions) assert.ok((await run()).status < 300, name);
  // Not sensitive: unblocking, and blocking someone without owner.
  assert.equal((await call(setStatus, env, `/api/auth/admin/users/${owner2}/status`, { json: { status: "active" }, browser, params: { id: owner2 } })).status, 200);
});
