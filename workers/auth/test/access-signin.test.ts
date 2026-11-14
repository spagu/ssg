// GO-139: who a verified Access token signs in. A linked identity → a session
// of method "access"; a blocked account, an unknown person, a token for
// another application → refused, with no users row; an invitation for the
// address → accepted; an existing account by address → only when the
// administrator opted in, and audited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { identityInsert } from "../functions/api/auth/_provider-identities.ts";
import { setUserStatus } from "../functions/api/auth/_users.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { Browser, T0, call, useClock } from "./fakes.ts";
import { invite, tokenOf } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";
import { count, events, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, OTHER_AUD, PERSON, accessEnv, enter, errorCode, mint, where } from "./access-fake.ts";

const users = (env: Env) => count(env, "SELECT COUNT(*) AS n FROM users");
const sqlite = (env: Env) => (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { get(...a: unknown[]): unknown; all(...a: unknown[]): unknown[] } } }).sqlite;

async function world(over: Partial<Env> = {}) {
  useClock();
  const env = accessEnv(over);
  const fake = new FakeAccess().install();
  return { env, fake };
}

test("a linked Access identity signs in: a session of method access, cookies, the signin event; header or cookie", async () => {
  const { env, fake } = await world();
  try {
    const id = await providerUser(env, "ada@example.com", "access", PERSON.sub);
    for (const via of ["header", "cookie"] as const) {
      const browser = new Browser();
      const res = await enter(env, await mint(), { via, next: "/newsletter-admin/", browser });
      assert.equal(res.status, 303, via);
      assert.equal(where(res), "/newsletter-admin/");
      assert.ok(browser.jar.has(ACCESS_COOKIE) && browser.jar.has(REFRESH_COOKIE));
      const me = await call(session, env, "/api/auth/session", { browser });
      assert.equal((await me.json()).user.id, id);
    }
    assert.deepEqual(sqlite(env).prepare("SELECT DISTINCT method FROM session_methods").all().map((r) => ({ ...(r as object) })), [{ method: "access" }]);
    const signins = events(env, "signin");
    assert.equal(signins.length, 2);
    assert.deepEqual([signins[0].user_id, signins[0].method], [id, "access"]);
    assert.ok((sqlite(env).prepare("SELECT last_used_at FROM identities").get() as { last_used_at: string }).last_used_at);
    // Without `next`: AUTH_AFTER_LOGIN_URL (default "/"); a hostile next falls back too.
    assert.equal(where(await enter(env, await mint())), "/");
    assert.equal(where(await enter(env, await mint(), { next: "//evil.example.com/" })), "/");
  } finally {
    fake.restore();
  }
});

test("refused: no token, a token for another application, a blocked account — and nobody gets a users row", async () => {
  const { env, fake } = await world();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const id = await providerUser(env, "ada@example.com", "access", PERSON.sub);
    const before = users(env);
    const none = await enter(env, null, { next: "/x" });
    assert.deepEqual([none.status, where(none)], [303, "/auth/login?error=access_failed&next=%2Fx"]);
    assert.equal(errorCode(await enter(env, await mint(PERSON, { aud: [OTHER_AUD] }))), "access_failed");
    await setUserStatus(env, id, "blocked", T0);
    const blocked = await enter(env, await mint());
    assert.equal(errorCode(blocked), "account_blocked");
    assert.equal(blocked.headers.getSetCookie().length, 0, "no session cookie");
    assert.equal(users(env), before);
    const refused = events(env, "signin_refused");
    assert.deepEqual(
      refused.map((e) => [e.detail.reason, e.detail.cause ?? null, e.user_id]),
      [["access_failed", "no token", null], ["access_failed", "audience", null], ["account_blocked", null, id]],
    );
    assert.ok(boss);
  } finally {
    fake.restore();
  }
});

test("an unknown person is refused (not_invited) and leaves no row; an identity whose account vanished likewise", async () => {
  const { env, fake } = await world();
  try {
    await seedUser(env, "someone@example.com");
    const before = users(env);
    const res = await enter(env, await mint({ sub: "stranger", email: "stranger@example.com" }));
    assert.equal(errorCode(res), "not_invited");
    assert.equal(users(env), before);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
    // A dangling identity row (the account removed by hand in SQL).
    await identityInsert(env, "gone", "access", { subject: "ghost", email: "ghost@example.com", emailVerified: true, name: "" }, T0).run();
    assert.equal(errorCode(await enter(env, await mint({ sub: "ghost", email: "ghost@example.com" }))), "not_invited");
  } finally {
    fake.restore();
  }
});

