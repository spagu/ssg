// GO-141: POST /api/auth/token — a service secret for a short access token.
// Success, every failure alike, expiry, revocation, lockouts, cookies ignored.
import { test } from "node:test";
import assert from "node:assert/strict";
import { LEEWAY_SECONDS } from "../functions/api/auth/_jwt.ts";
import { TOUCH_MS, parseSecret, serviceSettings } from "../functions/api/auth/_service-tokens.ts";
import { onRequestDelete as revoke } from "../functions/api/auth/admin/service-accounts/[id]/tokens/[tokenId].ts";
import { onRequestPut as updateAccount } from "../functions/api/auth/admin/service-accounts/[id]/index.ts";
import { configFrom, verifySession } from "../portable/session-verifier.ts";
import { KEYS, T0, call, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, claimsOf, secretFor, swap } from "./service.ts";

type Env = ReturnType<typeof makeEnv>;

async function setup(perms = ["web-push:notify", "newsletter:read"], over: Record<string, string> = {}) {
  const clock = useClock();
  const env = makeEnv(over);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, perms);
  const secret = await secretFor(env, owner.browser, id);
  return { clock, env, owner, id, secret, tokenId: (parseSecret(secret) as { id: string }).id };
}

const consumer = configFrom({ X_AUTH_ISSUER: "https://example.com/api/auth", X_AUTH_PUBLIC_KEY: KEYS.pub, X_AUTH_LOGIN_URL: "/auth/login" }, "X")!;
const asBearer = (jwt: string) => new Request("https://example.com/api/x", { method: "POST", headers: { authorization: `Bearer ${jwt}` } });

test("exchange: a valid secret gives a 10-minute service token, no cookies, no refresh token", async () => {
  const { env, id, secret, tokenId, clock } = await setup();
  const res = await swap(env, `Bearer ${secret}`);
  assert.equal(res.status, 200);
  assert.deepEqual(res.headers.getSetCookie(), []);
  assert.equal(res.headers.get("cache-control"), "no-store");
  const body = await res.json();
  assert.deepEqual(Object.keys(body).sort(), ["accessToken", "expiresAt", "expiresIn", "permissions", "tokenType"]);
  assert.equal(body.tokenType, "Bearer");
  assert.equal(body.expiresIn, 600);
  assert.equal(body.expiresAt, new Date(T0 + 600_000).toISOString());
  assert.deepEqual(body.permissions, ["newsletter:read", "web-push:notify"]);
  const c = claimsOf(body.accessToken);
  assert.deepEqual([c.kind, c.sub, c.sid, c.email, c.exp - c.iat, c.perms], ["service", id, tokenId, "", 600, body.permissions]);
  const v = await verifySession(asBearer(body.accessToken), consumer, { now: () => clock.now });
  assert.ok(v.ok && v.kind === "service" && v.user.id === id);
});

test("exchange: the lifetime follows AUTH_SERVICE_ACCESS_MINUTES; settings defaults and caps", async () => {
  const { env, secret } = await setup(undefined, { AUTH_SERVICE_ACCESS_MINUTES: "2" });
  assert.equal((await (await swap(env, `Bearer ${secret}`)).json()).expiresIn, 120);
  assert.deepEqual(serviceSettings({} as never), { maxDays: 365, defaultDays: 90, accessSeconds: 600 });
  assert.deepEqual(serviceSettings({ AUTH_SERVICE_TOKEN_MAX_DAYS: "30" } as never), { maxDays: 30, defaultDays: 30, accessSeconds: 600 });
});

