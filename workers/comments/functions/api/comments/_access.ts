// Cloudflare Access JWT verification for the moderation endpoints. A leading
// underscore keeps this file out of the Pages route table — it is imported by
// _lib's requireAdmin, never served.
//
// When COMMENTS_ACCESS_TEAM + COMMENTS_ACCESS_AUD are set, the panel sits behind
// a Cloudflare Access application: Access authenticates the moderator with your
// IdP and forwards a signed JWT in the Cf-Access-Jwt-Assertion header (and a
// CF_Authorization cookie). We verify that JWT against Access's public keys, so
// there is no shared password to store, rotate, or leak.

import { Env, json } from "./_lib";

interface JWK {
  kid: string;
  kty: string;
  n: string;
  e: string;
}

// Per-isolate JWKS cache. Access keys rotate, so it is refreshed hourly.
let jwksCache: { host: string; keys: Record<string, CryptoKey>; fetchedAt: number } | null = null;
const JWKS_TTL_MS = 60 * 60 * 1000;

// A team may be given as the bare name or the full hostname.
function teamHost(team: string): string {
  return team.includes(".") ? team : `${team}.cloudflareaccess.com`;
}

function b64urlToBytes(s: string): Uint8Array {
  let b = s.replace(/-/g, "+").replace(/_/g, "/");
  while (b.length % 4) b += "=";
  const bin = atob(b);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function jsonFromB64url(s: string): Record<string, unknown> {
  return JSON.parse(new TextDecoder().decode(b64urlToBytes(s)));
}

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
  if (jwksCache && jwksCache.host === host && now - jwksCache.fetchedAt < JWKS_TTL_MS) {
    return jwksCache.keys;
  }
  const keys = await importKeys(host);
  jwksCache = { host, keys, fetchedAt: now };
  return keys;
}

function cookie(request: Request, name: string): string {
  const raw = request.headers.get("cookie") || "";
  const m = raw.match(new RegExp("(?:^|;\\s*)" + name + "=([^;]+)"));
  return m ? m[1] : "";
}

// verifyAccess returns null when a valid Access JWT for this application is
// present, or a Response (401/403/503) describing why it was rejected. Callers
// must have already checked that COMMENTS_ACCESS_TEAM/AUD are configured.
export async function verifyAccess(request: Request, env: Env): Promise<Response | null> {
  const token = request.headers.get("cf-access-jwt-assertion") || cookie(request, "CF_Authorization");
  if (!token) return json({ error: "Cloudflare Access required" }, 401);

  const parts = token.split(".");
  if (parts.length !== 3) return json({ error: "malformed Access token" }, 401);

  let header: Record<string, unknown>;
  let payload: Record<string, unknown>;
  try {
    header = jsonFromB64url(parts[0]);
    payload = jsonFromB64url(parts[1]);
  } catch {
    return json({ error: "malformed Access token" }, 401);
  }
  if (header.alg !== "RS256") return json({ error: "unexpected token algorithm" }, 401);

  const host = teamHost(env.COMMENTS_ACCESS_TEAM as string);
  let keys: Record<string, CryptoKey>;
  try {
    keys = await keysFor(host);
  } catch {
    return json({ error: "cannot reach Cloudflare Access to verify" }, 503);
  }
  const key = keys[header.kid as string];
  if (!key) return json({ error: "unknown Access signing key" }, 401);

  const signed = new TextEncoder().encode(parts[0] + "." + parts[1]);
  const ok = await crypto.subtle.verify(
    { name: "RSASSA-PKCS1-v1_5" },
    key,
    b64urlToBytes(parts[2]),
    signed,
  );
  if (!ok) return json({ error: "invalid Access signature" }, 401);

  // Claims: the token must be for THIS application (aud), issued by our team,
  // and unexpired. Checking aud is what stops a valid token minted for another
  // Access app on the same team from moderating here.
  const auds = Array.isArray(payload.aud) ? payload.aud : [payload.aud];
  if (!auds.includes(env.COMMENTS_ACCESS_AUD)) return json({ error: "wrong Access audience" }, 403);
  if (payload.iss !== `https://${host}`) return json({ error: "wrong Access issuer" }, 403);
  if (typeof payload.exp === "number" && Date.now() / 1000 > payload.exp) {
    return json({ error: "Access token expired" }, 401);
  }
  return null;
}
