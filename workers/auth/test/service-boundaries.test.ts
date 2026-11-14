// GO-141: what a service account can never be — a person who signs in, is
// invited or reset, joins a group or holds up the owner invariant — and the
// channel rule on auth's own routes: a service token is never a browser session.
import { test } from "node:test";
import assert from "node:assert/strict";
import { verifyToken } from "../functions/api/auth/_jwt.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { onRequestGet as listGrants, onRequestPost as grant } from "../functions/api/auth/admin/grants/index.ts";
import { onRequestDelete as removeGrant } from "../functions/api/auth/admin/grants/[id].ts";
import { onRequestPost as addMember } from "../functions/api/auth/admin/groups/[id]/members/index.ts";
import { onRequestPost as resetLink } from "../functions/api/auth/admin/users/[id]/reset-link.ts";
import { onRequestPost as revokeSessions } from "../functions/api/auth/admin/users/[id]/revoke-sessions.ts";
import { onRequestGet as listAccounts } from "../functions/api/auth/admin/service-accounts/index.ts";
import { createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { ACCESS_COOKIE } from "../functions/api/auth/_cookies.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { roleId, signedIn } from "./seed.ts";
import { account, secretFor, swap } from "./service.ts";
import { KEYSET, claims, sign } from "./verifier-fixtures.ts";

async function setup() {
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, ["auth:admin", "web-push:notify"], "robot");
  return { env, owner, id };
}

test("no way in through the login: neither its name nor its id is an address", async () => {
  const { env, id } = await setup();
  for (const email of ["robot", id, `${id}@example.com`, "robot@example.com"]) {
    const res = await call(login, env, "/api/auth/login", { json: { email, password: "correct horse battery" }, browser: new Browser(`192.0.2.${email.length}`) });
    assert.deepEqual([res.status, (await res.json()).error], [401, "invalid_credentials"], email);
  }
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM users WHERE id = ?").get(id)?.n, 0, "no users row at all");
});

test("no invitation, reset link, session revocation or group membership can name it", async () => {
  const { env, owner, id } = await setup();
  const reset = await call(resetLink, env, "/x", { method: "POST", browser: owner.browser, params: { id } });
  assert.deepEqual([reset.status, (await reset.json()).error], [404, "user_not_found"]);
  assert.equal((await call(revokeSessions, env, "/x", { method: "POST", browser: owner.browser, params: { id } })).status, 404);
  const group = (await createGroup(env, "Robots", "", T0)) as Group;
  const member = await call(addMember, env, "/x", { json: { userId: id }, browser: owner.browser, params: { id: group.id } });
  assert.deepEqual([member.status, (await member.json()).error], [404, "user_not_found"]);
  // An invitation creates a new person from an address; there is no field that could point at an account.
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM group_members WHERE user_id = ?").get(id)?.n, 0);
});

test("grants: a service account takes roles through the existing endpoint; unknown ids are still refused", async () => {
  const { env, owner, id } = await setup();
  const reader = roleId(env, "newsletter-viewer");
  const res = await call(grant, env, "/x", { json: { roleId: reader, userId: id }, browser: owner.browser });
  assert.equal(res.status, 201);
  const items = (await (await call(listGrants, env, `/x?userId=${id}`, { browser: owner.browser })).json()).items;
  assert.equal(items.length, 2, "the role from setup and this one");
  assert.ok(items.some((g: { roleName: string; userId: string }) => g.roleName === "newsletter-viewer" && g.userId === id));
  const bad = await call(grant, env, "/x", { json: { roleId: reader, userId: "nobody" }, browser: owner.browser });
  assert.deepEqual([bad.status, (await bad.json()).error], [404, "user_not_found"]);
});

test("owner invariant: a service account holding owner never keeps the last person's grant alive", async () => {
  const { env, owner, id } = await setup();
  const ownerRole = roleId(env, "owner");
  assert.equal((await call(grant, env, "/x", { json: { roleId: ownerRole, userId: id }, browser: owner.browser })).status, 201);
  const items = (await (await call(listGrants, env, `/x?roleId=${ownerRole}`, { browser: owner.browser })).json()).items;
  const human = items.find((g: { userId: string }) => g.userId === owner.id);
  const res = await call(removeGrant, env, "/x", { method: "DELETE", browser: owner.browser, params: { id: human.id } });
  assert.deepEqual([res.status, (await res.json()).error], [409, "owner_required"]);
  const robots = items.find((g: { userId: string }) => g.userId === id);
  assert.equal((await call(removeGrant, env, "/x", { method: "DELETE", browser: owner.browser, params: { id: robots.id } })).status, 200, "its own grant goes freely");
});

test("channel rule: a service token is never a session in auth — not as the cookie, not as a bearer", async () => {
  const { env, owner, id } = await setup();
  const secret = await secretFor(env, owner.browser, id, { permissions: ["web-push:notify"] });
  const jwt = (await (await swap(env, `Bearer ${secret}`)).json()).accessToken as string;
  const asCookie = await call(listAccounts, env, "/x", { headers: { cookie: `${ACCESS_COOKIE}=${jwt}` } });
  assert.deepEqual([asCookie.status, await asCookie.json()], [401, { error: "not_authenticated" }]);
  const asBearer = await call(listAccounts, env, "/x", { headers: { authorization: `Bearer ${jwt}` } });
  assert.equal(asBearer.status, 401);
  assert.equal((await call(session, env, "/api/auth/session", { headers: { cookie: `${ACCESS_COOKIE}=${jwt}` } })).status, 401);
  assert.equal((await call(listAccounts, env, "/x", { browser: owner.browser })).status, 200, "a person's cookie still works");
});

test("verifyToken: kind absent or null = user, user and service known, anything else refused as a format", async () => {
  const opts = { keys: KEYSET.verify, issuer: claims().iss, audience: "example.com", nowSeconds: claims().iat };
  for (const kind of [undefined, null, "user", "service"]) assert.ok((await verifyToken(await sign({ kind }), opts)).ok, String(kind));
  for (const kind of ["robot", "", 1, ["service"]]) assert.deepEqual(await verifyToken(await sign({ kind }), opts), { ok: false, reason: "version" }, String(kind));
});
