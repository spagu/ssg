// GO-140: which roles need a second factor, and what a session without one
// carries — across password and provider sign-ins and after refresh; the
// grace that lets an owner without a factor in to enrol, and why it is not a
// bypass; /api/auth/me/permissions and the pages saying what is missing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestGet as mine } from "../functions/api/auth/me/permissions.ts";
import { onRequestGet as listRoles } from "../functions/api/auth/admin/roles/index.ts";
import { onRequestPost as reset } from "../functions/api/auth/mfa/reset.ts";
import { onRequestGet as panels } from "../functions/auth/index.ts";
import { onRequestPost as totpStart } from "../functions/api/auth/mfa/totp/start.ts";
import { onRequestGet as factors } from "../functions/api/auth/mfa/factors/index.ts";
import { factorRoles, mfaOverrides, roleNeedsFactor } from "../functions/api/auth/_mfa-policy.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { MFA_KEY, code, enrolTotp, mfaEnv, signIn, verify } from "./mfa.ts";
import { grantPerms, roleId, seedUser } from "./seed.ts";
import { fakeProviders, providerEnv, providerUser, roundTrip } from "./go151.ts";

const permsOf = (b: Browser): string[] => JSON.parse(Buffer.from((b.jar.get("__Host-ssg-auth") as string).split(".")[1], "base64url").toString()).perms;

async function owner(env = mfaEnv(), email = "ada@example.com") {
  const id = await seedUser(env, email);
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "test", T0);
  return id;
}

test("default policy: sensitive permissions (isSensitive) and …:send need a second factor; the rest do not", () => {
  for (const [perms, needs] of [
    [["*"], true],
    [["auth:admin"], true],
    [["newsletter:erase"], true],
    [["comments:*"], true],
    [["newsletter:send"], true],
    [["newsletter:read", "comments:moderate"], false],
    [[], false],
  ] as [string[], boolean][]) {
    assert.equal(roleNeedsFactor("r", perms, null), needs, perms.join(","));
  }
});

test("AUTH_MFA_ROLES: true/false per role overrides the default; anything else fails closed for every role", () => {
  const env = (v: string) => mfaEnv({ AUTH_MFA_ROLES: v });
  assert.equal(mfaOverrides(env("")), null);
  assert.deepEqual(mfaOverrides(env('{"owner": false, "viewer": true}')), new Map([["owner", false], ["viewer", true]]));
  for (const bad of ["[]", "{", '{"owner": "yes"}', "null"]) assert.equal(mfaOverrides(env(bad)), "invalid", bad);
  const roles = new Map([["owner", ["*"]], ["viewer", ["newsletter:read"]], ["editor", ["newsletter:send"]]]);
  assert.deepEqual([...factorRoles(env('{"owner": false, "viewer": true}'), roles)].sort(), ["editor", "viewer"]);
  const errors: string[] = [];
  const real = console.error;
  console.error = (m: string) => errors.push(m);
  try {
    assert.deepEqual([...factorRoles(env("not json"), roles)].sort(), ["editor", "owner", "viewer"]);
  } finally {
    console.error = real;
  }
  assert.match(errors[0], /AUTH_MFA_ROLES/);
  assert.equal(factorRoles(makeEnv(), roles).size, 0, "the second factor off: nothing needs it");
});

test("password sign-in without a factor: the owner's * is left out, the others stay; me/permissions and the pages say why", async () => {
  useClock();
  const env = mfaEnv();
  const id = await owner(env);
  await grantPerms(env, id, ["newsletter:read"]);
  const { res, body, browser } = await signIn(env, "ada@example.com");
  assert.equal(res.status, 200);
  assert.equal(body.enrol, true);
  assert.deepEqual(permsOf(browser), ["newsletter:read"]);
  const me = await (await call(mine, env, "/api/auth/me/permissions", { browser })).json();
  assert.deepEqual(me.secondFactor, { roles: ["owner"], hasFactor: false, url: "/auth/mfa/enrol" });
  assert.deepEqual(me.token, ["newsletter:read"]);
  const page = await (await call(panels, env, "/auth/", { browser })).text();
  assert.match(page, /Roles waiting for a second factor in this session: owner\. <a href="\/auth\/mfa\/enrol\?next=%2Fauth%2F">Set up a second factor<\/a>/);
  const admin = await call(listRoles, env, "/api/auth/admin/roles", { browser });
  assert.equal(admin.status, 403, "no admin route on the password alone");
  const f = await (await call(factors, env, "/api/auth/mfa/factors", { browser })).json();
  assert.deepEqual([f.enabled, f.state, f.factors, f.requiredBy, f.sessionFactor], [true, "ready", [], ["owner"], null]);
  // Without the second factor on, the same account has everything, as before.
  const plain = makeEnv();
  await owner(plain);
  const off = await signIn(plain, "ada@example.com");
  assert.deepEqual(permsOf(off.browser), ["*"]);
  assert.equal(off.body.enrol, undefined);
  assert.equal((await (await call(mine, plain, "/api/auth/me/permissions", { browser: off.browser })).json()).secondFactor, undefined);
});

