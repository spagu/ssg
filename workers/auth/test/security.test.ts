// Cross-cutting guarantees: every state-changing route refuses a request
// without the CSRF defence; an unconfigured worker answers 503 and never mints
// a token; the private key and raw refresh tokens never appear in an answer
// body, a non-cookie header, a log line or a database column; every answer
// carries the security headers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as logout } from "../functions/api/auth/logout.ts";
import { onRequestPost as logoutAll } from "../functions/api/auth/logout-all.ts";
import { onRequestPost as changePassword } from "../functions/api/auth/password.ts";
import { onRequestPost as bootstrap } from "../functions/api/auth/bootstrap.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { onRequestGet as jwks } from "../functions/api/auth/jwks.ts";
import { REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { Browser, KEYS, ORIGIN, PASSWORD, call, makeEnv, useClock } from "./fakes.ts";
import type { ReqOptions } from "./fakes.ts";
import { loggedIn } from "./seed.ts";

const POSTS: [string, unknown, boolean][] = [
  ["/api/auth/login", login, true],
  ["/api/auth/refresh", refresh, false],
  ["/api/auth/logout", logout, true],
  ["/api/auth/logout-all", logoutAll, true],
  ["/api/auth/password", changePassword, true],
  ["/api/auth/bootstrap", bootstrap, false],
];

test("CSRF: every state-changing route refuses a request without the defence", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: "t".repeat(40) });
  const { browser } = await loggedIn(env);
  const json = { email: "ada@example.com", password: PASSWORD };
  const attacks: [string, ReqOptions][] = [
    ["no header, no Origin", { method: "POST", json, csrf: false }],
    ["Origin but no header", { method: "POST", json, csrf: false, headers: { origin: ORIGIN } }],
    ["header from another origin", { method: "POST", json, headers: { origin: "https://evil.example.com" } }],
    ["header, Sec-Fetch-Site cross-site", { method: "POST", json, headers: { "sec-fetch-site": "cross-site" } }],
    ["header, Sec-Fetch-Site same-site", { method: "POST", json, headers: { "sec-fetch-site": "same-site" } }],
    ["form from another origin", { form: json as Record<string, string>, headers: { origin: "https://evil.example.com" } }],
    ["form with Origin null", { form: json as Record<string, string>, headers: { origin: "null" } }],
    ["form with no evidence", { form: json as Record<string, string>, csrf: false }],
  ];
  for (const [path, handler, formRoute] of POSTS) {
    for (const [name, o] of attacks) {
      const res = await call(handler, env, path, { ...o, browser: Object.assign(new Browser(), { jar: new Map(browser.jar) }) });
      assert.equal(res.status, 403, `${path}: ${name}`);
      assert.match((await res.json()).error, /^(cross_origin|csrf_check_failed)$/);
    }
    if (!formRoute) {
      const res = await call(handler, env, path, { form: { a: "b" }, browser: new Browser() });
      assert.equal(res.status, 403, `${path} accepts no HTML form`);
    }
  }
  assert.equal((await call(session, env, "/api/auth/session", { browser })).status, 200, "the session survived every attempt");
  const sameOriginForm = await call(login, env, "/api/auth/login", { form: json as Record<string, string>, csrf: false, headers: { "sec-fetch-site": "same-origin" }, browser: new Browser() });
  assert.equal(sameOriginForm.status, 303, "Sec-Fetch-Site: same-origin is proof enough for a form");
});

test("unconfigured: 503 with a readable hint on every route, never a token", async () => {
  useClock();
  const routes: [string, unknown, ReqOptions][] = [
    ...POSTS.map(([p, h]) => [p, h, { method: "POST", json: {} }] as [string, unknown, ReqOptions]),
    ["/api/auth/session", session, {}],
  ];
  for (const over of [{ AUTH_DB: undefined }, { AUTH_SIGNING_KEY: undefined }, { AUTH_SIGNING_KEY: "{}" }, { AUTH_SIGNING_KEY: KEYS.pub }]) {
    const env = makeEnv(over as never);
    for (const [path, handler, o] of routes) {
      const res = await call(handler, env, path, o);
      assert.equal(res.status, 503, `${path} with ${JSON.stringify(Object.keys(over))}`);
      const body = await res.json();
      assert.equal(body.error, "not_configured");
      assert.match(body.hint, /AUTH_DB|AUTH_SIGNING_KEY/);
      assert.equal(res.headers.getSetCookie().length, 0);
    }
  }
  assert.equal((await call(jwks, makeEnv({ AUTH_SIGNING_KEY: undefined }), "/api/auth/jwks")).status, 503);
});

test("jwks: public keys only, cacheable, no database needed", async () => {
  const res = await call(jwks, makeEnv({ AUTH_DB: undefined } as never), "/api/auth/jwks");
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("cache-control"), "public, max-age=300");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  const { keys } = await res.json();
  assert.equal(keys.length, 1);
  assert.deepEqual(Object.keys(keys[0]).sort(), ["alg", "crv", "kid", "kty", "use", "x", "y"]);
});