test("exchange: missing, malformed, unknown, revoked, expired and disabled answer exactly alike", async () => {
  const { env, owner, id, secret, clock } = await setup();
  const expired = await secretFor(env, owner.browser, id, { expiresInDays: 1 });
  const revoked = await secretFor(env, owner.browser, id);
  await call(revoke, env, "/x", { method: "DELETE", browser: owner.browser, params: { id, tokenId: (parseSecret(revoked) as { id: string }).id } });
  const other = await account(env, owner.browser, ["web-push:notify"]);
  const disabled = await secretFor(env, owner.browser, other);
  await call(updateAccount, env, "/x", { method: "PUT", browser: owner.browser, params: { id: other }, json: { status: "disabled" } });
  clock.advance(86_400_000);
  const unknown = secret.slice(0, -4) + (secret.endsWith("AAAA") ? "BBBB" : "AAAA");
  const cases: [string, string | null][] = [
    ["missing", null],
    ["basic", `Basic ${secret}`],
    ["empty bearer", "Bearer "],
    ["malformed", "Bearer ssgsvc_nothing"],
    ["user-shaped", "Bearer eyJhbGciOi.x.y"],
    ["unknown", `Bearer ${unknown}`],
    ["revoked", `Bearer ${revoked}`],
    ["expired", `Bearer ${expired}`],
    ["disabled account", `Bearer ${disabled}`],
  ];
  const answers = new Set<string>();
  for (const [i, [name, auth]] of cases.entries()) {
    const res = await swap(env, auth, {}, `192.0.2.${i + 10}`); // a fresh address each: no lockout in the way
    assert.equal(res.status, 401, name);
    answers.add(JSON.stringify([res.status, [...res.headers].sort(), await res.text()]));
  }
  assert.equal(answers.size, 1, [...answers].join("\n"));
  assert.ok([...answers][0].includes('error=\\"invalid_token\\"'));
  assert.equal((await swap(env, `Bearer ${secret}`, {}, "192.0.2.200")).status, 200, "the live one still works");
});

test("expiry: a token works until its expiresAt and not a moment after", async () => {
  const { env, owner, id, clock } = await setup();
  const s = await secretFor(env, owner.browser, id, { expiresInDays: 2 });
  clock.set(T0 + 2 * 86_400_000 - 1);
  assert.equal((await swap(env, `Bearer ${s}`)).status, 200);
  clock.set(T0 + 2 * 86_400_000);
  assert.equal((await swap(env, `Bearer ${s}`)).status, 401);
});

test("revocation: the next exchange fails; an issued token dies within 10 min + 30 s", async () => {
  const { env, owner, id, secret, tokenId, clock } = await setup();
  const jwt = (await (await swap(env, `Bearer ${secret}`)).json()).accessToken as string;
  const res = await call(revoke, env, "/x", { method: "DELETE", browser: owner.browser, params: { id, tokenId } });
  assert.deepEqual([res.status, await res.json()], [200, { ok: true }]);
  assert.equal((await swap(env, `Bearer ${secret}`)).status, 401, "refused at the very next exchange");
  // Workers never ask auth: the token already issued lives out its lifetime plus the leeway.
  const at = (s: number) => verifySession(asBearer(jwt), consumer, { now: () => T0 + s * 1000 });
  assert.ok((await at(600 + LEEWAY_SECONDS - 1)).ok, "still accepted at 10 min 29 s");
  assert.deepEqual(await at(600 + LEEWAY_SECONDS), { ok: false, reason: "expired" }, "refused from 10 min 30 s");
  assert.equal(clock.now, T0);
});

test("permissions are read at each exchange: a removed role shows in the next token", async () => {
  const { env, id, secret } = await setup(["web-push:notify"]);
  assert.deepEqual((await (await swap(env, `Bearer ${secret}`)).json()).permissions, ["web-push:notify"]);
  env.AUTH_DB.sqlite.prepare("DELETE FROM grants WHERE user_id = ?").run(id);
  assert.deepEqual((await (await swap(env, `Bearer ${secret}`)).json()).permissions, []);
});

test("cookies are ignored: a person's session neither authenticates nor changes the answer", async () => {
  const { env, owner, secret } = await setup();
  const cookie = owner.browser.cookieHeader();
  const none = await swap(env, null, { cookie });
  assert.equal(none.status, 401, "a session cookie alone is no credential here");
  const both = await swap(env, `Bearer ${secret}`, { cookie, origin: "https://evil.example.com", "sec-fetch-site": "cross-site" });
  assert.equal(both.status, 200, "no CSRF check: there is no ambient credential to abuse");
  assert.equal(claimsOf((await both.json()).accessToken).kind, "service");
});

