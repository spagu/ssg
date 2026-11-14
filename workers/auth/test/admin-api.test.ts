// The admin API's behaviour beyond the gate (GO-133): validation, not-found,
// conflicts, the escalation rule, unknown permissions flagged, provenance and
// the registry as the routes answer them.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { onRequestGet as getModule } from "../functions/api/auth/module/index.ts";
import { onRequestGet as registry } from "../functions/api/auth/permissions.ts";
import { onRequestGet as mine } from "../functions/api/auth/me/permissions.ts";
import { onRequestGet as listRolesRoute, onRequestPost as postRole } from "../functions/api/auth/admin/roles/index.ts";
import { onRequestDelete as deleteRole, onRequestGet as getRole, onRequestPut as putRole } from "../functions/api/auth/admin/roles/[id].ts";
import { onRequestPost as postGroup } from "../functions/api/auth/admin/groups/index.ts";
import { onRequestDelete as deleteGroup, onRequestGet as getGroup, onRequestPut as putGroup } from "../functions/api/auth/admin/groups/[id]/index.ts";
import { onRequestPost as postMember } from "../functions/api/auth/admin/groups/[id]/members/index.ts";
import { onRequestDelete as deleteMember } from "../functions/api/auth/admin/groups/[id]/members/[userId].ts";
import { onRequestGet as listGrantsRoute, onRequestPost as postGrant } from "../functions/api/auth/admin/grants/index.ts";
import { onRequestDelete as deleteGrant } from "../functions/api/auth/admin/grants/[id].ts";
import { onRequestGet as userPerms } from "../functions/api/auth/admin/users/[id]/permissions.ts";
import { call, makeEnv, useClock } from "./fakes.ts";
import type { ReqOptions } from "./fakes.ts";
import { roleId, seedUser, signedIn } from "./seed.ts";


async function setup(perms: string[] | "owner" = "owner", over = {}) {
  useClock();
  const env = makeEnv(over);
  const { browser, id } = await signedIn(env, "admin@example.com", perms);
  const go = async (h: unknown, path: string, o: ReqOptions = {}) => {
    const res = await call(h, env, path, { browser, ...o });
    return { status: res.status, body: await res.json() };
  };
  return { env, browser, id, go };
}

test("roles API: create, read, list, update, delete — with validation and unknown names flagged", async () => {
  const { env, go } = await setup();
  const bad: [unknown, string][] = [
    [{ name: "Bad Name", permissions: [] }, "name"],
    [{ name: "x".repeat(65), permissions: [] }, "name"],
    [{ permissions: [] }, "name"],
    [{ name: "ok", description: 5, permissions: [] }, "description"],
    [{ name: "ok", description: "d".repeat(501), permissions: [] }, "description"],
    [{ name: "ok", permissions: "a:b" }, "array"],
    [{ name: "ok", permissions: ["*:erase"] }, "not a permission"],
  ];
  for (const [json, detail] of bad) {
    const r = await go(postRole, "/api/auth/admin/roles", { json });
    assert.equal(r.status, 400, JSON.stringify(json));
    assert.match(r.body.detail, new RegExp(detail));
  }
  assert.equal((await go(postRole, "/api/auth/admin/roles", { method: "POST", headers: { "content-type": "application/json" } })).status, 400, "no body");
  const created = await go(postRole, "/api/auth/admin/roles", { json: { name: "sender", description: " Sends ", permissions: ["newsletter:send", "private:thing", "ghost:*"] } });
  assert.equal(created.status, 201);
  const role = created.body.role;
  assert.equal(role.description, "Sends");
  assert.deepEqual(role.unknownPermissions, ["ghost:*", "newsletter:send", "private:thing"], "without AUTH_KNOWN_PERMISSIONS auth knows only its own names");
  assert.equal((await go(postRole, "/api/auth/admin/roles", { json: { name: "sender", permissions: [] } })).status, 409);
  const p = { params: { id: role.id } };
  assert.equal((await go(getRole, `/api/auth/admin/roles/${role.id}`, p)).body.role.name, "sender");
  assert.equal((await go(getRole, "/api/auth/admin/roles/x", { params: { id: "x" } })).body.error, "role_not_found");
  assert.ok((await go(listRolesRoute, "/api/auth/admin/roles")).body.items.some((r: { name: string }) => r.name === "owner"));
  const put = (json: unknown, id = role.id) => go(putRole, `/api/auth/admin/roles/${id}`, { method: "PUT", json, params: { id } });
  assert.equal((await put({ name: "Bad" })).status, 400);
  assert.equal((await put({ description: 1 })).status, 400);
  assert.equal((await put({ permissions: ["bad"] })).status, 400);
  assert.equal((await put({ name: "owner" })).status, 409);
  assert.equal((await put({ name: "x" }, "missing")).status, 404);
  const updated = await put({ name: "mailer", description: "new", permissions: ["auth:admin"] });
  assert.deepEqual([updated.body.role.name, updated.body.role.description, updated.body.role.permissions, updated.body.role.unknownPermissions], ["mailer", "new", ["auth:admin"], []]);
  assert.equal((await go(deleteRole, `/api/auth/admin/roles/${role.id}`, { method: "DELETE", ...p })).status, 200);
  assert.equal((await go(deleteRole, `/api/auth/admin/roles/${role.id}`, { method: "DELETE", ...p })).status, 404);
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM role_permissions WHERE role_id = ?").get(role.id)?.n, 0);
});

