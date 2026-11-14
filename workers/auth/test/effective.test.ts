// Effective permissions on a seeded database (GO-133): the union over the
// roles granted to the user and to the user's groups, de-duplicated, sorted,
// wildcards kept — and where each one comes from.
import { test } from "node:test";
import assert from "node:assert/strict";
import { addGrant, effectivePermissions, groupPermissions, listGrants, permissionSources, removeGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup, deleteGroup, listGroups, listMembers, removeMember, updateGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createRole, deleteRole, getRole, listRoles, updateRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { T0, makeEnv, useClock } from "./fakes.ts";
import { roleId, seedUser } from "./seed.ts";

type Env = ReturnType<typeof makeEnv>;

async function world() {
  useClock();
  const env = makeEnv();
  const ada = await seedUser(env, "ada@example.com");
  const bob = await seedUser(env, "bob@example.com");
  const role = async (name: string, permissions: string[]) => (await createRole(env, { name, description: "", permissions }, T0)) as Role;
  const editors = (await createGroup(env, "Editors", "", T0)) as Group;
  const mods = (await createGroup(env, "Moderators", "", T0)) as Group;
  const writer = await role("writer", ["newsletter:edit", "newsletter:read"]);
  const sender = await role("sender", ["newsletter:send", "newsletter:read"]);
  const moderator = await role("moderator", ["comments:*"]);
  return { env, ada, bob, editors, mods, writer, sender, moderator };
}

test("effective permissions: direct and group grants, union, de-duplicated, sorted, wildcard kept", async () => {
  const w = await world();
  const { env, ada, bob } = w;
  assert.deepEqual(await effectivePermissions(env, ada), [], "nothing granted: nothing held");
  await addGrant(env, w.writer.id, { userId: ada }, "x", T0);
  await addGrant(env, w.sender.id, { groupId: w.editors.id }, "x", T0);
  await addGrant(env, w.moderator.id, { groupId: w.mods.id }, "x", T0);
  await addMember(env, w.editors.id, ada, "x", T0);
  await addMember(env, w.mods.id, ada, "x", T0);
  await addMember(env, w.editors.id, bob, "x", T0);
  assert.deepEqual(await effectivePermissions(env, ada), ["comments:*", "newsletter:edit", "newsletter:read", "newsletter:send"]);
  assert.deepEqual(await effectivePermissions(env, bob), ["newsletter:read", "newsletter:send"]);

  // Removal from a group takes that group's roles away, and only those.
  assert.equal(await removeMember(env, w.mods.id, ada), "ok");
  assert.deepEqual(await effectivePermissions(env, ada), ["newsletter:edit", "newsletter:read", "newsletter:send"]);
  assert.equal(await removeMember(env, w.mods.id, ada), "not_found");

  // Revoking a role: read stays, because the group's role also gives it.
  const direct = (await listGrants(env, { userId: ada }))[0];
  assert.equal(await removeGrant(env, direct.id), "ok");
  assert.deepEqual(await effectivePermissions(env, ada), ["newsletter:read", "newsletter:send"]);
  assert.equal(await removeGrant(env, direct.id), "not_found");

  // Editing a role changes every holder at once; deleting a group removes its path.
  await updateRole(env, w.sender.id, { permissions: ["newsletter:*"] }, T0);
  assert.deepEqual(await effectivePermissions(env, bob), ["newsletter:*"]);
  assert.equal(await deleteGroup(env, w.editors.id), "ok");
  assert.deepEqual(await effectivePermissions(env, bob), []);
  assert.equal(await deleteGroup(env, w.editors.id), "not_found");
});

test("provenance: each permission lists every role that gives it, directly or via which group", async () => {
  const w = await world();
  const { env, ada } = w;
  await addGrant(env, w.writer.id, { userId: ada }, "x", T0);
  await addGrant(env, w.sender.id, { groupId: w.editors.id }, "x", T0);
  await addGrant(env, w.sender.id, { groupId: w.mods.id }, "x", T0);
  await addMember(env, w.editors.id, ada, "x", T0);
  await addMember(env, w.mods.id, ada, "x", T0);
  const got = await permissionSources(env, ada);
  const writer = { roleId: w.writer.id, roleName: "writer", via: "user" };
  const viaEditors = { roleId: w.sender.id, roleName: "sender", via: "group", groupId: w.editors.id, groupName: "Editors" };
  const viaMods = { roleId: w.sender.id, roleName: "sender", via: "group", groupId: w.mods.id, groupName: "Moderators" };
  assert.deepEqual(got, [
    { name: "newsletter:edit", sources: [writer] },
    { name: "newsletter:read", sources: [viaEditors, viaMods, writer] },
    { name: "newsletter:send", sources: [viaEditors, viaMods] },
  ]);
  assert.deepEqual(await groupPermissions(env, w.editors.id), ["newsletter:read", "newsletter:send"]);
  // A grant to a group that no longer exists counts for nobody.
  env.AUTH_DB.sqlite.exec(`DELETE FROM groups WHERE id = '${w.mods.id}'`);
  assert.deepEqual((await permissionSources(env, ada)).find((p) => p.name === "newsletter:send")?.sources, [viaEditors]);
});

test("roles, groups and grants: the data layer's own answers", async () => {
  const w = await world();
  const env: Env = w.env;
  assert.equal(await createRole(env, { name: "writer", description: "", permissions: [] }, T0), "name_taken");
  assert.equal(await createGroup(env, "Editors", "", T0), "name_taken");
  assert.equal(await updateRole(env, "nope", { description: "x" }, T0), "not_found");
  assert.equal(await updateRole(env, w.writer.id, { name: "sender" }, T0), "name_taken");
  assert.equal(await updateRole(env, roleId(env, "newsletter-viewer"), { name: "viewer" }, T0), "built_in_name");
  const viewer = (await updateRole(env, roleId(env, "newsletter-viewer"), { name: "newsletter-viewer", description: "edited" }, T0)) as Role;
  assert.equal(viewer.description, "edited", "a built-in role is editable");
  assert.equal(await updateGroup(env, "nope", {}, T0), "not_found");
  assert.equal(await updateGroup(env, w.mods.id, { name: "Editors" }, T0), "name_taken");
  assert.equal(((await updateGroup(env, w.mods.id, { description: "d" }, T0)) as Group).description, "d");
  assert.equal(await addMember(env, w.mods.id, w.ada, "x", T0), true);
  assert.equal(await addMember(env, w.mods.id, w.ada, "x", T0), false, "already a member");
  assert.equal(await addMember(env, "no-group", w.ada, "x", T0), false);
  assert.equal((await listGroups(env)).find((g) => g.id === w.mods.id)?.memberCount, 1);
  assert.deepEqual((await listMembers(env, w.mods.id)).map((m) => m.email), ["ada@example.com"]);
  assert.equal(await addGrant(env, w.writer.id, { userId: w.ada }, "x", T0) !== "exists", true);
  assert.equal(await addGrant(env, w.writer.id, { userId: w.ada }, "x", T0), "exists");
  assert.equal(await addGrant(env, "no-role", { userId: w.ada }, "x", T0), "exists");
  assert.equal((await listGrants(env, { roleId: w.writer.id, userId: w.ada })).length, 1);
  assert.equal(await deleteRole(env, w.writer.id), "ok");
  assert.equal(await deleteRole(env, w.writer.id), "not_found");
  assert.equal(await getRole(env, w.writer.id), null);
  assert.equal((await listGrants(env, { userId: w.ada })).length, 0, "its grants went with it");
  assert.deepEqual((await listRoles(env)).map((r) => r.name).slice(0, 2), ["comments-moderator", "moderator"]);
});

test("data layer: unexpected database errors are not swallowed", async () => {
  const w = await world();
  const { env } = w;
  const boom = /forced failure/;
  env.AUTH_DB.state.failOn = /INSERT INTO roles/;
  await assert.rejects(createRole(env, { name: "z", description: "", permissions: [] }, T0), boom);
  env.AUTH_DB.state.failOn = /UPDATE roles/;
  await assert.rejects(updateRole(env, w.writer.id, { description: "x" }, T0), boom);
  env.AUTH_DB.state.failOn = /DELETE FROM roles/;
  await assert.rejects(deleteRole(env, w.writer.id), boom);
  env.AUTH_DB.state.failOn = /INSERT INTO groups/;
  await assert.rejects(createGroup(env, "G", "", T0), boom);
  env.AUTH_DB.state.failOn = /UPDATE groups/;
  await assert.rejects(updateGroup(env, w.mods.id, {}, T0), boom);
  env.AUTH_DB.state.failOn = /DELETE FROM groups/;
  await assert.rejects(deleteGroup(env, w.mods.id), boom);
  env.AUTH_DB.state.failOn = /DELETE FROM group_members/;
  await assert.rejects(removeMember(env, w.mods.id, w.ada), boom);
  env.AUTH_DB.state.failOn = /DELETE FROM grants/;
  await assert.rejects(removeGrant(env, "x"), boom);
  env.AUTH_DB.state.failOn = null;
});