test("grace: enrolling the FIRST factor in a fresh sign-in counts for that session (a new token with *), and survives refresh", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  await enrolTotp(env, browser, clock.now);
  assert.deepEqual(permsOf(browser), ["*"], "the confirm answer carries the new access cookie");
  clock.advance(11 * 60_000);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser })).status, 200);
  assert.deepEqual(permsOf(browser), ["*"], "refresh keeps it: the session passed a second factor");
});

test("grace is not a bypass: an old session cannot enrol; a factor makes the password alone useless; a second factor needs a step-up", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await owner(env);
  const stale = await signIn(env, "ada@example.com");
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: stale.browser });
  const late = await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser: stale.browser });
  assert.deepEqual([late.status, (await late.json()).error], [403, "reauth_required"], "a stolen, old session cannot add its own authenticator");
  const fresh = await signIn(env, "ada@example.com");
  const { secret } = await enrolTotp(env, fresh.browser, clock.now);
  // The old session still lacks * (it never passed a second factor) …
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: stale.browser });
  assert.deepEqual(permsOf(stale.browser), []);
  const me = await (await call(mine, env, "/api/auth/me/permissions", { browser: stale.browser })).json();
  assert.deepEqual(me.secondFactor, { roles: ["owner"], hasFactor: true, url: "/auth/mfa/step-up" });
  // … and cannot add another factor without one.
  const add = await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser: stale.browser });
  assert.deepEqual([add.status, (await add.json()).error], [403, "step_up_required"]);
  // The password alone no longer opens a session at all.
  clock.advance(60_000);
  const again = await signIn(env, "ada@example.com");
  assert.equal(again.res.status, 401);
  // Nor can a factor-less session reset anyone.
  assert.equal((await call(reset, env, "/api/auth/mfa/reset", { json: { userId: "x" }, browser: stale.browser })).status, 403);
  const ok = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: again.browser });
  assert.equal(ok.status, 200);
  assert.deepEqual(permsOf(again.browser), ["*"]);
});

test("roles: a sender needs the factor, a viewer does not; an override exempts one", async () => {
  useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "eva@example.com");
  await addGrant(env, roleId(env as never, "newsletter-editor"), { userId: id }, "t", T0);
  await addGrant(env, roleId(env as never, "comments-moderator"), { userId: id }, "t", T0);
  assert.deepEqual(permsOf((await signIn(env, "eva@example.com")).browser), ["comments:moderate", "comments:read"]);
  const exempt = { ...env, AUTH_MFA_ROLES: '{"newsletter-editor": false}' };
  assert.deepEqual(permsOf((await signIn(exempt, "eva@example.com")).browser), ["comments:moderate", "comments:read", "newsletter:edit", "newsletter:read", "newsletter:send"]);
});

test("provider sign-in: an owner without a factor is sent to enrol, with * left out — the provider's own checks count for nothing", async () => {
  useClock();
  const env = providerEnv({ AUTH_MFA_KEY: MFA_KEY } as never);
  const id = await providerUser(env, "pat@example.com", "github", "77");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const fake = fakeProviders();
  try {
    const b = new Browser();
    const r = await roundTrip(env, fake, b, "github", { sub: "77", email: "pat@example.com", verified: true }, { next: "/auth/account" });
    assert.equal(r.location, "/auth/mfa/enrol?required=1&next=%2Fauth%2Faccount");
    assert.deepEqual(permsOf(b), []);
  } finally {
    fake.restore();
  }
});
