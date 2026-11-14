// Service tokens and the second-factor policy: a permission that people may
// use only after a second factor reaches a machine's token only when an owner
// listed it in the token's allowSensitive. Pure rule (machineSensitive,
// tokenPermissions) and the exchange end to end (serviceGrant + token.ts).
import { test } from "node:test";
import assert from "node:assert/strict";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { serviceGrant } from "../functions/api/auth/_service-policy.ts";
import { machineSensitive, tokenPermissions } from "../functions/api/auth/_service-perms.ts";
import { T0, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, newToken, secretFor, swap } from "./service.ts";

test("machineSensitive: isSensitive always; otherwise only what no factor-free role gives", () => {
  assert.equal(machineSensitive("auth:admin", ["auth:admin"]), true);
  assert.equal(machineSensitive("newsletter:send", []), true);
  assert.equal(machineSensitive("newsletter:send", ["newsletter:send"]), false);
  assert.equal(machineSensitive("newsletter:read", ["newsletter:*"]), false, "a factor-free role's wildcard frees the names it covers…");
  assert.equal(machineSensitive("newsletter:*", ["newsletter:*"]), true, "…while the wildcard itself stays sensitive");
  assert.equal(machineSensitive("web-push:notify", ["web-push:notify"]), false);
});

test("tokenPermissions: allowSensitive lets a bound permission through; free defaults to the whole account", () => {
  assert.deepEqual(tokenPermissions(["newsletter:read", "newsletter:send"], null, [], ["newsletter:read"]), ["newsletter:read"]);
  assert.deepEqual(tokenPermissions(["newsletter:read", "newsletter:send"], null, ["newsletter:send"], ["newsletter:read"]), ["newsletter:read", "newsletter:send"]);
  assert.deepEqual(tokenPermissions(["newsletter:send"], null, []), ["newsletter:send"], "without a free list only isSensitive filters");
});

async function role(env: ReturnType<typeof makeEnv>, name: string, permissions: string[], userId: string) {
  const r = (await createRole(env, { name, description: "", permissions }, T0)) as Role;
  await addGrant(env, r.id, { userId }, "test", T0);
}

test("serviceGrant: a role needing a factor binds all its permissions; another role can free one", async () => {
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  await role(env, "sender", ["newsletter:read", "newsletter:send"], id);
  let g = await serviceGrant(env, id);
  assert.deepEqual(g.perms.sort(), ["newsletter:read", "newsletter:send"]);
  assert.deepEqual(g.free, [], "the …:send role needs a factor, so nothing it gives is free");
  await role(env, "reader", ["newsletter:read"], id);
  g = await serviceGrant(env, id);
  assert.deepEqual(g.free, ["newsletter:read"]);
});

async function exchanged(env: ReturnType<typeof makeEnv>, json: Record<string, unknown> = {}) {
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  await role(env, "sender", ["newsletter:send"], id);
  const secret = await secretFor(env, owner.browser, id, json);
  const res = await swap(env, `Bearer ${secret}`);
  assert.equal(res.status, 200);
  return { perms: ((await res.json()) as { permissions: string[] }).permissions, owner, id, secret };
}

test("exchange: newsletter:send through a factor-bound role is withheld without allowSensitive", async () => {
  useClock();
  assert.deepEqual((await exchanged(makeEnv())).perms, []);
});

test("exchange: an owner's allowSensitive lets newsletter:send through", async () => {
  useClock();
  assert.deepEqual((await exchanged(makeEnv(), { allowSensitive: ["newsletter:send"] })).perms, ["newsletter:send"]);
});

test("exchange: a non-owner cannot set allowSensitive", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:send"]);
  const id = await account(env, admin.browser, []);
  const r = await newToken(env, admin.browser, id, { allowSensitive: ["newsletter:send"] });
  assert.equal(r.status, 403);
});

test("exchange: AUTH_MFA_ROLES marking the role as factor-free frees its permissions; marking a custom role binds them", async () => {
  useClock();
  assert.deepEqual((await exchanged(makeEnv({ AUTH_MFA_ROLES: '{"sender": false}' } as never))).perms, ["newsletter:send"]);
  useClock();
  const env = makeEnv({ AUTH_MFA_ROLES: '{"pusher": true}' } as never);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  await role(env, "pusher", ["web-push:notify"], id);
  const res = await swap(env, `Bearer ${await secretFor(env, owner.browser, id)}`);
  assert.deepEqual(((await res.json()) as { permissions: string[] }).permissions, []);
});

test("exchange: an invalid AUTH_MFA_ROLES fails closed — nothing is free", async () => {
  useClock();
  const env = makeEnv({ AUTH_MFA_ROLES: "not json" } as never);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  await role(env, "pusher", ["web-push:notify"], id);
  const res = await swap(env, `Bearer ${await secretFor(env, owner.browser, id)}`);
  assert.deepEqual(((await res.json()) as { permissions: string[] }).permissions, []);
});

test("exchange: the rule holds with the second factor switched off (no AUTH_MFA_KEY)", async () => {
  useClock();
  const env = makeEnv();
  assert.equal((env as { AUTH_MFA_KEY?: string }).AUTH_MFA_KEY, undefined);
  assert.deepEqual((await exchanged(env)).perms, []);
});

test("exchange: an existing token loses a permission at its next exchange once its role needs a factor", async () => {
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  const r = (await createRole(env, { name: "pusher", description: "", permissions: ["web-push:notify"] }, T0)) as Role;
  await addGrant(env, r.id, { userId: id }, "test", T0);
  const secret = await secretFor(env, owner.browser, id);
  let res = await swap(env, `Bearer ${secret}`);
  assert.deepEqual(((await res.json()) as { permissions: string[] }).permissions, ["web-push:notify"]);
  const { updateRole } = await import("../functions/api/auth/_roles.ts");
  await updateRole(env, r.id, { permissions: ["web-push:notify", "newsletter:send"] }, T0);
  res = await swap(env, `Bearer ${secret}`);
  assert.deepEqual(((await res.json()) as { permissions: string[] }).permissions, []);
});
