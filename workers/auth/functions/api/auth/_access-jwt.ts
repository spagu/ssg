// Verifying the token Cloudflare Access puts in front of /auth/access (GO-139).
// A leading underscore keeps this file out of the Pages route table.
//
// Access signs a JWT (RS256) for the person it let through and hands it to the
// origin in the Cf-Access-Jwt-Assertion header, and to the browser as the
// CF_Authorization cookie; either is read (the header first). Every check
// refuses; nothing falls back to "let them in":
//   - the algorithm is RS256 and nothing else (no "none", no HS256 with the
//     public key as a secret), the key is one of THIS team's (_access-keys.ts);
//   - aud contains this application's AUD — a token Access issued for another
//     application of the same team is refused;
//   - iss is the team's own host;
//   - exp is REQUIRED and not passed (the older copies in other templates let
//     a token without exp through; this one does not), nbf / iat, when
//     present, not in the future (60 s of clock skew);
//   - type, when present, is "app" (an application token, not the team's own);
//   - sub — the person's stable id at Access — is present.
// The worker copies of verifyAccess elsewhere (newsletter-ui, comments) only
// answer "may enter"; this one hands back who: sub, the address, and the
// groups when the identity provider sends them. No token or claim value ever
// reaches a log: reasons only.

import { ACCESS_COOKIE_NAME, ACCESS_HEADER, accessIssuer } from "./_access-config";
import type { AccessSetup } from "./_access-config";
import { accessKey } from "./_access-keys";
import { readCookie } from "./_cookies";
import { b64urlDecode } from "./_lib";
import { byCodeUnit } from "./_perms";

export interface AccessPerson {
  subject: string;
  email: string; // as Access gave it, trimmed; "" when absent
  groups: string[] | null; // null = the token carries no groups claim at all
}

export type AccessVerified = { ok: true; person: AccessPerson } | { ok: false; reason: string; unavailable: boolean };

const SKEW_SECONDS = 60;
const MAX_TOKEN = 16_384;
const MAX_GROUPS = 200;

type Obj = Record<string, unknown>;

function jsonPart(part: string): Obj | null {
  const bytes = b64urlDecode(part);
  if (!bytes) return null;
  try {
    const v: unknown = JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(bytes));
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Obj) : null;
  } catch {
    return null;
  }
}

// accessTokenOf: the header Access sets, else the cookie it gives the browser.
export const accessTokenOf = (request: Request): string => (request.headers.get(ACCESS_HEADER) ?? "").trim() || readCookie(request, ACCESS_COOKIE_NAME);

// groupsOf reads the groups an identity provider passed through Access: a
// top-level `groups`, or `custom.groups` (an OIDC claim passed through); an
// entry is a name, or an object whose id and name both count. null when
// neither claim is there — then group memberships are left alone.
export function groupsOf(c: Obj): string[] | null {
  const custom = c.custom && typeof c.custom === "object" ? (c.custom as Obj).groups : undefined;
  const raw = Array.isArray(c.groups) ? c.groups : Array.isArray(custom) ? custom : null;
  if (!raw) return null;
  const out = new Set<string>();
  for (const g of raw.slice(0, MAX_GROUPS)) {
    const values = typeof g === "string" ? [g] : g && typeof g === "object" ? [(g as Obj).id, (g as Obj).name] : [];
    for (const v of values) if (typeof v === "string" && v.trim() && v.length <= 256) out.add(v.trim());
  }
  return [...out].sort(byCodeUnit);
}

const notNumberOrFuture = (v: unknown, now: number): boolean => v !== undefined && (typeof v !== "number" || now + SKEW_SECONDS < v);

// claimProblem: why the (signature-checked) claims are not acceptable, or null.
export function claimProblem(c: Obj, s: AccessSetup, now: number): string | null {
  const auds = Array.isArray(c.aud) ? c.aud : [c.aud];
  if (!auds.includes(s.aud)) return "audience";
  if (c.iss !== accessIssuer(s)) return "issuer";
  if (typeof c.exp !== "number" || !Number.isFinite(c.exp)) return "no expiry";
  if (now >= c.exp) return "expired";
  if (notNumberOrFuture(c.nbf, now)) return "not yet valid";
  if (notNumberOrFuture(c.iat, now)) return "issued in the future";
  if (c.type !== undefined && c.type !== "app") return "token type";
  if (typeof c.sub !== "string" || !c.sub || c.sub.length > 255) return "no subject";
  return null;
}

export async function verifyAccessToken(request: Request, s: AccessSetup, nowMs: number): Promise<AccessVerified> {
  const fail = (reason: string, unavailable = false): AccessVerified => ({ ok: false, reason, unavailable });
  const token = accessTokenOf(request);
  if (!token) return fail("no token");
  const parts = token.split(".");
  if (token.length > MAX_TOKEN || parts.length !== 3) return fail("malformed");
  const header = jsonPart(parts[0]);
  const claims = jsonPart(parts[1]);
  const sig = b64urlDecode(parts[2]);
  if (!header || !claims || !sig?.length) return fail("malformed");
  if (header.alg !== "RS256") return fail("algorithm");
  if (typeof header.kid !== "string" || !header.kid) return fail("no key id");
  const key = await accessKey(s.host, header.kid, nowMs);
  if (key === "unavailable") return fail("keys unavailable", true);
  if (!key) return fail("unknown key id");
  const signed = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  if (!(await crypto.subtle.verify("RSASSA-PKCS1-v1_5", key, sig as BufferSource, signed))) return fail("signature");
  const problem = claimProblem(claims, s, Math.floor(nowMs / 1000));
  if (problem) return fail(problem);
  const email = typeof claims.email === "string" ? claims.email.trim() : "";
  return { ok: true, person: { subject: claims.sub as string, email, groups: groupsOf(claims) } };
}
