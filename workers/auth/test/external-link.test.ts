// #435 follow-up: linking an external issuer's identity to a signed-in
// account. The token arrives at POST /api/auth/jwt/link
// (functions/api/auth/jwt/link.ts) and links nothing; the signed-in person
// confirms on /auth/jwt/link (functions/auth/jwt/link.ts), a form of this
// origin only; the handle works once and for five minutes; the usual
// recent-sign-in and step-up rules; every refusal audited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as arrival } from "../functions/api/auth/jwt/link.ts";
import { onRequestGet as linkGet, onRequestPost as linkPost } from "../functions/auth/jwt/link.ts";
import { onRequestGet as accountGet } from "../functions/auth/account.ts";
import { onRequestGet as methodsGet } from "../functions/api/auth/methods/index.ts";
import { LINK_COOKIE, LINK_SECONDS } from "../functions/api/auth/_external-link-store.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { count, events, providerUser } from "./go151.ts";
import { loggedIn } from "./seed.ts";
import { FakeIssuer, ISS, errorCode, jwtEnv, mint, post, where } from "./external-fake.ts";

const METHOD = `jwt:${ISS}`;
const LINK_URL = "https://id.example.com/connect";
const FORM_ORIGIN = "https://id.example.com";

async function world(over: Record<string, string> = {}) {
  const clock = useClock();
  const env = jwtEnv({ AUTH_JWT_LINK_URL: LINK_URL, AUTH_JWT_FORM_ORIGINS: FORM_ORIGIN, ...over });
  const fake = new FakeIssuer().install();
  const { browser, id } = await loggedIn(env);
  return { clock, env, fake, browser, id };
}

// arrive: the issuer's page posts the token as a plain form, across origins.
const arrive = async (env: Env, browser: Browser, token: string | null, origin = FORM_ORIGIN) =>
  call(arrival, env, "/api/auth/jwt/link", { browser, form: token ? { token } : {}, headers: { origin } });
const page = (env: Env, browser: Browser) => call(linkGet, env, "/auth/jwt/link", { browser, csrf: false });
const confirm = (env: Env, browser: Browser, headers: Record<string, string> = {}) => call(linkPost, env, "/auth/jwt/link", { browser, form: {}, headers });
const pending = (env: Env) => count(env, "SELECT COUNT(*) AS n FROM jwt_link_pending");
const quiet = async <T>(fn: () => Promise<T>): Promise<T> => {
  const real = console.warn;
  console.warn = () => {};
  try {
    return await fn();
  } finally {
    console.warn = real;
  }
};

test("account page offers Connect <issuer>; the token arrives, the person confirms, the identity is linked; then sign-in works", async () => {
  const { env, fake, browser, id } = await world();
  try {
    const before = await (await call(accountGet, env, "/auth/account", { browser, csrf: false })).text();
    assert.match(before, /<a class="btn btn-provider" href="https:\/\/id\.example\.com\/connect" data-provider="jwt">Connect https:\/\/id\.example\.com<\/a>/);
    const json = await (await call(methodsGet, env, "/api/auth/methods", { browser })).json();
    assert.deepEqual(json.available, [{ id: METHOD, name: ISS, linkUrl: LINK_URL }]);

    const arrived = await arrive(env, browser, await mint({ email: "ada.work@example.com" }));
    assert.equal(where(arrived), "/auth/jwt/link");
    assert.match(arrived.headers.get("set-cookie") as string, new RegExp(`^${LINK_COOKIE}=[\\w-]{43}; Max-Age=${LINK_SECONDS}; Path=/; Secure; HttpOnly; SameSite=Lax$`));
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0, "the arrival links nothing");

    const shown = await page(env, browser);
    assert.equal(shown.status, 200);
    const html = await shown.text();
    assert.match(html, /https:\/\/id\.example\.com says you are ada\.work@example\.com\. Connect this identity to your account ada@example\.com\?/);
    assert.match(html, /<form class="stack" method="post" action="\/auth\/jwt\/link">/);
    assert.match(html, /<title>Connect your sign-in from https:\/\/id\.example\.com/);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0, "a GET links nothing");

    const done = await confirm(env, browser);
    assert.equal(where(done), "/auth/account?ok=method_linked");
    assert.ok(!browser.jar.has(LINK_COOKIE), "the handle cookie is cleared");
    assert.equal(pending(env), 0, "the handle is used up");
    const linked = events(env, "method_linked")[0];
    assert.deepEqual([linked.user_id, linked.method, linked.detail.via, linked.detail.email], [id, METHOD, "account", "ada.work@example.com"]);

    const after = await (await call(accountGet, env, "/auth/account", { browser, csrf: false })).text();
    assert.match(after, /<tr class="method"><td data-label="Method">https:\/\/id\.example\.com<\/td><td data-label="Details">ada\.work@example\.com<\/td>/);
    assert.doesNotMatch(after, /data-provider="jwt"/);
    assert.equal(where(await post(env, await mint())), "/", "the linked identity signs in");
  } finally {
    fake.restore();
  }
});

