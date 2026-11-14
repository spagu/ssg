// GO-151, configuration: a provider exists only with its variables (404 and
// no button otherwise), wrong variables are a named 503, the password login
// can be turned off, and turning everything off is a named 503.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as loginPage } from "../functions/auth/login.ts";
import { onRequestGet as invitePage } from "../functions/auth/invite.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as accept } from "../functions/api/auth/invite.ts";
import { onRequestGet as jwks } from "../functions/api/auth/jwks.ts";
import { enabledProviders, lookupProvider } from "../functions/api/auth/_provider.ts";
import { nameOf } from "../functions/api/auth/providers/_apple.ts";
import { Browser, PASSWORD, call, makeEnv, useClock } from "./fakes.ts";
import { PROVIDER_ENV } from "./providers-fake.ts";
import { invite, pageOf, tokenOf } from "./go134.ts";
import { signedIn } from "./seed.ts";
import { callbackGet, callbackPost, providerEnv, start } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const startJson = (env: Env, provider: string) => call(start, env, `/api/auth/oauth/${provider}/start`, { json: {}, params: { provider } });

test("a provider without its variables does not exist: 404 on its routes, no button; half a configuration is none", async () => {
  useClock();
  const env = makeEnv({ AUTH_GOOGLE_CLIENT_ID: "only-the-id", AUTH_GITHUB_CLIENT_SECRET: "only-the-secret" } as Partial<Env>);
  for (const p of ["google", "github", "microsoft", "apple", "facebook", "myspace", ""]) {
    const s = await startJson(env, p);
    assert.deepEqual([s.status, await s.json()], [404, { error: "not_found" }], p);
    assert.equal((await call(callbackGet, env, `/api/auth/oauth/${p}/callback?code=c&state=s`, { params: { provider: p } })).status, 404);
    assert.equal((await call(callbackPost, env, `/api/auth/oauth/${p}/callback`, { form: { code: "c" }, csrf: false, params: { provider: p } })).status, 404);
  }
  const { html } = await pageOf(loginPage, env, "/auth/login");
  assert.ok(!html.includes("/api/auth/oauth/") && !html.includes("providers"), "no provider block at all");
  const full = providerEnv({ AUTH_GOOGLE_CLIENT_SECRET: "" } as Partial<Env>);
  const page = (await pageOf(loginPage, full, "/auth/login")).html;
  assert.ok(!page.includes("/api/auth/oauth/google/") && page.includes("/api/auth/oauth/github/start"));
  assert.equal((await call(callbackPost, full, "/api/auth/oauth/google/callback", { form: { code: "c" }, csrf: false, params: { provider: "google" } })).status, 404);
  assert.equal((await call(callbackPost, full, "/api/auth/oauth/github/callback", { form: { code: "c" }, csrf: false, params: { provider: "github" } })).status, 404, "only form_post providers take a POST");
});

test("wrong variables: no button, 503 naming the variable, logged once per request", async () => {
  useClock();
  const logged: string[] = [];
  const real = console.error;
  console.error = (m: string) => logged.push(m);
  try {
    const cases: [string, Record<string, string>, RegExp][] = [
      ["microsoft", { AUTH_MICROSOFT_TENANT: "evil.example.com/x" }, /AUTH_MICROSOFT_TENANT/],
      ["apple", { AUTH_APPLE_TEAM_ID: "team" }, /AUTH_APPLE_TEAM_ID/],
      ["apple", { AUTH_APPLE_PRIVATE_KEY: "not a key" }, /AUTH_APPLE_PRIVATE_KEY/],
      ["facebook", { AUTH_FACEBOOK_API_VERSION: "latest" }, /AUTH_FACEBOOK_API_VERSION/],
    ];
    for (const [p, over, hint] of cases) {
      const env = providerEnv(over as Partial<Env>);
      const s = await startJson(env, p);
      const body = await s.json();
      assert.deepEqual([s.status, body.error], [503, "provider_misconfigured"], p);
      assert.match(body.hint, hint);
      assert.ok(!(await pageOf(loginPage, env, "/auth/login")).html.includes(`/api/auth/oauth/${p}/`));
      assert.equal(lookupProvider(env, p).kind, "misconfigured");
    }
    assert.ok(logged.some((l) => /login provider apple is not offered/.test(l)));
    assert.deepEqual(enabledProviders(providerEnv({ AUTH_FACEBOOK_API_VERSION: "v25.1", AUTH_MICROSOFT_TENANT: "Organizations" } as Partial<Env>)).map((e) => e.provider.id), ["google", "microsoft", "apple", "github", "facebook"]);
  } finally {
    console.error = real;
  }
  assert.equal(nameOf("not json"), "");
  assert.equal(nameOf(JSON.stringify({ name: { firstName: " ", lastName: 7 } })), "");
});

test("AUTH_PASSWORD_LOGIN=off: no password form, no forgot link, the login and the invitation refuse a password", async () => {
  useClock();
  const env = providerEnv({ AUTH_PASSWORD_LOGIN: " off ", AUTH_RESEND_API_KEY: "re_x", AUTH_MAIL_FROM: "a@example.com" } as Partial<Env>);
  const { html, res } = await pageOf(loginPage, env, "/auth/login");
  assert.equal(res.status, 200);
  assert.ok(!html.includes('action="/api/auth/login"') && !html.includes("/auth/forgot") && !html.includes('id="providers-title"'));
  assert.ok(html.includes("Sign in with the account you connected to example.com."));
  const json = await call(login, env, "/api/auth/login", { json: { email: "a@example.com", password: PASSWORD } });
  assert.deepEqual([json.status, await json.json()], [404, { error: "method_disabled" }]);
  const form = await call(login, env, "/api/auth/login", { form: { email: "a@example.com", password: PASSWORD } });
  assert.equal(form.headers.get("location"), "/auth/login?error=method_disabled");
  // An invitation (sent while the password was still on) offers the
  // providers only once it is off, and refuses a password.
  const pw = providerEnv();
  const boss = await signedIn(pw, "boss@example.com", "owner");
  const token = tokenOf((await invite(pw, boss.browser, { email: "n@example.com", delivery: "link" })).body.link);
  Object.assign(pw, { AUTH_PASSWORD_LOGIN: "off" });
  const inv = (await pageOf(invitePage, pw, `/auth/invite?token=${token}`)).html;
  assert.ok(!inv.includes('action="/api/auth/invite"') && inv.includes('name="purpose" value="invite"'));
  const r = await call(accept, pw, "/api/auth/invite", { json: { token, password: PASSWORD } });
  assert.deepEqual([r.status, await r.json()], [400, { error: "unsupported_method" }]);
});

test("every method off: a named 503 on every route that needs the database, the JWKS still served", async () => {
  useClock();
  const env = makeEnv({ AUTH_PASSWORD_LOGIN: "off" } as Partial<Env>);
  for (const [handler, path] of [[loginPage, "/auth/login"], [login, "/api/auth/login"]] as const) {
    const res = await call(handler, env, path, { json: path.startsWith("/api") ? {} : undefined, browser: new Browser() });
    const body = await res.json();
    assert.deepEqual([res.status, body.error], [503, "no_sign_in_method"]);
    assert.match(body.hint, /AUTH_PASSWORD_LOGIN/);
  }
  assert.equal((await call(jwks, env, "/api/auth/jwks")).status, 200);
  assert.equal((await call(login, makeEnv({ ...PROVIDER_ENV, AUTH_PASSWORD_LOGIN: "off" } as Partial<Env>), "/api/auth/login", { json: {} })).status, 404, "a provider alone is enough");
});
