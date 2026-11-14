// GO-140: the pages — the challenge, step-up and enrolment pages, the account
// page's section, the backup-codes page: five languages, no inline script or
// style, the code fields' attributes (WCAG 3.3.8), passkeys only as an addition.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as challenge } from "../functions/auth/mfa/index.ts";
import { onRequestGet as enrol } from "../functions/auth/mfa/enrol.ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { onRequestPost as options } from "../functions/api/auth/mfa/passkey/options.ts";
import { onRequestPost as passkeyRegister } from "../functions/api/auth/mfa/passkey/register.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { LANGS } from "../functions/api/auth/_i18n.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { SoftAuthenticator, enrolTotp, mfaEnv, signIn, totpStart } from "./mfa.ts";
import { roleId, seedUser } from "./seed.ts";

const clean = (html: string) => {
  assert.ok(!/<script(?![^>]*src=)/.test(html), "no inline script");
  assert.ok(!/<style|style=/.test(html), "no inline style");
  assert.ok(!/ on[a-z]+=/.test(html), "no inline handler");
};

async function setup(env = mfaEnv()) {
  const clock = useClock();
  const id = await seedUser(env, "ada@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "ada@example.com");
  return { clock, env, id, browser };
}

test("challenge page: in five languages; the code field for paste and autofill; backup and help folded; no handle → login", async () => {
  const { env, clock, browser } = await setup(mfaEnv({ AUTH_MFA_RESET_TOKEN: "x".repeat(40) }));
  await enrolTotp(env, browser, clock.now);
  clock.advance(60_000);
  const pending = (await signIn(env, "ada@example.com")).browser;
  for (const lang of LANGS) {
    const res = await call(challenge, env, `/auth/mfa?lang=${lang}`, { browser: pending });
    assert.equal(res.status, 200);
    const html = await res.text();
    clean(html);
    assert.match(html, new RegExp(`<html lang="${lang}">`));
    assert.match(html, /<input id="mfa-code" name="code" type="text" inputmode="numeric" autocomplete="one-time-code" spellcheck="false" maxlength="64" required aria-describedby="mfa-code-hint">/);
    assert.match(html, /<details class="card fold"><summary>/);
    assert.match(html, /action="\/api\/auth\/mfa\/break-glass"/);
    assert.ok(!html.includes('data-passkey="get"'), "no passkey registered: no passkey block");
    assert.ok(html.includes("ada@example.com"));
  }
  const err = await (await call(challenge, env, "/auth/mfa?error=invalid_code&use=backup&lang=en", { browser: pending })).text();
  assert.match(err, /<details class="card fold" open><summary>Use a backup code/);
  assert.match(err, /id="mfa-backup"[^>]*aria-describedby="mfa-backup-hint notice" aria-invalid="true"/);
  assert.match(err, /<title>Error: Confirm it is you<\/title>/);
  const none = await call(challenge, env, "/auth/mfa");
  assert.equal(none.headers.get("location"), "/auth/login?error=mfa_expired");
  const off = await call(challenge, makeEnv(), "/auth/mfa", { browser: pending });
  assert.equal(off.headers.get("location"), "/auth/login?error=mfa_expired");
});

test("challenge page with a passkey only: a hidden passkey form with its own challenge, the backup code, and a no-script note", async () => {
  const { env, browser } = await setup();
  const key = await new SoftAuthenticator().init();
  const c = (await (await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "register" }, browser })).json()).publicKey.challenge;
  await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { json: { credential: await key.create(c) }, browser });
  const pending = (await signIn(env, "ada@example.com")).browser;
  const html = await (await call(challenge, env, "/auth/mfa", { browser: pending })).text();
  clean(html);
  assert.ok(!html.includes('id="mfa-code"'), "no app: no code field");
  assert.match(html, /<section class="card" aria-labelledby="passkey-title" data-passkey="get" data-options="\{&quot;challenge&quot;:&quot;[\w-]{43}&quot;/);
  assert.match(html, /<input type="hidden" name="passkey" value="">/);
  assert.match(html, /hidden>\n<h2 id="passkey-title">/);
  assert.match(html, /<noscript><p>A passkey needs JavaScript/);
  assert.ok(!html.includes("break-glass"), "no recovery secret configured: no form for it");
});