test("without AUTH_JWT_LINK_URL there is no button, and the link still works; the confirmation shows the subject without an address", async () => {
  const { env, fake, browser } = await world({ AUTH_JWT_LINK_URL: "" });
  try {
    const html = await (await call(accountGet, env, "/auth/account", { browser, csrf: false })).text();
    assert.doesNotMatch(html, /data-provider="jwt"/);
    await arrive(env, browser, await mint({ email: undefined, email_verified: undefined }));
    assert.match(await (await page(env, browser)).text(), /says you are ext-1001\./);
    assert.equal(where(await confirm(env, browser)), "/auth/account?ok=method_linked");
  } finally {
    fake.restore();
  }
});

test("arrival refused: off (404), another origin (403), a bad or replayed token, a domain not allowed — audited, never linked", async () => {
  const { env, fake, browser } = await world({ AUTH_JWT_ALLOWED_DOMAINS: "example.com" });
  try {
    assert.equal((await arrive(makeEnv(), browser, await mint())).status, 404);
    assert.equal((await arrive(env, browser, await mint(), "https://evil.example.net")).status, 403);
    assert.equal(where(await quiet(async () => arrive(env, browser, "not-a-jwt"))), "/auth/account?error=jwt_failed");
    const token = await mint();
    await arrive(env, browser, token);
    assert.equal(errorCode(await arrive(env, browser, token)), "jwt_failed", "a token works once");
    assert.equal(errorCode(await arrive(env, browser, await mint({ email: "ada@elsewhere.example" }))), "domain_not_allowed");
    const refused = events(env, "signin_refused");
    assert.deepEqual(
      refused.map((e) => [e.detail.reason, e.detail.purpose, e.method]),
      [
        ["jwt_failed", "link", METHOD],
        ["jwt_failed", "link", METHOD],
        ["domain_not_allowed", "link", METHOD],
      ],
    );
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
  } finally {
    fake.restore();
  }
});

