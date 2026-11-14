// Accepting an invitation (GO-134): once, atomically, with the roles and
// groups that still exist, signed in at the end; the form path; refusals that
// do not use the invitation up.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { effectivePermissions } from "../functions/api/auth/_grants.ts";
import { createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { Browser, PASSWORD, T0, call, counting, makeEnv, useClock } from "./fakes.ts";
import { accept, acceptWith, invite, rows, tokenOf } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";

async function setup(env = makeEnv()) {
  const boss = await signedIn(env, "boss@example.com", "owner");
  const reader = (await createRole(env, { name: "reader", description: "", permissions: ["newsletter:read"] }, T0)) as Role;
  const mods = (await createRole(env, { name: "mods", description: "", permissions: ["comments:moderate"] }, T0)) as Role;
  const group = (await createGroup(env, "Moderators", "", T0)) as Group;
  await addGrant(env, mods.id, { groupId: group.id }, "x", T0);
  const inv = await invite(env, boss.browser, { email: "new@example.com", name: "Nina", roleIds: [reader.id], groupIds: [group.id] });
  return { env, boss, reader, mods, group, token: tokenOf(inv.body.link), id: inv.body.invitation.id as string };
}

test("accept: the account with the invited roles and groups, signed in; the link works once", async () => {
  useClock();
  const { env, token, boss, group } = await setup();
  const first = await acceptWith(env, token);
  assert.equal(first.status, 201);
  assert.deepEqual(first.body.user, { id: first.body.user.id, email: "new@example.com", name: "Nina", mustChangePassword: false });
  assert.deepEqual(first.body.skipped, { roles: [], groups: [] });
  assert.ok(first.browser.jar.has(ACCESS_COOKIE) && first.browser.jar.has(REFRESH_COOKIE));
  assert.deepEqual(await effectivePermissions(env, first.body.user.id), ["comments:moderate", "newsletter:read"]);
  const grant = rows(env, "grants").find((g) => g.user_id === first.body.user.id) as Record<string, unknown>;
  assert.equal(grant.granted_by, boss.id, "the grant names the inviter");
  assert.equal((rows(env, "group_members").find((m) => m.user_id === first.body.user.id) as Record<string, unknown>).group_id, group.id);
  const again = await acceptWith(env, token);
  assert.deepEqual([again.status, again.body.error], [410, "invitation_invalid"]);
  assert.equal(rows(env, "users").length, 2);
});

test("accept: two at once — exactly one account", async () => {
  useClock();
  const { env, token } = await setup();
  const results = await Promise.all([acceptWith(env, token), acceptWith(env, token), acceptWith(env, token)]);
  assert.deepEqual(results.map((r) => r.status).sort(), [201, 410, 410]);
  assert.equal(rows(env, "users").filter((u) => u.email === "new@example.com").length, 1);
  const user = rows(env, "users").find((u) => u.email === "new@example.com") as Record<string, unknown>;
  assert.equal(rows(env, "grants").filter((g) => g.user_id === user.id).length, 1, "one reader grant, no duplicates");
  assert.equal(rows(env, "group_members").filter((m) => m.user_id === user.id).length, 1);
});

test("accept: expired, withdrawn, unknown and malformed tokens get one answer", async () => {
  const clock = useClock();
  const { env, token } = await setup();
  for (const t of ["", "short", "A".repeat(43)]) assert.deepEqual((await acceptWith(env, t)).body, { error: "invitation_invalid" });
  clock.advance(72 * 3600_000);
  assert.equal((await acceptWith(env, token)).status, 410, "expired at exactly 72 h");
  assert.equal(rows(env, "users").length, 1);
});

test("accept: a refused password or method does not use the invitation up", async () => {
  useClock();
  const { env, token } = await setup();
  const short = await acceptWith(env, token, { password: "short" });
  assert.deepEqual([short.status, short.body.error], [400, "password_too_short"]);
  assert.equal((await acceptWith(env, token, { password: "NEW@example.com" })).body.error, "password_equals_email");
  assert.equal((await acceptWith(env, token, { method: "github" })).body.error, "unsupported_method");
  assert.equal((await acceptWith(env, token, { method: "password", name: "  " + "N".repeat(250) + "  " })).status, 201);
  assert.equal((rows(env, "users").find((u) => u.email === "new@example.com") as Record<string, unknown>).display_name, "N".repeat(200));
});

test("accept: a role or group deleted meanwhile is skipped and reported, never granted", async () => {
  useClock();
  const { env, token, reader, group } = await setup();
  await env.AUTH_DB.batch([env.AUTH_DB.prepare("DELETE FROM roles WHERE id = ?").bind(reader.id), env.AUTH_DB.prepare("DELETE FROM groups WHERE id = ?").bind(group.id)]);
  const r = await acceptWith(env, token);
  assert.equal(r.status, 201);
  assert.deepEqual(r.body.skipped, { roles: [reader.id], groups: [group.id] });
  assert.deepEqual(await effectivePermissions(env, r.body.user.id), []);
  assert.equal(rows(env, "grants").filter((g) => g.user_id === r.body.user.id).length, 0);
});

test("accept: an address that got an account meanwhile is 409, and nothing changes", async () => {
  useClock();
  const { env, token } = await setup();
  await seedUser(env, "new@example.com");
  const r = await acceptWith(env, token);
  assert.deepEqual([r.status, r.body.error], [409, "account_exists"]);
  assert.equal((rows(env, "invitations")[0] as Record<string, unknown>).accepted_at, null, "the batch rolled back whole");
});

test("accept: a plain form — 303 to Your panels (welcome, skipped count) with the cookies; failures go back to the page", async () => {
  useClock();
  const { env, token, reader } = await setup();
  const form = (fields: Record<string, string>, browser = new Browser()) => call(accept, env, "/api/auth/invite", { form: fields, browser });
  const bad = await form({ token, method: "password", password: "short" });
  assert.equal(bad.headers.get("location"), `/auth/invite?token=${token}&error=password_too_short`);
  const gone = await form({ token: "x", password: PASSWORD });
  assert.equal(gone.headers.get("location"), "/auth/invite?error=invitation_invalid", "no token in the URL when the token failed");
  await env.AUTH_DB.prepare("DELETE FROM roles WHERE id = ?").bind(reader.id).run();
  const browser = new Browser();
  const ok = await form({ token, method: "password", password: PASSWORD, name: "" }, browser);
  assert.equal(ok.status, 303);
  assert.equal(ok.headers.get("location"), "/auth/?welcome=1&skipped=1");
  assert.ok(browser.jar.has(ACCESS_COOKIE));
  assert.equal((rows(env, "users").find((u) => u.email === "new@example.com") as Record<string, unknown>).display_name, "Nina", "the invited name when none is typed");
  await seedUser(env, "other@example.com");
  const second = await setup();
  await seedUser(second.env, "new@example.com");
  const exists = await call(accept, second.env, "/api/auth/invite", { form: { token: second.token, password: PASSWORD }, browser: new Browser() });
  assert.equal(exists.headers.get("location"), `/auth/invite?token=${second.token}&error=account_exists`);
  const plain = await setup();
  const welcome = await call(accept, plain.env, "/api/auth/invite", { form: { token: plain.token, password: PASSWORD }, browser: new Browser() });
  assert.equal(welcome.headers.get("location"), "/auth/?welcome=1");
});

test("accept: CSRF — a form from elsewhere is refused before anything happens", async () => {
  useClock();
  const { env, token } = await setup();
  const res = await call(accept, env, "/api/auth/invite", { form: { token, password: PASSWORD }, headers: { origin: "https://evil.example.com" } });
  assert.equal(res.status, 403);
  assert.equal((await acceptWith(env, token)).status, 201);
});

test("accept: the runtime refusing the hash cost is 503 and keeps the invitation", async () => {
  useClock();
  const { env, token } = await setup(makeEnv({ AUTH_PBKDF2_ITERATIONS: "200000" }));
  const c = counting(100_000);
  try {
    const r = await acceptWith(env, token);
    assert.deepEqual([r.status, r.body.error], [503, "hashing_unavailable"]);
  } finally {
    c.restore();
  }
  assert.equal((rows(env, "invitations")[0] as Record<string, unknown>).accepted_at, null);
});

test("accept: permissions that cannot fit in a cookie — the account exists, no session, a named reason", async () => {
  useClock();
  const env = makeEnv();
  const boss = await signedIn(env, "boss@example.com", "owner");
  const perms = Array.from({ length: 60 }, (_, i) => `w${i}:` + "a".repeat(60));
  const big = (await createRole(env, { name: "big", description: "", permissions: perms }, T0)) as Role;
  const a = await invite(env, boss.browser, { email: "a@example.com", roleIds: [big.id] });
  const error = console.error;
  console.error = () => {};
  try {
    const r = await acceptWith(env, tokenOf(a.body.link));
    assert.deepEqual([r.status, r.body.error], [500, "permissions_too_large"]);
    assert.equal(r.browser.jar.size, 0);
    const b = await invite(env, boss.browser, { email: "b@example.com", roleIds: [big.id] });
    const form = await call(accept, env, "/api/auth/invite", { form: { token: tokenOf(b.body.link), password: PASSWORD }, browser: new Browser() });
    assert.equal(form.headers.get("location"), "/auth/login?error=permissions_too_large");
  } finally {
    console.error = error;
  }
});

test("accept: it is the account's first sign-in — last_login_at is set, as a password login sets it", async () => {
  const clock = useClock();
  const { env, token } = await setup();
  const r = await acceptWith(env, token);
  assert.equal(r.status, 201);
  const row = env.AUTH_DB.sqlite.prepare("SELECT last_login_at FROM users WHERE id = ?").get(r.body.user.id) as { last_login_at: string | null };
  assert.equal(row.last_login_at, new Date(clock.now).toISOString());
});