test("enrolment page: the choices, the roles waiting, the QR page with the key to type; freshness first", async () => {
  const { env, clock, browser } = await setup();
  const intro = await (await call(enrol, env, "/auth/mfa/enrol?required=1&next=/auth/", { browser })).text();
  clean(intro);
  assert.match(intro, /These roles need a second factor: owner\./);
  assert.match(intro, /<form method="post" action="\/api\/auth\/mfa\/totp\/start"><input type="hidden" name="next" value="\/auth\/">/);
  assert.match(intro, /data-passkey="create" data-options="\{&quot;challenge&quot;/);
  assert.match(intro, /<a href="\/auth\/">Not now<\/a>/);
  const loc = (await call(totpStart, env, "/api/auth/mfa/totp/start", { form: { next: "/auth/" }, browser })).headers.get("location") as string;
  const qr = await (await call(enrol, env, loc, { browser })).text();
  clean(qr);
  assert.match(qr, /<div class="qr-box"><svg class="qr" xmlns="http:\/\/www.w3.org\/2000\/svg" viewBox="0 0 \d+ \d+" role="img" aria-label="QR code to scan with your authenticator app"/);
  assert.match(qr, /<code class="secret">([A-Z2-7]{4} ){7}[A-Z2-7]{4}<\/code>/);
  assert.match(qr, /name="factorId"/);
  for (const lang of LANGS) clean(await (await call(enrol, env, `${loc}&lang=${lang}`, { browser })).text());
  const gone = await (await call(enrol, env, "/auth/mfa/enrol?factor=nope&lang=en", { browser })).text();
  assert.match(gone, /This setup has expired or was already finished/);
  const unreadable = await (await call(enrol, { ...env, AUTH_MFA_KEY: "another-long-key-another-long-key-xx" }, `${loc}&lang=en`, { browser })).text();
  assert.match(unreadable, /cannot be checked right now/);
  // Off: to the account page. Old sign-in without a factor: sign in again.
  assert.equal((await call(enrol, { ...env, AUTH_MFA_KEY: undefined }, "/auth/mfa/enrol", { browser })).headers.get("location"), "/auth/account");
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  assert.equal((await call(enrol, env, "/auth/mfa/enrol", { browser })).headers.get("location"), "/auth/login?reauth=1&next=%2Fauth%2Fmfa%2Fenrol");
  // With a factor and no recent step-up: the step-up page first.
  const fresh = (await signIn(env, "ada@example.com")).browser;
  await enrolTotp(env, fresh, clock.now);
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: fresh });
  assert.equal((await call(enrol, env, "/auth/mfa/enrol", { browser: fresh })).headers.get("location"), "/auth/mfa/step-up?next=%2Fauth%2Fmfa%2Fenrol");
  assert.equal((await call(enrol, env, "/auth/mfa/enrol")).status, 303, "not signed in");
});

test("account page: the second factor's section — none yet, the factors with their actions, a key too short", async () => {
  const { env, clock, browser } = await setup();
  const none = await (await call(account, env, "/auth/account", { browser })).text();
  assert.match(none, /<h2 id="mfa-title">Second factor<\/h2>/);
  assert.match(none, /You have no second factor yet\./);
  await enrolTotp(env, browser, clock.now, "Phone");
  for (const lang of LANGS) {
    const html = await (await call(account, env, `/auth/account?lang=${lang}&ok=factor_added`, { browser })).text();
    clean(html);
    assert.match(html, /action="\/api\/auth\/mfa\/factors\/[0-9a-f-]{36}"><input type="hidden" name="next" value="\/auth\/account">/);
    assert.match(html, /action="\/api\/auth\/mfa\/backup-codes"/);
    assert.match(html, /href="\/auth\/mfa\/enrol\?next=%2Fauth%2Faccount"/);
  }
  const en = await (await call(account, env, "/auth/account?ok=factor_added&lang=en", { browser })).text();
  assert.match(en, /Your second factor is on\./);
  assert.match(en, /Authenticator app — Phone/);
  assert.match(en, /Unused backup codes: 10\./);
  assert.match(en, /Not yet/);
  const broken = await (await call(account, { ...env, AUTH_MFA_KEY: "short" }, "/auth/account?lang=en", { browser })).text();
  assert.match(broken, /cannot be checked right now/);
  const off = await (await call(account, makeEnv(), "/auth/account", { browser: new Browser() })).text();
  assert.ok(!off.includes("mfa-title"));
});
