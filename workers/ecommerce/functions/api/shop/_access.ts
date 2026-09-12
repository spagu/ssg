// Cloudflare Access JWT verification, for the shop's admin surface.
//
// Access authenticates the person against the team's IdP and forwards a signed
// JWT in Cf-Access-Jwt-Assertion (and a CF_Authorization cookie). Verifying it
// against Access's published keys means there is no shared password anywhere to
// store, rotate or leak.
//
// This grew out of workers/comments/functions/api/comments/_access.ts. It is a
// copy rather than a shared module because Pages Functions resolve imports by
// relative path and copy only one worker's functions/ tree into the build, so a
// module above that tree cannot be reached from the deployed output. Folding
// the two together needs a build step the generator does not have yet.

interface JWK {
  kid: string;
  kty: string;
  n: string;
  e: string;
}

export interface AccessOptions {
  /** "myteam" or "myteam.cloudflareaccess.com". */
  team: string;
  /** The application's AUD tag, from Access → the app → Overview. */
  aud: string;
}

export type AccessVerdict =
  | { ok: true; email: string; sub: string; payload: Record<string, unknown> }
  | { ok: false; code: string; message: string; status: number };

/** Per-isolate key cache. Access rotates its signing keys, so it is refreshed
 *  hourly rather than held for the life of the isolate. */
let jwksCache: { host: string; keys: Record<string, CryptoKey>; fetchedAt: number } | null = null;
const JWKS_TTL_MS = 60 * 60 * 1000;

/** Exposed for tests, which need to start from an empty cache. */
export function resetAccessCache(): void {
  jwksCache = null;
}

function teamHost(team: string): string {
  return team.includes(".") ? team : `${team}.cloudflareaccess.com`;
}

function b64urlToBytes(s: string): Uint8Array {
  let b = s.replaceAll("-", "+").replaceAll("_", "/");
  while (b.length % 4) b += "=";
  const bin = atob(b);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function jsonFromB64url(s: string): Record<string, unknown> {
  return JSON.parse(new TextDecoder().decode(b64urlToBytes(s))) as Record<string, unknown>;
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
  const raw = request.headers.get("cookie") ?? "";
  const m = new RegExp(`(?:^|;\\s*)${name}=([^;]+)`).exec(raw);
  return m?.[1] ?? "";
}

/** Verifies the Access token on a request, or explains why it will not.
 *
 *  Checking `aud` is what stops a token minted for a different Access
 *  application on the same team from working here. */
export async function verifyAccessJwt(request: Request, opts: AccessOptions): Promise<AccessVerdict> {
  const token =
    request.headers.get("cf-access-jwt-assertion") || cookie(request, "CF_Authorization");
  if (!token) {
    return { ok: false, code: "access_required", message: "Cloudflare Access is required here.", status: 401 };
  }

  const parts = token.split(".");
  if (parts.length !== 3) {
    return { ok: false, code: "malformed_token", message: "That Access token is malformed.", status: 401 };
  }

  let header: Record<string, unknown>;
  let payload: Record<string, unknown>;
  try {
    header = jsonFromB64url(parts[0]!);
    payload = jsonFromB64url(parts[1]!);
  } catch {
    return { ok: false, code: "malformed_token", message: "That Access token is malformed.", status: 401 };
  }
  if (header.alg !== "RS256") {
    return { ok: false, code: "unexpected_algorithm", message: "Unexpected token algorithm.", status: 401 };
  }

  const host = teamHost(opts.team);
  let keys: Record<string, CryptoKey>;
  try {
    keys = await keysFor(host);
  } catch {
    return { ok: false, code: "access_unreachable", message: "Cannot reach Cloudflare Access to verify.", status: 503 };
  }
  const key = keys[String(header.kid ?? "")];
  if (!key) {
    return { ok: false, code: "unknown_key", message: "Unknown Access signing key.", status: 401 };
  }

  const signed = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  const valid = await crypto.subtle.verify(
    { name: "RSASSA-PKCS1-v1_5" },
    key,
    b64urlToBytes(parts[2]!),
    signed,
  );
  if (!valid) {
    return { ok: false, code: "invalid_signature", message: "Invalid Access signature.", status: 401 };
  }

  const auds = Array.isArray(payload.aud) ? payload.aud : [payload.aud];
  if (!auds.includes(opts.aud)) {
    return { ok: false, code: "wrong_audience", message: "That token is for a different application.", status: 403 };
  }
  if (payload.iss !== `https://${host}`) {
    return { ok: false, code: "wrong_issuer", message: "That token was issued by a different team.", status: 403 };
  }
  if (typeof payload.exp === "number" && Date.now() / 1000 > payload.exp) {
    return { ok: false, code: "token_expired", message: "That Access session has expired.", status: 401 };
  }

  return {
    ok: true,
    email: String(payload.email ?? ""),
    sub: String(payload.sub ?? ""),
    payload,
  };
}
