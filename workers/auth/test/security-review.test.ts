// Regression tests for the findings of the security review
// (docs/SECURITY-REVIEW.md). Each test names its finding (SR-nn); each was
// written against the vulnerable code first and failed there.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import { createInvitation } from "../functions/api/auth/_invites.ts";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { parseSecret } from "../functions/api/auth/_service-tokens.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as logout } from "../functions/api/auth/logout.ts";
import { onRequestPost as password } from "../functions/api/auth/password.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as breakGlass } from "../functions/api/auth/mfa/break-glass.ts";
import { onRequestPost as totpStart } from "../functions/api/auth/mfa/totp/start.ts";
import { onRequestDelete as removeFactor } from "../functions/api/auth/mfa/factors/[id].ts";
import { onRequestDelete as deleteGrant } from "../functions/api/auth/admin/grants/[id].ts";
import { onRequestDelete as deleteGroup } from "../functions/api/auth/admin/groups/[id]/index.ts";
import { onRequestDelete as removeMember } from "../functions/api/auth/admin/groups/[id]/members/[userId].ts";
import { onRequestDelete as deleteRole, onRequestPut as putRole } from "../functions/api/auth/admin/roles/[id].ts";
import { onRequestPost as revokeSessions } from "../functions/api/auth/admin/users/[id]/revoke-sessions.ts";
import { onRequestPost as setStatus } from "../functions/api/auth/admin/users/[id]/status.ts";
import { onRequestPost as resetLink } from "../functions/api/auth/admin/users/[id]/reset-link.ts";
import { onRequestDelete as endSession } from "../functions/api/auth/admin/sessions/[id].ts";
import { onRequestPost as resend } from "../functions/api/auth/admin/invitations/[id]/resend.ts";
import { onRequestPost as mapGroup } from "../functions/api/auth/admin/access-groups/index.ts";
import { onRequestDelete as unmapGroup } from "../functions/api/auth/admin/access-groups/[id].ts";
import { onRequestDelete as deleteService, onRequestPut as putService } from "../functions/api/auth/admin/service-accounts/[id]/index.ts";
import { onRequestDelete as revokeToken } from "../functions/api/auth/admin/service-accounts/[id]/tokens/[tokenId].ts";
import { Browser, PASSWORD, T0, call, makeEnv, useClock } from "./fakes.ts";
import { code, enrolTotp, mfaEnv, signIn, stepUp, verify } from "./mfa.ts";
import { grantPerms, roleId, seedUser, signedIn } from "./seed.ts";
import { account, secretFor, swap } from "./service.ts";
import { invite } from "./go134.ts";
import { errorOf, events, fakeProviders, providerEnv, roundTrip, startAt } from "./go151.ts";

const errorOfRes = async (r: Response): Promise<[number, string]> => [r.status, ((await r.clone().json()) as { error: string }).error];
const statuses = (list: Response[]) => list.reduce<Record<number, number>>((m, r) => ((m[r.status] = (m[r.status] ?? 0) + 1), m), {});
const sql = (env: unknown) => (env as { AUTH_DB: { sqlite: { prepare(s: string): { get(...a: unknown[]): unknown; all(...a: unknown[]): unknown[] } } } }).AUTH_DB.sqlite;

// ---- SR-01: a rotated refresh token is not a session ----

test("SR-01: an old, rotated refresh token no longer authenticates (step-up, factor changes) and revokes the session", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await seedUser(env, "ada@example.com");
  const { browser } = await signIn(env, "ada@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  const r1 = browser.jar.get(REFRESH_COOKIE) as string;
  clock.advance(60_000);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser })).status, 200);
  assert.notEqual(browser.jar.get(REFRESH_COOKIE), r1);
  clock.advance(3600_000);
  const thief = new Browser("192.0.2.9");
  thief.jar.set(REFRESH_COOKIE, r1);
  const up = await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser: thief });
  assert.equal(up.status, 401);
  assert.equal(thief.jar.has(ACCESS_COOKIE), false);
  assert.equal((await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser: thief })).status, 401);
  // Theft detected: the session is over for both parties, as /refresh would do.
  const row = sql(env).prepare("SELECT revoked_at, revoke_reason FROM sessions").get() as { revoked_at: string | null; revoke_reason: string };
  assert.ok(row.revoked_at);
  assert.equal(row.revoke_reason, "refresh_reuse");
});

