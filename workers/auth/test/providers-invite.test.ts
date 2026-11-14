// GO-151, who gets in: an invitation accepted through each provider (by the
// link — any address — or by a VERIFIED address matching it), and every
// account-takeover attempt refused: an unverified address, a verified address
// equal to an existing account's without an invitation, a provider account
// already linked elsewhere. No invitation → no users row.
import { test } from "node:test";
import assert from "node:assert/strict";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { deleteUser } from "../functions/api/auth/_users.ts";
import { Browser, call, useClock } from "./fakes.ts";
import { ALL } from "./providers-fake.ts";
import type { Person, Pid } from "./providers-fake.ts";
import { invite, tokenOf } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";
import { count, errorOf, events, fakeProviders, providerEnv, providerUser, roundTrip, start } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

async function world(over: Partial<Env> = {}) {
  useClock();
  jwksCache.clear();
  const env = providerEnv(over);
  const boss = await signedIn(env, "boss@example.com", "owner");
  return { env, boss, fake: fakeProviders() };
}
const inviteTo = async (env: Env, boss: { browser: Browser }, email: string, name = "") =>
  tokenOf((await invite(env, boss.browser, { email, name, delivery: "link" })).body.link);
const users = (env: Env) => count(env, "SELECT COUNT(*) AS n FROM users");
// GitHub and Facebook subjects are numeric ids.
let n = 1000;
const sub = () => String(++n);

for (const p of ALL) {
  test(`${p}: the invitation link + this provider → the invited account, linked; another address is fine and recorded`, async () => {
    const { env, boss, fake } = await world();
    try {
      const token = await inviteTo(env, boss, "anna@example.com", p === "apple" ? "" : "Anna");
      const person: Person = { sub: "900", email: "someone.else@example.org", verified: false, name: "From Provider" };
      const { location } = await roundTrip(env, fake, new Browser(), p, person, { purpose: "invite", token });
      assert.equal(location, "/auth/?welcome=1");
      const row = env.AUTH_DB.sqlite.prepare("SELECT u.id, u.email, u.display_name, u.password_hash, i.provider, i.subject, i.email AS seen FROM users u JOIN identities i ON i.user_id = u.id WHERE u.email = 'anna@example.com'").get() as Record<string, unknown>;
      assert.deepEqual({ ...row, id: undefined }, { id: undefined, email: "anna@example.com", display_name: p === "apple" ? "Ann Apple" : "Anna", password_hash: null, provider: p, subject: "900", seen: "someone.else@example.org" });
      const accepted = events(env, "invitation_accepted")[0];
      assert.deepEqual([accepted.user_id, accepted.method, accepted.detail.via, accepted.detail.mismatch, accepted.detail.invitedEmail, accepted.detail.providerEmail], [row.id, p, "invitation_link", true, "anna@example.com", "someone.else@example.org"]);
      assert.equal(events(env, "method_linked")[0].user_id, row.id);
      // The link is used up: the same token with another provider account is refused.
      const again = await call(start, env, `/api/auth/oauth/${p}/start`, { form: { purpose: "invite", token }, browser: new Browser(), params: { provider: p } });
      assert.equal(again.headers.get("location"), "/auth/invite?error=invitation_invalid");
    } finally {
      fake.restore();
    }
  });
}

test("a VERIFIED address matching an open invitation accepts it on the login page; Facebook and unverified addresses never do", async () => {
  const { env, boss, fake } = await world();
  try {
    for (const [p, verified, expect] of [["google", true, null], ["github", true, null], ["apple", true, null], ["microsoft", true, null], ["facebook", true, "email_unverified"], ["google", false, "email_unverified"], ["microsoft", false, "email_unverified"]] as [Pid, boolean, string | null][]) {
      const email = `${p}-${verified}@example.com`;
      await inviteTo(env, boss, email);
      const before = users(env);
      const { location } = await roundTrip(env, fake, new Browser(), p, { sub: sub(), email, verified });
      assert.equal(errorOf(location), expect, `${p} verified=${verified}`);
      assert.equal(users(env), before + (expect ? 0 : 1));
      if (!expect) assert.equal(events(env, "invitation_accepted").at(-1)?.detail.via, "verified_email");
    }
  } finally {
    fake.restore();
  }
});

