// GO-139: the address rules (no way into someone else's account through case,
// Unicode look-alikes or a variant of the address), allowed domains, the
// per-role method policy, and a session too large for a cookie.
import { test } from "node:test";
import assert from "node:assert/strict";
import { identityOf, matchableEmail } from "../functions/api/auth/_access-identity.ts";
import { onRequestGet as me } from "../functions/api/auth/me/permissions.ts";
import { onRequestGet as sessionGet } from "../functions/api/auth/session.ts";
import { Browser, call, useClock } from "./fakes.ts";
import { invite } from "./go134.ts";
import { grantPerms, seedUser, signedIn } from "./seed.ts";
import { count, events, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, PERSON, accessEnv, enter, errorCode, mint, where } from "./access-fake.ts";

// [what Access says, what it may match ("" = nothing)]
const TABLE: [string, string][] = [
  ["kate@example.com", "kate@example.com"],
  ["Kate@Example.COM", "kate@example.com"],
  ["  kate@example.com ", ""], // the verifier trims; a raw space is not an address
  ["kate+admin@example.com", "kate+admin@example.com"], // a plus-tag is ANOTHER address
  ["k.ate@example.com", "k.ate@example.com"],
  ["kate@example.com.", "kate@example.com."], // a trailing dot: another string, matches nothing stored
  ["Kate@example.com", ""], // KELVIN SIGN lower-cases to "k"
  ["kate@examрle.com", ""], // Cyrillic р
  ["kate@bücher.example", ""], // IDN in Unicode form
  ["kate@xn--bcher-kva.example", "kate@xn--bcher-kva.example"], // its punycode form: only itself
  ["İnfo@example.com", ""], // dotted capital I
  ["kate@example", ""],
  ["", ""],
  ["kate@exa mple.com", ""],
  ["kate\u0000@example.com", ""],
];

test("matchableEmail: one normalisation (trim + lower-case), printable ASCII only, nothing folded", () => {
  for (const [given, expected] of TABLE) assert.equal(matchableEmail(given), expected, JSON.stringify(given));
  assert.deepEqual(identityOf({ subject: "s", email: "Kate@example.com", groups: null }), { subject: "s", email: "Kate@example.com".toLowerCase(), emailVerified: false, name: "" });
});

test("no way into another account by a variant of its address: an invitation or account for kate@ is reached only by kate@", async () => {
  useClock();
  const env = accessEnv({ AUTH_ACCESS_LINK_BY_EMAIL: "on" });
  const fake = new FakeAccess().install();
  try {
    const kate = await seedUser(env, "kate@example.com");
    const boss = await signedIn(env, "boss@example.com", "owner");
    await invite(env, boss.browser, { email: "noor@example.com", delivery: "link" });
    let n = 0;
    for (const variant of ["kate+admin@example.com", "kate@example.com.", "Kate@example.com", "kate@examрle.com", "k.ate@example.com", "noor+x@example.com", "noor@example.com.", "nоor@example.com"]) {
      const res = await enter(env, await mint({ sub: `v${++n}`, email: variant }));
      assert.ok(["not_invited", "access_email_unusable"].includes(errorCode(res) as string), `${variant}: ${errorCode(res)}`);
    }
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0, "nothing linked");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM users WHERE email LIKE 'noor%'"), 0, "the invitation is still open");
    // Mixed case of the real address is the same person.
    assert.equal(where(await enter(env, await mint({ sub: "kate-real", email: "KATE@EXAMPLE.COM" }))), "/");
    assert.equal(events(env, "method_linked")[0].user_id, kate);
  } finally {
    fake.restore();
  }
});

test("AUTH_ACCESS_ALLOWED_DOMAINS: only addresses in those domains, also for a linked identity", async () => {
  useClock();
  const env = accessEnv({ AUTH_ACCESS_ALLOWED_DOMAINS: "example.org" });
  const fake = new FakeAccess().install();
  try {
    await providerUser(env, "ada@example.com", "access", PERSON.sub);
    assert.equal(errorCode(await enter(env, await mint())), "domain_not_allowed");
    await providerUser(env, "bo@example.org", "access", "bo-sub");
    assert.equal(where(await enter(env, await mint({ sub: "bo-sub", email: "bo@example.org" }))), "/");
  } finally {
    fake.restore();
  }
});

test("AUTH_ROLE_METHODS applies to access like any method; a session too large for a cookie is refused", async () => {
  useClock();
  const env = accessEnv({ AUTH_ROLE_METHODS: JSON.stringify({ "test-role-1": ["password"] }) });
  const fake = new FakeAccess().install();
  try {
    const id = await providerUser(env, "ada@example.com", "access", PERSON.sub);
    await grantPerms(env, id, ["newsletter:send"]); // test-role-1: password sessions only
    await grantPerms(env, id, ["comments:read"]);
    const browser = new Browser();
    await enter(env, await mint(), { browser });
    const token = (await (await call(me, env, "/api/auth/me/permissions", { browser })).json()).token;
    assert.deepEqual(token, ["comments:read"]);
    assert.ok((await call(sessionGet, env, "/api/auth/session", { browser })).ok);

    const huge = accessEnv({ AUTH_ISSUER: "https://example.com/" + "x".repeat(4000) } as Partial<Env>);
    await providerUser(huge, "ada@example.com", "access", PERSON.sub);
    const quiet = console.error;
    console.error = () => {};
    try {
      assert.equal(errorCode(await enter(huge, await mint())), "permissions_too_large");
    } finally {
      console.error = quiet;
    }
  } finally {
    fake.restore();
  }
});