test("secrets: the private key and raw refresh tokens are nowhere but where they must be", async () => {
  const clock = useClock();
  const BOOT = "b".repeat(40);
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: BOOT, AUTH_HASH_SALT: "pepper", AUTH_TURNSTILE_SECRET: "turnstile-secret" });
  const logs: string[] = [];
  const saved = { log: console.log, warn: console.warn, error: console.error, info: console.info };
  for (const k of Object.keys(saved) as (keyof typeof saved)[]) console[k] = (...a: unknown[]) => logs.push(a.map(String).join(" "));
  const responses: Response[] = [];
  const refreshTokens = new Set<string>();
  const keep = (b: Browser) => {
    const t = b.jar.get(REFRESH_COOKIE);
    if (t) refreshTokens.add(t);
  };
  const realFetch = globalThis.fetch;
  globalThis.fetch = (async () => Response.json({ success: false })) as typeof fetch;
  try {
    const owner = new Browser();
    const go = async (h: unknown, path: string, o: ReqOptions) => {
      const r = await call(h, env, path, o);
      responses.push(r.clone());
      if (o.browser) keep(o.browser);
      return r;
    };
    await go(bootstrap, "/api/auth/bootstrap", { json: { token: "wrong", email: "o@example.com", password: PASSWORD }, browser: owner });
    await go(bootstrap, "/api/auth/bootstrap", { json: { token: BOOT, email: "o@example.com", password: PASSWORD }, browser: owner });
    const stolen = Object.assign(new Browser(), { jar: new Map(owner.jar) });
    for (let i = 0; i < 3; i++) await go(refresh, "/api/auth/refresh", { method: "POST", browser: owner });
    await go(session, "/api/auth/session", { browser: owner });
    for (let i = 0; i < 4; i++) await go(login, "/api/auth/login", { json: { email: "o@example.com", password: "wrong" + i }, browser: new Browser() });
    clock.advance(60_000);
    await go(refresh, "/api/auth/refresh", { method: "POST", browser: stolen });
    const again = new Browser("198.51.100.77");
    await go(login, "/api/auth/login", { json: { email: "o@example.com", password: PASSWORD }, browser: again });
    await go(changePassword, "/api/auth/password", { json: { current: PASSWORD, password: "another long passphrase" }, browser: again });
    await go(jwks, "/api/auth/jwks", {});
    await go(logout, "/api/auth/logout", { method: "POST", browser: again });
  } finally {
    Object.assign(console, saved);
    globalThis.fetch = realFetch;
  }
  assert.ok(refreshTokens.size >= 4, `collected ${refreshTokens.size} refresh tokens`);

  const forbiddenEverywhere = [KEYS.d, BOOT, PASSWORD, "another long passphrase", "turnstile-secret", "pepper"];
  const bodies: string[] = [];
  for (const r of responses) {
    bodies.push(await r.text());
    for (const [k, v] of r.headers) if (k !== "set-cookie") bodies.push(v);
    for (const c of r.headers.getSetCookie()) for (const s of forbiddenEverywhere) assert.ok(!c.includes(s), "secret in a cookie");
  }
  const tables = env.AUTH_DB.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all() as { name: string }[];
  const rows = tables.flatMap(({ name }) => env.AUTH_DB.sqlite.prepare(`SELECT * FROM ${name}`).all().map((r) => JSON.stringify(r)));
  assert.ok(rows.length >= 5, `only ${rows.length} rows`);
  const haystacks: [string, string[]][] = [
    ["answers", bodies],
    ["logs", logs],
    ["database", rows],
    ["statements", env.AUTH_DB.calls],
  ];
  for (const [where, hay] of haystacks) {
    const all = hay.join("\n");
    for (const s of [...forbiddenEverywhere, ...refreshTokens]) assert.ok(!all.includes(s), `${where} contain a secret (${s.slice(0, 6)}…)`);
    assert.doesNotMatch(all, /198\.51\.100\.77|203\.0\.113\.7/, `${where} contain a client IP`);
  }
});

test("headers: every answer is no-store and carries the security headers", async () => {
  useClock();
  const env = makeEnv();
  const { browser, res } = await loggedIn(env);
  const answers = [
    res,
    await call(session, env, "/api/auth/session", { browser }),
    await call(session, env, "/api/auth/session", {}),
    await call(refresh, env, "/api/auth/refresh", { method: "POST", browser }),
    await call(login, env, "/api/auth/login", { form: { email: "a@example.com", password: "x" }, browser: new Browser() }),
    await call(login, env, "/api/auth/login", { json: {}, csrf: false }),
  ];
  for (const r of answers) {
    assert.equal(r.headers.get("cache-control"), "no-store");
    assert.equal(r.headers.get("x-frame-options"), "DENY");
    assert.equal(r.headers.get("referrer-policy"), "no-referrer");
    assert.equal(r.headers.get("x-robots-tag"), "noindex, nofollow");
  }
});
