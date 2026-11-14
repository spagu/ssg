// GO-151, the account's own sign-in methods: listing, linking a provider
// (only with a recent sign-in), removing (never the last — also when two
// removals race), and setting a first password.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as list } from "../functions/api/auth/methods/index.ts";
import { onRequestPost as setPassword } from "../functions/api/auth/methods/set-password.ts";
import { onRequestDelete as removeJson, onRequestPost as remove } from "../functions/api/auth/methods/[id].ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { removeIdentity, removePassword } from "../functions/api/auth/_provider-identities.ts";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { revokeUserSessions } from "../functions/api/auth/_sessions.ts";
import { Browser, PASSWORD, call, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { count, errorOf, events, fakeProviders, providerEnv, providerUser, roundTrip, startAt } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const ids = (env: Env, userId: string) => env.AUTH_DB.sqlite.prepare("SELECT id, provider FROM identities WHERE user_id = ? ORDER BY provider").all(userId) as { id: string; provider: string }[];
const del = (env: Env, browser: Browser, id: string) => call(remove, env, `/api/auth/methods/${id}`, { form: { x: "1" }, browser, params: { id } });

async function setup(over: Partial<Env> = {}) {
  const clock = useClock();
  jwksCache.clear();
  const env = providerEnv(over);
  return { clock, env, fake: fakeProviders() };
}

test("methods: the list; linking a provider from the account page; the same provider twice; one linked elsewhere", async () => {
  const { env, fake } = await setup();
  try {
    assert.equal((await call(list, env, "/api/auth/methods")).status, 401);
    const { browser, id } = await loggedIn(env);
    const before = await (await call(list, env, "/api/auth/methods", { browser })).json();
    assert.deepEqual(before, { password: true, identities: [], available: ["google", "microsoft", "apple", "github", "facebook"].map((p) => ({ id: p, name: { google: "Google", microsoft: "Microsoft", apple: "Apple", github: "GitHub", facebook: "Facebook" }[p] })), sessionMethod: "password", recentSignIn: true, recentMinutes: 10 });
    const { location } = await roundTrip(env, fake, browser, "github", { sub: "42", email: "ada@github.example.com", verified: false }, { purpose: "link" });
    assert.equal(location, "/auth/account?ok=method_linked");
    const after = await (await call(list, env, "/api/auth/methods", { browser })).json();
    assert.deepEqual([after.identities.length, after.identities[0].provider, after.identities[0].email, after.available.length], [1, "github", "ada@github.example.com", 4]);
    assert.equal(events(env, "method_linked")[0].user_id, id);
    const twice = await startAt(env, browser, "github", { purpose: "link" });
    assert.equal(twice.headers.get("location"), "/auth/account?error=provider_already_linked");
    await providerUser(env, "other@example.com", "google", "g-other");
    assert.equal((await roundTrip(env, fake, browser, "google", { sub: "g-other" }, { purpose: "link" })).location, "/auth/account?error=identity_in_use");
  } finally {
    fake.restore();
  }
});

test("methods: linking needs a recent sign-in — at the start, and still a live session at the callback", async () => {
  const { clock, env, fake } = await setup({ AUTH_RECENT_SIGNIN_MINUTES: "5" } as Partial<Env>);
  try {
    const anon = await startAt(env, new Browser(), "google", { purpose: "link" });
    assert.equal(anon.headers.get("location"), "/auth/login?next=%2Fauth%2Faccount");
    const { browser } = await loggedIn(env);
    clock.advance(6 * 60_000);
    assert.equal((await startAt(env, browser, "google", { purpose: "link" })).headers.get("location"), "/auth/login?reauth=1&next=%2Fauth%2Faccount");
    const json = await call((await import("./go151.ts")).start, env, "/api/auth/oauth/google/start", { json: { purpose: "link" }, browser, params: { provider: "google" } });
    assert.deepEqual([json.status, await json.json()], [403, { error: "reauth_required" }]);
    const { browser: fresh, id: bea } = await loggedIn(env, "bea@example.com");
    const started = await startAt(env, fresh, "google", { purpose: "link" });
    const back = await fake.authorize(started.headers.get("location") as string, { sub: "g-bea" });
    await revokeUserSessions(env, bea, "test", clock.now);
    const { answer } = await import("./go151.ts");
    assert.equal(errorOf((await answer(env, fresh, "google", back)).headers.get("location") as string), "reauth_required");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
  } finally {
    fake.restore();
  }
});

test("methods: remove a provider, never the last method; unknown and foreign ids are not found; JSON too", async () => {
  const { env, fake } = await setup();
  try {
    const { browser, id } = await loggedIn(env);
    await roundTrip(env, fake, browser, "google", { sub: "g1" }, { purpose: "link" });
    const [google] = ids(env, id);
    const theirs = await providerUser(env, "other@example.com", "github", "9");
    assert.equal((await del(env, browser, ids(env, theirs)[0].id)).headers.get("location"), "/auth/account?error=method_not_found");
    assert.equal((await del(env, browser, "nope")).headers.get("location"), "/auth/account?error=method_not_found");
    assert.equal((await del(env, browser, "password")).headers.get("location"), "/auth/account?ok=method_removed");
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM users WHERE id = '${id}' AND password_hash IS NULL`), 1);
    const last = await call(removeJson, env, `/api/auth/methods/${google.id}`, { method: "DELETE", browser, params: { id: google.id } });
    assert.deepEqual([last.status, await last.json()], [409, { error: "method_last" }]);
    assert.equal((await del(env, browser, "password")).headers.get("location"), "/auth/account?error=method_not_found", "no password to remove");
    assert.deepEqual(events(env, "method_unlinked").map((e) => e.method), ["password"]);
  } finally {
    fake.restore();
  }
});

test("methods: the last-method invariant holds when removals race — in the routes and in the data layer", async () => {
  const { env, fake } = await setup();
  try {
    for (let round = 0; round < 5; round++) {
      const { browser, id } = await loggedIn(env, `race${round}@example.com`);
      await roundTrip(env, fake, browser, "github", { sub: String(100 + round) }, { purpose: "link" });
      const [gh] = ids(env, id);
      const answers = await Promise.all(round % 2 ? [del(env, browser, gh.id), del(env, browser, "password")] : [del(env, browser, "password"), del(env, browser, gh.id)]);
      const codes = answers.map((a) => a.headers.get("location"));
      assert.equal(codes.filter((c) => c?.endsWith("ok=method_removed")).length, 1, codes.join(" "));
      assert.equal(codes.filter((c) => c?.endsWith("error=method_last")).length, 1, codes.join(" "));
      const left = count(env, `SELECT (SELECT COUNT(*) FROM identities WHERE user_id = '${id}') + (SELECT COUNT(*) FROM users WHERE id = '${id}' AND password_hash IS NOT NULL) AS n`);
      assert.equal(left, 1);
    }
    const lone = await providerUser(env, "lone@example.com", "apple", "a-lone");
    const [r1, r2] = await Promise.all([removeIdentity(env, lone, ids(env, lone)[0].id), removePassword(env, lone, 0)]);
    assert.deepEqual([r1, r2], ["last_method", "not_found"]);
    assert.throws(() => env.AUTH_DB.sqlite.exec(`UPDATE identities SET user_id = 'someone' WHERE user_id = '${lone}'`), /method_required/);
  } finally {
    fake.restore();
  }
});

test("methods: a provider-only account sets a first password (recently signed in, rules apply), then signs in with it", async () => {
  const { clock, env, fake } = await setup();
  try {
    await providerUser(env, "pat@example.com", "facebook", "31337");
    const browser = new Browser();
    await roundTrip(env, fake, browser, "facebook", { sub: "31337" });
    const set = (password: string, b = browser) => call(setPassword, env, "/api/auth/methods/set-password", { form: { password }, browser: b });
    assert.equal((await set("short")).headers.get("location"), "/auth/account?error=password_too_short");
    assert.equal((await set(PASSWORD)).headers.get("location"), "/auth/account?ok=password_set");
    assert.equal((await set(PASSWORD)).headers.get("location"), "/auth/account?error=password_exists");
    assert.equal((await call(login, env, "/api/auth/login", { json: { email: "pat@example.com", password: PASSWORD } })).status, 200);
    assert.equal(events(env, "password_set").length, 1);
    clock.advance(11 * 60_000);
    assert.equal((await set(PASSWORD)).headers.get("location"), "/auth/login?reauth=1&next=%2Fauth%2Faccount");
    assert.equal((await set(PASSWORD, new Browser())).headers.get("location"), "/auth/login?next=%2Fauth%2Faccount");
    const json = await call(setPassword, env, "/api/auth/methods/set-password", { json: { password: PASSWORD } });
    assert.deepEqual([json.status, await json.json()], [401, { error: "not_authenticated" }]);
  } finally {
    fake.restore();
  }
});

test("methods: with the password login off there is no password to set; JSON answers", async () => {
  const { env, fake } = await setup({ AUTH_PASSWORD_LOGIN: "OFF" } as Partial<Env>);
  try {
    await providerUser(env, "pat@example.com", "google", "p1");
    const browser = new Browser();
    await roundTrip(env, fake, browser, "google", { sub: "p1" });
    const r = await call(setPassword, env, "/api/auth/methods/set-password", { json: { password: PASSWORD }, browser });
    assert.deepEqual([r.status, await r.json()], [404, { error: "method_disabled" }]);
    const ok = await call(removeJson, env, "/api/auth/methods/nope", { method: "DELETE", browser, params: { id: "nope" } });
    assert.deepEqual([ok.status, await ok.json()], [404, { error: "method_not_found" }]);
  } finally {
    fake.restore();
  }
});