// Security review SR-08: this test asserted a lock per token id ("after 5
// failures even the right secret waits"). The id is public (the panel, the
// audit log, the secret's own prefix) and the 256-bit secret cannot be
// guessed, so that lock protected nothing and let anyone lock a working
// token out. The rule is now the opposite, asserted here.
test("no lockout per token id: failures from many addresses never lock the right secret out", async () => {
  const { env, secret } = await setup();
  const wrong = secret.slice(0, -1) + (secret.endsWith("A") ? "B" : "A");
  for (let i = 0; i < 6; i++) assert.equal((await swap(env, `Bearer ${wrong}`, {}, `192.0.2.${i}`)).status, 401);
  assert.equal((await swap(env, `Bearer ${secret}`, {}, "192.0.2.99")).status, 200);
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM login_attempts WHERE kind = 'service_token'").get()?.n, 0, "no id counter at all");
});

test("lockout per IP: 20 failures from one address lock it, whatever the secret", async () => {
  const { env, secret } = await setup();
  for (let i = 0; i < 20; i++) assert.equal((await swap(env, "Bearer junk")).status, 401);
  assert.equal((await swap(env, `Bearer ${secret}`)).status, 429);
  assert.equal((await swap(env, `Bearer ${secret}`, {}, "192.0.2.50")).status, 200, "another address is not affected");
  const kinds = env.AUTH_DB.sqlite.prepare("SELECT DISTINCT kind FROM login_attempts").all().map((r: unknown) => (r as { kind: string }).kind);
  assert.deepEqual(kinds, ["service_ip"], "separate from the login's counters");
});

test("last use is written at most once per 5 minutes", async () => {
  const { env, secret, tokenId, clock } = await setup();
  const lastUsed = () => env.AUTH_DB.sqlite.prepare("SELECT last_used_at FROM service_tokens WHERE id = ?").get(tokenId)?.last_used_at;
  assert.equal(lastUsed(), null);
  await swap(env, `Bearer ${secret}`);
  assert.equal(lastUsed(), new Date(T0).toISOString());
  clock.advance(TOUCH_MS - 1);
  await swap(env, `Bearer ${secret}`);
  assert.equal(lastUsed(), new Date(T0).toISOString(), "not rewritten within the interval");
  clock.advance(1);
  await swap(env, `Bearer ${secret}`);
  assert.equal(lastUsed(), new Date(T0 + TOUCH_MS).toISOString());
});

test("a token over what the verifier accepts is refused with the reason, nothing issued", async (t) => {
  const { env, owner } = await setup([]);
  const many = (m: string) => Array.from({ length: 64 }, (_, i) => `${m}-${String(i).padStart(2, "0")}:${"a".repeat(50)}`);
  const id = await account(env, owner.browser, many("one"));
  const other = await account(env, owner.browser, many("two"));
  env.AUTH_DB.sqlite.prepare("UPDATE grants SET user_id = ? WHERE user_id = ?").run(id, other);
  const s = await secretFor(env, owner.browser, id, { expiresInDays: 1 });
  const errors: string[] = [];
  t.mock.method(console, "error", (m: string) => errors.push(m));
  const res = await swap(env, `Bearer ${s}`);
  assert.equal(res.status, 500);
  const body = await res.json();
  assert.equal(body.error, "permissions_too_large");
  assert.match(body.detail, /128 permissions/);
  assert.equal(errors.length, 1);
  assert.ok(!errors[0].includes(s.slice(20)) && !JSON.stringify(body).includes(s.slice(20)));
});

test("not configured: no database → 503 before anything is read", async () => {
  const env = makeEnv({ AUTH_DB: undefined as never });
  assert.equal((await swap(env as Env, "Bearer x")).status, 503);
});
