// GO-138: the edges of the new admin routes — bodies that are not JSON,
// step-up on ending sessions, unreadable audit details, keys whose age is
// unknown, an unreadable second-factor policy, the shell without a database.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestDelete as endOne } from "../functions/api/auth/admin/sessions/[id].ts";
import { onRequestPost as everyone } from "../functions/api/auth/admin/sessions/revoke-all.ts";
import { onRequestPost as setStatus } from "../functions/api/auth/admin/users/[id]/status.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestGet as shell } from "../functions/auth/admin/index.ts";
import { listEvents } from "../functions/api/auth/_admin-events.ts";
import { ROTATION_DAYS, keyAges } from "../functions/api/auth/_admin-keys.ts";
import { adminStatus } from "../functions/api/auth/_admin-status.ts";
import { prepare } from "../functions/api/auth/_context.ts";
import type { Ctx } from "../functions/api/auth/_context.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { T0, call, makeEnv, req, useClock } from "./fakes.ts";
import { enrolTotp, mfaEnv, signIn } from "./mfa.ts";
import { roleId, seedUser, signedIn } from "./seed.ts";

const DAY = 86_400_000;
const notJson = (browser: never, params = {}) => ({ method: "POST", browser, params, headers: { "content-type": "text/plain", "x-ssg-auth": "1" } });

test("bodies that are not JSON objects: 400", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin"]);
  const r1 = await call(everyone, env, "/api/auth/admin/sessions/revoke-all", notJson(admin.browser as never));
  const r2 = await call(setStatus, env, "/api/auth/admin/users/x/status", notJson(admin.browser as never, { id: "x" }));
  assert.deepEqual([r1.status, r2.status], [400, 400]);
});

test("step-up: ending another account's session, and everyone's", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "boss@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "boss@example.com");
  await enrolTotp(env, browser, clock.now);
  await seedUser(env, "eva@example.com");
  await signIn(env, "eva@example.com");
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  const sid = (env.AUTH_DB.sqlite.prepare("SELECT s.id FROM sessions s JOIN users u ON u.id = s.user_id WHERE u.email = 'eva@example.com'").get() as { id: string }).id;
  const r = await call(endOne, env, `/api/auth/admin/sessions/${sid}`, { method: "DELETE", browser, params: { id: sid } });
  assert.deepEqual([r.status, (await r.json()).error], [403, "step_up_required"]);
  const all = await call(everyone, env, "/api/auth/admin/sessions/revoke-all", { json: { confirm: "revoke-all" }, browser });
  assert.deepEqual([all.status, (await all.json()).error], [403, "step_up_required"]);
});

test("audit details that are not a JSON object read as none", async () => {
  useClock();
  const env = makeEnv();
  await ensureSchema(env);
  const put = env.AUTH_DB.sqlite.prepare("INSERT INTO auth_events (id, at, kind, detail) VALUES (?, ?, 'signin', ?)");
  put.run("a", new Date(T0).toISOString(), "not json");
  put.run("b", new Date(T0 + 1).toISOString(), "[1]");
  put.run("c", new Date(T0 + 2).toISOString(), '{"by": 5}');
  const { items } = await listEvents(env, { kind: "", user: "", from: "", to: "", cursor: "" }, 10);
  assert.deepEqual(items.map((e) => [e.detail, e.by]), [[{ by: 5 }, null], [{}, null], [{}, null]]);
});

test("key ages: unknown when never recorded; only the current key is ever 'rotate'", async () => {
  const env = makeEnv();
  await ensureSchema(env);
  const put = env.AUTH_DB.sqlite.prepare("INSERT INTO signing_keys (kid, first_used_at) VALUES (?, ?)");
  put.run("old-current", new Date(T0 - (ROTATION_DAYS + 1) * DAY).toISOString());
  put.run("old-previous", new Date(T0 - 900 * DAY).toISOString());
  const ages = await keyAges(env, ["old-current", "old-previous", "never"], "old-current", T0);
  assert.deepEqual(ages.map((k) => [k.kid, k.current, k.ageDays, k.rotate]), [["old-current", true, ROTATION_DAYS + 1, true], ["old-previous", false, 900, false], ["never", false, null, false]]);
  const young = await keyAges(env, ["never"], "never", T0);
  assert.deepEqual([young[0].ageDays, young[0].rotate], [null, false]);
});

test("status: an unreadable AUTH_MFA_ROLES, modules sorted whatever the order they came in", async () => {
  useClock();
  const env = mfaEnv({ AUTH_MFA_ROLES: "nonsense", AUTH_KNOWN_PERMISSIONS: '[{"id":"zeta","permissions":["zeta:run"]},{"id":"alpha","permissions":["alpha:run"]}]' });
  const ctx = (await prepare(req("/api/auth/admin/status"), env)) as Ctx;
  const errors = console.error;
  console.error = () => {};
  const s = await adminStatus(ctx);
  console.error = errors;
  assert.equal(s.mfa.policy, "invalid");
  assert.deepEqual(s.modules.map((m) => m.id), ["alpha", "auth", "zeta"]);
});

test("the shell without a database: the worker's own 503", async () => {
  const res = await call(shell, { AUTH_SIGNING_KEY: makeEnv().AUTH_SIGNING_KEY } as never, "/auth/admin/");
  assert.equal(res.status, 503);
});
