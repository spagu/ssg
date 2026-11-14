// Password reset links (GO-134): issued by an administrator, used once,
// expiring, ending every session and clearing the lock; "forgot password"
// only with mail, the same answer for every address, rate-limited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as resetLink } from "../functions/api/auth/admin/users/[id]/reset-link.ts";
import { FORGOT_PER_ACCOUNT, FORGOT_PER_IP, onRequestPost as forgot } from "../functions/api/auth/forgot.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as reset } from "../functions/api/auth/reset.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { Browser, PASSWORD, call, counting, makeEnv, req, useClock } from "./fakes.ts";
import { captureMail, linkIn, rows, tokenOf, withMail } from "./go134.ts";
import { loggedIn, seedUser, signedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const NEW = "a brand new passphrase";
const linkFor = async (env: Env, browser: Browser, id: string, json?: unknown) => {
  const res = await call(resetLink, env, `/api/auth/admin/users/${id}/reset-link`, { method: "POST", browser, params: { id }, ...(json ? { json } : {}) });
  return { status: res.status, body: await res.json() };
};
const use = (env: Env, token: string, password = NEW) => call(reset, env, "/api/auth/reset", { json: { token, password } });
const loginAs = (env: Env, email: string, password: string) => call(login, env, "/api/auth/login", { json: { email, password }, browser: new Browser("198.51.100.9") });

test("reset link: an administrator gets a one-time link; using it sets the password, ends every session, clears the lock", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:read"]);
  const ada = await loggedIn(env, "ada@example.com");
  await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: new Browser() });
  for (let i = 0; i < 5; i++) await loginAs(env, "ada@example.com", "wrong guess number " + i);
  assert.equal((await loginAs(env, "ada@example.com", PASSWORD)).status, 429, "locked before the reset");
  const { status, body } = await linkFor(env, admin.browser, ada.id);
  assert.equal(status, 201);
  assert.equal(body.delivery, "link");
  assert.equal(body.expiresAt, new Date(Date.parse("2026-11-13T10:00:00.000Z") + 3600_000).toISOString());
  assert.match(body.link, /^https:\/\/example\.com\/auth\/reset\?token=[A-Za-z0-9_-]{43}&lang=en$/);
  assert.ok(!JSON.stringify(rows(env, "password_resets")).includes(tokenOf(body.link)));
  const res = await use(env, tokenOf(body.link));
  assert.deepEqual(await res.json(), { ok: true, revoked: 2 });
  assert.equal((await call(session, env, "/api/auth/session", { browser: ada.browser })).status, 401, "her sessions ended");
  assert.equal((await loginAs(env, "ada@example.com", PASSWORD)).status, 401, "the old password is gone");
  assert.equal((await loginAs(env, "ada@example.com", NEW)).status, 200, "the lock was cleared");
  assert.deepEqual(await (await use(env, tokenOf(body.link), "another fine passphrase")).json(), { error: "reset_invalid" }, "used once");
});

test("reset link: escalation — an administrator cannot take over an account that holds more", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin"]);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const r = await linkFor(env, admin.browser, owner.id);
  assert.deepEqual([r.status, r.body.error, r.body.permissions], [403, "escalation", ["*"]]);
  assert.equal((await linkFor(env, owner.browser, admin.id)).status, 201);
  assert.equal((await linkFor(env, admin.browser, "nobody")).status, 404);
  assert.equal((await linkFor(env, admin.browser, admin.id, { lang: "xx" })).status, 400);
  assert.equal((await linkFor(env, admin.browser, admin.id, { delivery: "fax" })).status, 400);
  assert.equal((await linkFor(env, admin.browser, admin.id, { delivery: "mail" })).body.error, "mail_not_configured");
});