test("SR-01: inside the two-tab grace window the previous refresh token still finds the session (logout works)", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "ada@example.com", []);
  const r1 = browser.jar.get(REFRESH_COOKIE) as string;
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser })).status, 200);
  clock.advance(5_000);
  const other = new Browser();
  other.jar.set(REFRESH_COOKIE, r1);
  assert.equal((await call(logout, env, "/api/auth/logout", { json: {}, browser: other })).status, 200);
  const row = sql(env).prepare("SELECT revoke_reason FROM sessions").get() as { revoke_reason: string };
  assert.equal(row.revoke_reason, "logout");
});

// ---- SR-02: lockouts hold under parallel requests ----

test("SR-02: parallel wrong second-factor codes are counted before they are checked", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await seedUser(env, "ada@example.com");
  const { browser } = await signIn(env, "ada@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  const pending = await signIn(env, "ada@example.com");
  const right = await code(secret, clock.now);
  const wrong = (i: number) => String((Number(right) + 1 + i) % 1_000_000).padStart(6, "0");
  const res = await Promise.all(
    Array.from({ length: 40 }, (_, i) =>
      call(verify, env, "/api/auth/mfa/verify", { json: { code: wrong(i) }, browser: new Browser(`198.51.100.${i + 1}`), headers: { cookie: pending.browser.cookieHeader() } }),
    ),
  );
  const n = statuses(res);
  assert.ok((n[401] ?? 0) <= 5, `at most 5 codes checked, got ${JSON.stringify(n)}`);
  assert.equal((n[401] ?? 0) + (n[429] ?? 0), 40);
  const late = await call(verify, env, "/api/auth/mfa/verify", { json: { code: right }, browser: new Browser("198.51.100.200"), headers: { cookie: pending.browser.cookieHeader() } });
  assert.equal(late.status, 429);
});

test("SR-02: parallel wrong passwords from many addresses are counted before they are checked", async () => {
  useClock();
  const env = makeEnv();
  await seedUser(env, "ada@example.com");
  const res = await Promise.all(
    Array.from({ length: 30 }, (_, i) => call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: `wrong-${i}-xxxxxxxx` }, browser: new Browser(`198.51.100.${i + 1}`) })),
  );
  const n = statuses(res);
  assert.ok((n[401] ?? 0) <= 5, `at most 5 passwords checked, got ${JSON.stringify(n)}`);
  assert.equal((n[401] ?? 0) + (n[429] ?? 0), 30);
});

test("SR-02: a success gives back the attempt it reserved — the IP counter is not grown by people who sign in", async () => {
  useClock();
  const env = makeEnv({ AUTH_LOCK_IP_FAILURES: "3" });
  await seedUser(env, "ada@example.com");
  const b = new Browser("198.51.100.77");
  for (let i = 0; i < 6; i++) {
    const r = await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: new Browser(b.ip) });
    assert.equal(r.status, 200, `sign-in ${i}`);
  }
  // The 3rd wrong password locks the IP; the reserved-and-refunded successes never counted.
  for (let i = 0; i < 2; i++) assert.equal((await call(login, env, "/api/auth/login", { json: { email: "x@example.com", password: "wrong-password-1" }, browser: new Browser(b.ip) })).status, 401);
  assert.equal((await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: new Browser(b.ip) })).status, 200);
});

test("SR-02: parallel wrong current passwords on the password route stop at the account lock", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "ada@example.com", []);
  const res = await Promise.all(
    Array.from({ length: 20 }, (_, i) => call(password, env, "/api/auth/password", { json: { current: `nope-${i}-xxxxxxxx`, password: "a brand new passphrase" }, browser })),
  );
  const n = statuses(res);
  assert.ok((n[401] ?? 0) <= 5, JSON.stringify(n));
});

