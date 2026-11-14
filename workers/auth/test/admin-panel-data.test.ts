// GO-138: what the panel reads besides accounts — the audit log (filters,
// paging, CSV with the formula guard, retention purge, the administrative
// events), everyone's sessions (paging, ending one, break-glass), the
// signing keys' age and the Status answer.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as events } from "../functions/api/auth/admin/events/index.ts";
import { onRequestGet as exportCsv } from "../functions/api/auth/admin/events/export.ts";
import { onRequestGet as sessions } from "../functions/api/auth/admin/sessions/index.ts";
import { onRequestDelete as endOne } from "../functions/api/auth/admin/sessions/[id].ts";
import { onRequestPost as everyone } from "../functions/api/auth/admin/sessions/revoke-all.ts";
import { onRequestGet as status } from "../functions/api/auth/admin/status.ts";
import { onRequestPost as grant } from "../functions/api/auth/admin/grants/index.ts";
import { onRequestDelete as ungrant } from "../functions/api/auth/admin/grants/[id].ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { csvCell, eventsCsv } from "../functions/api/auth/_admin-events.ts";
import { emitEvent } from "../functions/api/auth/_audit.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { Browser, T0, call, makeEnv, newKeyPair, useClock } from "./fakes.ts";
import { enrolTotp, mfaEnv, signIn } from "./mfa.ts";
import { seedUser, signedIn } from "./seed.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";

const DAY = 86_400_000;
const body = async (r: Response) => [r.status, await r.json()] as const;

test("audit log: administrative events, filters by kind, account and dates, paging, CSV with the formula guard", async () => {
  const clock = useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:read"]);
  const eva = await signedIn(env, "=hyperlink(1)@example.com", ["comments:read"]);
  const role = (await createRole(env, { name: "reader", description: "", permissions: ["newsletter:read"] }, T0)) as Role;
  const g = await (await call(grant, env, "/api/auth/admin/grants", { json: { roleId: role.id, userId: eva.id }, browser: admin.browser })).json();
  clock.advance(1000);
  await call(ungrant, env, `/api/auth/admin/grants/${g.grant.id}`, { method: "DELETE", browser: admin.browser, params: { id: g.grant.id } });
  for (let i = 0; i < 105; i++) await emitEvent(env, { kind: "signin", userId: admin.id, method: "password" }, T0 - 2 * DAY + i);
  await emitEvent(env, { kind: "mfa_reset", userId: eva.id }, T0 - DAY);
  const q = async (s: string) => body(await call(events, env, `/api/auth/admin/events${s}`, { browser: admin.browser }));
  const [, granted] = await q("?kind=grant_added");
  assert.equal(granted.items.length, 1);
  assert.deepEqual([granted.items[0].email, granted.items[0].by, granted.items[0].byEmail, granted.items[0].detail.role], ["=hyperlink(1)@example.com", admin.id, "admin@example.com", "reader"]);
  assert.equal(granted.retentionDays, 365);
  const [, aboutEva] = await q(`?user=${eva.id}`);
  assert.deepEqual(aboutEva.items.map((e: { kind: string }) => e.kind).filter((k: string) => k.startsWith("grant")), ["grant_removed", "grant_added"]);
  const [, byAdmin] = await q(`?user=${admin.id}&kind=grant_removed`);
  assert.equal(byAdmin.items.length, 1, "the administrator who acted matches too");
  const [, first] = await q("?kind=signin");
  assert.equal(first.items.length, 100);
  const [, second] = await q(`?kind=signin&cursor=${encodeURIComponent(first.nextCursor)}`);
  assert.equal(first.items.length + second.items.length, 107, "105 + the two sign-ins of the setup");
  assert.equal(second.nextCursor, null);
  const day = new Date(T0 - DAY).toISOString().slice(0, 10);
  const [, oneDay] = await q(`?from=${day}&to=${new Date(T0).toISOString().slice(0, 10)}`);
  assert.deepEqual(oneDay.items.map((e: { kind: string }) => e.kind), ["mfa_reset"]);
  for (const bad of ["?kind=Bad-Kind", "?from=yesterday", "?to=2026-13-45"]) assert.equal((await q(bad))[0], 400, bad);
  const csv = await call(exportCsv, env, "/api/auth/admin/events/export?kind=grant_added", { browser: admin.browser });
  assert.equal(csv.headers.get("content-type"), "text/csv; charset=utf-8");
  assert.match(csv.headers.get("content-disposition") ?? "", /^attachment; filename="auth-audit-\d{4}-\d{2}-\d{2}\.csv"$/);
  const bytes = new Uint8Array(await csv.arrayBuffer());
  assert.deepEqual([...bytes.slice(0, 3)], [0xef, 0xbb, 0xbf], "a byte-order mark");
  const text = new TextDecoder().decode(bytes);
  assert.ok(text.startsWith("at,kind,account_id,account_email,by_id,by_email,method,detail\r\n"));
  assert.ok(text.includes(",'=hyperlink(1)@example.com,"), "a formula cell is text");
  assert.equal((await call(exportCsv, env, "/api/auth/admin/events/export?from=x", { browser: admin.browser })).status, 400);
});

