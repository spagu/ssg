// End to end (GO-135): a demonstration consumer (test/demo-consumer/admin.ts) that uses only its
// byte-identical copy of the portable verifier accepts the tokens the real `auth` login flow
// issues; a refresh carries changed grants; logout does NOT stop an already issued access token
// before it expires — the staleness CONTRACT.md states. The copy is guarded with copy-guard.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { LEEWAY_SECONDS } from "../functions/api/auth/_jwt.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as logout } from "../functions/api/auth/logout.ts";
import { guardCopies } from "../portable/copy-guard.ts";
import { onRequestGet as demo } from "./demo-consumer/admin.ts";
import { Browser, KEYS, call, makeEnv, useClock } from "./fakes.ts";
import { grantPerms, signedIn } from "./seed.ts";

guardCopies(import.meta.url, { "./demo-consumer/_session-verifier.ts": "session-verifier.ts" });

// As test/mail-copies.test.ts in newsletter-ui does: the verifier's own suites run once more
// against the COPY (a resolve hook redirects their import), so the copy is exercised in full too.
const COPY = new URL("./demo-consumer/_session-verifier.ts", import.meta.url).href;
const SUITES = new URL("./", import.meta.url).href;
registerHooks({
  resolve(specifier, context, next) {
    const fromSuite = context.parentURL?.startsWith(`${SUITES}verifier-`);
    return fromSuite && /^\.\.\/portable\/session-verifier(\.ts)?$/.test(specifier) ? { url: COPY, shortCircuit: true } : next(specifier, context);
  },
});
for (const suite of ["attacks", "keys", "config", "differential"]) await import(`${SUITES}verifier-${suite}.test.ts`);

const DEMO = { DEMO_AUTH_ISSUER: "https://example.com/api/auth", DEMO_AUTH_PUBLIC_KEY: KEYS.pub, DEMO_AUTH_LOGIN_URL: "/auth/login" };
type DemoEnv = Record<string, unknown>;
const visit = (b: Browser | null, env: DemoEnv = DEMO, headers: Record<string, string> = {}) =>
  demo({ request: new Request("https://example.com/demo/admin?tab=1", { headers: { ...(b ? { cookie: b.cookieHeader() } : {}), ...headers } }), env });
const clone = (b: Browser) => Object.assign(new Browser(b.ip), { jar: new Map(b.jar) });

// One clock for both sides: auth's (useClock) and the consumer's (Date.now, which the copy reads).
function clocks(t: { mock: { method: (o: object, m: string, f: () => number) => unknown } }) {
  const c = useClock();
  t.mock.method(Date, "now", () => c.now);
  return c;
}

test("e2e: login → cookie → the consumer accepts; without the permission → 403", async (t) => {
  clocks(t);
  const env = makeEnv();
  const ed = await signedIn(env, "ed@example.com", ["demo:read"]);
  const res = await visit(ed.browser);
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { hello: "ed@example.com" });
  const bo = await signedIn(env, "bo@example.com", ["comments:read"]);
  const no = await visit(bo.browser);
  assert.equal(no.status, 403);
  assert.deepEqual(await no.json(), { error: "forbidden", permission: "demo:read" });
  const owner = await signedIn(env, "root@example.com", "owner");
  assert.equal((await visit(owner.browser)).status, 200, "the owner's * covers demo:read");
});

test("e2e: a granted role reaches the consumer at the next refresh, not before", async (t) => {
  const clock = clocks(t);
  const env = makeEnv();
  const ed = await signedIn(env, "ed@example.com", ["comments:read"]);
  assert.equal((await visit(ed.browser)).status, 403);
  await grantPerms(env, ed.id, ["demo:read"]);
  assert.equal((await visit(ed.browser)).status, 403, "the token in hand is unchanged");
  clock.advance(60_000);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: ed.browser })).status, 200);
  assert.equal((await visit(ed.browser)).status, 200);
});

test("e2e: logout does not invalidate an access token already issued — until it expires", async (t) => {
  const clock = clocks(t);
  const env = makeEnv();
  const ed = await signedIn(env, "ed@example.com", ["demo:read"]);
  const copied = clone(ed.browser); // the token as a copy held elsewhere (another tab, a thief)
  assert.equal((await call(logout, env, "/api/auth/logout", { method: "POST", browser: ed.browser })).status, 200);
  assert.equal(ed.browser.jar.size, 0, "the browser that logged out holds nothing");
  assert.equal((await visit(ed.browser)).status, 401);
  assert.equal((await visit(copied)).status, 200, "STALENESS: the consumer does not ask auth, so the copy still works");
  clock.advance(10 * 60_000 + LEEWAY_SECONDS * 1000 - 1000);
  assert.equal((await visit(copied)).status, 200, "up to AUTH_TOKEN_MINUTES (10) + the 30 s leeway");
  clock.advance(1000);
  const gone = await visit(copied);
  assert.equal(gone.status, 401);
  assert.deepEqual(await gone.json(), { error: "unauthorized", reason: "expired" });
});

test("e2e: no session — a page is sent to log in, an API call gets 401", async (t) => {
  clocks(t);
  const page = await visit(null, DEMO, { accept: "text/html" });
  assert.equal(page.status, 302);
  assert.equal(page.headers.get("location"), "https://example.com/auth/login?next=%2Fdemo%2Fadmin%3Ftab%3D1");
  const api = await visit(null);
  assert.equal(api.status, 401);
  assert.deepEqual(await api.json(), { error: "unauthorized", reason: "no_session" });
});

test("e2e: modes — half-configured is a loud 503; no DEMO_AUTH_* falls to the password", async (t) => {
  clocks(t);
  const env = makeEnv();
  const ed = await signedIn(env, "ed@example.com", ["demo:read"]);
  const half = await visit(ed.browser, { DEMO_AUTH_ISSUER: DEMO.DEMO_AUTH_ISSUER, DEMO_ADMIN_PASSWORD: "pw" });
  assert.equal(half.status, 503, "never a silent fall-through to the password");
  assert.match((await half.json()).detail, /DEMO_AUTH_LOGIN_URL is not set/);
  const basic = { authorization: `Basic ${btoa("admin:pw")}` };
  const pw = await visit(null, { DEMO_ADMIN_PASSWORD: "pw" }, basic);
  assert.deepEqual(await pw.json(), { hello: "password" });
  assert.equal((await visit(null, { DEMO_ADMIN_PASSWORD: "other" }, basic)).status, 401);
  assert.equal((await visit(null, { DEMO_ADMIN_PASSWORD: "pw" })).status, 401, "no credentials");
  assert.equal((await visit(ed.browser, {})).status, 503, "no mode at all: closed");
});
