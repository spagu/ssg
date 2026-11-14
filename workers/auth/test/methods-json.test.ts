// GO-151, the routes the pages use as plain forms, called with JSON — and the
// corners of their forms: an unknown purpose, no body, no database, hashing
// refused, a password set by someone else meanwhile, a sign-in grown old.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as setPassword } from "../functions/api/auth/methods/set-password.ts";
import { onRequestDelete as removeJson, onRequestPost as removeForm } from "../functions/api/auth/methods/[id].ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { PASSWORD, call, counting, useClock } from "./fakes.ts";
import { pageOf } from "./go134.ts";
import { loggedIn } from "./seed.ts";
import { before, callbackGet, callbackPost, events, providerEnv, providerUser, start, startAt } from "./go151.ts";

test("routes in JSON, and the corners of their forms", async () => {
  const clock = useClock();
  const env = providerEnv();
  const { browser, id } = await loggedIn(env);
  // start: JSON gives the URL; an unknown purpose; an unreadable body; no DB.
  const ok = await call(start, env, "/api/auth/oauth/google/start", { json: { purpose: "" }, browser, params: { provider: "google" } });
  assert.equal(ok.status, 200);
  assert.match((await ok.json()).authorizationUrl, /^https:\/\/accounts\.google\.com\/o\/oauth2\/v2\/auth\?client_id=/);
  const bad = await call(start, env, "/api/auth/oauth/google/start", { json: { purpose: "steal" }, params: { provider: "google" } });
  assert.deepEqual([bad.status, await bad.json()], [400, { error: "invalid_request" }]);
  assert.equal((await startAt(env, browser, "google", { purpose: "steal" })).headers.get("location"), "/auth/login?error=invalid_request");
  const raw = await call(start, env, "/api/auth/oauth/google/start", { method: "POST", headers: { "content-type": "text/plain" }, params: { provider: "google" } });
  assert.equal(raw.status, 200, "no body = sign in");
  const bounce = await call(callbackPost, env, "/api/auth/oauth/apple/callback", { method: "POST", csrf: false, params: { provider: "apple" } });
  assert.equal(bounce.headers.get("location"), "/api/auth/oauth/apple/callback?");
  const noDb = await call(callbackGet, { ...env, AUTH_DB: undefined } as never, "/api/auth/oauth/google/callback", { params: { provider: "google" } });
  assert.equal(noDb.status, 503);
  // methods: JSON success, JSON reauth, hashing refused.
  await call(removeForm, env, "/api/auth/methods/password", { json: {}, browser, params: { id: "password" } });
  await providerUser(env, "x@example.com", "github", "9");
  env.AUTH_DB.sqlite.exec(`INSERT INTO identities (id, provider, subject, user_id, linked_at) VALUES ('mine', 'github', '10', '${id}', '2026-11-13T10:00:00.000Z')`);
  const del = await call(removeJson, env, "/api/auth/methods/password", { method: "DELETE", browser, params: { id: "password" } });
  assert.deepEqual([del.status, await del.json()], [200, { ok: true }]);
  assert.match((await pageOf(account, env, "/auth/account", browser)).html, /<td data-label="Details">—<\/td>/, "an identity without an address");
  const k = counting(5000);
  try {
    const r = await call(setPassword, env, "/api/auth/methods/set-password", { json: { password: PASSWORD }, browser });
    assert.deepEqual([r.status, (await r.json()).error], [503, "hashing_unavailable"]);
  } finally {
    k.restore();
  }
  before(env, /^UPDATE users SET password_hash = \?, must_change_password = 0/, () => env.AUTH_DB.sqlite.exec(`UPDATE users SET password_hash = 'x' WHERE id = '${id}'`));
  const race = await call(setPassword, env, "/api/auth/methods/set-password", { json: { password: PASSWORD }, browser });
  assert.deepEqual([race.status, await race.json()], [409, { error: "password_exists" }]);
  const done = await call(setPassword, env, "/api/auth/methods/set-password", { json: { password: PASSWORD }, browser });
  assert.equal(done.status, 409);
  clock.advance(11 * 60_000);
  const old = await call(removeJson, env, "/api/auth/methods/mine", { method: "DELETE", browser, params: { id: "mine" } });
  assert.deepEqual([old.status, await old.json()], [403, { error: "reauth_required", minutes: 10 }]);
  assert.equal(events(env, "method_unlinked").length, 1);
});