test("an open invitation for the Access address is accepted: account, roles, Access linked, audited", async () => {
  const { env, fake } = await world();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const roles = sqlite(env).prepare("SELECT id FROM roles WHERE name = 'newsletter-viewer'").get() as { id: string };
    tokenOf((await invite(env, boss.browser, { email: "noor@example.com", name: "Noor", roleIds: [roles.id], delivery: "link" })).body.link);
    const browser = new Browser();
    const res = await enter(env, await mint({ sub: "noor-sub", email: "Noor@Example.com" }), { browser });
    assert.equal(where(res), "/auth/?welcome=1");
    const row = sqlite(env).prepare("SELECT u.id, u.display_name, u.password_hash, i.subject FROM users u JOIN identities i ON i.user_id = u.id WHERE u.email = 'noor@example.com' AND i.provider = 'access'").get() as Record<string, unknown>;
    assert.deepEqual([row.display_name, row.password_hash, row.subject], ["Noor", null, "noor-sub"]);
    const me = await (await call(session, env, "/api/auth/session", { browser })).json();
    assert.equal(me.user.id, row.id);
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM grants WHERE user_id = '${row.id}'`), 1);
    const accepted = events(env, "invitation_accepted")[0];
    assert.deepEqual([accepted.method, accepted.detail.via, accepted.detail.invitedEmail, accepted.detail.providerEmail], ["access", "access_email", "noor@example.com", "noor@example.com"]);
    assert.equal(events(env, "method_linked")[0].detail.via, "invitation");
    // Signing in again goes through the linked identity, not the (used) invitation.
    assert.equal(where(await enter(env, await mint({ sub: "noor-sub", email: "noor@example.com" }))), "/");
  } finally {
    fake.restore();
  }
});

test("an invitation with a role deleted meanwhile is accepted with ?skipped=1; a concurrent acceptance loses cleanly", async () => {
  const { env, fake } = await world();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const role = sqlite(env).prepare("SELECT id FROM roles WHERE name = 'comments-moderator'").get() as { id: string };
    await invite(env, boss.browser, { email: "kim@example.com", roleIds: [role.id], delivery: "link" });
    sqlite(env).prepare("DELETE FROM roles WHERE id = ?").all(role.id);
    assert.equal(where(await enter(env, await mint({ sub: "kim", email: "kim@example.com" }))), "/auth/?welcome=1&skipped=1");

    await invite(env, boss.browser, { email: "lee@example.com", delivery: "link" });
    const [a, b] = await Promise.all([enter(env, await mint({ sub: "lee-1", email: "lee@example.com" })), enter(env, await mint({ sub: "lee-2", email: "lee@example.com" }))]);
    const codes = [errorCode(a), errorCode(b)];
    assert.equal(codes.filter((c) => c === null).length, 1, `exactly one wins: ${codes}`);
    assert.ok(["account_exists", "invitation_invalid", "access_not_linked"].includes(codes.find((c) => c) as string), `the other is refused: ${codes}`);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM users WHERE email = 'lee@example.com'"), 1);
  } finally {
    fake.restore();
  }
});

test("an existing account by address: off by default (access_not_linked, nothing changes); on → linked, entered, audited", async () => {
  for (const flag of ["off", "on"]) {
    const { env, fake } = await world({ AUTH_ACCESS_LINK_BY_EMAIL: flag });
    try {
      const id = await seedUser(env, "ada@example.com");
      const res = await enter(env, await mint({ sub: "ada-sub", email: "ADA@example.com" }));
      if (flag === "off") {
        assert.equal(errorCode(res), "access_not_linked");
        assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
        assert.equal(count(env, "SELECT COUNT(*) AS n FROM sessions"), 0);
        assert.equal(events(env, "signin_refused")[0].user_id, id);
      } else {
        assert.equal(where(res), "/");
        const linked = events(env, "method_linked")[0];
        assert.deepEqual([linked.user_id, linked.method, linked.detail.via, linked.detail.email], [id, "access", "email", "ada@example.com"]);
        // A blocked account is not entered even with the opt-in.
        const other = await seedUser(env, "bo@example.com");
        await setUserStatus(env, other, "blocked", T0);
        assert.equal(errorCode(await enter(env, await mint({ sub: "bo-sub", email: "bo@example.com" }))), "account_blocked");
        // An account already linked to ANOTHER Access identity is not taken over.
        assert.equal(errorCode(await enter(env, await mint({ sub: "ada-other-sub", email: "ada@example.com" }))), "provider_already_linked");
      }
    } finally {
      fake.restore();
    }
  }
});
