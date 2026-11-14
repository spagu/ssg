// Cloudflare Access JWT verification. A leading underscore keeps this file out
// of the Pages route table.
//
// When CORE_ACCESS_TEAM + CORE_ACCESS_AUD are set, core sits behind a
// Cloudflare Access application: Access signs the person in with your IdP and
// forwards a signed JWT in the Cf-Access-Jwt-Assertion header (and a
// CF_Authorization cookie). We verify that JWT against Access's public keys.
// Same checks as the copies in comments, newsletter and newsletter-ui; this one
// also returns the signed-in address, which core shows as `user`.

import { problem } from "./_lib";

interface JWK {
  kid: string;
  kty: string;
  n: string;
  e: string;
}

export type AccessResult = { ok: true; email: string | null } | { ok: false; response: Response };

// Per-isolate JWKS cache. Access keys rotate, so it is refreshed hourly.
let jwksCache: { host: string; keys: Record<string, CryptoKey>; fetchedAt: number } | null = null;
const JWKS_TTL_MS = 60 * 60 * 1000;

// A team may be given as the bare name or the full hostname.
const teamHost = (team: string): string => (team.includes(".") ? team : `${team}.cloudflareaccess.com`);

function b64urlToBytes(s: string): Uint8Array<ArrayBuffer> {
  let b = s.replaceAll("-", "+").replaceAll("_", "/");
  while (b.length % 4) b += "=";
  const bin = atob(b);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

const jsonFromB64url = (s: string): Record<string, unknown> => JSON.parse(new TextDecoder().decode(b64urlToBytes(s)));

async function importKeys(host: string): Promise<Record<string, CryptoKey>> {
  const res = await fetch(`https://${host}/cdn-cgi/access/certs`);
  if (!res.ok) throw new Error(`JWKS HTTP ${res.status}`);
  const { keys } = (await res.json()) as { keys: JWK[] };
  const out: Record<string, CryptoKey> = {};
  for (const k of keys) {
    if (k.kty !== "RSA") continue;
    out[k.kid] = await crypto.subtle.importKey(
      "jwk",
      { kty: "RSA", n: k.n, e: k.e, alg: "RS256", ext: true },
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      false,
      ["verify"],
    );
  }
  return out;
}

async function keysFor(host: string): Promise<Record<string, CryptoKey>> {
  const now = Date.now();
  if (jwksCache && jwksCache.host === host && now - jwksCache.fetchedAt < JWKS_TTL_MS) return jwksCache.keys;
  const keys = await importKeys(host);
  jwksCache = { host, keys, fetchedAt: now };
  return keys;
}

function cookie(request: Request, name: string): string {
  const raw = request.headers.get("cookie") || "";
  const m = raw.match(new RegExp("(?:^|;\\s*)" + name + "=([^;]+)"));
  return m ? m[1] : "";
}

const refuse = (error: string, status: number): AccessResult => ({ ok: false, response: problem(error, status) });

// verifyAccess checks the Access JWT for this application (`aud`) and team.
export async function verifyAccess(request: Request, team: string, aud: string): Promise<AccessResult> {
  const token = request.headers.get("cf-access-jwt-assertion") || cookie(request, "CF_Authorization");
  if (!token) return refuse("access_required", 401);
  const parts = token.split(".");
  if (parts.length !== 3) return refuse("access_token_malformed", 401);

  let header: Record<string, unknown>;
  let payload: Record<string, unknown>;
  try {
    header = jsonFromB64url(parts[0]);
    payload = jsonFromB64url(parts[1]);
  } catch {
    return refuse("access_token_malformed", 401);
  }
  // The algorithm is ours to choose, never the token's.
  if (header.alg !== "RS256") return refuse("access_token_algorithm", 401);

  const host = teamHost(team);
  let keys: Record<string, CryptoKey>;
  try {
    keys = await keysFor(host);
  } catch {
    return refuse("access_unreachable", 503);
  }
  const key = keys[header.kid as string];
  if (!key) return refuse("access_key_unknown", 401);

  const signed = new TextEncoder().encode(parts[0] + "." + parts[1]);
  const ok = await crypto.subtle.verify({ name: "RSASSA-PKCS1-v1_5" }, key, b64urlToBytes(parts[2]), signed);
  if (!ok) return refuse("access_signature_invalid", 401);

  // The token must be for THIS application, from our team, and unexpired. A
  // token without exp was not minted by Access, which always sets it.
  const auds = Array.isArray(payload.aud) ? payload.aud : [payload.aud];
  if (!auds.includes(aud)) return refuse("access_audience_wrong", 403);
  if (payload.iss !== `https://${host}`) return refuse("access_issuer_wrong", 403);
  if (typeof payload.exp !== "number" || Date.now() / 1000 > payload.exp) return refuse("access_token_expired", 401);
  return { ok: true, email: typeof payload.email === "string" ? payload.email : null };
}
