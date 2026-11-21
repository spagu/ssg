// #435: the roles claim through AUTH_JWT_ROLE_MAP. A mapped external role
// gives its roles and groups before the session starts (the first token
// carries them); one the token no longer carries takes back only what the
// mapping gave; nothing matching → nothing given; the owner role is never
// reachable; unknown names are skipped; every change is audited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { rolesClaim } from "../functions/api/auth/_external-identity.ts";
import { onRequestGet as me } from "../functions/api/auth/me/permissions.ts";
import { Browser, T0, call, useClock } from "./fakes.ts";
import { events, providerUser } from "./go151.ts";
import { roleId } from "./seed.ts";
import { FakeIssuer, ISS, jwtEnv, mint, post, sqlite, where } from "./external-fake.ts";

const METHOD = `jwt:${ISS}`;
const MAP = {
  editor: { roles: ["newsletter-editor"], groups: ["Editors"] },
  viewer: { roles: ["newsletter-viewer"] },
  boss: { groups: ["Owners"] },
  ghost: { roles: ["no-such-role"], groups: ["No such group"] },
};
type E = ReturnType<typeof jwtEnv>;
const rolesOf = (env: E, userId: string): string[] =>
  (sqlite(env).prepare("SELECT r.name FROM grants g JOIN roles r ON r.id = g.role_id WHERE g.user_id = ? ORDER BY r.name").all(userId) as { name: string }[]).map((r) => r.name);
const groupsOf = (env: E, userId: string): string[] =>
  (sqlite(env).prepare("SELECT g.name FROM group_members m JOIN groups g ON g.id = m.group_id WHERE m.user_id = ? ORDER BY g.name").all(userId) as { name: string }[]).map((r) => r.name);

async function world(map: unknown = MAP) {
  useClock();
  const env = jwtEnv({ AUTH_JWT_ROLE_MAP: JSON.stringify(map) });
  const fake = new FakeIssuer().install();
  const id = await providerUser(env, "ada@example.com", METHOD, "ext-1001");
  const owners = (await createGroup(env, "Owners", "", T0)) as Group;
  await addGrant(env, roleId(env as never, "owner"), { groupId: owners.id }, "test", T0);
  const editors = (await createGroup(env, "Editors", "", T0)) as Group;
  const manual = (await createGroup(env, "Manual", "", T0)) as Group;
  return { env, fake, id, editors, manual };
}

const quietly = async <T>(fn: () => Promise<T>): Promise<{ value: T; warned: string[] }> => {
  const warned: string[] = [];
  const real = console.warn;
  console.warn = (m: string) => warned.push(m);
  try {
    return { value: await fn(), warned };
  } finally {
    console.warn = real;
  }
};

test("mapped roles and groups are given before the first token; unmapped or unknown values give nothing", async () => {
  const { env, fake, id } = await world();
  try {
    const browser = new Browser();
    const { value: res, warned } = await quietly(async () => post(env, await mint({ roles: ["editor", "unmapped", "ghost"] }), { browser }));
    assert.equal(where(res), "/");
    assert.ok(warned.some((w) => /a role "no-such-role" that does not exist/.test(w)) && warned.some((w) => /a group "No such group"/.test(w)));
    assert.deepEqual(rolesOf(env, id), ["newsletter-editor"]);
    assert.deepEqual(groupsOf(env, id), ["Editors"]);
    const perms = (await (await call(me, env, "/api/auth/me/permissions", { browser })).json()).token as string[];
    assert.ok(perms.some((p) => p.startsWith("newsletter:")), "the first token already carries them");
    const ev = events(env, "roles_synced").at(-1);
    assert.deepEqual([ev?.method, ev?.detail.rolesAdded, ev?.detail.groupsAdded], [METHOD, "newsletter-editor", "Editors"]);
  } finally {
    fake.restore();
  }
});

test("a role the token no longer carries is taken back — only what the mapping gave; manual grants stay", async () => {
  const { env, fake, id, manual, editors } = await world();
  try {
    await addGrant(env, roleId(env as never, "newsletter-viewer"), { userId: id }, "admin", T0);
    await addMember(env, manual.id, id, "admin", T0);
    await post(env, await mint({ roles: "editor viewer" }));
    assert.deepEqual(rolesOf(env, id), ["newsletter-editor", "newsletter-viewer"]);
    const before = events(env, "roles_synced").length;
    await post(env, await mint({ roles: "editor viewer" }));
    assert.equal(events(env, "roles_synced").length, before, "no change, no event");
    await post(env, await mint({ roles: [] }));
    assert.deepEqual(rolesOf(env, id), ["newsletter-viewer"], "the manual grant stays; the synced one goes");
    assert.deepEqual(groupsOf(env, id), ["Manual"]);
    const ev = events(env, "roles_synced").at(-1);
    assert.deepEqual([ev?.detail.rolesRemoved, ev?.detail.groupsRemoved], ["newsletter-editor", "Editors"]);
    // No claim at all: nothing matches, nothing from the mapping.
    await addMember(env, editors.id, id, "admin", T0);
    await post(env, await mint({ roles: undefined }));
    assert.deepEqual(groupsOf(env, id), ["Editors", "Manual"], "a membership made by hand is never removed");
  } finally {
    fake.restore();
  }
});

test("the owner role is never reachable: a mapped group holding it is skipped and logged", async () => {
  const { env, fake, id } = await world();
  try {
    const { warned } = await quietly(async () => post(env, await mint({ roles: ["boss"] })));
    assert.deepEqual(groupsOf(env, id), []);
    assert.deepEqual(rolesOf(env, id), []);
    assert.ok(warned.some((w) => /holds the owner role; skipped/.test(w)));
  } finally {
    fake.restore();
  }
});

test("without AUTH_JWT_ROLE_MAP the roles claim is ignored: roles are the administrator's", async () => {
  useClock();
  const env = jwtEnv();
  const fake = new FakeIssuer().install();
  try {
    const id = await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    await post(env, await mint({ roles: ["editor"] }));
    assert.deepEqual(rolesOf(env, id), []);
    assert.equal(events(env, "roles_synced").length, 0);
  } finally {
    fake.restore();
  }
});

test("rolesClaim: an array or a space-separated string; trimmed, deduplicated, bounded; anything else is none", () => {
  assert.deepEqual(rolesClaim({ r: ["a", " a ", "b", 5, "", "x".repeat(257)] }, "r"), ["a", "b"]);
  assert.deepEqual(rolesClaim({ r: "a  b a" }, "r"), ["a", "b"]);
  assert.deepEqual(rolesClaim({ r: { a: 1 } }, "r"), []);
  assert.deepEqual(rolesClaim({}, "r"), []);
  assert.equal(rolesClaim({ r: Array.from({ length: 300 }, (_, i) => `r${i}`) }, "r").length, 200);
});
