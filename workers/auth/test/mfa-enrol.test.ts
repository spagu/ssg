// GO-140: adding and removing factors through the routes — the app (start,
// confirm), passkeys (register), backup codes (first factor, regenerate), the
// list, removal and the last-factor rule; and an invitation into a role that
// needs a second factor.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as backupCodes } from "../functions/api/auth/mfa/backup-codes.ts";
import { onRequestGet as factors } from "../functions/api/auth/mfa/factors/index.ts";
import { onRequestDelete as removeFactor, onRequestPost as removeFactorForm } from "../functions/api/auth/mfa/factors/[id].ts";
import { onRequestPost as options } from "../functions/api/auth/mfa/passkey/options.ts";
import { onRequestPost as passkeyRegister } from "../functions/api/auth/mfa/passkey/register.ts";
import { onRequestPost as acceptInvite } from "../functions/api/auth/invite.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { Browser, PASSWORD, T0, call, useClock } from "./fakes.ts";
import { SoftAuthenticator, code, enrolTotp, mfaEnv, signIn, stepUp, totpConfirm, totpStart } from "./mfa.ts";
import { grantPerms, roleId, seedUser } from "./seed.ts";
import { events } from "./go151.ts";

async function fresh(env = mfaEnv(), isOwner = false) {
  const clock = useClock();
  const id = await seedUser(env, "ada@example.com");
  if (isOwner) await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "ada@example.com");
  return { clock, env, id, browser };
}

test("authenticator app: start (JSON: secret, URI, QR) and confirm; the first factor brings ten backup codes; events", async () => {
  const { env, clock, browser } = await fresh();
  const started = await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser });
  assert.equal(started.status, 201);
  const s = await started.json();
  assert.match(s.secret, /^[A-Z2-7]{32}$/);
  assert.equal(s.uri, `otpauth://totp/example.com:ada%40example.com?secret=${s.secret}&issuer=example.com&algorithm=SHA1&digits=6&period=30`);
  assert.match(s.qrSvg, /^<svg class="qr"/);
  const wrong = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: "000000" }, browser });
  assert.deepEqual([wrong.status, (await wrong.json()).error], [400, "invalid_code"]);
  const ok = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: await code(s.secret, clock.now), name: "  Work phone  " }, browser });
  assert.equal(ok.status, 200);
  const body = await ok.json();
  assert.equal(body.backupCodes.length, 10);
  assert.deepEqual([body.factor.kind, body.factor.name, body.factor.id], ["totp", "Work phone", s.factorId]);
  assert.deepEqual(events(env, "mfa_enrolled").at(-1)?.detail, { factor: "totp", factorId: s.factorId, first: true });
  const again = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: await code(s.secret, clock.now) }, browser });
  assert.deepEqual([again.status, (await again.json()).error], [410, "enrolment_expired"], "confirmed already");
  // A second app: no new backup codes (the set stays).
  clock.advance(1000);
  const second = await enrolTotp(env, browser, clock.now, "Tablet");
  assert.equal(second.codes, undefined);
  const list = await (await call(factors, env, "/api/auth/mfa/factors", { browser })).json();
  assert.deepEqual(list.factors.map((f: { name: string }) => f.name), ["Work phone", "Tablet"]);
  assert.equal(list.backupCodesLeft, 10);
  assert.equal(list.sessionFactor.factor, "totp");
});

test("enrolment expires after 15 minutes; someone else's cannot be confirmed; an enrolment that cannot be opened is 503", async () => {
  const { env, clock, browser } = await fresh();
  const s = await (await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser })).json();
  await seedUser(env, "bob@example.com");
  const bob = (await signIn(env, "bob@example.com")).browser;
  const s2 = await (await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser: bob })).json();
  const foreign = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: await code(s.secret, clock.now) }, browser: bob });
  assert.equal(foreign.status, 410);
  const broken = await call(totpConfirm, { ...env, AUTH_MFA_KEY: "another-long-key-another-long-key-xx" }, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: "123456" }, browser });
  assert.deepEqual([broken.status, (await broken.json()).error], [503, "mfa_unavailable"]);
  clock.advance(11 * 60_000);
  const old = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s.factorId, code: await code(s.secret, clock.now) }, browser: (await signIn(env, "ada@example.com")).browser });
  assert.equal(old.status, 200, "a new sign-in may still finish it within 15 minutes");
  clock.advance(16 * 60_000);
  const late = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s2.factorId, code: await code(s2.secret, clock.now) }, browser: (await signIn(env, "bob@example.com")).browser });
  assert.deepEqual([late.status, (await late.json()).error], [410, "enrolment_expired"]);
  const stale = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId: s2.factorId, code: "123456" }, browser: bob });
  assert.deepEqual([stale.status, (await stale.json()).error], [403, "reauth_required"], "bob's first session is too old to enrol");
  const unusable = await call(totpStart, { ...env, AUTH_MFA_KEY: "short" }, "/api/auth/mfa/totp/start", { json: {}, browser: (await signIn(env, "bob@example.com")).browser });
  assert.deepEqual([unusable.status, (await unusable.json()).error], [503, "mfa_unavailable"]);
});