test("csv: the guard and the quoting", () => {
  assert.deepEqual(["=1", "+1", "-1", "@a", "\tx", "\rx", "a,b", 'say "hi"', " pad", null, 5].map(csvCell), ["'=1", "'+1", "'-1", "'@a", "'\tx", "\"'\rx\"", '"a,b"', '"say ""hi"""', '" pad"', "", "5"]);
  assert.equal(eventsCsv([]), "\ufeffat,kind,account_id,account_email,by_id,by_email,method,detail\r\n");
});

test("retention: every write removes events older than AUTH_AUDIT_RETENTION_DAYS", async () => {
  useClock();
  const env = makeEnv({ AUTH_AUDIT_RETENTION_DAYS: "30" });
  await signedIn(env, "a@example.com", ["auth:admin"]);
  await emitEvent(env, { kind: "signin" }, T0 - 31 * DAY);
  await emitEvent(env, { kind: "signin" }, T0 - 29 * DAY);
  await emitEvent(env, { kind: "password_set" }, T0);
  const ats = env.AUTH_DB.sqlite.prepare("SELECT at FROM auth_events ORDER BY at").all().map((r) => (r as { at: string }).at);
  assert.ok(ats.every((a) => Date.parse(a) >= T0 - 30 * DAY), ats.join());
  assert.ok(ats.includes(new Date(T0 - 29 * DAY).toISOString()));
});

test("sessions: everyone's live ones with method and factor, paged; end one, end everyone but me", async () => {
  const clock = useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin"]);
  const eva = await signedIn(env, "eva@example.com", ["newsletter:read"]);
  for (let i = 0; i < 3; i++) {
    clock.advance(1000);
    await signedIn(env, `p${i}@example.com`, ["comments:read"]);
  }
  const [, all] = await body(await call(sessions, env, "/api/auth/admin/sessions?limit=2", { browser: admin.browser }));
  assert.equal(all.items.length, 2);
  assert.deepEqual(Object.keys(all.items[0]).sort(), ["createdAt", "email", "expiresAt", "id", "lastUsedAt", "method", "name", "secondFactor", "userId"]);
  const [, rest] = await body(await call(sessions, env, `/api/auth/admin/sessions?limit=200&cursor=${encodeURIComponent(all.nextCursor)}`, { browser: admin.browser }));
  assert.equal(all.items.length + rest.items.length, 5);
  assert.equal(rest.nextCursor, null);
  assert.equal((await call(sessions, env, "/api/auth/admin/sessions?limit=0", { browser: admin.browser })).status, 400);
  const [, evas] = await body(await call(sessions, env, `/api/auth/admin/sessions?userId=${eva.id}&cursor=nonsense`, { browser: admin.browser }));
  assert.equal(evas.items.length, 1);
  const sid = evas.items[0].id;
  assert.deepEqual(await body(await call(endOne, env, `/api/auth/admin/sessions/${sid}`, { method: "DELETE", browser: admin.browser, params: { id: sid } })), [200, { ok: true, current: false }]);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: eva.browser })).status, 401, "its next refresh fails");
  assert.equal((await call(endOne, env, `/api/auth/admin/sessions/${sid}`, { method: "DELETE", browser: admin.browser, params: { id: sid } })).status, 404);
  assert.equal((await call(everyone, env, "/api/auth/admin/sessions/revoke-all", { json: {}, browser: admin.browser })).status, 400);
  assert.deepEqual(await body(await call(everyone, env, "/api/auth/admin/sessions/revoke-all", { json: { confirm: "revoke-all" }, browser: admin.browser })), [200, { ok: true, revoked: 3 }]);
  const [, left] = await body(await call(sessions, env, "/api/auth/admin/sessions", { browser: admin.browser }));
  assert.deepEqual(left.items.map((s: { email: string }) => s.email), ["admin@example.com"]);
  const own = left.items[0].id;
  assert.deepEqual(await body(await call(endOne, env, `/api/auth/admin/sessions/${own}`, { method: "DELETE", browser: admin.browser, params: { id: own } })), [200, { ok: true, current: true }]);
});

