// Differential test (GO-135): a generated corpus of valid and mutated tokens must get the same
// decision — and the corresponding reason — from the portable verifier and from auth's own
// verifyToken, the reference. The `kind` claim (which verifyToken does not read) and tokens over
// MAX_TOKEN (which it does not cap) are covered by verifier-attacks.test.ts instead.
import { test } from "node:test";
import assert from "node:assert/strict";
import { loadKeys } from "../functions/api/auth/_keys.ts";
import type { KeySet } from "../functions/api/auth/_keys.ts";
import { LEEWAY_SECONDS, verifyToken } from "../functions/api/auth/_jwt.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { verifySession } from "../portable/session-verifier.ts";
import { KEYS, newKeyPair } from "./fakes.ts";
import { ISSUER, KEYSET, NOW, NOW_MS, claims, config, keySet, part, withCookie } from "./verifier-fixtures.ts";

const CORPUS = 3000;
// verifyToken's reasons, named as the portable verifier names them.
const SAME: Record<string, string> = {
  malformed: "malformed",
  claims: "malformed",
  algorithm: "bad_signature",
  signature: "bad_signature",
  unknown_key: "unknown_key",
  version: "unsupported_contract",
  issuer: "wrong_issuer",
  audience: "wrong_audience",
  expired: "expired",
  future: "not_yet_valid",
};

// A small seeded generator (mulberry32), so a failure is reproducible.
function rng(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const MISSING = Symbol("missing");
const VARIANTS: Record<string, unknown[]> = {
  iss: [ISSUER, "https://evil.example.com/api/auth", `${ISSUER}/`, 1, null, MISSING],
  aud: ["example.com", "evil.example.com", ["example.com"], "", null, MISSING],
  sub: ["u2", "", 7, null, MISSING],
  sid: ["s2", 0, [], MISSING],
  email: ["bo@example.com", "", false, MISSING],
  iat: [NOW - 3600, NOW + LEEWAY_SECONDS, NOW + LEEWAY_SECONDS + 1, NOW + 0.5, "now", null, MISSING],
  exp: [NOW - LEEWAY_SECONDS, NOW - LEEWAY_SECONDS + 1, NOW + 1e9, 1e300, "never", null, MISSING],
  ver: [2, 0, "1", 1.0, null, MISSING],
  perms: [[], ["*"], "owner", [1], ["a", null], {}, MISSING],
};
const ALGS = ["none", "HS256", "RS256", "ES384", "es256", ""];

const enc = new TextEncoder();
async function signRaw(header: unknown, payload: unknown, keys: KeySet): Promise<string> {
  const unsigned = `${part(header)}.${part(payload)}`;
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, keys.signing.privateKey, enc.encode(unsigned));
  return `${unsigned}.${b64urlEncode(new Uint8Array(sig))}`;
}

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

test("differential: portable verifySession agrees with verifyToken on a generated corpus", async (t) => {
  const old = await newKeyPair();
  const oldSet = await keySet(old.priv);
  const stranger = await keySet((await newKeyPair()).priv);
  const reference = (await loadKeys({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: old.pub } as Env)) as KeySet;
  const portableConfig = config({ publicKey: JSON.stringify({ keys: [JSON.parse(KEYS.pub), JSON.parse(old.pub)] }) });
  const opts = { keys: reference.verify, issuer: ISSUER, audience: "example.com", nowSeconds: NOW };
  const rand = rng(135);
  const pick = <T>(list: readonly T[]): T => list[Math.floor(rand() * list.length)];
  const seen = new Map<string, number>();

  for (let i = 0; i < CORPUS; i++) {
    const payload: Record<string, unknown> = { ...claims() };
    for (const [name, variants] of Object.entries(VARIANTS)) {
      if (rand() < 0.12) {
        const v = pick(variants);
        if (v === MISSING) delete payload[name];
        else payload[name] = v;
      }
    }
    const signer = rand() < 0.1 ? oldSet : rand() < 0.05 ? stranger : KEYSET;
    const header: Record<string, unknown> = { alg: rand() < 0.08 ? pick(ALGS) : "ES256", typ: "JWT", kid: signer.signing.kid };
    if (rand() < 0.04) header.kid = pick([undefined, 1, "", "nope", KEYSET.signing.kid]);
    if (rand() < 0.03) header.crit = ["exp"];
    let token = await signRaw(header, payload, signer);
    const roll = rand();
    if (roll < 0.06) {
      // flip one character anywhere (dots included): signature, payload or header damage
      const at = Math.floor(rand() * token.length);
      token = token.slice(0, at) + pick([...ALPHABET, "."]) + token.slice(at + 1);
    } else if (roll < 0.08) {
      token = pick([`${token}.x`, token.split(".").slice(0, 2).join("."), token.slice(0, Math.floor(rand() * token.length)) || "A"]);
    } else if (roll < 0.09) {
      token = token.replace(/^[^.]+/, part({ alg: "none", kid: KEYSET.signing.kid }));
    }

    const ref = await verifyToken(token, opts);
    const got = await verifySession(withCookie(token), portableConfig, { now: () => NOW_MS });
    const want = ref.ok ? "ok" : SAME[ref.reason];
    const have = got.ok ? "ok" : got.reason;
    assert.equal(have, want, `token #${i}: ${token}`);
    if (ref.ok && got.ok) {
      assert.deepEqual([got.user.id, got.user.email, got.sessionId, got.perms], [ref.claims.sub, ref.claims.email, ref.claims.sid, ref.claims.perms]);
    }
    seen.set(want, (seen.get(want) ?? 0) + 1);
  }
  // The corpus must actually exercise both sides and every reason.
  for (const reason of ["ok", ...new Set(Object.values(SAME))]) assert.ok((seen.get(reason) ?? 0) > 0, `no ${reason} case generated: ${JSON.stringify([...seen])}`);
  assert.ok((seen.get("ok") as number) > CORPUS / 4, `accepted: ${seen.get("ok")}`);
  t.diagnostic(`decisions: ${JSON.stringify(Object.fromEntries([...seen].sort()))}`);
});
