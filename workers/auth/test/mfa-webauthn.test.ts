// GO-140: passkeys — the CBOR decoder against malformed input, the byte-level
// checks, and registration + assertion against the database with answers from
// a software authenticator (test/mfa.ts): valid ones accepted (ES256, RS256,
// EdDSA), tampered client data, wrong origin, wrong rpId, a replayed or
// foreign challenge, a decreased counter, a wrong key and a missing user
// presence all refused.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CborError, MAX_BYTES, MAX_DEPTH, decode, decodeFirst } from "../functions/api/auth/_webauthn-cbor.ts";
import { WebAuthnError, checkClientData, coseToJwk, derToP1363, parseAuthData, verifyRegistration } from "../functions/api/auth/_webauthn.ts";
import { assertOptions, checkAssertion, issueChallenge, register, registerOptions } from "../functions/api/auth/_webauthn-store.ts";
import { b64urlDecode, b64urlEncode } from "../functions/api/auth/_lib.ts";
import { findUserById } from "../functions/api/auth/_users.ts";
import type { User } from "../functions/api/auth/_users.ts";
import { useClock } from "./fakes.ts";
import { SoftAuthenticator, cbor, mfaEnv, p1363ToDer } from "./mfa.ts";
import type { Alg } from "./mfa.ts";
import { seedUser } from "./seed.ts";

const bytes = (...b: number[]) => Uint8Array.from(b);
const REQ = new Request("https://example.com/api/auth/mfa/verify");

test("CBOR: what WebAuthn uses decodes; integers, strings, arrays, maps, simple values", () => {
  const v = decode(cbor(new Map<string | number, never>([["a", [1, -1, 300, 70000, -500] as never], [3, "é" as never], ["b", new Uint8Array([9]) as never], ["t", true as never], ["f", false as never], ["n", null as never]])));
  assert.ok(v instanceof Map);
  assert.deepEqual(v.get("a"), [1, -1, 300, 70000, -500]);
  assert.equal(v.get(3), "é");
  assert.deepEqual(v.get("b"), new Uint8Array([9]));
  assert.deepEqual([v.get("t"), v.get("f"), v.get("n")], [true, false, null]);
  assert.equal(decode(bytes(0x1b, 0, 0, 0, 0, 0, 0, 1, 0)), 256, "a 64-bit head within 2^53");
  assert.deepEqual(decodeFirst(bytes(0x01, 0xff)), { value: 1, length: 1 }, "decodeFirst stops after one item");
});

test("CBOR: malformed or hostile input is refused with CborError, never a crash or a huge allocation", () => {
  const nested = (d: number) => Uint8Array.from([...new Array(d).fill(0x81), 0x01]);
  const cases: [string, Uint8Array][] = [
    ["empty", bytes()],
    ["truncated head", bytes(0x19, 0x01)],
    ["byte string longer than the input", bytes(0x5a, 0xff, 0xff, 0xff, 0xff)],
    ["array count beyond the input", bytes(0x9a, 0x7f, 0xff, 0xff, 0xff)],
    ["map count beyond the input", bytes(0xba, 0x7f, 0xff, 0xff, 0xff)],
    ["indefinite length", bytes(0x5f, 0x41, 0x00, 0xff)],
    ["reserved additional information", bytes(0x1c)],
    ["tag", bytes(0xc0, 0x01)],
    ["float", bytes(0xf9, 0x3c, 0x00)],
    ["undefined", bytes(0xf7)],
    ["integer over 2^53", bytes(0x1b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)],
    ["invalid UTF-8", bytes(0x62, 0xc3, 0x28)],
    ["text longer than the input", bytes(0x65, 0x61)],
    ["duplicate map key", bytes(0xa2, 0x01, 0x01, 0x01, 0x02)],
    ["map key of another type", bytes(0xa1, 0x41, 0x00, 0x01)],
    ["too deep", nested(MAX_DEPTH + 1)],
    ["too many items", Uint8Array.from([0x99, 0x02, 0x01, ...new Array(513).fill(0x01)])],
    ["trailing bytes", bytes(0x01, 0x02)],
    ["over the size cap", new Uint8Array(MAX_BYTES + 1)],
  ];
  for (const [name, input] of cases) assert.throws(() => decode(input), CborError, name);
  assert.equal(decode(nested(MAX_DEPTH)) instanceof Array, true, "the deepest allowed");
});