test("reset link: by mail in the chosen language; a newer link retires the older; a refused mail is 502", async () => {
  useClock();
  const env = withMail({ AUTH_RESET_MINUTES: "30" });
  const mail = captureMail();
  try {
    const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:read"]);
    const id = await seedUser(env, "ada@example.com");
    const first = await linkFor(env, admin.browser, id, { lang: "fr" });
    assert.deepEqual(first.body, { expiresAt: first.body.expiresAt, delivery: "mail" });
    assert.equal(mail.sent[0].subject, "Nouveau mot de passe pour example.com");
    assert.match(mail.sent[0].text, /pendant 30 minutes/);
    const old = tokenOf(linkIn(mail.sent[0].text));
    await linkFor(env, admin.browser, id);
    assert.equal((await use(env, old)).status, 410, "retired by the newer link");
    assert.equal((await use(env, tokenOf(linkIn(mail.sent[1].text)))).status, 200);
    mail.status = 429;
    const warn = console.warn;
    console.warn = () => {};
    try {
      assert.deepEqual((await linkFor(env, admin.browser, id)).body.error, "mail_failed");
    } finally {
      console.warn = warn;
    }
  } finally {
    mail.restore();
  }
});

test("reset: expiry, a refused password keeps the link, the form path, two at once", async () => {
  const clock = useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:read"]);
  const id = await seedUser(env, "ada@example.com");
  const t1 = tokenOf((await linkFor(env, admin.browser, id)).body.link);
  assert.deepEqual(await (await use(env, t1, "short")).json(), { error: "password_too_short" });
  const form = (fields: Record<string, string>) => call(reset, env, "/api/auth/reset", { form: fields });
  assert.equal((await form({ token: t1, password: "ada@example.com" })).headers.get("location"), `/auth/reset?token=${t1}&error=password_equals_email`);
  assert.equal((await form({ token: "nope", password: NEW })).headers.get("location"), "/auth/reset?error=reset_invalid");
  assert.equal((await form({ token: t1, password: NEW })).headers.get("location"), "/auth/login?ok=password_reset");
  const t2 = tokenOf((await linkFor(env, admin.browser, id)).body.link);
  const both = await Promise.all([use(env, t2, "first parallel passphrase"), use(env, t2, "second parallel passphrase")]);
  assert.deepEqual(both.map((r) => r.status).sort(), [200, 410]);
  const t3 = tokenOf((await linkFor(env, admin.browser, id)).body.link);
  clock.advance(60 * 60_000);
  assert.equal((await use(env, t3)).status, 410, "expired at exactly 60 minutes");
});

test("reset: the runtime refusing the hash cost is 503 and keeps the link", async () => {
  useClock();
  const env = makeEnv({ AUTH_PBKDF2_ITERATIONS: "200000" });
  const c = counting(100_000);
  try {
    const admin = await signedIn(env, "owner@example.com", "owner");
    const id = await seedUser(env, "ada@example.com");
    const token = tokenOf((await linkFor(env, admin.browser, id)).body.link);
    assert.equal((await use(env, token)).status, 503);
    assert.equal(rows(env, "password_resets").filter((r) => r.used_at === null).length, 1);
  } finally {
    c.restore();
  }
});

test("forgot: absent without a transport (404), an error with a broken one (503)", async () => {
  useClock();
  assert.deepEqual(await (await call(forgot, makeEnv(), "/api/auth/forgot", { json: { email: "a@example.com" } })).json(), { error: "not_found" });
  const broken = await call(forgot, makeEnv({ AUTH_SMTP_HOST: "smtp.example.com" }), "/api/auth/forgot", { json: { email: "a@example.com" } });
  assert.deepEqual([broken.status, (await broken.json()).error], [503, "mail_misconfigured"]);
});