test("status: keys and their age, freshness, hashing, methods, second factor and who lacks it, mail, modules", async () => {
  const clock = useClock();
  // The admin's own role is exempted from the second factor, so its token holds auth:admin.
  const env = mfaEnv({ AUTH_RESEND_API_KEY: "re_x", AUTH_KNOWN_PERMISSIONS: '[{"id":"comments","permissions":["comments:read"]}, 5]', AUTH_AUDIT_RETENTION_DAYS: "90", AUTH_MFA_ROLES: '{"panel-admin": false}' });
  const adminId = await seedUser(env, "admin@example.com");
  const panelAdmin = (await createRole(env, { name: "panel-admin", description: "", permissions: ["auth:admin"] }, T0)) as Role;
  await addGrant(env, panelAdmin.id, { userId: adminId }, "t", T0);
  const admin = await signIn(env, "admin@example.com");
  await signedIn(env, "sender@example.com", ["newsletter:send"]);
  const ownerB = await signedIn(env, "keeper@example.com", ["comments:read"]);
  await enrolTotp(env, ownerB.browser, clock.now);
  let viewer = admin.browser;
  const read = async (e = env) => (await call(status, e, "/api/auth/admin/status", { browser: viewer })).json();
  const s = await read();
  assert.equal(s.keys.length, 1);
  assert.deepEqual([s.keys[0].current, s.keys[0].ageDays, s.keys[0].rotate, s.rotationDays], [true, 0, false, 365]);
  assert.deepEqual(s.tokens, { accessMinutes: 10, stalenessSeconds: 630, sessionDays: 14, serviceAccessMinutes: 10 });
  assert.deepEqual(s.hashing, { scheme: "pbkdf2-sha256", iterations: 10000, passwordMin: 12, passwordMax: 256 });
  assert.deepEqual(s.signIn, { methods: ["password"], password: true });
  assert.equal(s.access, "absent");
  assert.equal(s.mfa.state, "ready");
  assert.equal(s.mfa.policy, "custom");
  assert.ok(s.mfa.rolesNeedingFactor.includes("owner"));
  assert.equal(s.mfa.accountsMissingFactor, 1, "the sender, not the admin (exempt) or the keeper (has one)");
  assert.deepEqual(s.mail, { state: "broken", problem: "AUTH_MAIL_FROM is not set: a transport needs a verified sender address" });
  assert.deepEqual(s.audit, { retentionDays: 90 });
  assert.deepEqual(s.modules, [{ id: "auth", permissions: 2 }, { id: "comments", permissions: 1 }]);
  assert.equal(s.registryProblems.length, 1);
  const ready = await read({ ...env, AUTH_MAIL_FROM: "a@example.com", AUTH_MFA_KEY: undefined } as never);
  assert.deepEqual([ready.mail, ready.mfa.state, ready.mfa.accountsMissingFactor], [{ state: "ready", transport: "resend" }, "off", 0]);
  const none = await read({ ...env, AUTH_RESEND_API_KEY: undefined } as never);
  assert.equal(none.mail.state, "none");
  // A year on, with a rotated key: the old one is still published, the new one young.
  clock.advance(366 * DAY);
  viewer = (await signIn(env, "admin@example.com")).browser; // the session (14 days) is long gone
  assert.equal((await read()).keys[0].rotate, true, "older than the advice");
  const fresh = await newKeyPair();
  const rotated = { ...env, AUTH_SIGNING_KEY: fresh.priv, AUTH_PREVIOUS_PUBLIC_KEY: JSON.parse(JSON.stringify(env.AUTH_SIGNING_KEY)) };
  const b = new Browser();
  const { onRequestPost: login } = await import("../functions/api/auth/login.ts");
  await call(login, rotated, "/api/auth/login", { json: { email: "admin@example.com", password: "correct horse battery" }, browser: b });
  const answer = await call(status, rotated, "/api/auth/admin/status", { browser: b });
  const r = await answer.json();
  assert.equal(answer.status, 200, JSON.stringify(r));
  assert.deepEqual(r.keys.map((k: { current: boolean; ageDays: number }) => [k.current, k.ageDays]), [[true, 0], [false, 366]]);
  // The new key id in use is in the audit log (the first key's line is past the 90-day retention).
  const seen = env.AUTH_DB.sqlite.prepare("SELECT detail FROM auth_events WHERE kind = 'signing_key_seen'").all() as { detail: string }[];
  assert.deepEqual(seen.map((e) => JSON.parse(e.detail).kid), [r.keys[0].kid]);
});
