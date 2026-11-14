// GO-141: the admin API for service accounts and their tokens — validation,
// escalation, the owner-only sensitive list, revocation.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as listAccounts, onRequestPost as createAccount } from "../functions/api/auth/admin/service-accounts/index.ts";
import * as one from "../functions/api/auth/admin/service-accounts/[id]/index.ts";
import { onRequestGet as listTokensRoute } from "../functions/api/auth/admin/service-accounts/[id]/tokens/index.ts";
import { onRequestDelete as revoke } from "../functions/api/auth/admin/service-accounts/[id]/tokens/[tokenId].ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { roleId, signedIn } from "./seed.ts";
import { account, newToken, secretFor } from "./service.ts";
import { parseSecret } from "../functions/api/auth/_service-tokens.ts";

async function setup() {
  useClock();
  const env = makeEnv();
  const owner = (await signedIn(env, "owner@example.com", "owner")).browser;
  const admin = (await signedIn(env, "admin@example.com", ["auth:admin", "web-push:notify"])).browser;
  return { env, owner, admin };
}
const post = (env: ReturnType<typeof makeEnv>, b: Browser, json: unknown) => call(createAccount, env, "/api/auth/admin/service-accounts", { json, browser: b });
const at = (id: string) => `/api/auth/admin/service-accounts/${id}`;

test("service accounts: create, list, read, rename, disable, delete", async () => {
  const { env, admin } = await setup();
  const res = await post(env, admin, { name: "ci-deploy", description: "GitHub Actions after each deploy" });
  assert.equal(res.status, 201);
  const sa = (await res.json()).serviceAccount;
  assert.deepEqual([sa.name, sa.description, sa.status, typeof sa.createdBy], ["ci-deploy", "GitHub Actions after each deploy", "active", "string"]);
  assert.equal((await post(env, admin, { name: "ci-deploy" })).status, 409);
  const list = await (await call(listAccounts, env, "/x", { browser: admin })).json();
  assert.deepEqual([list.items.map((i: { name: string }) => i.name), list.nextCursor], [["ci-deploy"], null]);

  const got = await (await call(one.onRequestGet, env, at(sa.id), { browser: admin, params: { id: sa.id } })).json();
  assert.deepEqual([got.serviceAccount.id, got.permissions, got.tokens], [sa.id, [], []]);
  const put = (json: unknown) => call(one.onRequestPut, env, at(sa.id), { method: "PUT", json, browser: admin, params: { id: sa.id } });
  const renamed = await (await put({ name: "ci", description: "d", status: "disabled" })).json();
  assert.deepEqual([renamed.serviceAccount.name, renamed.serviceAccount.description, renamed.serviceAccount.status], ["ci", "d", "disabled"]);
  assert.equal((await (await put({})).json()).serviceAccount.name, "ci", "an empty patch changes nothing");
  await post(env, admin, { name: "taken" });
  assert.equal((await put({ name: "taken" })).status, 409);
  for (const bad of [{ name: "Not A Slug" }, { name: "" }, { description: 7 }, { status: "deleted" }]) assert.equal((await put(bad)).status, 400, JSON.stringify(bad));
  assert.equal((await call(one.onRequestPut, env, at("nope"), { method: "PUT", json: {}, browser: admin, params: { id: "nope" } })).status, 404);
  assert.equal((await call(one.onRequestPut, env, at("nope"), { method: "PUT", body: "x", browser: admin, params: { id: "nope" } } as never)).status, 400);

  await secretFor(env, admin, sa.id);
  const del = await call(one.onRequestDelete, env, at(sa.id), { method: "DELETE", browser: admin, params: { id: sa.id } });
  assert.deepEqual([del.status, await del.json()], [200, { ok: true }]);
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM service_tokens").get()?.n, 0, "its tokens go with it");
  assert.equal((await call(one.onRequestDelete, env, at(sa.id), { method: "DELETE", browser: admin, params: { id: sa.id } })).status, 404);
  assert.equal((await call(one.onRequestGet, env, at(sa.id), { browser: admin, params: { id: sa.id } })).status, 404);
});

