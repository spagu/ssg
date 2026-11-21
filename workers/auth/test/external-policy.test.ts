// #435: who an unknown person is, by AUTH_JWT_UNKNOWN — reject (default),
// invite (an open invitation for a verified address), create (an account
// with no role) — and by AUTH_JWT_IDENTITY_CLAIM: with `sub` an existing
// account is never entered by address; with `email` the address is the
// identity. Addresses count only when verified (unless switched off), and
// AUTH_JWT_ALLOWED_DOMAINS narrows them like a provider's.
import { test } from "node:test";
import assert from "node:assert/strict";
import { Browser, useClock } from "./fakes.ts";
import { invite, tokenOf } from "./go134.ts";
import { count, events, providerUser } from "./go151.ts";
import { seedUser, signedIn } from "./seed.ts";
import { FakeIssuer, ISS, errorCode, jwtEnv, mint, post, sqlite, where } from "./external-fake.ts";

const METHOD = `jwt:${ISS}`;
type E = ReturnType<typeof jwtEnv>;
const users = (env: E) => count(env, "SELECT COUNT(*) AS n FROM users");
const identity = (env: E, email: string) => ({
  ...(sqlite(env).prepare("SELECT i.provider, i.subject FROM identities i JOIN users u ON u.id = i.user_id WHERE u.email = ?").get(email) as object),
});

async function world(over: Record<string, string> = {}) {
  useClock();
  const env = jwtEnv(over);
  const fake = new FakeIssuer().install();
  return { env, fake };
}

test("reject (the default): an unknown person is refused, no users row; an open invitation is not used", async () => {
  const { env, fake } = await world();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    await invite(env, boss.browser, { email: "noor@example.com", delivery: "link" });
    const before = users(env);
    const res = await post(env, await mint({ email: "noor@example.com" }));
    assert.equal(errorCode(res), "not_invited");
    assert.equal(users(env), before);
    assert.equal(events(env, "signin_refused").at(-1)?.method, METHOD);
  } finally {
    fake.restore();
  }
});

test("invite: an open invitation for the verified address is accepted, the identity linked; without one refused", async () => {
  const { env, fake } = await world({ AUTH_JWT_UNKNOWN: "invite" });
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    tokenOf((await invite(env, boss.browser, { email: "noor@example.com", name: "Noor", delivery: "link" })).body.link);
    const browser = new Browser();
    const res = await post(env, await mint({ sub: "ext-noor", email: "Noor@Example.com" }), { browser });
    assert.match(where(res), /^\/auth\/\?welcome=1/);
    assert.deepEqual(identity(env, "noor@example.com"), { provider: METHOD, subject: "ext-noor" });
    const accepted = events(env, "invitation_accepted").at(-1);
    assert.deepEqual([accepted?.method, accepted?.detail.via], [METHOD, "jwt_email"]);
    assert.equal(errorCode(await post(env, await mint({ sub: "ext-x", email: "stranger@example.com" }))), "not_invited");
    assert.equal(errorCode(await post(env, await mint({ sub: "ext-y", email: "noor2@example.com", email_verified: false }))), "jwt_email_unusable");
  } finally {
    fake.restore();
  }
});

test("create: a new account with no role, its identity linked, audited; the address must be verified and free", async () => {
  const { env, fake } = await world({ AUTH_JWT_UNKNOWN: "create" });
  try {
    await seedUser(env, "taken@example.com");
    const res = await post(env, await mint({ sub: "ext-new", email: "new@example.com" }));
    assert.equal(where(res), "/");
    assert.deepEqual(identity(env, "new@example.com"), { provider: METHOD, subject: "ext-new" });
    const uid = (sqlite(env).prepare("SELECT id FROM users WHERE email = 'new@example.com'").get() as { id: string }).id;
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM grants WHERE user_id = '${uid}'`), 0, "no permissions from nowhere");
    assert.equal(events(env, "account_created").at(-1)?.method, METHOD);
    assert.equal(errorCode(await post(env, await mint({ sub: "ext-dup", email: "taken@example.com" }))), "jwt_not_linked", "an existing address is never taken over with sub");
    assert.equal(errorCode(await post(env, await mint({ sub: "ext-z", email: "\u212Aate@example.com" }))), "jwt_email_unusable", "U+212A");
    assert.equal(errorCode(await post(env, await mint({ sub: "ext-q", email: undefined }))), "jwt_email_unusable");
  } finally {
    fake.restore();
  }
});

test("identity claim email: the address is the identity — an existing account is entered and linked; sub is not needed", async () => {
  const { env, fake } = await world({ AUTH_JWT_IDENTITY_CLAIM: "email" });
  try {
    const id = await seedUser(env, "ada@example.com");
    const res = await post(env, await mint({ sub: undefined, email: "ADA@example.com" }));
    assert.equal(where(res), "/");
    assert.deepEqual(identity(env, "ada@example.com"), { provider: METHOD, subject: "ada@example.com" });
    assert.equal(events(env, "method_linked").at(-1)?.detail.via, "email");
    assert.equal(where(await post(env, await mint({ email: "ada@example.com" }))), "/", "linked: the next sign-in finds it");
    assert.equal(events(env, "signin").filter((e) => e.user_id === id).length, 2);
    assert.equal(errorCode(await post(env, await mint({ email: "ada@example.com", email_verified: false }))), "jwt_email_unusable");
    assert.equal(errorCode(await post(env, await mint({ email: "nobody@example.com" }))), "not_invited");
  } finally {
    fake.restore();
  }
});

test("AUTH_JWT_EMAIL_VERIFIED=off trusts the address without the claim; AUTH_JWT_ALLOWED_DOMAINS narrows it", async () => {
  const { env, fake } = await world({ AUTH_JWT_UNKNOWN: "create", AUTH_JWT_EMAIL_VERIFIED: "off", AUTH_JWT_ALLOWED_DOMAINS: "example.com" });
  try {
    assert.equal(where(await post(env, await mint({ sub: "a", email: "a@example.com", email_verified: undefined }))), "/");
    assert.equal(errorCode(await post(env, await mint({ sub: "b", email: "b@other.example.org", email_verified: "true" }))), "domain_not_allowed");
    // A linked identity whose account was deleted meanwhile: refused, nothing made.
    await providerUser(env, "gone@example.com", METHOD, "ghost");
    sqlite(env).prepare("DELETE FROM users WHERE email = 'gone@example.com'").run();
    assert.equal(errorCode(await post(env, await mint({ sub: "ghost", email: "gone@example.com" }))), "not_invited");
  } finally {
    fake.restore();
  }
});
