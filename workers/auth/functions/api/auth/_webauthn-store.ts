// Passkey ceremonies against the database (GO-140): the challenges, the
// options the browser's WebAuthn API is called with, and the two checks with
// what is stored. A leading underscore keeps this file out of the Pages route
// table. The byte-level checks are _webauthn.ts.
//
// A challenge is 32 random bytes kept server-side (mfa_challenges), like a
// provider flow's state: it names the account, the purpose (register,
// signin, step-up) and what it was issued to (the pending handle's hash for a
// sign-in, the session id otherwise). It is taken — marked used in the
// statement that checks it is unused and unexpired — before anything else is
// believed, so an answer replayed, or carried to another account, purpose or
// session, finds nothing.
//
// The relying party is this host: rpId = the request's host name, origin =
// its origin. A passkey therefore belongs to one host; the docs say so.

import { b64urlDecode, b64urlEncode, iso, randomToken } from "./_lib";
import type { Env } from "./_lib";
import { addPasskey, listFactors, moveCounter } from "./_mfa-factors";
import type { FactorRow } from "./_mfa-factors";
import type { User } from "./_users";
import { ALGORITHMS, WebAuthnError, verifyAssertion, verifyRegistration } from "./_webauthn";
import type { Expected } from "./_webauthn";
import { CborError } from "./_webauthn-cbor";

export type Purpose = "register" | "signin" | "step-up";
export const CHALLENGE_SECONDS = 300;

export async function issueChallenge(env: Env, userId: string, purpose: Purpose, boundTo: string, now: number): Promise<string> {
  const challenge = randomToken();
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("DELETE FROM mfa_challenges WHERE expires_at <= ?").bind(iso(now)),
    env.AUTH_DB.prepare("INSERT INTO mfa_challenges (challenge, user_id, purpose, bound_to, expires_at) VALUES (?, ?, ?, ?, ?)").bind(
      challenge,
      userId,
      purpose,
      boundTo,
      iso(now + CHALLENGE_SECONDS * 1000),
    ),
  ]);
  return challenge;
}

async function takeChallenge(env: Env, challenge: string, userId: string, purpose: Purpose, boundTo: string, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare(
    "UPDATE mfa_challenges SET used_at = ? WHERE challenge = ? AND user_id = ? AND purpose = ? AND bound_to = ? AND used_at IS NULL AND expires_at > ?",
  )
    .bind(iso(now), challenge, userId, purpose, boundTo, iso(now))
    .run();
  return r.meta.changes === 1;
}

export const passkeysOf = async (env: Env, userId: string): Promise<FactorRow[]> => (await listFactors(env, userId)).filter((f) => f.kind === "passkey");

const descriptor = (f: FactorRow) => ({ type: "public-key", id: f.credential_id as string, transports: JSON.parse(f.transports) as string[] });

// registerOptions: what navigator.credentials.create() is called with.
export async function registerOptions(env: Env, request: Request, user: User, sessionId: string, now: number) {
  const url = new URL(request.url);
  return {
    challenge: await issueChallenge(env, user.id, "register", sessionId, now),
    rp: { id: url.hostname, name: url.host },
    user: { id: b64urlEncode(new TextEncoder().encode(user.id)), name: user.email, displayName: user.display_name || user.email },
    pubKeyCredParams: ALGORITHMS.map((alg) => ({ type: "public-key", alg })),
    timeout: CHALLENGE_SECONDS * 1000,
    attestation: "none",
    excludeCredentials: (await passkeysOf(env, user.id)).map(descriptor),
    authenticatorSelection: { residentKey: "discouraged", userVerification: "preferred" },
  };
}

