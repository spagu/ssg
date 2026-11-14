// GO-140: TOTP (RFC 6238 vectors, the ±1 window, replay), base32, the
// otpauth URI, the encryption of the secrets at rest and its rotation, and the
// backup codes' data layer.
import { test } from "node:test";
import assert from "node:assert/strict";
import { DIGITS, base32Decode, base32Encode, groupSecret, hotp, matchStep, normCode, otpauthUri, stepAt } from "../functions/api/auth/_totp.ts";
import { MIN_KEY_LENGTH, mfaEnabled, mfaState, open, seal } from "../functions/api/auth/_mfa-crypto.ts";
import { backupCodesLeft, newBackupCodes, normBackupCode, startTotp, useBackupCode } from "../functions/api/auth/_mfa-factors.ts";
import { checkTotp } from "../functions/api/auth/_mfa-verify.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { T0, makeEnv, useClock } from "./fakes.ts";
import { MFA_KEY, code, enrolTotp, mfaEnv, signIn } from "./mfa.ts";
import { seedUser } from "./seed.ts";

const SEED = new TextEncoder().encode("12345678901234567890"); // RFC 6238 Appendix B, SHA-1

test("RFC 6238 Appendix B (SHA-1): the eight-digit vectors, and the six-digit codes apps show", async () => {
  const vectors: [number, string][] = [
    [59, "94287082"],
    [1111111109, "07081804"],
    [1111111111, "14050471"],
    [1234567890, "89005924"],
    [2000000000, "69279037"],
    [20000000000, "65353130"],
  ];
  for (const [t, expected] of vectors) {
    assert.equal(await hotp(SEED, Math.floor(t / 30), 8), expected, `T=${t}`);
    assert.equal(await hotp(SEED, stepAt(t * 1000)), expected.slice(-DIGITS), `T=${t}, six digits`);
  }
  // RFC 4226 Appendix D, counters 0 and 9.
  assert.equal(await hotp(SEED, 0), "755224");
  assert.equal(await hotp(SEED, 9), "520489");
});

test("base32: RFC 4648 vectors both ways; case, spaces and padding ignored; junk refused", () => {
  const enc = (s: string) => base32Encode(new TextEncoder().encode(s));
  assert.deepEqual(["", "f", "fo", "foo", "foob", "fooba", "foobar"].map(enc), ["", "MY", "MZXQ", "MZXW6", "MZXW6YQ", "MZXW6YTB", "MZXW6YTBOI"]);
  assert.equal(new TextDecoder().decode(base32Decode("mzxw 6ytb oi======") as Uint8Array), "foobar");
  assert.equal(base32Decode("MZ1"), null);
  assert.equal(groupSecret("ABCDEFGHIJ"), "ABCD EFGH IJ");
});

test("normCode: six digits, spaces and dashes allowed, anything else refused", () => {
  assert.equal(normCode(" 123 456 "), "123456");
  assert.equal(normCode("123-456"), "123456");
  for (const bad of ["12345", "1234567", "12a456", 123456, null, "1".repeat(40)]) assert.equal(normCode(bad), null, String(bad));
});

test("matchStep: the current step and one either side; two steps away is refused", async () => {
  const now = T0 + 7_000;
  const at = async (steps: number) => hotp(SEED, stepAt(now) + steps);
  assert.equal(await matchStep(SEED, await at(0), now), stepAt(now));
  assert.equal(await matchStep(SEED, await at(-1), now), stepAt(now) - 1);
  assert.equal(await matchStep(SEED, await at(1), now), stepAt(now) + 1);
  assert.equal(await matchStep(SEED, await at(-2), now), null);
  assert.equal(await matchStep(SEED, await at(2), now), null);
});

test("otpauth URI: the label and the parameters authenticator apps read", () => {
  const uri = otpauthUri("example.com", "ada+x@example.com", "JBSWY3DPEHPK3PXP");
  assert.equal(uri, "otpauth://totp/example.com:ada%2Bx%40example.com?secret=JBSWY3DPEHPK3PXP&issuer=example.com&algorithm=SHA1&digits=6&period=30");
});

test("mfaState: off without the key, misconfigured when too short, ready otherwise", () => {
  assert.equal(mfaState(makeEnv()), "off");
  assert.equal(mfaEnabled(makeEnv()), false);
  assert.equal(mfaState(makeEnv({ AUTH_MFA_KEY: "short" })), "misconfigured");
  assert.equal(mfaEnabled(makeEnv({ AUTH_MFA_KEY: "short" })), true);
  assert.equal(mfaState(makeEnv({ AUTH_MFA_KEY: "x".repeat(MIN_KEY_LENGTH) })), "ready");
});

test("seal/open: AES-GCM bound to its row; a tampered, moved or foreign row does not open; rotation reads the old key", async () => {
  const env = mfaEnv();
  const plain = crypto.getRandomValues(new Uint8Array(20));
  const sealed = (await seal(env, plain, "f1", "u1")) as string;
  assert.match(sealed, /^v1\.[0-9a-f]{12}\.[\w-]+\.[\w-]+$/);
  assert.deepEqual((await open(env, sealed, "f1", "u1"))?.plain, plain);
  assert.equal((await open(env, sealed, "f1", "u1"))?.stale, false);
  assert.equal(await open(env, sealed, "f2", "u1"), null, "moved to another factor");
  assert.equal(await open(env, sealed, "f1", "u2"), null, "moved to another account");
  const [v, id, iv, ct] = sealed.split(".");
  const flipped = ct.slice(0, -2) + (ct.at(-2) === "A" ? "B" : "A") + ct.at(-1);
  assert.equal(await open(env, [v, id, iv, flipped].join("."), "f1", "u1"), null, "tampered");
  for (const junk of ["v2.x.y.z", "v1", "v1.abc.!!.??", ""]) assert.equal(await open(env, junk, "f1", "u1"), null, junk);
  assert.equal(await open(makeEnv({ AUTH_MFA_KEY: "another-key-another-key-another-key!" }), sealed, "f1", "u1"), null, "another key");
  const rotated = makeEnv({ AUTH_MFA_KEY: "the-new-key-the-new-key-the-new-key-123", AUTH_MFA_KEY_PREVIOUS: MFA_KEY });
  assert.deepEqual(await open(rotated, sealed, "f1", "u1"), { plain, stale: true });
  assert.equal(await seal(makeEnv({ AUTH_MFA_KEY: "short" }), plain, "f", "u"), null, "a key too short seals nothing");
  assert.equal(await open(makeEnv({ AUTH_MFA_KEY: "short" }), sealed, "f1", "u1"), null);
});