test("groups API: create, read with members and grants, rename, members, delete", async () => {
  const { env, go } = await setup();
  const ada = await seedUser(env, "ada@example.com");
  for (const json of [{}, { name: "  " }, { name: "x".repeat(101) }, { name: "a\u0007b" }, { name: "ok", description: [] }]) {
    assert.equal((await go(postGroup, "/api/auth/admin/groups", { json })).status, 400, JSON.stringify(json));
  }
  const g = (await go(postGroup, "/api/auth/admin/groups", { json: { name: " Editors ", description: "people who write" } })).body.group;
  assert.deepEqual([g.name, g.description, g.memberCount], ["Editors", "people who write", 0]);
  assert.equal((await go(postGroup, "/api/auth/admin/groups", { json: { name: "Editors" } })).status, 409);
  const p = { params: { id: g.id } };
  const member = (json: unknown, id = g.id) => go(postMember, `/api/auth/admin/groups/${id}/members`, { json, params: { id } });
  assert.equal((await member({})).status, 400);
  assert.equal((await member({ userId: ada }, "nope")).body.error, "group_not_found");
  assert.equal((await member({ userId: "nobody" })).body.error, "user_not_found");
  assert.deepEqual(await member({ userId: ada }), { status: 201, body: { ok: true, added: true } });
  assert.deepEqual(await member({ userId: ada }), { status: 200, body: { ok: true, added: false } });
  await go(postGrant, "/api/auth/admin/grants", { json: { roleId: roleId(env, "newsletter-viewer"), groupId: g.id } });
  const detail = (await go(getGroup, `/api/auth/admin/groups/${g.id}`, p)).body;
  assert.deepEqual(detail.members.map((m: { email: string }) => m.email), ["ada@example.com"]);
  assert.deepEqual(detail.grants.map((x: { roleName: string }) => x.roleName), ["newsletter-viewer"]);
  assert.equal((await go(getGroup, "/api/auth/admin/groups/x", { params: { id: "x" } })).status, 404);
  const put = (json: unknown, id = g.id) => go(putGroup, `/api/auth/admin/groups/${id}`, { method: "PUT", json, params: { id } });
  assert.equal((await put({ name: "" })).status, 400);
  assert.equal((await put({ description: 3 })).status, 400);
  assert.equal((await put({ name: "x" }, "nope")).status, 404);
  await go(postGroup, "/api/auth/admin/groups", { json: { name: "Other" } });
  assert.equal((await put({ name: "Other" })).status, 409);
  assert.equal((await put({ name: "Writers", description: "" })).body.group.name, "Writers");
  const del = (userId: string) => go(deleteMember, `/api/auth/admin/groups/${g.id}/members/${userId}`, { method: "DELETE", params: { id: g.id, userId } });
  assert.equal((await del(ada)).status, 200);
  assert.equal((await del(ada)).body.error, "member_not_found");
  assert.equal((await go(deleteGroup, `/api/auth/admin/groups/${g.id}`, { method: "DELETE", ...p })).status, 200);
  assert.equal((await go(deleteGroup, `/api/auth/admin/groups/${g.id}`, { method: "DELETE", ...p })).status, 404);
});