test("no invitation: refused with not_invited, and no users row, no identity", async () => {
  const { env, fake } = await world();
  try {
    const before = users(env);
    for (const p of ALL) {
      const { location } = await roundTrip(env, fake, new Browser(), p, { sub: sub(), email: `nobody-${p}@example.com`, verified: p !== "facebook" });
      assert.equal(errorOf(location), p === "facebook" ? "email_unverified" : "not_invited");
    }
    assert.equal(users(env), before);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
    assert.equal(events(env, "signin_refused").length, ALL.length);
  } finally {
    fake.restore();
  }
});

test("takeover: an address equal to an existing account's — verified or not — never signs into it or links to it", async () => {
  const { env, fake } = await world();
  try {
    const ada = await seedUser(env, "ada@example.com");
    for (const p of ALL) {
      for (const verified of [false, true]) {
        const browser = new Browser();
        const { location } = await roundTrip(env, fake, browser, p, { sub: sub(), email: "ada@example.com", verified });
        assert.ok(["not_invited", "email_unverified"].includes(errorOf(location) as string), `${p} ${verified}`);
        assert.ok(!browser.jar.has("__Host-ssg-auth"));
      }
    }
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM identities WHERE user_id = '${ada}'`), 0);
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM sessions WHERE user_id = '${ada}'`), 0);
  } finally {
    fake.restore();
  }
});

test("an invitation for an address that has an account meanwhile, a provider account linked elsewhere, a withdrawn link", async () => {
  const { env, boss, fake } = await world();
  try {
    const token = await inviteTo(env, boss, "late@example.com");
    await seedUser(env, "late@example.com");
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "google", { sub: "g-late", email: "x@example.com" }, { purpose: "invite", token })).location), "account_exists");
    await providerUser(env, "taken@example.com", "github", "77");
    const t2 = await inviteTo(env, boss, "new@example.com");
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", { sub: "77", email: "taken@example.com", verified: true }, { purpose: "invite", token: t2 })).location), "identity_in_use");
    const t3 = await inviteTo(env, boss, "gone@example.com");
    const browser = new Browser();
    const started = await call(start, env, "/api/auth/oauth/google/start", { form: { purpose: "invite", token: t3 }, browser, params: { provider: "google" } });
    env.AUTH_DB.sqlite.exec("UPDATE invitations SET revoked_at = '2026-01-01T00:00:00.000Z' WHERE email = 'gone@example.com'");
    const back = await fake.authorize(started.headers.get("location") as string, { sub: "g-gone", email: "gone@example.com", verified: true });
    const { answer } = await import("./go151.ts");
    assert.equal(errorOf((await answer(env, browser, "google", back)).headers.get("location") as string), "invitation_invalid");
    for (const bad of ["", "not-a-token", "A".repeat(43)]) {
      const r = await call(start, env, "/api/auth/oauth/google/start", { json: { purpose: "invite", token: bad }, params: { provider: "google" } });
      assert.deepEqual([r.status, await r.json()], [410, { error: "invitation_invalid" }]);
    }
  } finally {
    fake.restore();
  }
});

test("a blocked account, an identity whose account was deleted, and a domain allowlist", async () => {
  const { env, fake } = await world({ AUTH_GITHUB_ALLOWED_DOMAINS: "example.com, Example.ORG" } as Partial<Env>);
  try {
    const blocked = await providerUser(env, "blocked@example.com", "google", "b1");
    env.AUTH_DB.sqlite.exec(`UPDATE users SET status = 'blocked' WHERE id = '${blocked}'`);
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "google", { sub: "b1" })).location), "account_blocked");
    const gone = await providerUser(env, "gone@example.com", "google", "d1");
    assert.equal(await deleteUser(env, gone), "ok");
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM identities WHERE user_id = '${gone}'`), 0, "deleting an account unlinks its identities");
    env.AUTH_DB.sqlite.exec(`INSERT INTO identities (id, provider, subject, user_id, email, linked_at) VALUES ('orphan', 'google', 'o1', 'no-such-user', '', '2026-01-01')`);
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "google", { sub: "o1" })).location), "not_invited");
    await providerUser(env, "dev@example.org", "github", "500");
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", { sub: "500", email: "dev@example.org", verified: true })).location), null);
    for (const person of [{ sub: "500", email: "dev@example.net", verified: true }, { sub: "500", email: "dev@example.com", verified: false }, { sub: "500" }]) {
      assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", person)).location), "domain_not_allowed");
    }
  } finally {
    fake.restore();
  }
});