test("DER → P1363: padding, short integers, and malformed signatures", () => {
  const sig = crypto.getRandomValues(new Uint8Array(64));
  sig[0] = 0x80; // needs a leading zero in DER
  sig[32] = 0;
  sig[33] = 0x05; // a short s
  assert.deepEqual(derToP1363(p1363ToDer(sig)), sig);
  const good = p1363ToDer(sig);
  const bad: Uint8Array[] = [bytes(0x30, 0x02, 0x02, 0x00), Uint8Array.from([0x31, ...good.slice(1)]), Uint8Array.from([...good.slice(0, 1), good[1] + 1, ...good.slice(2)])];
  const r33 = Uint8Array.from([0x30, 0x26, 0x02, 0x21, 1, ...new Array(32).fill(1), 0x02, 0x01, 0x01]);
  bad.push(r33, Uint8Array.from([0x30, 0x06, 0x03, 0x01, 0x01, 0x02, 0x01, 0x01]), Uint8Array.from([0x30, 0x06, 0x02, 0x00, 0x02, 0x02, 0x01, 0x01]));
  bad.push(Uint8Array.from([0x30, 0x07, 0x02, 0x01, 0x01, 0x02, 0x01, 0x01, 0x00]), Uint8Array.from([0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x03, 0x01]));
  for (const b of bad) assert.equal(derToP1363(b), null, Buffer.from(b).toString("hex"));
});

test("COSE keys: the three algorithms; anything else refused", async () => {
  for (const alg of [-7, -257, -8] as Alg[]) assert.equal(coseToJwk(await (await new SoftAuthenticator(alg).init()).coseKey()).alg, alg);
  const refuse = (k: unknown, why: RegExp) => assert.throws(() => coseToJwk(k as never), (e: unknown) => e instanceof WebAuthnError && why.test((e as Error).message));
  refuse("x", /not a map/);
  refuse(new Map([[1, 2], [3, -7], [-1, 1], [-2, new Uint8Array(31)], [-3, new Uint8Array(32)]]), /parameter -2/);
  refuse(new Map([[1, 2], [3, -7], [-1, 2]]), /unsupported/);
  refuse(new Map([[1, 3], [3, -257], [-1, new Uint8Array(128)], [-2, bytes(1, 0, 1)]]), /2048/);
  refuse(new Map([[1, 1], [3, -8], [-1, 6], [-2, "x"]]), /parameter -2/);
  refuse(new Map([[1, 2], [3, -35]]), /unsupported/);
});

test("authenticator data and client data: lengths, flags, type, challenge, origin, cross-origin", () => {
  assert.throws(() => parseAuthData(new Uint8Array(36)), /too short/);
  const at = new Uint8Array(55);
  at[32] = 0x41;
  assert.throws(() => parseAuthData(at.slice(0, 50)), /attested credential data too short/);
  assert.throws(() => parseAuthData(at), /bad credential id length/);
  const e = { challenge: "abc", origin: "https://example.com", rpId: "example.com" };
  const cd = (o: unknown) => new TextEncoder().encode(JSON.stringify(o));
  checkClientData(cd({ type: "webauthn.get", challenge: "abc", origin: "https://example.com" }), "webauthn.get", e);
  const no = (raw: Uint8Array, why: RegExp) => assert.throws(() => checkClientData(raw, "webauthn.get", e), why);
  no(new TextEncoder().encode("{"), /not JSON/);
  no(cd(5), /not an object/);
  no(cd({ type: "webauthn.create", challenge: "abc", origin: "https://example.com" }), /type/);
  no(cd({ type: "webauthn.get", challenge: "abd", origin: "https://example.com" }), /challenge/);
  no(cd({ type: "webauthn.get", challenge: 5, origin: "https://example.com" }), /challenge/);
  no(cd({ type: "webauthn.get", challenge: "abc", origin: "https://evil.example.com" }), /origin/);
  no(cd({ type: "webauthn.get", challenge: "abc", origin: "https://example.com", crossOrigin: true }), /cross-origin/);
});

async function fixture(alg: Alg = -7, counts = true) {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "ada@example.com");
  const user = (await findUserById(env, id)) as User;
  const key = await new SoftAuthenticator(alg, counts).init();
  const opts = await registerOptions(env, REQ, user, "sess-1", clock.now);
  const factorId = await register(env, REQ, id, "sess-1", await key.create(opts.challenge), "Key", clock.now);
  assert.ok(factorId, "registered");
  const challenge = () => issueChallenge(env, id, "signin", "pending-1", clock.now);
  return { clock, env, id, user, key, factorId: factorId as string, challenge };
}