test("forgot: the same answer whatever the address; mail only to an active account; the link works", async () => {
  useClock();
  const env = withMail();
  const mail = captureMail();
  try {
    await seedUser(env, "ada@example.com");
    await seedUser(env, "blocked@example.com");
    env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'blocked' WHERE email = 'blocked@example.com'");
    const seen = new Set<string>();
    for (const email of ["ada@example.com", "nobody@example.com", "blocked@example.com"]) {
      const res = await call(forgot, env, "/api/auth/forgot", { json: { email }, browser: new Browser(`192.0.2.${seen.size + 1}`) });
      seen.add(`${res.status} ${await res.text()} ${[...res.headers.keys()].join(",")}`);
    }
    assert.equal(seen.size, 1, [...seen].join("\n"));
    assert.deepEqual(mail.sent.map((m) => m.to), ["ada@example.com"]);
    assert.equal((await use(env, tokenOf(linkIn(mail.sent[0].text)))).status, 200);
    const sent = await call(forgot, env, "/api/auth/forgot", { form: { email: "nobody@example.com" } });
    assert.equal(sent.headers.get("location"), "/auth/forgot?sent=1");
    assert.equal((await call(forgot, env, "/api/auth/forgot", { form: { email: "x" } })).headers.get("location"), "/auth/forgot?error=invalid_email");
    assert.equal((await call(forgot, env, "/api/auth/forgot", { json: { email: "x" } })).status, 400);
  } finally {
    mail.restore();
  }
});

test("forgot: the work runs after the answer (waitUntil); failures are logged without the link", async () => {
  useClock();
  const env = withMail();
  const mail = captureMail();
  mail.status = 500;
  const warn = console.warn;
  const error = console.error;
  const logged: string[] = [];
  console.warn = console.error = (...a: unknown[]) => void logged.push(a.join(" "));
  try {
    await seedUser(env, "ada@example.com");
    const later: Promise<unknown>[] = [];
    const res = await forgot({ request: req("/api/auth/forgot", { json: { email: "ada@example.com" } }), env, waitUntil: (p) => void later.push(p) });
    assert.equal(res.status, 200);
    assert.equal(later.length, 1);
    await Promise.all(later);
    assert.match(logged.join("\n"), /reset mail not sent: resend returned 500/);
    assert.ok(!logged.join("\n").includes(tokenOf(linkIn(mail.sent[0].text))));
    env.AUTH_DB.state.failOn = /SELECT \* FROM users WHERE email/;
    await forgot({ request: req("/api/auth/forgot", { json: { email: "ada@example.com" } }), env, waitUntil: (p) => void later.push(p) });
    await Promise.all(later);
    assert.match(logged.join("\n"), /forgot-password: Error: D1_ERROR/);
  } finally {
    console.warn = warn;
    console.error = error;
    mail.restore();
  }
});

test("forgot: per address and per IP limits, counted for every address, then one an hour", async () => {
  const clock = useClock();
  const env = withMail();
  const mail = captureMail();
  try {
    const ask = (email: string, ip: string) => call(forgot, env, "/api/auth/forgot", { json: { email }, browser: new Browser(ip) });
    for (let i = 0; i < FORGOT_PER_ACCOUNT; i++) assert.equal((await ask("nobody@example.com", `192.0.2.${i}`)).status, 200);
    const limited = await ask("nobody@example.com", "192.0.2.99");
    assert.deepEqual([limited.status, (await limited.json()).error, limited.headers.get("retry-after")], [429, "too_many_requests", "3600"]);
    assert.equal((await call(forgot, env, "/api/auth/forgot", { form: { email: "nobody@example.com" } })).headers.get("location"), "/auth/forgot?error=too_many_requests");
    for (let i = 0; i < FORGOT_PER_IP; i++) assert.equal((await ask(`p${i}@example.com`, "198.51.100.1")).status, 200);
    assert.equal((await ask("fresh@example.com", "198.51.100.1")).status, 429, "per IP");
    clock.advance(3600_000);
    assert.equal((await ask("nobody@example.com", "192.0.2.50")).status, 200, "one more after an hour");
    assert.equal((await ask("nobody@example.com", "192.0.2.51")).status, 429);
    assert.equal(mail.sent.length, 0, "no account, no mail");
  } finally {
    mail.restore();
  }
});