test("plain forms: start goes to the QR page, a wrong code back to it, a first factor answers with the backup-codes page", async () => {
  const { env, clock, browser } = await fresh();
  const started = await call(totpStart, env, "/api/auth/mfa/totp/start", { form: { next: "/auth/" }, browser });
  const location = started.headers.get("location") as string;
  assert.match(location, /^\/auth\/mfa\/enrol\?factor=[0-9a-f-]{36}&next=%2Fauth%2F$/);
  const factorId = new URL(location, "https://example.com").searchParams.get("factor") as string;
  const wrong = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { form: { factorId, code: "1", next: "/auth/" }, browser });
  assert.equal(wrong.headers.get("location"), `/auth/mfa/enrol?factor=${factorId}&next=%2Fauth%2F&error=invalid_code`);
  const row = env.AUTH_DB.sqlite.prepare("SELECT secret FROM mfa_factors WHERE id = ?").get(factorId) as { secret: string };
  const { open } = await import("../functions/api/auth/_mfa-crypto.ts");
  const { base32Encode } = await import("../functions/api/auth/_totp.ts");
  const uid = (env.AUTH_DB.sqlite.prepare("SELECT id FROM users").get() as { id: string }).id;
  const secret = base32Encode(((await open(env, row.secret, factorId, uid)) as { plain: Uint8Array }).plain);
  const done = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { form: { factorId, code: await code(secret, clock.now), next: "/auth/" }, browser });
  assert.equal(done.status, 200);
  const html = await done.text();
  assert.equal((html.match(/<li><code>[0-9A-Z]{4}-/g) ?? []).length, 10);
  assert.match(html, /<a class="btn btn-primary" href="\/auth\/">/);
  assert.ok(done.headers.getSetCookie().some((c) => c.startsWith("__Host-ssg-auth=")), "the new access cookie");
  const regen = await call(backupCodes, env, "/api/auth/mfa/backup-codes", { form: { next: "/auth/account" }, browser });
  assert.equal((((await regen.text()).match(/<li><code>/g)) ?? []).length, 10);
  // A second factor through the form: back to `next` with ?ok=.
  const s2 = await (await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser })).json();
  const second = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { form: { factorId: s2.factorId, code: await code(s2.secret, clock.now), next: "/auth/account" }, browser });
  assert.equal(second.headers.get("location"), "/auth/account?ok=factor_added");
});

test("backup codes: regenerating replaces the set and needs a factor and a recent one", async () => {
  const { env, clock, browser } = await fresh();
  const none = await call(backupCodes, env, "/api/auth/mfa/backup-codes", { json: {}, browser });
  assert.deepEqual([none.status, (await none.json()).error], [409, "no_factor"]);
  const { codes } = await enrolTotp(env, browser, clock.now);
  const res = await call(backupCodes, env, "/api/auth/mfa/backup-codes", { json: {}, browser });
  const fresh2 = (await res.json()).backupCodes as string[];
  assert.equal(fresh2.length, 10);
  assert.notDeepEqual(fresh2, codes);
  assert.ok(events(env, "mfa_backup_codes_regenerated").length === 1);
  clock.advance(11 * 60_000);
  const { onRequestPost: refresh } = await import("../functions/api/auth/refresh.ts");
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  const stale = await call(backupCodes, env, "/api/auth/mfa/backup-codes", { json: {}, browser });
  assert.equal((await stale.json()).error, "step_up_required");
  const staleForm = await call(backupCodes, env, "/api/auth/mfa/backup-codes", { form: { next: "/auth/account" }, browser });
  assert.equal(staleForm.headers.get("location"), "/auth/mfa/step-up?next=%2Fauth%2Faccount");
});