test("the TOTP secret is never in the database in clear: every table scanned for its base32 and its bytes", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await seedUser(env, "ada@example.com");
  const { browser } = await signIn(env, "ada@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  const raw = base32Decode(secret) as Uint8Array;
  const hex = [...raw].map((b) => b.toString(16).padStart(2, "0")).join("");
  const db = env.AUTH_DB.sqlite;
  const tables = (db.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all() as { name: string }[]).map((r) => r.name);
  for (const table of tables) {
    for (const row of db.prepare(`SELECT * FROM "${table}"`).all() as Record<string, unknown>[]) {
      for (const value of Object.values(row)) {
        const s = value instanceof Uint8Array ? Buffer.from(value).toString("hex") : String(value);
        assert.ok(!s.toUpperCase().includes(secret) && !s.toLowerCase().includes(hex), `${table} holds the secret`);
      }
    }
  }
});

test("checkTotp: a code works once; an older step after a newer one is refused; a stale-key row is rewritten", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "ada@example.com");
  const { browser } = await signIn(env, "ada@example.com");
  const { secret, factorId } = await enrolTotp(env, browser, clock.now);
  clock.advance(60_000);
  const now = await code(secret, clock.now);
  assert.equal(await checkTotp(env, id, now, clock.now), true);
  assert.equal(await checkTotp(env, id, now, clock.now), false, "the same code twice");
  assert.equal(await checkTotp(env, id, await code(secret, clock.now, -1), clock.now), false, "the step before, after the current one");
  assert.equal(await checkTotp(env, id, "not a code", clock.now), false);
  // Rotation: the row was sealed with MFA_KEY; with it as the previous key a
  // pass rewrites the row under the new one.
  const rotated = { ...env, AUTH_MFA_KEY: "the-new-key-the-new-key-the-new-key-123", AUTH_MFA_KEY_PREVIOUS: MFA_KEY };
  clock.advance(30_000);
  assert.equal(await checkTotp(rotated, id, await code(secret, clock.now), clock.now), true);
  const row = env.AUTH_DB.sqlite.prepare("SELECT secret FROM mfa_factors WHERE id = ?").get(factorId) as { secret: string };
  assert.ok((await open({ ...rotated, AUTH_MFA_KEY_PREVIOUS: undefined }, row.secret, factorId, id)) !== null, "now under the new key alone");
  // A row nothing can open is skipped (and said so), never a pass.
  const errors: string[] = [];
  const real = console.error;
  console.error = (m: string) => errors.push(m);
  try {
    clock.advance(30_000);
    assert.equal(await checkTotp({ ...env, AUTH_MFA_KEY: "a-key-that-never-sealed-anything-here!" }, id, await code(secret, clock.now), clock.now), false);
  } finally {
    console.error = real;
  }
  assert.match(errors.join("\n"), /cannot be decrypted/);
  assert.ok(!errors.join("\n").includes(secret));
});

test("startTotp refuses without a usable key and writes nothing", async () => {
  const env = makeEnv({ AUTH_MFA_KEY: "short" });
  await ensureSchema(env);
  assert.equal(await startTotp(env, "u1", T0), null);
  assert.equal((env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM mfa_factors").get() as { n: number }).n, 0);
});

test("backup codes: 10 of 80 bits, only hashes stored, each works once, a new set replaces the old", async () => {
  const env = mfaEnv();
  await ensureSchema(env);
  const codes = await newBackupCodes(env, "u1", T0);
  assert.equal(codes.length, 10);
  for (const c of codes) assert.match(c, /^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$/);
  const stored = env.AUTH_DB.sqlite.prepare("SELECT code_hash FROM mfa_backup_codes").all() as { code_hash: string }[];
  assert.equal(stored.length, 10);
  for (const c of codes) assert.ok(!stored.some((r) => r.code_hash.includes(c.replaceAll("-", ""))));
  assert.equal(await useBackupCode(env, "u2", codes[0], T0), false, "another account's code");
  assert.equal(await useBackupCode(env, "u1", codes[0].toLowerCase().replaceAll("-", " "), T0), true, "case and spaces do not matter");
  assert.equal(await useBackupCode(env, "u1", codes[0], T0), false, "used once");
  assert.equal(await backupCodesLeft(env, "u1"), 9);
  const fresh = await newBackupCodes(env, "u1", T0);
  assert.equal(await useBackupCode(env, "u1", codes[1], T0), false, "the old set is gone");
  assert.equal(await useBackupCode(env, "u1", fresh[1], T0), true);
  assert.equal(normBackupCode("o0iL-1111-2222-3333"), "0011111122223333", "Crockford: O is 0, I and L are 1");
  for (const bad of ["UUUU-UUUU-UUUU-UUUU", "short", 42, "A".repeat(80)]) assert.equal(normBackupCode(bad), null);
  assert.equal(await useBackupCode(env, "u1", "nonsense", T0), false);
});