test("grants API: user and group targets, filters, conflicts, revoke", async () => {
  const { env, go } = await setup();
  const ada = await seedUser(env, "ada@example.com");
  const viewer = roleId(env, "newsletter-viewer");
  const g = (await go(postGroup, "/api/auth/admin/groups", { json: { name: "G" } })).body.group;
  for (const json of [{}, { roleId: viewer }, { roleId: viewer, userId: ada, groupId: g.id }, { userId: ada }]) {
    assert.equal((await go(postGrant, "/api/auth/admin/grants", { json })).status, 400, JSON.stringify(json));
  }
  assert.equal((await go(postGrant, "/api/auth/admin/grants", { json: { roleId: "x", userId: ada } })).body.error, "role_not_found");
  assert.equal((await go(postGrant, "/api/auth/admin/grants", { json: { roleId: viewer, userId: "x" } })).body.error, "user_not_found");
  assert.equal((await go(postGrant, "/api/auth/admin/grants", { json: { roleId: viewer, groupId: "x" } })).body.error, "group_not_found");
  const toUser = await go(postGrant, "/api/auth/admin/grants", { json: { roleId: viewer, userId: ada } });
  assert.equal(toUser.status, 201);
  assert.deepEqual([toUser.body.grant.roleName, toUser.body.grant.userId, toUser.body.grant.groupId], ["newsletter-viewer", ada, null]);
  assert.equal((await go(postGrant, "/api/auth/admin/grants", { json: { roleId: viewer, userId: ada } })).body.error, "grant_exists");
  assert.equal((await go(postGrant, "/api/auth/admin/grants", { json: { roleId: viewer, groupId: g.id } })).status, 201);
  const list = async (q: string) => (await go(listGrantsRoute, `/api/auth/admin/grants${q}`)).body.items.length;
  assert.equal(await list(""), 3, "owner + two");
  assert.equal(await list(`?roleId=${viewer}`), 2);
  assert.equal(await list(`?roleId=${viewer}&groupId=${g.id}`), 1);
  const id = toUser.body.grant.id;
  assert.equal((await go(deleteGrant, `/api/auth/admin/grants/${id}`, { method: "DELETE", params: { id } })).status, 200);
  assert.equal((await go(deleteGrant, `/api/auth/admin/grants/${id}`, { method: "DELETE", params: { id } })).body.error, "grant_not_found");
});

test("escalation: an administrator cannot hand out what they do not hold — unless they hold *", async () => {
  const { env, go } = await setup(["auth:admin", "newsletter:read"]);
  const ada = await seedUser(env, "ada@example.com");
  const create = await go(postRole, "/api/auth/admin/roles", { json: { name: "eraser", permissions: ["newsletter:erase", "newsletter:read"] } });
  assert.deepEqual([create.status, create.body.error, create.body.permissions], [403, "escalation", ["newsletter:erase"]]);
  assert.equal((await go(postRole, "/api/auth/admin/roles", { json: { name: "all", permissions: ["newsletter:*"] } })).status, 403);
  const reader = (await go(postRole, "/api/auth/admin/roles", { json: { name: "reader", permissions: ["newsletter:read"] } })).body.role;
  const add = await go(putRole, `/api/auth/admin/roles/${reader.id}`, { method: "PUT", json: { permissions: ["newsletter:read", "newsletter:send"] }, params: { id: reader.id } });
  assert.deepEqual([add.status, add.body.permissions], [403, ["newsletter:send"]], "adding to a role is handing out");
  // A role that already holds something the caller lacks can still be edited without adding to it.
  const editor = roleId(env, "newsletter-editor");
  assert.equal((await go(putRole, `/api/auth/admin/roles/${editor}`, { method: "PUT", json: { permissions: ["newsletter:read"] }, params: { id: editor } })).status, 200, "narrowing is fine");
  const owner = await go(postGrant, "/api/auth/admin/grants", { json: { roleId: roleId(env, "owner"), userId: ada } });
  assert.deepEqual([owner.status, owner.body.permissions], [403, ["*"]], "nobody but a * holder makes an owner");
  const g = (await go(postGroup, "/api/auth/admin/groups", { json: { name: "Owners" } })).body.group;
  env.AUTH_DB.sqlite.prepare("INSERT INTO grants (id, role_id, group_id, granted_at) VALUES ('g1', ?, ?, 'now')").run(roleId(env, "owner"), g.id);
  const join = await go(postMember, `/api/auth/admin/groups/${g.id}/members`, { json: { userId: ada }, params: { id: g.id } });
  assert.deepEqual([join.status, join.body.permissions], [403, ["*"]], "joining a group hands out its roles");
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM grants WHERE user_id = ?").get(ada)?.n, 0);
});