// ---- SR-03: break-glass token strength and per-account lock ----

async function ownerPending(env: ReturnType<typeof mfaEnv>, clock: { now: number }) {
  const id = await seedUser(env, "boss@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "boss@example.com");
  await enrolTotp(env, browser, clock.now);
  return (await signIn(env, "boss@example.com")).browser;
}

test("SR-03: a break-glass token shorter than 32 characters is treated as unset", async () => {
  const clock = useClock();
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: "short1" });
  const pending = await ownerPending(env, clock);
  const r = await call(breakGlass, env, "/api/auth/mfa/break-glass", { json: { token: "short1" }, browser: pending });
  assert.deepEqual(await errorOfRes(r), [404, "not_found"]);
});

test("SR-03: wrong break-glass tokens from many addresses lock the account's break-glass", async () => {
  const clock = useClock();
  const token = "break-glass-0123456789-abcdefghijklmnop";
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: token });
  const pending = await ownerPending(env, clock);
  const res = await Promise.all(
    Array.from({ length: 12 }, (_, i) =>
      call(breakGlass, env, "/api/auth/mfa/break-glass", { json: { token: `wrong-${i}` }, browser: new Browser(`198.51.100.${i + 1}`), headers: { cookie: pending.cookieHeader() } }),
    ),
  );
  const n = statuses(res);
  assert.ok((n[401] ?? 0) <= 5, JSON.stringify(n));
  const right = await call(breakGlass, env, "/api/auth/mfa/break-glass", { json: { token }, browser: new Browser("198.51.100.99"), headers: { cookie: pending.cookieHeader() } });
  assert.equal(right.status, 429);
});

// ---- SR-04: taking permissions away follows the rule for handing them out ----

async function ownersAndAdmin() {
  useClock();
  const env = makeEnv();
  const o1 = await seedUser(env, "o1@example.com");
  const o2 = await seedUser(env, "o2@example.com");
  const owner = roleId(env as never, "owner");
  await addGrant(env, owner, { userId: o1 }, "t", T0);
  const g2 = (await addGrant(env, owner, { userId: o2 }, "t", T0)) as { id: string };
  const low = await signedIn(env, "low@example.com", ["auth:admin"]);
  return { env, o1, o2, g2, owner, low };
}

test("SR-04: auth:admin cannot take a co-owner's owner grant away", async () => {
  const { env, g2, low } = await ownersAndAdmin();
  const r = await call(deleteGrant, env, `/api/auth/admin/grants/${g2.id}`, { method: "DELETE", browser: low.browser, params: { id: g2.id } });
  assert.deepEqual(await errorOfRes(r), [403, "escalation"]);
  assert.ok(sql(env).prepare("SELECT 1 AS x FROM grants WHERE id = ?").get(g2.id));
});

test("SR-04: a grant that is gone answers 404, never 500", async () => {
  const { env, low } = await ownersAndAdmin();
  const r = await call(deleteGrant, env, "/api/auth/admin/grants/missing", { method: "DELETE", browser: low.browser, params: { id: "missing" } });
  assert.deepEqual(await errorOfRes(r), [404, "grant_not_found"]);
});

test("SR-04: auth:admin cannot remove a member from, or delete, a group holding owner", async () => {
  const { env, o2, owner, low } = await ownersAndAdmin();
  const g = (await createGroup(env, "Owners", "", T0)) as { id: string };
  await addGrant(env, owner, { groupId: g.id }, "t", T0);
  await addMember(env, g.id, o2, "t", T0);
  const r1 = await call(removeMember, env, "/x", { method: "DELETE", browser: low.browser, params: { id: g.id, userId: o2 } });
  assert.deepEqual(await errorOfRes(r1), [403, "escalation"]);
  const r2 = await call(deleteGroup, env, "/x", { method: "DELETE", browser: low.browser, params: { id: g.id } });
  assert.deepEqual(await errorOfRes(r2), [403, "escalation"]);
});

