// GO-140: the edges — races lost between a check and a write, blocked
// accounts, unreadable bodies, an unnamed or used factor on the pages, a QR
// code too long to draw, a misconfigured worker.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as breakGlass } from "../functions/api/auth/mfa/break-glass.ts";
import { onRequestPost as reset } from "../functions/api/auth/mfa/reset.ts";
import { onRequestPost as removeFactorForm } from "../functions/api/auth/mfa/factors/[id].ts";
import { onRequestPost as options } from "../functions/api/auth/mfa/passkey/options.ts";
import { onRequestPost as passkeyRegister } from "../functions/api/auth/mfa/passkey/register.ts";
import { onRequestGet as challenge } from "../functions/auth/mfa/index.ts";
import { onRequestGet as enrol } from "../functions/auth/mfa/enrol.ts";
import { onRequestGet as stepUpPage } from "../functions/auth/mfa/step-up.ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { onRequestGet as panels } from "../functions/auth/index.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { checkTotp } from "../functions/api/auth/_mfa-verify.ts";
import { checkAssertion, issueChallenge, register, registerOptions } from "../functions/api/auth/_webauthn-store.ts";
import type { User } from "../functions/api/auth/_users.ts";
import { KEYS, T0, call, makeEnv, useClock } from "./fakes.ts";
import { SoftAuthenticator, code, enrolTotp, mfaEnv, signIn, totpConfirm, totpStart, verify } from "./mfa.ts";
import { roleId, seedUser } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const REQ = new Request("https://example.com/");
const TOKEN = "z".repeat(40);

// once: the next statement starting with `prefix` is replaced by `sql`.
function once(env: Env, prefix: string, sql: string) {
  const db = env.AUTH_DB as unknown as { prepare(s: string): unknown };
  const real = db.prepare.bind(db);
  db.prepare = (s: string) => {
    if (!s.startsWith(prefix)) return real(s);
    db.prepare = real;
    return real(sql);
  };
}

async function owner(env: Env, email = "ada@example.com") {
  const id = await seedUser(env, email);
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  return id;
}

test("pages: a passkey without a name and a used factor on the account page; the step-up link on 'Your panels'", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  const key = await new SoftAuthenticator().init();
  const c = (await (await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "register" }, browser })).json()).publicKey.challenge;
  await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { json: { credential: await key.create(c) }, browser });
  clock.advance(1000); // the passkey is the older factor: checked (and skipped) first
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(60_000);
  // A code checked past the passkey row (the loop skips it) marks the app used.
  const id = (env.AUTH_DB.sqlite.prepare("SELECT id FROM users").get() as { id: string }).id;
  assert.equal(await checkTotp(env, id, await code(secret, clock.now), clock.now), true);
  const html = await (await call(account, env, "/auth/account?lang=en", { browser })).text();
  assert.match(html, /<td data-label="Factor">Passkey<\/td>/);
  assert.match(html, /Authenticator app — Phone<\/td><td data-label="Added">[^<]+<\/td><td data-label="Last active">Nov/);
  // A session that never passed a factor, on an account that has one: step up.
  const old = await signIn({ ...env, AUTH_MFA_KEY: undefined }, "ada@example.com");
  const page = await (await call(panels, env, "/auth/?lang=en", { browser: old.browser })).text();
  assert.match(page, /<a href="\/auth\/mfa\/step-up\?next=%2Fauth%2F">Confirm your second factor<\/a>/);
  // The step-up page without an error; the challenge page's recovery fold with one.
  const up = await (await call(stepUpPage, env, "/auth/mfa/step-up?lang=en", { browser: old.browser })).text();
  assert.ok(!up.includes('id="notice"'));
  const pending = (await signIn({ ...env, AUTH_MFA_RESET_TOKEN: TOKEN }, "ada@example.com")).browser;
  const recover = await (await call(challenge, { ...env, AUTH_MFA_RESET_TOKEN: TOKEN }, "/auth/mfa?use=recover&error=invalid_token&lang=en", { browser: pending })).text();
  assert.match(recover, /<details class="card fold" open><summary>Lost your phone or key\?/);
  assert.match(recover, /id="mfa-token"[^>]*aria-invalid="true"/);
  assert.match(await (await call(enrol, env, "/auth/mfa/enrol?error=invalid_passkey&lang=en", { browser })).text(), /The passkey could not be added/);
});

test("enrolment page: an address too long for a QR code still gets the key to type", async () => {
  useClock();
  const env = mfaEnv();
  const email = "%".repeat(230) + "@example.com";
  await seedUser(env, email);
  const { browser } = await signIn(env, email);
  const loc = (await call(totpStart, env, "/api/auth/mfa/totp/start", { form: { next: "/auth/" }, browser })).headers.get("location") as string;
  const html = await (await call(enrol, env, loc, { browser })).text();
  assert.ok(!html.includes("<svg"));
  assert.match(html, /<code class="secret">/);
});

test("the pages and routes without a database answer 503; reset while the second factor is off is absent", async () => {
  const bare = { AUTH_SIGNING_KEY: KEYS.priv } as unknown as Env;
  for (const page of [challenge, enrol, stepUpPage]) assert.equal((await call(page, bare, "/auth/mfa")).status, 503);
  useClock();
  const env = makeEnv();
  await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  assert.deepEqual([(await call(reset, env, "/api/auth/mfa/reset", { json: { userId: "x" }, browser })).status], [404]);
});