test("provenance and the registry as the routes answer them", async () => {
  const known = JSON.stringify([{ id: "newsletter-ui", permissions: [{ name: "newsletter:read", description: { en: "View." } }, "newsletter:send"] }]);
  const { env, id, go } = await setup(["newsletter:read", "auth:admin"], { AUTH_KNOWN_PERMISSIONS: known });
  const g = (await go(postGroup, "/api/auth/admin/groups", { json: { name: "Readers" } })).body.group;
  await go(postMember, `/api/auth/admin/groups/${g.id}/members`, { json: { userId: id }, params: { id: g.id } });
  await go(postGrant, "/api/auth/admin/grants", { json: { roleId: roleId(env, "newsletter-viewer"), groupId: g.id } });
  const view = await go(userPerms, `/api/auth/admin/users/${id}/permissions`, { params: { id } });
  assert.equal(view.status, 200);
  assert.deepEqual(view.body.permissions.map((p: { name: string; known: boolean; sources: unknown[] }) => [p.name, p.known, p.sources.length]), [
    ["auth:admin", true, 1],
    ["newsletter:read", true, 2],
  ]);
  assert.deepEqual(view.body.permissions[1].sources[0], { roleId: roleId(env, "newsletter-viewer"), roleName: "newsletter-viewer", via: "group", groupId: g.id, groupName: "Readers" });
  assert.equal((await go(userPerms, "/api/auth/admin/users/x/permissions", { params: { id: "x" } })).body.error, "user_not_found");
  const me = await go(mine, "/api/auth/me/permissions");
  assert.deepEqual(me.body.permissions, view.body.permissions);
  assert.deepEqual(me.body.token, ["auth:admin", "newsletter:read"]);
  env.AUTH_DB.sqlite.prepare("INSERT INTO role_permissions (role_id, permission) VALUES (?, 'private:thing')").run(roleId(env, "newsletter-viewer"));
  const reg = (await go(registry, "/api/auth/permissions")).body;
  assert.deepEqual(
    reg.items.map((i: { name: string; module: string | null; known: boolean }) => [i.name, i.module, i.known]),
    [
      ["auth:admin", "auth", true],
      ["auth:invite", "auth", true],
      ["comments:moderate", null, false],
      ["comments:read", null, false],
      ["newsletter:edit", null, false],
      ["newsletter:read", "newsletter-ui", true],
      ["newsletter:send", "newsletter-ui", true],
      ["private:thing", null, false],
    ],
  );
  assert.deepEqual(reg.problems, []);
  assert.equal(reg.nextCursor, null);
});

test("module: the manifest route serves module.json behind auth:admin", async () => {
  const { go } = await setup();
  const r = await go(getModule, "/api/auth/module");
  assert.deepEqual(r.body, JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8")));
  const outsider = await signedIn(makeEnv(), "x@example.com", []);
  assert.equal((await call(getModule, makeEnv(), "/api/auth/module", { browser: outsider.browser })).status, 401, "a session of another database is none");
});

test("edge cases: unreadable bodies, an array route parameter, a row deleted mid-update", async () => {
  const { env, go, browser } = await setup();
  const viewer = roleId(env, "newsletter-viewer");
  const g = (await go(postGroup, "/api/auth/admin/groups", { json: { name: "G" } })).body.group;
  const garbage = { method: "POST", headers: { "content-type": "application/json" } } as const;
  const sendGarbage = async (h: unknown, method: string, params: Record<string, string> = {}) => {
    const res = await call(h, env, "/api/auth/admin/x", { ...garbage, method, params, browser });
    return res.status;
  };
  for (const [h, method, params] of [
    [postRole, "POST", {}],
    [putRole, "PUT", { id: viewer }],
    [postGroup, "POST", {}],
    [putGroup, "PUT", { id: g.id }],
    [postMember, "POST", { id: g.id }],
    [postGrant, "POST", {}],
  ] as [unknown, string, Record<string, string>][]) {
    assert.equal(await sendGarbage(h, method, params), 400, `${method} without a JSON body`);
  }
  const arr = await call(getRole, env, "/api/auth/admin/roles/a/b", { browser, params: { id: ["a", "b"] } as never });
  assert.equal(arr.status, 404, "a catch-all array is no id");
  // Another request deletes the row between the update and the re-read.
  env.AUTH_DB.sqlite.exec(`CREATE TRIGGER vanish_role AFTER UPDATE ON roles WHEN NEW.description = 'vanish' BEGIN DELETE FROM roles WHERE id = NEW.id; END`);
  env.AUTH_DB.sqlite.exec(`CREATE TRIGGER vanish_group AFTER UPDATE ON groups WHEN NEW.description = 'vanish' BEGIN DELETE FROM groups WHERE id = NEW.id; END`);
  assert.equal((await go(putRole, `/api/auth/admin/roles/${viewer}`, { method: "PUT", json: { description: "vanish" }, params: { id: viewer } })).body.error, "role_not_found");
  assert.equal((await go(putGroup, `/api/auth/admin/groups/${g.id}`, { method: "PUT", json: { description: "vanish" }, params: { id: g.id } })).body.error, "group_not_found");
});