test("SR-04: auth:admin cannot delete a role holding auth:*, nor take auth:* out of a role", async () => {
  const { env, low } = await ownersAndAdmin();
  const holder = await seedUser(env, "h@example.com");
  const big = await grantPerms(env, holder, ["auth:*", "newsletter:send"]);
  const r1 = await call(deleteRole, env, "/x", { method: "DELETE", browser: low.browser, params: { id: big } });
  assert.deepEqual(await errorOfRes(r1), [403, "escalation"]);
  const r2 = await call(putRole, env, "/x", { json: { permissions: ["newsletter:send"] }, method: "PUT", browser: low.browser, params: { id: big } });
  assert.deepEqual(await errorOfRes(r2), [403, "escalation"]);
  // Taking out a permission that is not an administrator's wildcard stays allowed ("narrowing is fine").
  const r3 = await call(putRole, env, "/x", { json: { permissions: ["auth:*"] }, method: "PUT", browser: low.browser, params: { id: big } });
  assert.equal(r3.status, 200);
});

test("SR-04: auth:admin cannot end the sessions of, or block, an owner", async () => {
  const { env, o2, low } = await ownersAndAdmin();
  const o2b = new Browser();
  await call(login, env, "/api/auth/login", { json: { email: "o2@example.com", password: PASSWORD }, browser: o2b });
  const r1 = await call(revokeSessions, env, "/x", { method: "POST", browser: low.browser, params: { id: o2 } });
  assert.deepEqual(await errorOfRes(r1), [403, "escalation"]);
  const sid = (sql(env).prepare("SELECT id FROM sessions WHERE user_id = ?").get(o2) as { id: string }).id;
  const r2 = await call(endSession, env, "/x", { method: "DELETE", browser: low.browser, params: { id: sid } });
  assert.deepEqual(await errorOfRes(r2), [403, "escalation"]);
  const r3 = await call(setStatus, env, "/x", { json: { status: "blocked" }, browser: low.browser, params: { id: o2 } });
  assert.deepEqual(await errorOfRes(r3), [403, "escalation"]);
});

test("SR-04: auth:admin cannot disable, delete or revoke tokens of a service account holding auth:*", async () => {
  const { env, o1, low } = await ownersAndAdmin();
  const boss = new Browser();
  await call(login, env, "/api/auth/login", { json: { email: "o1@example.com", password: PASSWORD }, browser: boss });
  void o1;
  const sa = await account(env, boss, ["auth:*"]);
  const secret = await secretFor(env, boss, sa);
  const tokenId = (parseSecret(secret) as { id: string }).id;
  assert.deepEqual(await errorOfRes(await call(putService, env, "/x", { json: { status: "disabled" }, method: "PUT", browser: low.browser, params: { id: sa } })), [403, "escalation"]);
  assert.deepEqual(await errorOfRes(await call(deleteService, env, "/x", { method: "DELETE", browser: low.browser, params: { id: sa } })), [403, "escalation"]);
  assert.deepEqual(await errorOfRes(await call(revokeToken, env, "/x", { method: "DELETE", browser: low.browser, params: { id: sa, tokenId } })), [403, "escalation"]);
  assert.equal((await swap(env, `Bearer ${secret}`)).status, 200);
});

// ---- SR-05: step-up on reset links, invitation resends and Access mappings ----

async function staleOwner() {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "boss@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "boss@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  return { clock, env, id, browser, secret };
}

test("SR-05: a reset link for another account and an owner invitation resend need a recent second factor", async () => {
  const { clock, env, id, browser, secret } = await staleOwner();
  const owner2 = await seedUser(env, "owner2@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: owner2 }, "t", T0);
  const r1 = await call(resetLink, env, "/x", { json: { delivery: "link" }, browser, params: { id: owner2 } });
  assert.deepEqual(await errorOfRes(r1), [403, "step_up_required"]);
  const { invitation } = await createInvitation(env, { email: "y@example.com", name: "", roleIds: [roleId(env as never, "owner")], groupIds: [], lang: "en", invitedBy: id }, clock.now);
  const r2 = await call(resend, env, "/x", { json: { delivery: "link" }, browser, params: { id: invitation.id } });
  assert.deepEqual(await errorOfRes(r2), [403, "step_up_required"]);
  // After a step-up both go through.
  assert.equal((await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser })).status, 200);
  assert.equal((await call(resetLink, env, "/x", { json: { delivery: "link" }, browser, params: { id: owner2 } })).status, 201);
  assert.equal((await call(resend, env, "/x", { json: { delivery: "link" }, browser, params: { id: invitation.id } })).status, 200);
});