test("races and odd input: confirm, counter, break-glass token, an unreadable body, a blocked account", async () => {
  const clock = useClock();
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: TOKEN });
  const id = await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  // Confirmed meanwhile.
  const s = await (await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser })).json();
  once(env, "UPDATE mfa_factors SET confirmed_at", "UPDATE mfa_factors SET confirmed_at = ?, last_step = ?, name = ? WHERE 0 AND id = ? AND user_id = ? AND confirmed_at IS NULL");
  const lost = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: await code(s.secret, clock.now) }, browser });
  assert.deepEqual([lost.status, (await lost.json()).error], [410, "enrolment_expired"]);
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(60_000);
  // The passkey counter moved by another request between the check and the write.
  const user = { id, email: "ada@example.com", display_name: "" } as User;
  const key = await new SoftAuthenticator().init();
  const opts = await registerOptions(env, REQ, user, "s", clock.now);
  assert.equal(opts.user.displayName, "ada@example.com", "no name: the address");
  assert.ok(await register(env, REQ, id, "s", { response: { ...(await key.create(opts.challenge)).response, transports: "usb" } }, "", clock.now), "transports not a list: none");
  once(env, "UPDATE mfa_factors SET sign_count", "UPDATE mfa_factors SET sign_count = ?, last_used_at = ? WHERE 0 AND id = ? AND sign_count = ?");
  assert.equal(await checkAssertion(env, REQ, id, "signin", "p", await key.get(await issueChallenge(env, id, "signin", "p", clock.now)), clock.now), null);
  // The break-glass value spent by another request a moment before.
  const pending = await signIn(env, "ada@example.com");
  once(env, "INSERT OR IGNORE INTO mfa_reset_tokens", "INSERT OR IGNORE INTO mfa_reset_tokens (token_hash, user_id, used_at) SELECT ?, ?, ? WHERE 0");
  assert.equal((await call(breakGlass, env, "/api/auth/mfa/break-glass", { json: { token: TOKEN }, browser: pending.browser })).status, 404);
  // An unreadable body is no answer at all.
  const bad = await call(verify, env, "/api/auth/mfa/verify", { method: "POST", browser: pending.browser, headers: { "content-type": "application/json", "x-ssg-auth": "1" } });
  assert.deepEqual([bad.status, (await bad.json()).error], [401, "invalid_code"], "read as an empty answer");
  void secret;
  // A pending step whose account was blocked meanwhile is gone.
  env.AUTH_DB.sqlite.exec("DROP TRIGGER IF EXISTS auth_owner_user_update");
  env.AUTH_DB.sqlite.prepare("UPDATE users SET status = 'blocked' WHERE id = ?").run(id);
  const blocked = await call(verify, env, "/api/auth/mfa/verify", { json: { code: "123456" }, browser: pending.browser });
  assert.equal(blocked.status, 401);
});

test("break-glass: a form without `next` goes to enrolment for the panels; permissions too large are said", async () => {
  const clock = useClock();
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: TOKEN });
  const id = await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  await enrolTotp(env, browser, clock.now);
  clock.advance(60_000);
  const b = (await signIn(env, "ada@example.com")).browser;
  const form = await call(breakGlass, env, "/api/auth/mfa/break-glass", { form: { token: TOKEN }, browser: b });
  assert.equal(form.headers.get("location"), "/auth/mfa/enrol?required=1&next=%2Fauth%2F");
  // Too large: a fresh owner with a huge role, a new break-glass value.
  const env2 = mfaEnv({ AUTH_MFA_RESET_TOKEN: TOKEN + "2" });
  const id2 = await owner(env2);
  const b2 = (await signIn(env2, "ada@example.com")).browser;
  await enrolTotp(env2, b2, clock.now);
  clock.advance(60_000);
  const big = (await createRole(env2, { name: "big", description: "", permissions: Array.from({ length: 60 }, (_, i) => `w${i}:` + "a".repeat(60)) }, T0)) as Role;
  await addGrant(env2, big.id, { userId: id2 }, "t", T0);
  const quiet = console.error;
  console.error = () => {};
  try {
    const p2 = (await signIn(env2, "ada@example.com")).browser;
    const r = await call(breakGlass, env2, "/api/auth/mfa/break-glass", { json: { token: TOKEN + "2" }, browser: p2 });
    assert.deepEqual([r.status, (await r.json()).error], [500, "permissions_too_large"]);
  } finally {
    console.error = quiet;
  }
  assert.ok(id);
});

test("removing the last factor through the form while a role needs it: back to the account page with the reason", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await owner(env);
  const { browser } = await signIn(env, "ada@example.com");
  const { factorId } = await enrolTotp(env, browser, clock.now);
  const res = await call(removeFactorForm, env, `/api/auth/mfa/factors/${factorId}`, { form: { next: "/auth/account" }, browser, params: { id: factorId } });
  assert.equal(res.headers.get("location"), "/auth/account?error=factor_required");
});
