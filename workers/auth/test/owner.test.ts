// The owner invariant (GO-133): there is always at least one active account
// holding the owner role, directly or through a group, and the role keeps
// `*`. Every path that could end it fails with 409 (API) / "owner_required"
// (data layer) and leaves the data as it was — concurrent attempts included.
import { test } from "node:test";
import assert from "node:assert/strict";
import { addGrant, listGrants } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { deleteUser, setUserStatus } from "../functions/api/auth/_users.ts";
import { onRequestDelete as revokeGrant } from "../functions/api/auth/admin/grants/[id].ts";
import { onRequestDelete as deleteGroupRoute } from "../functions/api/auth/admin/groups/[id]/index.ts";
import { onRequestDelete as removeMemberRoute } from "../functions/api/auth/admin/groups/[id]/members/[userId].ts";
import { onRequestDelete as deleteRoleRoute, onRequestPut as putRole } from "../functions/api/auth/admin/roles/[id].ts";
import { T0, call, makeEnv, useClock } from "./fakes.ts";
import { roleId, seedUser, signedIn } from "./seed.ts";

type Env = ReturnType<typeof makeEnv>;
const snapshot = (env: Env) =>
  JSON.stringify(["users", "roles", "role_permissions", "groups", "group_members", "grants"].map((t) => env.AUTH_DB.sqlite.prepare(`SELECT * FROM ${t} ORDER BY 1, 2`).all()));
const activeOwners = (env: Env) => env.AUTH_DB.sqlite.prepare("SELECT id FROM active_owners ORDER BY id").all().map((r) => r.id);

// world: `root` (an owner, signed in, does the API calls) and `last`, who holds
// owner only through the group "Owners". Root's own grant is then revoked, so
// `last` is the last active owner while root's token still says `*`.
async function world() {
  useClock();
  const env = makeEnv();
  const { browser, id: root } = await signedIn(env, "root@example.com", "owner");
  const last = await seedUser(env, "last@example.com");
  const group = (await createGroup(env, "Owners", "", T0)) as Group;
  const owner = roleId(env, "owner");
  await addGrant(env, owner, { groupId: group.id }, root, T0);
  await addMember(env, group.id, last, root, T0);
  env.AUTH_DB.sqlite.prepare("DELETE FROM grants WHERE user_id = ?").run(root);
  assert.deepEqual(activeOwners(env), [last]);
  const groupGrant = (await listGrants(env, { groupId: group.id }))[0].id;
  return { env, browser, root, last, group, owner, groupGrant };
}

test("owner invariant: every API path that would end it answers 409 and changes nothing", async () => {
  const w = await world();
  const { env, browser } = w;
  const before = snapshot(env);
  const attempts: [string, unknown, string, Record<string, string>, unknown?][] = [
    ["revoke the last owner grant", revokeGrant, `/api/auth/admin/grants/${w.groupGrant}`, { id: w.groupGrant }],
    ["remove the last owner from the group", removeMemberRoute, `/api/auth/admin/groups/${w.group.id}/members/${w.last}`, { id: w.group.id, userId: w.last }],
    ["delete the group", deleteGroupRoute, `/api/auth/admin/groups/${w.group.id}`, { id: w.group.id }],
    ["delete the owner role", deleteRoleRoute, `/api/auth/admin/roles/${w.owner}`, { id: w.owner }],
  ];
  for (const [why, handler, path, params] of attempts) {
    const res = await call(handler, env, path, { method: "DELETE", browser, params });
    assert.equal(res.status, 409, why);
    assert.equal((await res.json()).error, "owner_required", why);
    assert.equal(snapshot(env), before, `${why}: data intact`);
  }
  const strip = await call(putRole, env, `/api/auth/admin/roles/${w.owner}`, { method: "PUT", json: { permissions: ["auth:admin"] }, browser, params: { id: w.owner } });
  assert.equal(strip.status, 409, "taking * out of owner");
  assert.equal((await strip.json()).error, "owner_required");
  const rename = await call(putRole, env, `/api/auth/admin/roles/${w.owner}`, { method: "PUT", json: { name: "boss" }, browser, params: { id: w.owner } });
  assert.equal(rename.status, 409);
  assert.equal((await rename.json()).error, "built_in_role");
  assert.equal(snapshot(env), before);
  const widen = await call(putRole, env, `/api/auth/admin/roles/${w.owner}`, { method: "PUT", json: { permissions: ["*", "auth:admin"] }, browser, params: { id: w.owner } });
  assert.equal(widen.status, 200, "keeping * while editing the list never passes through a state without it");
  assert.deepEqual(activeOwners(env), [w.last]);
});