// assertOptions: what navigator.credentials.get() is called with; null when
// the account has no passkey.
export async function assertOptions(env: Env, request: Request, userId: string, purpose: Purpose, boundTo: string, now: number) {
  const keys = await passkeysOf(env, userId);
  if (!keys.length) return null;
  return {
    challenge: await issueChallenge(env, userId, purpose, boundTo, now),
    rpId: new URL(request.url).hostname,
    allowCredentials: keys.map(descriptor),
    timeout: CHALLENGE_SECONDS * 1000,
    userVerification: "preferred",
  };
}

type Fields = Record<string, unknown>;
// obj: an object, or a JSON object in a string (the pages send the answer in
// a hidden form field).
function obj(v: unknown): Fields | null {
  if (typeof v === "string") {
    try {
      v = JSON.parse(v);
    } catch {
      return null;
    }
  }
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Fields) : null;
}
const bytes = (v: unknown): Uint8Array | null => (typeof v === "string" && v.length <= 22_000 ? b64urlDecode(v) : null);

// challengeOf reads the challenge out of clientDataJSON — only to find the
// stored one; checkClientData compares it again.
function challengeOf(clientData: Uint8Array): string | null {
  try {
    const c = JSON.parse(new TextDecoder().decode(clientData)) as { challenge?: unknown };
    return typeof c?.challenge === "string" ? c.challenge : null;
  } catch {
    return null;
  }
}

const expected = (request: Request, challenge: string): Expected => ({ challenge, origin: new URL(request.url).origin, rpId: new URL(request.url).hostname });

// checked runs a byte-level check; a refusal of any kind is a plain false.
async function checked<T>(fn: () => Promise<T>): Promise<T | null> {
  try {
    return await fn();
  } catch (e) {
    if (e instanceof WebAuthnError || e instanceof CborError || e instanceof DOMException || e instanceof TypeError) return null;
    throw e;
  }
}

// checkAssertion verifies a passkey answer for this account, purpose and
// binding; the factor id when it passes.
export async function checkAssertion(env: Env, request: Request, userId: string, purpose: Purpose, boundTo: string, input: unknown, now: number): Promise<string | null> {
  const a = obj(input);
  const r = obj(a?.response);
  const clientData = bytes(r?.clientDataJSON);
  const authData = bytes(r?.authenticatorData);
  const signature = bytes(r?.signature);
  if (!a || typeof a.id !== "string" || !clientData || !authData || !signature) return null;
  const key = (await passkeysOf(env, userId)).find((f) => f.credential_id === a.id);
  const challenge = challengeOf(clientData);
  if (!key || !challenge || !(await takeChallenge(env, challenge, userId, purpose, boundTo, now))) return null;
  const stored = { alg: key.algorithm as number, jwk: JSON.parse(key.public_key as string) as JsonWebKey, signCount: key.sign_count };
  const count = await checked(() => verifyAssertion(clientData, authData, signature, stored, expected(request, challenge)));
  if (count === null) return null;
  return (await moveCounter(env, key.id, key.sign_count, count, now)) ? key.id : null;
}

// register verifies a passkey registration for this session and stores it;
// the new factor's id, or null.
export async function register(env: Env, request: Request, userId: string, sessionId: string, input: unknown, name: string, now: number): Promise<string | null> {
  const r = obj(obj(input)?.response);
  const clientData = bytes(r?.clientDataJSON);
  const attestation = bytes(r?.attestationObject);
  if (!clientData || !attestation) return null;
  const challenge = challengeOf(clientData);
  if (!challenge || !(await takeChallenge(env, challenge, userId, "register", sessionId, now))) return null;
  const reg = await checked(() => verifyRegistration(clientData, attestation, expected(request, challenge)));
  if (!reg) return null;
  const transports = Array.isArray(r?.transports) ? (r.transports as unknown[]).filter((t): t is string => typeof t === "string" && t.length <= 20).slice(0, 8) : [];
  return addPasskey(env, userId, { credentialId: reg.credentialId, publicKey: JSON.stringify(reg.jwk), algorithm: reg.alg, signCount: reg.signCount, transports, name }, now);
}