test("SR-05: a reset link for one's own account needs no step-up", async () => {
  const { env, id, browser } = await staleOwner();
  assert.equal((await call(resetLink, env, "/x", { json: { delivery: "link" }, browser, params: { id } })).status, 201);
});

test("SR-05: an Access mapping onto a group holding owner needs a step-up, and both create and delete are audited", async () => {
  const { clock, env, browser, secret } = await staleOwner();
  const g = (await createGroup(env, "Owners", "", T0)) as { id: string };
  await addGrant(env, roleId(env as never, "owner"), { groupId: g.id }, "t", T0);
  const r1 = await call(mapGroup, env, "/x", { json: { accessGroup: "staff", groupId: g.id }, browser });
  assert.deepEqual(await errorOfRes(r1), [403, "step_up_required"]);
  assert.equal((await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser })).status, 200);
  const r2 = await call(mapGroup, env, "/x", { json: { accessGroup: "staff", groupId: g.id }, browser });
  assert.equal(r2.status, 201);
  const { mapping } = (await r2.json()) as { mapping: { id: string } };
  assert.equal((await call(unmapGroup, env, "/x", { method: "DELETE", browser, params: { id: mapping.id } })).status, 200);
  assert.deepEqual(
    events(env).filter((e) => e.kind.startsWith("access_mapping")).map((e) => [e.kind, e.detail.accessGroup]),
    [
      ["access_mapping_added", "staff"],
      ["access_mapping_removed", "staff"],
    ],
  );
});

// ---- SR-06: provider addresses that only look like ASCII ----

test("SR-06: a verified provider address with U+212A KELVIN SIGN does not accept kate@example.com's invitation", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const boss = await signedIn(env, "boss@example.com", "owner");
  const fake = fakeProviders();
  try {
    await invite(env, boss.browser, { email: "kate@example.com", name: "", delivery: "link" });
    const { location } = await roundTrip(env, fake, new Browser(), "github", { sub: "4242", email: "Kate@example.com", verified: true, name: "x" });
    assert.equal(errorOf(location), "email_unverified");
  } finally {
    fake.restore();
  }
});

// ---- SR-07: linking after a step-up ----

test("SR-07: an account with a factor that stepped up can link a provider although it signed in long ago", async () => {
  const clock = useClock();
  jwksCache.clear();
  const env = providerEnv({ AUTH_MFA_KEY: "mfa-test-key-0123456789-abcdefghijklmnop" } as never);
  await seedUser(env, "boss@example.com");
  const { browser } = await signIn(env, "boss@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(30 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  assert.equal((await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser })).status, 200);
  const fake = fakeProviders();
  try {
    const { location } = await roundTrip(env, fake, browser, "google", { sub: "g-77", email: "boss@example.com", verified: true, name: "B" }, { purpose: "link" });
    assert.equal(errorOf(location), null, location);
    // Without the step-up (its window passed), the same account is refused at the start.
    clock.advance(11 * 60_000);
    await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
    const again = await startAt(env, browser, "github", { purpose: "link" });
    assert.match(again.headers.get("location") as string, /\/auth\/mfa\/step-up/);
  } finally {
    fake.restore();
  }
});

// ---- SR-08: a service token's public id is not a lockout handle ----

test("SR-08: failures against a known token id do not lock the real secret out", async () => {
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const sa = await account(env, owner.browser, ["newsletter:send"]);
  const secret = await secretFor(env, owner.browser, sa);
  const { id } = parseSecret(secret) as { id: string };
  for (let i = 0; i < 8; i++) {
    const junk = `ssgsvc_${id}_${"A".repeat(42)}${i}`;
    await swap(env, `Bearer ${junk}`, {}, `198.51.100.${i + 1}`);
  }
  assert.equal((await swap(env, `Bearer ${secret}`, {}, "198.51.100.200")).status, 200);
});

// ---- SR-09: anonymous OAuth starts do not scan the session tables ----

test("SR-09: an OAuth start never touches session_methods; purgeStale sweeps the orphans", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  try {
    sql(env).prepare("SELECT 1").get();
    await seedUser(env, "x@example.com");
    (env.AUTH_DB as unknown as { sqlite: { exec(s: string): void } }).sqlite.exec("INSERT INTO session_methods (session_id, method) VALUES ('ghost', 'password')");
    const { started } = await roundTrip(env, fake, new Browser(), "google", { sub: "g-1", email: "nobody@example.com", verified: true, name: "N" });
    assert.equal(started.status, 303);
    assert.ok(sql(env).prepare("SELECT 1 AS x FROM session_methods WHERE session_id = 'ghost'").get());
    await call(login, env, "/api/auth/login", { json: { email: "x@example.com", password: PASSWORD }, browser: new Browser() });
    assert.equal(sql(env).prepare("SELECT 1 AS x FROM session_methods WHERE session_id = 'ghost'").get(), undefined);
  } finally {
    fake.restore();
  }
});