test("owner invariant: blocking or deleting the last owner, and raw SQL, are refused by the database", async () => {
  const w = await world();
  const { env } = w;
  const before = snapshot(env);
  assert.equal(await setUserStatus(env, w.last, "blocked", T0), "owner_required");
  assert.equal(await deleteUser(env, w.last), "owner_required");
  assert.equal(snapshot(env), before, "the batch rolled back whole");
  for (const sql of [
    "UPDATE roles SET name = 'boss' WHERE name = 'owner'",
    "UPDATE role_permissions SET permission = 'x:y' WHERE permission = '*'",
    `UPDATE grants SET role_id = 'x' WHERE id = '${w.groupGrant}'`,
    `UPDATE group_members SET user_id = 'x' WHERE user_id = '${w.last}'`,
    `UPDATE groups SET id = 'x' WHERE id = '${w.group.id}'`,
    `UPDATE users SET id = 'x' WHERE id = '${w.last}'`,
  ]) {
    assert.throws(() => env.AUTH_DB.sqlite.exec(sql), /owner_required/, sql);
  }
  assert.equal(snapshot(env), before);
  // With a second owner the same changes go through.
  await addGrant(env, w.owner, { userId: w.root }, w.root, T0);
  assert.equal(await setUserStatus(env, w.last, "blocked", T0), "ok");
  assert.equal(await setUserStatus(env, w.last, "active", T0), "ok");
  assert.equal(await deleteUser(env, w.last), "ok");
  assert.equal(await deleteUser(env, w.last), "not_found");
  assert.equal(await setUserStatus(env, w.last, "blocked", T0), "not_found");
  assert.deepEqual(activeOwners(env), [w.root]);
});

test("owner invariant: changes that do not touch an owner path are never blocked, even with no owner at all", async () => {
  useClock();
  const env = makeEnv();
  const ada = await seedUser(env);
  assert.deepEqual(activeOwners(env), []);
  assert.equal(await setUserStatus(env, ada, "blocked", T0), "ok");
  assert.equal(await deleteUser(env, ada), "ok");
});

test("owner invariant: data-layer errors other than the invariant are not swallowed", async () => {
  useClock();
  const env = makeEnv();
  const ada = await seedUser(env);
  env.AUTH_DB.state.failOn = /UPDATE users SET status/;
  await assert.rejects(setUserStatus(env, ada, "blocked", T0), /forced failure/);
  env.AUTH_DB.state.failOn = /DELETE FROM users/;
  await assert.rejects(deleteUser(env, ada), /forced failure/);
  env.AUTH_DB.state.failOn = null;
});

test("owner invariant: two concurrent removals of the last two owners — exactly one succeeds", async () => {
  useClock();
  const env = makeEnv();
  // The caller holds * through an ordinary role: it is not an owner itself,
  // so neither removal can lock the caller out mid-test.
  const { browser } = await signedIn(env, "admin@example.com", ["*"]);
  const a = await seedUser(env, "a@example.com");
  const b = await seedUser(env, "b@example.com");
  const owner = roleId(env, "owner");
  await addGrant(env, owner, { userId: a }, a, T0);
  await addGrant(env, owner, { userId: b }, a, T0);
  const [ga, gb] = (await listGrants(env, { roleId: owner })).map((g) => g.id);
  const revoke = (id: string) => call(revokeGrant, env, `/api/auth/admin/grants/${id}`, { method: "DELETE", browser, params: { id } });
  const statuses = (await Promise.all([revoke(ga), revoke(gb)])).map((r) => r.status).sort();
  assert.deepEqual(statuses, [200, 409]);
  assert.equal(activeOwners(env).length, 1);

  // Two different paths at once: block one owner while removing the other's group.
  const c = await seedUser(env, "c@example.com");
  const g = (await createGroup(env, "G", "", T0)) as Group;
  await addGrant(env, owner, { groupId: g.id }, a, T0);
  await addMember(env, g.id, c, a, T0);
  const [left] = activeOwners(env).filter((x) => x !== c);
  const results = await Promise.all([setUserStatus(env, left, "blocked", T0), call(deleteGroupRoute, env, `/api/auth/admin/groups/${g.id}`, { method: "DELETE", browser, params: { id: g.id } })]);
  const route = results[1] as Response;
  assert.ok([200, 409].includes(route.status));
  const outcomes = [results[0], route.status === 200 ? "ok" : (await route.json()).error].sort();
  assert.deepEqual(outcomes, ["ok", "owner_required"]);
  assert.equal(activeOwners(env).length, 1);
});