test("confirmation refused: no session, not recent, no or expired handle, used twice, another origin", async () => {
  const { clock, env, fake, browser } = await world({ AUTH_TOKEN_MINUTES: "30" });
  try {
    const stranger = new Browser();
    await arrive(env, stranger, await mint());
    assert.match(where(await page(env, stranger)), /^\/api\/auth\/resume\?next=%2Fauth%2Fjwt%2Flink/, "to sign in first, and back");
    assert.match(where(await confirm(env, stranger)), /^\/auth\/login\?next=%2Fauth%2Fjwt%2Flink/);

    assert.equal(where(await page(env, browser)), "/auth/account?error=jwt_link_expired", "no handle");
    await arrive(env, browser, await mint());
    assert.equal((await confirm(env, browser, { origin: "https://evil.example.net" })).status, 403);
    clock.advance((LINK_SECONDS + 1) * 1000);
    assert.equal(where(await page(env, browser)), "/auth/account?error=jwt_link_expired", "five minutes");

    await arrive(env, browser, await mint({}, undefined, T0 + (LINK_SECONDS + 1) * 1000));
    const handle = browser.jar.get(LINK_COOKIE) as string;
    assert.equal(where(await confirm(env, browser)), "/auth/account?ok=method_linked");
    browser.jar.set(LINK_COOKIE, handle);
    assert.equal(where(await confirm(env, browser)), "/auth/account?error=jwt_link_expired", "used once");

    clock.advance(11 * 60_000);
    assert.equal(where(await page(env, browser)), "/auth/login?reauth=1&next=%2Fauth%2Fjwt%2Flink", "a sign-in older than 10 minutes");
    assert.equal(events(env, "signin_refused").filter((e) => e.detail.reason === "jwt_link_expired").length, 3);
  } finally {
    fake.restore();
  }
});

test("the identity is already this account's, or another's, or the account has one of this issuer: refused on the page and at the confirmation", async () => {
  const { env, fake, browser, id } = await world();
  try {
    await providerUser(env, "grace@example.com", METHOD, "ext-2002");
    await arrive(env, browser, await mint({ sub: "ext-2002" }));
    assert.equal(where(await page(env, browser)), "/auth/account?error=identity_in_use");
    await arrive(env, browser, await mint({ sub: "ext-2002" }));
    assert.equal(where(await confirm(env, browser)), "/auth/account?error=identity_in_use", "the confirmation checks again");

    await arrive(env, browser, await mint({ sub: "ext-1001" }));
    assert.equal(where(await confirm(env, browser)), "/auth/account?ok=method_linked");
    await arrive(env, browser, await mint({ sub: "ext-3003" }));
    assert.equal(where(await page(env, browser)), "/auth/account?error=provider_already_linked");
    await arrive(env, browser, await mint({ sub: "ext-3003" }));
    assert.equal(where(await confirm(env, browser)), "/auth/account?error=provider_already_linked");
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM identities WHERE user_id = '${id}'`), 1);
  } finally {
    fake.restore();
  }
});

test("an account with a second factor needs a recent one (step-up) to link", async () => {
  const { env, fake, browser, id } = await world({ AUTH_MFA_KEY: "k".repeat(40) });
  try {
    (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { run(...a: unknown[]): unknown } } }).sqlite
      .prepare("INSERT INTO mfa_factors (id, user_id, kind, name, confirmed_at, created_at) VALUES (?, ?, 'totp', 'phone', ?, ?)")
      .run(crypto.randomUUID(), id, new Date(T0).toISOString(), new Date(T0).toISOString());
    await arrive(env, browser, await mint());
    assert.match(where(await page(env, browser)), /^\/auth\/mfa\/step-up\?next=%2Fauth%2Fjwt%2Flink/);
  } finally {
    fake.restore();
  }
});

test("a misconfigured method answers 503 on every link route", async () => {
  const { env, fake, browser } = await world({ AUTH_JWT_ALGORITHMS: "RS256,HS256" });
  const real = console.error;
  console.error = () => {};
  try {
    for (const res of [await arrive(env, browser, await mint()), await page(env, browser), await confirm(env, browser)]) {
      assert.equal(res.status, 503);
      assert.equal((await res.json()).error, "jwt_misconfigured");
    }
  } finally {
    console.error = real;
    fake.restore();
  }
});

test("no database: the link routes answer 503 before anything else; a confirmation without a handle is refused", async () => {
  const noDb = { ...jwtEnv(), AUTH_DB: undefined } as never;
  assert.equal((await arrive(noDb, new Browser(), await mint())).status, 503);
  assert.equal((await page(noDb, new Browser())).status, 503);
  const { env, fake, browser } = await world();
  try {
    assert.equal(where(await confirm(env, browser)), "/auth/account?error=jwt_link_expired");
  } finally {
    fake.restore();
  }
});