// ---- SR-13: two parallel removals cannot take the last factor a role needs ----

test("SR-13: an owner removing both factors at once keeps one", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "ada@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "ada@example.com");
  const a = await enrolTotp(env, browser, clock.now, "Phone");
  const b = await enrolTotp(env, browser, clock.now, "Spare");
  const del = (fid: string) => call(removeFactor, env, `/api/auth/mfa/factors/${fid}`, { method: "DELETE", browser, params: { id: fid } });
  // Both requests list the factors before either deletes (a barrier on the read).
  const db = env.AUTH_DB as unknown as { prepare(sql: string): Record<string, unknown> };
  const real = db.prepare.bind(db);
  let waiting = 0;
  let open: () => void = () => {};
  const gate = new Promise<void>((r) => (open = r));
  db.prepare = (q: string) => {
    const stmt = real(q);
    if (!q.startsWith("SELECT * FROM mfa_factors WHERE user_id = ? AND confirmed_at")) return stmt;
    return { bind: (...v: unknown[]) => ({ all: async () => { if (++waiting === 2) open(); await gate; return (stmt.bind as (...v: unknown[]) => { all(): unknown })(...v).all(); } }) };
  };
  const res = await Promise.all([del(a.factorId), del(b.factorId)]);
  db.prepare = real;
  assert.deepEqual(res.map((r) => r.status).sort(), [200, 409]);
  assert.equal((sql(env).prepare("SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?").get(id) as { n: number }).n, 1);
});

// ---- the paths only a race reaches, made deterministic ----

// afterRead runs `fn` once, right after the first statement matching `re` has
// been read — another request changing the row between this route's read and
// its write.
function afterRead(env: unknown, re: RegExp, fn: () => void) {
  const db = (env as { AUTH_DB: { prepare(sql: string): Record<string, unknown> } }).AUTH_DB;
  const real = db.prepare.bind(db);
  let done = false;
  db.prepare = (q: string) => {
    const stmt = real(q);
    if (done || !re.test(q)) return stmt;
    const wrap = (s: Record<string, unknown>): Record<string, unknown> => ({
      ...s,
      bind: (...a: unknown[]) => wrap((s.bind as (...a: unknown[]) => Record<string, unknown>)(...a)),
      first: async () => { const r = await (s.first as () => Promise<unknown>)(); done = true; fn(); return r; },
      all: async () => { const r = await (s.all as () => Promise<unknown>)(); done = true; fn(); return r; },
    });
    return wrap(stmt);
  };
}