test("removing factors: someone else's is not found; the last one is kept while a role needs it; with it go the backup codes", async () => {
  const { env, clock, browser } = await fresh(mfaEnv(), true);
  const { factorId } = await enrolTotp(env, browser, clock.now);
  const del = (fid: string, b = browser) => call(removeFactor, env, `/api/auth/mfa/factors/${fid}`, { method: "DELETE", browser: b, params: { id: fid } });
  assert.deepEqual([(await del("nope")).status], [404]);
  const last = await del(factorId);
  assert.deepEqual(await last.json(), { error: "factor_required", roles: ["owner"] });
  const second = await enrolTotp(env, browser, clock.now, "Spare");
  assert.equal((await del(second.factorId)).status, 200, "not the last");
  assert.deepEqual(events(env, "mfa_removed").at(-1)?.detail, { factor: "totp", factorId: second.factorId, last: false });
  // A viewer may drop the last one; the backup codes go with it.
  const viewerEnv = mfaEnv();
  const v = await fresh(viewerEnv);
  await grantPerms(viewerEnv, v.id, ["newsletter:read"]);
  const t = await enrolTotp(viewerEnv, v.browser, v.clock.now);
  const form = await call(removeFactorForm, viewerEnv, `/api/auth/mfa/factors/${t.factorId}`, { form: { next: "/auth/account" }, browser: v.browser, params: { id: t.factorId } });
  assert.equal(form.headers.get("location"), "/auth/account?ok=factor_removed");
  assert.equal((viewerEnv.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM mfa_backup_codes").get() as { n: number }).n, 0);
  // A race: removed between the read and the delete.
  const db = env.AUTH_DB as unknown as { prepare(sql: string): unknown };
  const real = db.prepare.bind(db);
  // (The factor really goes — a parallel request deleted it — so the route's
  // re-read after its own delete changed nothing finds it gone: 404. Since the
  // security review, SR-13, a delete refused by the "keep one" rule while the
  // factor still exists is 409 factor_required instead.)
  const third = await enrolTotp(env, browser, clock.now, "Third");
  db.prepare = (sql: string) => {
    if (sql.startsWith("DELETE FROM mfa_factors WHERE id")) env.AUTH_DB.sqlite.prepare("DELETE FROM mfa_factors WHERE id = ?").run(third.factorId);
    return real(sql);
  };
  const raced = await del(third.factorId);
  db.prepare = real;
  assert.deepEqual([raced.status, (await raced.json()).error], [404, "factor_not_found"]);
});

test("passkey registration through the routes: JSON (backup codes with the first factor) and the enrolment page's form", async () => {
  const { env, browser } = await fresh();
  const key = await new SoftAuthenticator(-8).init();
  const challenge = async () => (await (await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "register" }, browser })).json()).publicKey.challenge;
  const bad = await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { json: { credential: await key.create("never-issued") }, browser });
  assert.deepEqual([bad.status, (await bad.json()).error], [400, "invalid_passkey"]);
  const badForm = await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { form: { credential: "{}", next: "/auth/account" }, browser });
  assert.equal(badForm.headers.get("location"), "/auth/mfa/enrol?error=invalid_passkey&next=%2Fauth%2Faccount");
  const ok = await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { json: { credential: await key.create(await challenge()), name: "Laptop" }, browser });
  assert.equal(ok.status, 201);
  const body = await ok.json();
  assert.deepEqual([body.factor.kind, body.factor.name, body.backupCodes.length], ["passkey", "Laptop", 10]);
  const k2 = await new SoftAuthenticator().init();
  const viaForm = await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { form: { credential: JSON.stringify(await k2.create(await challenge())), name: "Key", next: "/auth/account" }, browser });
  assert.equal(viaForm.headers.get("location"), "/auth/account?ok=factor_added");
  // Step-up with a passkey.
  const up = (await (await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "step-up" }, browser })).json()).publicKey.challenge;
  const stepped = await call(stepUp, env, "/api/auth/mfa/step-up", { json: { passkey: await key.get(up) }, browser });
  assert.equal(stepped.status, 200);
  assert.equal((await stepped.json()).minutes, 10);
});

test("an invitation into a role that needs a second factor: the session without it, a form led to enrolment", async () => {
  useClock();
  const env = mfaEnv();
  await seedUser(env, "boss@example.com");
  const editor = roleId(env as never, "newsletter-editor");
  // The inviter's own role needs a second factor too, so it is not in their token: grant directly.
  const { createInvitation } = await import("../functions/api/auth/_invites.ts");
  const a = await createInvitation(env, { email: "eva@example.com", name: "Eva", roleIds: [editor], groupIds: [], lang: "en", invitedBy: "x" }, T0);
  const res = await call(acceptInvite, env, "/api/auth/invite", { json: { token: a.token, password: PASSWORD } });
  const body = await res.json();
  assert.equal(res.status, 201);
  assert.equal(body.enrol, true);
  const b = await createInvitation(env, { email: "fay@example.com", name: "Fay", roleIds: [editor], groupIds: [], lang: "en", invitedBy: "x" }, T0);
  const form = await call(acceptInvite, env, "/api/auth/invite", { form: { token: b.token, password: PASSWORD }, browser: new Browser() });
  assert.equal(form.headers.get("location"), "/auth/mfa/enrol?required=1&next=%2Fauth%2F%3Fwelcome%3D1");
});
