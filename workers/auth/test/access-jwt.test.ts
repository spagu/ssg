// GO-139: the Access token check, case by case — only a token Access signed
// with this team's key, for THIS application, unexpired (exp required), from
// the header or the cookie, says who the person is.
import { test } from "node:test";
import assert from "node:assert/strict";
import { claimProblem, groupsOf, verifyAccessToken } from "../functions/api/auth/_access-jwt.ts";
import { accessLookup } from "../functions/api/auth/_access-config.ts";
import type { AccessSetup } from "../functions/api/auth/_access-config.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import { T0 } from "./fakes.ts";
import { ACCESS_ENV, AUD, FakeAccess, ISS, OTHER_AUD, OTHER_KEY, PERSON, STRANGER_KEY, mint } from "./access-fake.ts";

const SETUP = (accessLookup(ACCESS_ENV as never) as { setup: AccessSetup }).setup;
const at = (token: string, where: "header" | "cookie" = "header", extra: Record<string, string> = {}) =>
  new Request("https://example.com/auth/access", { headers: where === "header" ? { "cf-access-jwt-assertion": token, ...extra } : { cookie: `a=b; CF_Authorization=${token}`, ...extra } });
const check = (req: Request, now = T0) => verifyAccessToken(req, SETUP, now);
const reason = async (req: Request, now = T0) => {
  const v = await check(req, now);
  return v.ok ? "ok" : v.reason;
};
const enc = (v: unknown) => b64urlEncode(new TextEncoder().encode(typeof v === "string" ? v : JSON.stringify(v)));

test("a valid token from the header or from the cookie names the person; groups null without a claim", async () => {
  const fake = new FakeAccess().install();
  try {
    const token = await mint();
    for (const where of ["header", "cookie"] as const) {
      const v = await check(at(token, where));
      assert.deepEqual(v, { ok: true, person: { subject: PERSON.sub, email: "ada@example.com", groups: null } }, where);
    }
    // aud as a plain string, as some tokens carry it.
    assert.equal(await reason(at(await mint(PERSON, { aud: AUD }))), "ok");
    // The header wins: a bad header is not rescued by a good cookie.
    assert.equal(await reason(at("x.y.z", "header", { cookie: `CF_Authorization=${token}` })), "malformed");
    assert.equal(fake.requests.length, 1, "keys fetched once, then cached");
  } finally {
    fake.restore();
  }
});

test("refused: another application of the same team, another team, expired, no exp, future nbf/iat, wrong type, no sub", async () => {
  const fake = new FakeAccess().install();
  try {
    const now = Math.floor(T0 / 1000);
    const cases: [Record<string, unknown>, string][] = [
      [{ aud: [OTHER_AUD] }, "audience"],
      [{ aud: undefined }, "audience"],
      [{ iss: "https://otherteam.cloudflareaccess.com" }, "issuer"],
      [{ iss: `${ISS}/` }, "issuer"],
      [{ exp: now }, "expired"],
      [{ exp: now - 1 }, "expired"],
      [{ exp: undefined }, "no expiry"],
      [{ exp: "9999999999" }, "no expiry"],
      [{ exp: Number.MAX_VALUE * 10 }, "no expiry"],
      [{ nbf: now + 120 }, "not yet valid"],
      [{ nbf: "0" }, "not yet valid"],
      [{ iat: now + 120 }, "issued in the future"],
      [{ type: "org" }, "token type"],
      [{ sub: "" }, "no subject"],
      [{ sub: 42 }, "no subject"],
      [{ sub: "s".repeat(256) }, "no subject"],
    ];
    for (const [claims, why] of cases) assert.equal(await reason(at(await mint(PERSON, claims))), why, JSON.stringify(claims));
    // Within the clock skew: nbf / iat a little ahead, no type at all → fine.
    assert.equal(await reason(at(await mint(PERSON, { nbf: now + 30, iat: now + 30, type: undefined }))), "ok");
    assert.equal(claimProblem({ aud: [AUD], iss: ISS, exp: now + 5, sub: "s" }, SETUP, now), null);
  } finally {
    fake.restore();
  }
});

test("refused: alg none, HS256 with the public key, a bad signature, an unknown key id, another team's key under our kid", async () => {
  const fake = new FakeAccess().install();
  try {
    assert.equal(await reason(at(await mint(PERSON, {}, { forge: "none" }))), "malformed", "alg none has no signature");
    assert.equal(await reason(at((await mint(PERSON, {}, { forge: "none" })) + "c2ln")), "algorithm");
    assert.equal(await reason(at(await mint(PERSON, {}, { forge: "hs256-public" }))), "algorithm");
    assert.equal(await reason(at(await mint(PERSON, {}, { forge: "bad-signature" }))), "signature");
    assert.equal(await reason(at(await mint(PERSON, {}, { key: OTHER_KEY }))), "unknown key id");
    assert.equal(await reason(at(await mint(PERSON, {}, { key: STRANGER_KEY }))), "signature");
    assert.equal(await reason(at(await mint(PERSON, {}, { header: { kid: "" } }))), "no key id");
    assert.equal(await reason(at(await mint(PERSON, {}, { header: { alg: "RS512" } }))), "algorithm");
  } finally {
    fake.restore();
  }
});

test("malformed tokens are refused before any key is fetched", async () => {
  const fake = new FakeAccess().install();
  try {
    const sig = enc("sig");
    for (const t of ["", "a.b", "a.b.c.d", `${enc("[1]")}.${enc({})}.${sig}`, `${enc({ alg: "RS256" })}.${enc("not json")}.${sig}`, `${enc({ alg: "RS256" })}.${enc({})}.`, `${enc({ alg: "RS256" })}.${enc({})}.@@`, `${b64urlEncode(new Uint8Array([0xff, 0xfe]))}.${enc({})}.${sig}`, `${enc({ alg: "RS256" })}.${"A".repeat(17_000)}.${sig}`]) {
      assert.equal(await reason(at(t)), t ? "malformed" : "no token", t.slice(0, 40));
    }
    assert.equal(await reason(new Request("https://example.com/auth/access")), "no token");
    assert.equal(fake.requests.length, 0);
  } finally {
    fake.restore();
  }
});

test("groups: a top-level list or custom.groups, names or {id, name} objects, trimmed, capped, sorted; none → null", () => {
  assert.equal(groupsOf({}), null);
  assert.equal(groupsOf({ groups: "admins" }), null);
  assert.equal(groupsOf({ custom: "x" }), null);
  assert.deepEqual(groupsOf({ groups: [] }), []);
  assert.deepEqual(groupsOf({ groups: [" b ", "a", "a", "", 7, null, { id: "g-1", name: "Editors" }, { name: 5 }, "x".repeat(257)] }), ["Editors", "a", "b", "g-1"]);
  assert.deepEqual(groupsOf({ custom: { groups: ["ops"] } }), ["ops"]);
  assert.deepEqual(groupsOf({ groups: ["top"], custom: { groups: ["ops"] } }), ["top"]);
  assert.equal(groupsOf({ groups: Array.from({ length: 300 }, (_, i) => `g${i}`) })?.length, 200);
});