// barrier makes the first `n` reads matching `re` wait for one another: every
// request of a burst reads before any writes.
function barrier(env: unknown, re: RegExp, n: number) {
  const db = (env as { AUTH_DB: { prepare(sql: string): Record<string, unknown> } }).AUTH_DB;
  const real = db.prepare.bind(db);
  let waiting = 0;
  let open: () => void = () => {};
  const gate = new Promise<void>((r) => (open = r));
  db.prepare = (q: string) => {
    const stmt = real(q);
    if (!re.test(q) || waiting >= n) return stmt;
    const wrap = (s: Record<string, unknown>): Record<string, unknown> => ({
      ...s,
      bind: (...a: unknown[]) => wrap((s.bind as (...a: unknown[]) => Record<string, unknown>)(...a)),
      first: async () => { if (++waiting === n) open(); await gate; return (s.first as () => Promise<unknown>)(); },
    });
    return wrap(stmt);
  };
  return () => (db.prepare = real);
}

test("SR-02: a burst that reads the lock before any write is still refused past the threshold (login, password, second factor, bootstrap)", async () => {
  useClock();
  // Login: 10 requests; the first lock read of each waits for the other nine.
  const env = makeEnv();
  await seedUser(env, "ada@example.com");
  const undo = barrier(env, /^SELECT failures, last_failure_at, locked_until FROM login_attempts/, 10);
  const res = await Promise.all(Array.from({ length: 10 }, (_, i) => call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: `wrong-${i}-xxxxxxxx` }, browser: new Browser(`198.51.100.${i + 1}`) })));
  undo();
  const n = statuses(res);
  assert.deepEqual([n[401], n[429]], [5, 5]);
  assert.equal((sql(env).prepare("SELECT MAX(failures) AS n FROM login_attempts").get() as { n: number }).n, 5, "refused attempts give their count back");
});

test("SR-02: refusal paths of the password change, the second factor and bootstrap", async () => {
  const clock = useClock();
  const { lockKey } = await import("../functions/api/auth/_lockout.ts");
  const primed = async (env: ReturnType<typeof makeEnv>, kind: string, value: string, failures = 5) =>
    sql(env).prepare("INSERT INTO login_attempts (key, kind, failures, last_failure_at, locked_until) VALUES (?, ?, ?, ?, NULL)").get(await lockKey(env, kind as never, value), kind, failures, new Date(clock.now).toISOString());
  // The count is at the threshold but its lock not yet written: a burst's next attempt.
  const env = makeEnv();
  const { browser } = await signedIn(env, "ada@example.com", []);
  await primed(env, "account", "ada@example.com");
  const pw = await call(password, env, "/api/auth/password", { json: { current: PASSWORD, password: "a brand new passphrase" }, browser });
  assert.deepEqual([pw.status, pw.headers.get("retry-after")], [429, "30"]);
  const menv = mfaEnv();
  const id = await seedUser(menv, "bo@example.com");
  const first = await signIn(menv, "bo@example.com");
  const { secret } = await enrolTotp(menv, first.browser, clock.now);
  const pending = await signIn(menv, "bo@example.com");
  await primed(menv, "mfa_account", id);
  const v = await call(verify, menv, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: pending.browser });
  assert.equal(v.status, 429);
  const benv = makeEnv({ AUTH_BOOTSTRAP_TOKEN: "bootstrap-token-0123456789" });
  const { ensureSchema } = await import("../functions/api/auth/_schema.ts");
  await ensureSchema(benv);
  await primed(benv, "bootstrap", "203.0.113.7");
  const { onRequestPost: bootstrap } = await import("../functions/api/auth/bootstrap.ts");
  const b = await call(bootstrap, benv, "/api/auth/bootstrap", { json: { token: "bootstrap-token-0123456789", email: "a@example.com", password: "a long enough passphrase" }, browser: new Browser() });
  assert.equal(b.status, 429);
});

