// GO-139: Access groups → groups here. At sign-in, memberships in MAPPED
// groups follow the token — added and removed — but only those the sync
// itself added; a membership an administrator added by hand is never touched,
// the last owner keeps the owner role, and a mapping change reaches everyone
// already seen (in their next token).
import { test } from "node:test";
import assert from "node:assert/strict";
import { createMapping } from "../functions/api/auth/_access-groups.ts";
import { applyAccessGroups, resyncEveryone, syncGroups } from "../functions/api/auth/_access-sync.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup, removeMember } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { onRequestGet as me } from "../functions/api/auth/me/permissions.ts";
import { Browser, T0, call, useClock } from "./fakes.ts";
import { roleId } from "./seed.ts";
import { events, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, PERSON, accessEnv, enter, mint, where } from "./access-fake.ts";

type E = ReturnType<typeof accessEnv>;
const groupsOf = (env: E, userId: string): string[] =>
  (env.AUTH_DB.sqlite.prepare("SELECT g.name FROM group_members m JOIN groups g ON g.id = m.group_id WHERE m.user_id = ? ORDER BY g.name").all(userId) as { name: string }[]).map((r) => r.name);
const marks = (env: E, userId: string): number => (env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM access_synced_members WHERE user_id = ?").get(userId) as { n: number }).n;

async function world() {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  const id = await providerUser(env, "ada@example.com", "access", PERSON.sub);
  const g = async (name: string, role?: string) => {
    const group = (await createGroup(env, name, "", T0)) as Group;
    if (role) await addGrant(env, roleId(env as never, role), { groupId: group.id }, "test", T0);
    return group;
  };
  const editors = await g("Editors", "newsletter-editor");
  const viewers = await g("Viewers", "newsletter-viewer");
  const mods = await g("Moderators", "comments-moderator");
  const manual = await g("Manual");
  await createMapping(env, "nl-editors", editors.id, "boss", T0);
  await createMapping(env, "nl-viewers", viewers.id, "boss", T0);
  await createMapping(env, "mods", mods.id, "boss", T0);
  await createMapping(env, "nl-editors", manual.id, "boss", T0);
  return { env, fake, id, editors, viewers, mods, manual };
}

const signIn = async (env: Env, groups: unknown, browser = new Browser()) => {
  const res = await enter(env, await mint({ ...PERSON, groups }), { browser });
  assert.equal(where(res), "/");
  return browser;
};

test("sign-in adds the mapped groups the token names and removes synced ones it no longer names; the token follows", async () => {
  const { env, fake, id } = await world();
  try {
    const browser = await signIn(env, ["nl-editors", "unmapped", "mods"]);
    assert.deepEqual(groupsOf(env, id), ["Editors", "Manual", "Moderators"]);
    assert.deepEqual((await (await call(me, env, "/api/auth/me/permissions", { browser })).json()).token, ["comments:moderate", "comments:read", "newsletter:edit", "newsletter:read", "newsletter:send"]);
    await signIn(env, ["nl-viewers"]);
    assert.deepEqual(groupsOf(env, id), ["Viewers"]);
    const synced = events(env, "groups_synced");
    assert.deepEqual(synced.map((e) => [e.detail.added, e.detail.removed, e.detail.via]), [["Editors, Manual, Moderators", "", "signin"], ["Viewers", "Editors, Manual, Moderators", "signin"]]);
    // The same groups again: nothing changes, nothing is logged.
    await signIn(env, ["nl-viewers"]);
    assert.equal(events(env, "groups_synced").length, 2);
    // No groups claim at all: memberships are left alone (an IdP that stopped sending groups empties nothing).
    await signIn(env, undefined);
    assert.deepEqual(groupsOf(env, id), ["Viewers"]);
    // An empty list is a statement: the synced membership goes.
    await signIn(env, []);
    assert.deepEqual(groupsOf(env, id), []);
  } finally {
    fake.restore();
  }
});

test("a membership added by hand is never removed nor taken over; a synced one removed by hand comes back at the next sign-in", async () => {
  const { env, fake, id, editors, viewers } = await world();
  try {
    await addMember(env, editors.id, id, "boss", T0); // by an administrator
    await signIn(env, ["nl-editors"]);
    assert.equal(marks(env, id), 1, "only Manual is marked; Editors stays manual");
    await signIn(env, []);
    assert.deepEqual(groupsOf(env, id), ["Editors"], "the manual membership survives");
    await signIn(env, ["nl-viewers"]);
    assert.equal(await removeMember(env, viewers.id, id), "ok"); // an administrator removes it
    await signIn(env, ["nl-viewers"]);
    assert.deepEqual(groupsOf(env, id), ["Editors", "Viewers"]);
    // A stale mark of a membership already gone is cleaned up quietly.
    await removeMember(env, viewers.id, id);
    const r = await syncGroups(env, id, [], T0);
    assert.deepEqual(r, { added: [], removed: [], kept: [] });
    assert.equal(marks(env, id), 0);
  } finally {
    fake.restore();
  }
});

test("the owner invariant: a sync that would remove the last owner's way to owner is skipped, logged and audited", async () => {
  const { env, fake, id } = await world();
  const warn = console.warn;
  const logged: string[] = [];
  console.warn = (m: string) => logged.push(m);
  try {
    const owners = (await createGroup(env, "Owners", "", T0)) as Group;
    await addGrant(env, roleId(env as never, "owner"), { groupId: owners.id }, "test", T0);
    await createMapping(env, "admins", owners.id, "boss", T0);
    await signIn(env, ["admins"]);
    assert.deepEqual(groupsOf(env, id), ["Owners"]);
    await signIn(env, ["mods"]);
    assert.deepEqual(groupsOf(env, id), ["Moderators", "Owners"], "kept: the only active owner");
    const last = events(env, "groups_synced").at(-1);
    assert.deepEqual([last?.detail.added, last?.detail.keptForOwner], ["Moderators", "Owners"]);
    assert.match(logged.join("\n"), /would leave no active owner/);
    // With another owner, the same sync goes through.
    const other = await providerUser(env, "bo@example.com", "access", "bo");
    await addGrant(env, roleId(env as never, "owner"), { userId: other }, "test", T0);
    await signIn(env, ["mods"]);
    assert.deepEqual(groupsOf(env, id), ["Moderators"]);
  } finally {
    console.warn = warn;
    fake.restore();
  }
});

test("concurrent sign-ins of one person leave exactly one membership and one mark per group", async () => {
  const { env, fake, id } = await world();
  try {
    const tokens = await Promise.all([1, 2, 3, 4].map(() => mint({ ...PERSON, groups: ["nl-viewers", "mods"] })));
    const results = await Promise.all(tokens.map((t) => enter(env, t)));
    assert.deepEqual(results.map(where), ["/", "/", "/", "/"]);
    assert.deepEqual(groupsOf(env, id), ["Moderators", "Viewers"]);
    assert.equal(marks(env, id), 2);
    await Promise.all([signIn(env, []), signIn(env, [])]);
    assert.deepEqual([groupsOf(env, id), marks(env, id)], [[], 0]);
  } finally {
    fake.restore();
  }
});

test("a mapping change is applied to everyone already seen; a deleted group or account is skipped; errors other than the owner's surface", async () => {
  const { env, fake, id, viewers } = await world();
  try {
    await signIn(env, ["later"]);
    assert.deepEqual(groupsOf(env, id), []);
    await createMapping(env, "later", viewers.id, "boss", T0);
    assert.equal(await resyncEveryone(env, T0), 1);
    assert.deepEqual(groupsOf(env, id), ["Viewers"]);
    assert.equal(events(env, "groups_synced").at(-1)?.detail.via, "mapping");
    env.AUTH_DB.sqlite.exec("DELETE FROM access_group_map");
    assert.equal(await resyncEveryone(env, T0), 1, "the synced membership no mapping earns is removed");
    assert.deepEqual(groupsOf(env, id), []);
    env.AUTH_DB.sqlite.exec("DELETE FROM users");
    assert.equal(await resyncEveryone(env, T0), 0);
    env.AUTH_DB.state.failOn = /DELETE FROM group_members WHERE group_id/;
    env.AUTH_DB.sqlite.exec(`INSERT INTO access_synced_members VALUES ('${viewers.id}', 'u1', 'x')`);
    await assert.rejects(applyAccessGroups(env, "u1", [], T0, "mapping"), /forced failure/);
  } finally {
    fake.restore();
  }
});