test("service accounts: create validates name and description", async () => {
  const { env, admin } = await setup();
  for (const bad of [{}, { name: "UPPER" }, { name: "x".repeat(65) }, { name: "ok", description: "x".repeat(501) }]) {
    const res = await post(env, admin, bad);
    assert.equal(res.status, 400, JSON.stringify(bad));
    assert.equal((await res.json()).error, "invalid_request");
  }
  assert.equal((await call(createAccount, env, "/x", { method: "POST", browser: admin, headers: { "content-type": "application/json" } })).status, 400, "no body");
});

test("tokens: the secret once, the defaults, the list without it", async () => {
  const { env, admin } = await setup();
  const id = await account(env, admin, ["web-push:notify"]);
  const r = await newToken(env, admin, id, { name: "deploy job" });
  assert.equal(r.status, 201);
  const { token, secret } = r.body;
  assert.match(secret, /^ssgsvc_[0-9a-f]{12}_[A-Za-z0-9_-]{43}$/);
  assert.equal(token.id, parseSecret(secret)?.id);
  assert.deepEqual([token.name, token.display, token.permissions, token.allowSensitive, token.status], ["deploy job", `ssgsvc_${token.id}_…`, null, [], "active"]);
  assert.equal(token.expiresAt, new Date(T0 + 90 * 86_400_000).toISOString(), "default: 90 days");
  const list = await (await call(listTokensRoute, env, "/x", { browser: admin, params: { id } })).json();
  assert.equal(list.items.length, 1);
  assert.ok(!JSON.stringify(list).includes(secret.slice(20)), "never listed");
  assert.equal((await call(listTokensRoute, env, "/x", { browser: admin, params: { id: "nope" } })).status, 404);
  const year = await newToken(env, admin, id, { expiresInDays: 365, permissions: ["web-push:notify"] });
  assert.deepEqual([year.status, year.body.token.permissions], [201, ["web-push:notify"]]);
});

test("tokens: validation and the expiry cap", async () => {
  const { env, admin } = await setup();
  const id = await account(env, admin, ["web-push:notify"]);
  for (const bad of [{ expiresInDays: 0 }, { expiresInDays: 366 }, { expiresInDays: 1.5 }, { expiresInDays: "30" }, { name: 3 }, { permissions: "web-push:notify" }, { permissions: ["nope"] }, { allowSensitive: ["*", 1] }]) {
    const r = await newToken(env, admin, id, bad);
    assert.equal(r.status, 400, JSON.stringify(bad));
  }
  assert.equal((await newToken(env, admin, "nope", {})).status, 404);
  assert.equal((await newToken(env, admin, id, { permissions: null, allowSensitive: null })).status, 201, "null = not given");
  const capped = makeEnv({ AUTH_SERVICE_TOKEN_MAX_DAYS: "30" });
  const a2 = (await signedIn(capped, "a@example.com", "owner")).browser;
  const id2 = await account(capped, a2, []);
  assert.equal((await newToken(capped, a2, id2, { expiresInDays: 31 })).status, 400);
  assert.equal((await newToken(capped, a2, id2, {})).body.token.expiresAt, new Date(T0 + 30 * 86_400_000).toISOString());
});

test("escalation: a token cannot reach beyond its creator — by list or by the account's roles", async () => {
  const { env, owner, admin } = await setup();
  const id = await account(env, owner, ["web-push:notify", "republish-trigger:run"]);
  const noList = await newToken(env, admin, id, {});
  assert.deepEqual([noList.status, noList.body.error, noList.body.permissions], [403, "escalation", ["republish-trigger:run"]]);
  const wide = await newToken(env, admin, id, { permissions: ["republish-trigger:run"] });
  assert.deepEqual([wide.status, wide.body.permissions], [403, ["republish-trigger:run"]]);
  assert.equal((await newToken(env, admin, id, { permissions: ["web-push:notify"] })).status, 201, "narrowed to what the admin holds");
});

test("sensitive permissions: only an owner may allow them, and they then reach the token", async () => {
  const { env, owner, admin } = await setup();
  const id = await account(env, owner, ["newsletter:*"]);
  const refused = await newToken(env, admin, id, { permissions: ["web-push:notify"], allowSensitive: ["newsletter:erase"] });
  assert.deepEqual([refused.status, refused.body.error], [403, "sensitive_needs_owner"]);
  const ok = await newToken(env, owner, id, { permissions: ["newsletter:settings", "newsletter:erase"], allowSensitive: ["newsletter:erase"] });
  assert.equal(ok.status, 201);
  assert.deepEqual(ok.body.token.allowSensitive, ["newsletter:erase"]);
});