for (const alg of [-7, -257, -8] as Alg[]) {
  test(`passkey ${alg}: a registered key's assertion passes once; its replay fails`, async () => {
    const f = await fixture(alg);
    const c = await f.challenge();
    const answer = await f.key.get(c);
    assert.equal(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", answer, f.clock.now), f.factorId);
    assert.equal(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", answer, f.clock.now), null, "replayed challenge");
    // The pages send the answer as JSON text in a form field.
    assert.equal(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", JSON.stringify(await f.key.get(await f.challenge())), f.clock.now), f.factorId);
  });
}

test("passkey: every tampering and every mismatch is refused, and refusing costs the challenge", async () => {
  const f = await fixture();
  const check = (answer: unknown, purpose: "signin" | "step-up" = "signin", bound = "pending-1", user = f.id) => checkAssertion(f.env, REQ, user, purpose, bound, answer, f.clock.now);
  const tamper = async (fn: (a: Awaited<ReturnType<SoftAuthenticator["get"]>>) => void) => {
    const a = await f.key.get(await f.challenge());
    fn(a);
    return check(a);
  };
  const edit = (raw: string) => b64urlEncode(new TextEncoder().encode(new TextDecoder().decode(b64urlDecode(raw) as Uint8Array).replace('"webauthn.get"', '"webauthn.get","x":1')));
  assert.equal(await tamper((a) => (a.response.clientDataJSON = edit(a.response.clientDataJSON))), null, "client data changed after signing");
  assert.equal(await check(await f.key.get(await f.challenge(), { origin: "https://evil.example.com" })), null, "wrong origin");
  assert.equal(await check(await f.key.get(await f.challenge(), { rpId: "evil.example.com" })), null, "wrong rpId");
  assert.equal(await check(await f.key.get(await f.challenge(), { type: "webauthn.create" })), null, "wrong ceremony");
  assert.equal(await check(await f.key.get(await f.challenge(), { flags: 0x04 })), null, "user not present");
  assert.equal(await tamper((a) => (a.response.signature = b64urlEncode(new Uint8Array(70)))), null, "malformed signature");
  const other = await new SoftAuthenticator().init();
  assert.equal(await tamper(async (a) => Object.assign(a, { id: other.id })), null, "a credential the account does not have");
  const signedByOther = await other.get(await f.challenge());
  assert.equal(await check({ ...signedByOther, id: f.key.id }), null, "signed with the wrong key");
  assert.equal(await check(await f.key.get(await issueChallenge(f.env, f.id, "step-up", "pending-1", f.clock.now))), null, "a challenge for another purpose");
  assert.equal(await check(await f.key.get(await issueChallenge(f.env, f.id, "signin", "pending-2", f.clock.now))), null, "a challenge for another sign-in");
  const otherUser = await seedUser(f.env, "bob@example.com");
  assert.equal(await check(await f.key.get(await issueChallenge(f.env, otherUser, "signin", "pending-1", f.clock.now))), null, "another account's challenge");
  assert.equal(await check(await f.key.get("made-up-challenge")), null, "a challenge never issued");
  const late = await f.challenge();
  f.clock.advance(301_000);
  assert.equal(await check(await f.key.get(late)), null, "an expired challenge");
  for (const junk of [null, "not json", { id: f.key.id }, { id: 5, response: {} }, { id: f.key.id, response: { clientDataJSON: "!!", authenticatorData: "", signature: "" } }]) assert.equal(await check(junk), null);
  const noChallenge = await f.key.get(await f.challenge());
  noChallenge.response.clientDataJSON = b64urlEncode(new TextEncoder().encode("{}"));
  assert.equal(await check(noChallenge), null, "client data without a challenge");
  noChallenge.response.clientDataJSON = b64urlEncode(new TextEncoder().encode("not json"));
  assert.equal(await check(noChallenge), null, "client data that is not JSON");
});

test("passkey counter: must grow; a lower or equal one is refused; an authenticator that always says 0 is accepted", async () => {
  const f = await fixture();
  assert.ok(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", await f.key.get(await f.challenge()), f.clock.now));
  f.key.counter -= 2; // a cloned authenticator lagging behind
  assert.equal(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", await f.key.get(await f.challenge()), f.clock.now), null, "decreased");
  f.key.counter = (f.env.AUTH_DB.sqlite.prepare("SELECT sign_count AS n FROM mfa_factors").get() as { n: number }).n - 1;
  assert.equal(await checkAssertion(f.env, REQ, f.id, "signin", "pending-1", await f.key.get(await f.challenge()), f.clock.now), null, "equal");
  const z = await fixture(-7, false);
  for (let i = 0; i < 2; i++) assert.ok(await checkAssertion(z.env, REQ, z.id, "signin", "pending-1", await z.key.get(await z.challenge()), z.clock.now), "zero, again");
  // Two answers checked at once against the same stored counter: one moves it.
  const g = await fixture();
  const [a, b] = [await g.key.get(await g.challenge()), await g.key.get(await g.challenge())];
  g.env.AUTH_DB.sqlite.exec("UPDATE mfa_factors SET sign_count = 1");
  assert.ok(await checkAssertion(g.env, REQ, g.id, "signin", "pending-1", b, g.clock.now));
  assert.equal(await checkAssertion(g.env, REQ, g.id, "signin", "pending-1", a, g.clock.now), null);
});

test("registration: the session's own unused challenge, our origin and rpId, a supported key; duplicates refused", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const id = await seedUser(env, "ada@example.com");
  const user = (await findUserById(env, id)) as User;
  const key = await new SoftAuthenticator().init();
  const reg = async (answer: unknown, session = "s1") => register(env, REQ, id, session, answer, "", clock.now);
  const ch = async () => (await registerOptions(env, REQ, user, "s1", clock.now)).challenge;
  assert.equal(await reg(await key.create(await ch(), { origin: "https://evil.example.com" })), null, "wrong origin");
  assert.equal(await reg(await key.create(await ch(), { rpId: "evil.example.com" })), null, "wrong rpId");
  assert.equal(await reg(await key.create(await ch()), "s2"), null, "another session's challenge");
  assert.equal(await reg(await key.create("never-issued")), null);
  assert.equal(await reg({ response: { clientDataJSON: b64urlEncode(new TextEncoder().encode("{}")), attestationObject: "AA" } }), null, "no challenge");
  assert.equal(await reg({ response: {} }), null);
  const c = await ch();
  const good = await key.create(c);
  const att = good.response.attestationObject;
  assert.equal(await reg({ ...good, response: { ...good.response, attestationObject: b64urlEncode(cbor(new Map([["fmt", 5]]))) } }), null, "bad attestation object");
  const c2 = await ch();
  const noAuth = await key.create(c2);
  assert.equal(await reg({ ...noAuth, response: { ...noAuth.response, attestationObject: b64urlEncode(cbor(new Map<string, never>([["fmt", "none" as never]]))) } }), null, "no authenticator data");
  const c3 = await ch();
  assert.ok(await reg({ ...(await key.create(c3)), response: { ...(await key.create(c3)).response, transports: ["usb", 5, "x".repeat(30)] } }), "registered");
  assert.equal(await reg({ ...good, response: { ...good.response, attestationObject: att } }), null, "challenge already used");
  assert.equal(await reg(await key.create(await ch())), null, "the same credential twice");
  const row = env.AUTH_DB.sqlite.prepare("SELECT transports FROM mfa_factors").get() as { transports: string };
  assert.deepEqual(JSON.parse(row.transports), ["usb"]);
  const opts = await registerOptions(env, REQ, user, "s1", clock.now);
  assert.deepEqual(opts.excludeCredentials.map((d) => d.id), [key.id]);
  assert.equal(opts.attestation, "none");
  assert.deepEqual(opts.rp, { id: "example.com", name: "example.com" });
  assert.equal(await assertOptions(env, REQ, "nobody", "signin", "p", clock.now), null, "no passkey: no options");
});

test("verifyRegistration: user presence and attested credential data are required; an off-curve key is refused", async () => {
  const key = await new SoftAuthenticator().init();
  const e = { challenge: "c", origin: "https://example.com", rpId: "example.com" };
  const cd = new TextEncoder().encode(JSON.stringify({ type: "webauthn.create", challenge: "c", origin: "https://example.com" }));
  const att = (authData: Uint8Array) => cbor(new Map<string, never>([["fmt", "packed" as never], ["attStmt", new Map() as never], ["authData", authData as never]]));
  const noUp = await key.authData("example.com", 0x00, true);
  await assert.rejects(verifyRegistration(cd, att(noUp), e), /user not present/);
  const noAt = await key.authData("example.com", 0x01, false);
  await assert.rejects(verifyRegistration(cd, att(noAt), e), /no credential/);
  const okAny = await key.authData("example.com", 0x01, true);
  assert.equal((await verifyRegistration(cd, att(okAny), e)).alg, -7, "an attestation statement is ignored, whatever its format");
  const offCurve = Uint8Array.from(okAny);
  offCurve[offCurve.length - 1] ^= 0xff; // the last byte of y
  await assert.rejects(verifyRegistration(cd, att(offCurve), e));
});