test("SR-02: of two attempts after a lock ran out, only the one that moves the lock on is checked", async () => {
  const clock = useClock();
  const { lockKey, recordFailure } = await import("../functions/api/auth/_lockout.ts");
  const env = makeEnv();
  const { ensureSchema } = await import("../functions/api/auth/_schema.ts");
  await ensureSchema(env);
  const key = await lockKey(env, "ip", "x");
  sql(env).prepare("INSERT INTO login_attempts (key, kind, failures, last_failure_at, locked_until) VALUES (?, 'ip', 5, ?, ?)").get(key, new Date(clock.now - 60_000).toISOString(), new Date(clock.now - 1000).toISOString());
  const policy = { lockBaseSeconds: 30, lockMaxSeconds: 3600 };
  // The other attempt moves the expired lock on between this one's count and its swap.
  afterRead(env, /^\s*INSERT INTO login_attempts/, () => {
    sql(env).prepare("UPDATE login_attempts SET locked_until = ? WHERE key = ?").get(new Date(clock.now + 60_000).toISOString(), key);
  });
  const wait = await recordFailure(env, [{ key, kind: "ip", threshold: 5 }], policy, clock.now);
  assert.equal(wait, 60);
  assert.equal((sql(env).prepare("SELECT failures FROM login_attempts").get() as { failures: number }).failures, 5, "the refused attempt gave its count back");
});

test("SR-04: a grant, group, role or mapping removed by someone else between the read and the delete is 404", async () => {
  useClock();
  const env = makeEnv();
  const boss = await signedIn(env, "boss@example.com", "owner");
  const holder = await seedUser(env, "h@example.com");
  const role = await grantPerms(env, holder, ["newsletter:send"]);
  const grant = sql(env).prepare("SELECT id FROM grants WHERE role_id = ?").get(role) as { id: string };
  afterRead(env, /ORDER BY r\.name, g\.granted_at, g\.id$/, () => sql(env).prepare("DELETE FROM grants WHERE id = ?").get(grant.id));
  assert.deepEqual(await errorOfRes(await call(deleteGrant, env, "/x", { method: "DELETE", browser: boss.browser, params: { id: grant.id } })), [404, "grant_not_found"]);
  const g = (await createGroup(env, "Gone", "", T0)) as { id: string };
  afterRead(env, /WHERE g\.id = \?$/, () => sql(env).prepare("DELETE FROM groups WHERE id = ?").get(g.id));
  assert.deepEqual(await errorOfRes(await call(deleteGroup, env, "/x", { method: "DELETE", browser: boss.browser, params: { id: g.id } })), [404, "group_not_found"]);
  afterRead(env, /^SELECT \* FROM roles WHERE id = \?$/, () => sql(env).prepare("DELETE FROM roles WHERE id = ?").get(role));
  assert.deepEqual(await errorOfRes(await call(deleteRole, env, "/x", { method: "DELETE", browser: boss.browser, params: { id: role } })), [404, "role_not_found"]);
  const g2 = (await createGroup(env, "Mapped", "", T0)) as { id: string };
  const made = await call(mapGroup, env, "/x", { json: { accessGroup: "staff", groupId: g2.id }, browser: boss.browser });
  const { mapping } = (await made.json()) as { mapping: { id: string } };
  afterRead(env, /ORDER BY m\.access_group/, () => sql(env).prepare("DELETE FROM access_group_map WHERE id = ?").get(mapping.id));
  assert.deepEqual(await errorOfRes(await call(unmapGroup, env, "/x", { method: "DELETE", browser: boss.browser, params: { id: mapping.id } })), [404, "mapping_not_found"]);
  assert.deepEqual(await errorOfRes(await call(unmapGroup, env, "/x", { method: "DELETE", browser: boss.browser, params: { id: "nope" } })), [404, "mapping_not_found"]);
});

test("SR-04: auth:admin cannot drop an Access mapping onto a group holding owner", async () => {
  const { env, owner, low } = await ownersAndAdmin();
  const g = (await createGroup(env, "Owners", "", T0)) as { id: string };
  await addGrant(env, owner, { groupId: g.id }, "t", T0);
  const { createMapping } = await import("../functions/api/auth/_access-groups.ts");
  const m = (await createMapping(env, "staff", g.id, "t", T0)) as { id: string };
  const r = await call(unmapGroup, env, "/x", { method: "DELETE", browser: low.browser, params: { id: m.id } });
  assert.deepEqual(await errorOfRes(r), [403, "escalation"]);
});