test("revoke: idempotent, keeps the first time; another account's token is not found", async () => {
  const { env, admin } = await setup();
  const clock = useClock();
  const id = await account(env, admin, []);
  const other = await account(env, admin, []);
  const tokenId = (parseSecret(await secretFor(env, admin, id)) as { id: string }).id;
  const del = (sa: string) => call(revoke, env, "/x", { method: "DELETE", browser: admin, params: { id: sa, tokenId } });
  assert.equal((await del(other)).status, 404);
  assert.equal((await del(id)).status, 200);
  clock.advance(60_000);
  assert.equal((await del(id)).status, 200);
  const t = (await (await call(listTokensRoute, env, "/x", { browser: admin, params: { id } })).json()).items[0];
  assert.deepEqual([t.status, t.revokedAt], ["revoked", new Date(T0).toISOString()]);
  await secretFor(env, admin, id, { expiresInDays: 1 });
  const { listTokens } = await import("../functions/api/auth/_service-tokens.ts");
  const later = await listTokens(env, id, T0 + 60_000 + 86_400_000);
  assert.deepEqual(later.map((i) => i.status).sort(), ["expired", "revoked"]);
});

test("the owner role on a service account: allowed by a * holder, never counted, and deletable", async () => {
  const { env, owner } = await setup();
  const id = await account(env, owner, []);
  const { addGrant } = await import("../functions/api/auth/_grants.ts");
  const g = await addGrant(env, roleId(env, "owner"), { userId: id }, "test", T0);
  assert.notEqual(g, "exists");
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM active_owners").get()?.n, 1, "only the person counts");
  assert.equal((await call(one.onRequestDelete, env, at(id), { method: "DELETE", browser: owner, params: { id } })).status, 200);
});

test("delete: refused, and nothing deleted, when the invariant is already broken", async () => {
  useClock();
  const env = makeEnv();
  const { ensureSchema } = await import("../functions/api/auth/_schema.ts");
  await ensureSchema(env);
  const { createServiceAccount, deleteServiceAccount } = await import("../functions/api/auth/_service-accounts.ts");
  const { addGrant } = await import("../functions/api/auth/_grants.ts");
  const sa = (await createServiceAccount(env, "lonely", "", "x", T0)) as { id: string };
  await addGrant(env, roleId(env, "owner"), { userId: sa.id }, "x", T0);
  assert.equal(await deleteServiceAccount(env, sa.id), "owner_required");
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM service_accounts").get()?.n, 1);
  env.AUTH_DB.state.failOn = /DELETE FROM grants/;
  await assert.rejects(deleteServiceAccount(env, sa.id), /forced failure/);
  env.AUTH_DB.state.failOn = /^INSERT INTO service_accounts/;
  await assert.rejects(createServiceAccount(env, "x", "", "x", T0), /forced failure/);
  env.AUTH_DB.state.failOn = /^UPDATE service_accounts/;
  const { updateServiceAccount } = await import("../functions/api/auth/_service-accounts.ts");
  await assert.rejects(updateServiceAccount(env, sa.id, {}, T0), /forced failure/);
});

test("routes: a broken invariant refuses the delete with 409; a token request needs a JSON object", async () => {
  useClock();
  const env = makeEnv();
  // Nobody holds owner yet. The caller holds `*` (through a role that is not
  // owner): deleting an account that holds `*` is refused to anyone who does
  // not (security review SR-04), and this test is about the 409 behind that.
  const admin = (await signedIn(env, "admin@example.com", ["*"])).browser;
  const id = await account(env, admin, []);
  const { addGrant } = await import("../functions/api/auth/_grants.ts");
  await addGrant(env, roleId(env, "owner"), { userId: id }, "x", T0);
  const res = await call(one.onRequestDelete, env, at(id), { method: "DELETE", browser: admin, params: { id } });
  assert.deepEqual([res.status, (await res.json()).error], [409, "owner_required"]);
  const { onRequestPost: createTokenRoute } = await import("../functions/api/auth/admin/service-accounts/[id]/tokens/index.ts");
  const bad = await call(createTokenRoute, env, "/x", { method: "POST", browser: admin, params: { id }, headers: { "content-type": "application/json" } });
  assert.equal(bad.status, 400);
});
