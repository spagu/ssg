// GO-139: linking Access to a signed-in account (the account page → a
// confirmation page → a form POST), with the same recent-sign-in rule as any
// method; unlinking it; and the last-method rule covering an account whose
// only way in is Access.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as linkGet, onRequestPost as linkPost } from "../functions/auth/access/link.ts";
import { onRequestGet as accountGet } from "../functions/auth/account.ts";
import { onRequestGet as methodsGet } from "../functions/api/auth/methods/index.ts";
import { onRequestDelete as methodDelete } from "../functions/api/auth/methods/[id].ts";
import { Browser, call, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { count, events, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, PERSON, accessEnv, enter, errorCode, mint, where } from "./access-fake.ts";

const header = async (p = PERSON) => ({ "cf-access-jwt-assertion": await mint(p) });
const get = async (env: Env, browser: Browser, p = PERSON) => call(linkGet, env, "/auth/access/link", { browser, headers: await header(p), csrf: false });
const post = async (env: Env, browser: Browser, p = PERSON) => call(linkPost, env, "/auth/access/link", { browser, form: {}, headers: await header(p) });

test("account page offers Connect Cloudflare Access; the confirmation names both addresses; the POST links; then sign-in works", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    const { browser, id } = await loggedIn(env);
    const page = await (await call(accountGet, env, "/auth/account", { browser, csrf: false })).text();
    assert.match(page, /<a class="btn btn-provider" href="\/auth\/access\/link" data-provider="access">Connect Cloudflare Access<\/a>/);
    const json = await (await call(methodsGet, env, "/api/auth/methods", { browser })).json();
    assert.deepEqual(json.available, [{ id: "access", name: "Cloudflare Access" }]);

    const confirm = await get(env, browser, { sub: PERSON.sub, email: "ada.work@example.com" });
    assert.equal(confirm.status, 200);
    const html = await confirm.text();
    assert.match(html, /Cloudflare Access says you are ada\.work@example\.com\. Connect this identity to your account ada@example\.com\?/);
    assert.match(html, /<form class="stack" method="post" action="\/auth\/access\/link">/);
    assert.match(confirm.headers.get("content-security-policy") as string, /form-action 'self' https:\/\/myteam\.cloudflareaccess\.com/);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0, "a GET links nothing");

    const done = await post(env, browser, { sub: PERSON.sub, email: "ada.work@example.com" });
    assert.equal(where(done), "/auth/account?ok=method_linked");
    const linked = events(env, "method_linked")[0];
    assert.deepEqual([linked.user_id, linked.method, linked.detail.via, linked.detail.email], [id, "access", "account", "ada.work@example.com"]);
    assert.equal(where(await get(env, browser)), "/auth/account?error=provider_already_linked");
    const after = await (await call(accountGet, env, "/auth/account", { browser, csrf: false })).text();
    assert.match(after, /<tr class="method"><td data-label="Method">Cloudflare Access<\/td><td data-label="Details">ada\.work@example\.com<\/td>/);
    assert.doesNotMatch(after, /Connect Cloudflare Access/);
    assert.equal(where(await enter(env, await mint())), "/");
  } finally {
    fake.restore();
  }
});

test("link refused: not signed in, not recently, no or bad token, keys down, a domain not allowed, Access already used elsewhere", async () => {
  const clock = useClock();
  const env = accessEnv({ AUTH_TOKEN_MINUTES: "30", AUTH_ACCESS_ALLOWED_DOMAINS: "example.com" });
  const fake = new FakeAccess().install();
  const warn = console.warn;
  console.warn = () => {};
  try {
    const anon = new Browser();
    assert.match(where(await get(env, anon)), /^\/api\/auth\/resume\?next=%2Fauth%2Faccess%2Flink$/);
    assert.equal(where(await post(env, anon)), "/auth/login?next=%2Fauth%2Faccount");

    const { browser, id } = await loggedIn(env);
    assert.equal(errorCode(await call(linkGet, env, "/auth/access/link", { browser, csrf: false })), "access_failed");
    assert.equal(errorCode(await get(env, browser, { sub: "x", email: "x@example.org" })), "domain_not_allowed");
    const other = await providerUser(env, "bo@example.com", "access", "bo-sub");
    assert.equal(errorCode(await get(env, browser, { sub: "bo-sub", email: "bo@example.com" })), "identity_in_use");
    assert.equal(errorCode(await post(env, browser, { sub: "bo-sub", email: "bo@example.com" })), "identity_in_use");
    const refused = events(env, "signin_refused");
    assert.ok(refused.every((e) => e.user_id === id && e.detail.purpose === "link"));
    assert.ok(other);
    // A second Access identity for an account that has one.
    assert.equal(where(await post(env, browser)), "/auth/account?ok=method_linked");
    assert.equal(errorCode(await post(env, browser, { sub: "another", email: "ada@example.com" })), "provider_already_linked");
    // Keys unavailable.
    fake.restore();
    fake.mode = "500";
    fake.install();
    assert.equal(errorCode(await get(env, browser)), "access_unavailable");
    // Too old a sign-in: sign in again first.
    clock.advance(15 * 60_000);
    assert.equal(where(await get(env, browser)), "/auth/login?reauth=1&next=%2Fauth%2Faccount");
    assert.equal(where(await post(env, browser)), "/auth/login?reauth=1&next=%2Fauth%2Faccount");
  } finally {
    console.warn = warn;
    fake.restore();
  }
});

test("unlinking Access: allowed while another way in remains; refused (method_last) for an account whose only way in is Access", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    // Access only: shown as the only way in, and the database refuses the removal.
    await providerUser(env, "solo@example.com", "access", "solo");
    const solo = new Browser();
    await enter(env, await mint({ sub: "solo", email: "solo@example.com" }), { browser: solo });
    const methods = await (await call(methodsGet, env, "/api/auth/methods", { browser: solo })).json();
    assert.deepEqual([methods.password, methods.identities.map((i: { provider: string }) => i.provider), methods.sessionMethod], [false, ["access"], "access"]);
    const page = await (await call(accountGet, env, "/auth/account", { browser: solo, csrf: false })).text();
    assert.match(page, /Cloudflare Access <span class="chip chip-current">Used for this session<\/span>/);
    assert.match(page, /Your only way in/);
    const del = await call(methodDelete, env, `/api/auth/methods/${methods.identities[0].id}`, { method: "DELETE", browser: solo, params: { id: methods.identities[0].id } });
    assert.deepEqual([del.status, await del.json()], [409, { error: "method_last" }]);

    // Password + Access: Access can go.
    const { browser } = await loggedIn(env);
    await post(env, browser);
    const mine = await (await call(methodsGet, env, "/api/auth/methods", { browser })).json();
    const access = mine.identities.find((i: { provider: string }) => i.provider === "access");
    const ok = await call(methodDelete, env, `/api/auth/methods/${access.id}`, { method: "DELETE", browser, params: { id: access.id } });
    assert.deepEqual(await ok.json(), { ok: true });
    assert.equal(events(env, "method_unlinked")[0].method, "access");
  } finally {
    fake.restore();
  }
});
