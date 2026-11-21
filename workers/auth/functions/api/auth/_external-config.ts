// An external JWT issuer as a way to sign in (#435): its variables, read and
// checked. A leading underscore keeps this file out of the Pages route table.
//
// A site that already runs its own login hands the person a short JWT; POST
// /api/auth/jwt verifies it here and exchanges it for a normal session. The
// issuer vouches for who the person is; what they may DO comes from this
// worker's roles (optionally from the token's roles claim, mapped). The
// method id is "jwt:<issuer>" (stored with the session and the identity;
// AUTH_ROLE_METHODS may name it). docs/EXTERNAL-JWT.md is the reference.
//
//   AUTH_JWT_ISSUER          the exact `iss` (required)
//   AUTH_JWT_AUDIENCE        the `aud` this site expects (required)
//   AUTH_JWT_ALGORITHMS      comma-separated (default: every asymmetric one
//                            below); `none` is never accepted
//   AUTH_JWT_JWKS_URL        the issuer's JWK Set (RFC 7517), https only   ┐
//   AUTH_JWT_PUBLIC_KEY      a JWK, a JWK Set or a PEM public key           ├ exactly one
//   AUTH_JWT_SECRET          secret: an HMAC key of ≥ 32 bytes, HS* only    ┘
//   AUTH_JWT_MAX_AGE_SECONDS the oldest `iat` accepted (default 300)
//   AUTH_JWT_IDENTITY_CLAIM  "sub" (default) or "email"
//   AUTH_JWT_EMAIL_VERIFIED  "required" (default) or "off": an address counts
//                            only with `email_verified: true`
//   AUTH_JWT_UNKNOWN         "reject" (default), "invite" or "create"
//   AUTH_JWT_ROLES_CLAIM     the claim with the person's roles (default "roles")
//   AUTH_JWT_ROLE_MAP        JSON {"<external role>": {"roles": [...], "groups": [...]}}
//   AUTH_JWT_ALLOW_ADMIN_ROLES "off" (default) or "on": without "on" the
//                            mapping may not reach a role holding an
//                            administrative permission (_external-roles.ts)
//   AUTH_JWT_FORM_ORIGINS    comma-separated origins whose pages may post the
//                            token as a plain form (cross-origin)
//   AUTH_JWT_ALLOWED_DOMAINS like every provider's (_provider-policy.ts)
//   AUTH_JWT_LINK_URL        optional https:// page of the issuer that signs a
//                            token and posts it to /api/auth/jwt/link: the
//                            account page's "Connect" button (_external-link.ts)
//
// Nothing set → the method does not exist (its route answers 404). Anything
// set but not usable — a missing issuer, two key sources, HS256 next to a
// public key (algorithm confusion, RFC 8725 §2.1/§3.1), a short secret →
// misconfigured: its route answers 503 naming what to fix. Never opened by a
// half configuration.

import { staticKeys } from "./_external-keys";
import type { Jwk } from "./_external-keys";
import type { Env } from "./_lib";
import { parseRoleMap } from "./_external-roles";

export const JWT_PREFIX = "jwt:";
export const JWT_ENV_ID = "jwt"; // AUTH_JWT_ALLOWED_DOMAINS

// Every algorithm this worker can verify, with its WebCrypto parameters.
export const ASYMMETRIC = ["RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"] as const;
export const SYMMETRIC = ["HS256", "HS384", "HS512"] as const;
const KNOWN = new Set<string>([...ASYMMETRIC, ...SYMMETRIC]);

export const MIN_SECRET_BYTES = 32;
export const SKEW_SECONDS = 60;

export type KeySource = { kind: "jwks"; url: string; host: string } | { kind: "static"; keys: Jwk[] } | { kind: "secret"; secret: Uint8Array };

export interface RoleTargets {
  roles: string[];
  groups: string[];
}

export interface ExternalSetup {
  method: string; // jwt:<issuer>
  issuer: string;
  audience: string;
  algorithms: string[];
  source: KeySource;
  maxAgeSeconds: number;
  identityClaim: "sub" | "email";
  emailVerified: boolean; // an address needs email_verified: true
  unknown: "reject" | "invite" | "create";
  rolesClaim: string;
  roleMap: Map<string, RoleTargets> | null;
  allowAdminRoles: boolean; // the mapping may give administrative roles
  formOrigins: string[];
  linkUrl: string; // "" when unset: no "Connect" button on the account page
}

export type ExternalLookup = { kind: "enabled"; setup: ExternalSetup } | { kind: "absent" } | { kind: "misconfigured"; problem: string };

const VARS = ["ISSUER", "AUDIENCE", "ALGORITHMS", "JWKS_URL", "PUBLIC_KEY", "SECRET", "MAX_AGE_SECONDS", "IDENTITY_CLAIM", "EMAIL_VERIFIED", "UNKNOWN", "ROLES_CLAIM", "ROLE_MAP", "ALLOW_ADMIN_ROLES", "FORM_ORIGINS", "LINK_URL"];

const read = (env: Env, name: string): string => {
  const v = (env as unknown as Record<string, unknown>)[`AUTH_JWT_${name}`];
  return typeof v === "string" ? v.trim() : "";
};

// A claim value or a name: printable, no spaces, bounded.
const TOKEN_TEXT = /^[\x21-\x7e]{1,512}$/;
const CLAIM_NAME = /^[A-Za-z0-9_:./-]{1,128}$/;

const list = (raw: string): string[] => raw.split(",").map((v) => v.trim()).filter(Boolean);

function keySource(env: Env): KeySource | string {
  const url = read(env, "JWKS_URL");
  const pub = read(env, "PUBLIC_KEY");
  const secret = (env as unknown as Record<string, unknown>).AUTH_JWT_SECRET;
  const set = [url, pub, typeof secret === "string" && secret ? "x" : ""].filter(Boolean).length;
  if (set !== 1) return "set exactly one key source: AUTH_JWT_JWKS_URL, AUTH_JWT_PUBLIC_KEY or the AUTH_JWT_SECRET secret";
  if (url) {
    let u: URL;
    try {
      u = new URL(url);
    } catch {
      return "AUTH_JWT_JWKS_URL is not a URL";
    }
    if (u.protocol !== "https:" || u.username || u.password) return "AUTH_JWT_JWKS_URL must be an https:// URL without credentials";
    return { kind: "jwks", url: u.href, host: u.host };
  }
  if (pub) {
    const keys = staticKeys(pub);
    return keys.length ? { kind: "static", keys } : "AUTH_JWT_PUBLIC_KEY must be a JWK, a JWK Set or a PEM public key (-----BEGIN PUBLIC KEY-----)";
  }
  const bytes = new TextEncoder().encode(secret as string);
  if (bytes.length < MIN_SECRET_BYTES) return `AUTH_JWT_SECRET must be at least ${MIN_SECRET_BYTES} bytes (openssl rand -base64 48)`;
  return { kind: "secret", secret: bytes };
}

function algorithms(raw: string, source: KeySource): string[] | string {
  const algs = raw ? list(raw) : [...ASYMMETRIC];
  const unknown = algs.filter((a) => !KNOWN.has(a));
  if (unknown.length) return `AUTH_JWT_ALGORITHMS names ${unknown.join(", ").slice(0, 80)}, which this worker does not accept (known: ${[...KNOWN].join(", ")})`;
  const hs = algs.filter((a) => a.startsWith("HS"));
  if (source.kind === "secret") {
    if (hs.length !== algs.length) return "AUTH_JWT_SECRET goes only with HS256/HS384/HS512 in AUTH_JWT_ALGORITHMS: a shared secret next to public-key algorithms invites algorithm confusion (RFC 8725)";
  } else if (hs.length) {
    return "HS256/HS384/HS512 need the AUTH_JWT_SECRET secret and no public key: a public key used as an HMAC secret is the algorithm-confusion attack (RFC 8725)";
  }
  return algs;
}

function origins(raw: string): string[] | string {
  const out: string[] = [];
  for (const o of list(raw)) {
    let u: URL | null = null;
    try {
      u = new URL(o);
    } catch {
      // reported below
    }
    if (!u || u.protocol !== "https:" || u.origin !== o.replace(/\/$/, "")) return `AUTH_JWT_FORM_ORIGINS must list origins like https://id.example.com (no path); "${o.slice(0, 80)}" is not one`;
    out.push(u.origin);
  }
  return out;
}

// linkUrl: the optional page that starts a link, https without credentials.
function linkUrl(raw: string): string | null {
  if (!raw) return "";
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  return u.protocol === "https:" && !u.username && !u.password ? u.href : null;
}

// choice reads a variable that must be one of `allowed` (the first is the default).
function choice<T extends string>(env: Env, name: string, allowed: readonly T[]): T | null {
  const v = read(env, name).toLowerCase() || allowed[0];
  return (allowed as readonly string[]).includes(v) ? (v as T) : null;
}

export function externalLookup(env: Env): ExternalLookup {
  if (!VARS.some((n) => read(env, n))) return { kind: "absent" };
  const bad = (problem: string): ExternalLookup => ({ kind: "misconfigured", problem });
  const issuer = read(env, "ISSUER");
  const audience = read(env, "AUDIENCE");
  if (!TOKEN_TEXT.test(issuer)) return bad("AUTH_JWT_ISSUER must be the issuer's exact `iss` value (no spaces)");
  if (!TOKEN_TEXT.test(audience)) return bad("AUTH_JWT_AUDIENCE must be the `aud` value the issuer puts in tokens for this site (no spaces)");
  const source = keySource(env);
  if (typeof source === "string") return bad(source);
  const algs = algorithms(read(env, "ALGORITHMS"), source);
  if (typeof algs === "string") return bad(algs);
  const age = read(env, "MAX_AGE_SECONDS") || "300";
  const maxAgeSeconds = /^\d{1,5}$/.test(age) ? Number(age) : 0;
  if (maxAgeSeconds < 1 || maxAgeSeconds > 3600) return bad("AUTH_JWT_MAX_AGE_SECONDS must be a whole number of seconds from 1 to 3600");
  const identityClaim = choice(env, "IDENTITY_CLAIM", ["sub", "email"] as const);
  if (!identityClaim) return bad('AUTH_JWT_IDENTITY_CLAIM must be "sub" or "email"');
  const verified = choice(env, "EMAIL_VERIFIED", ["required", "off"] as const);
  if (!verified) return bad('AUTH_JWT_EMAIL_VERIFIED must be "required" or "off"');
  const unknown = choice(env, "UNKNOWN", ["reject", "invite", "create"] as const);
  if (!unknown) return bad('AUTH_JWT_UNKNOWN must be "reject", "invite" or "create"');
  const rolesClaim = read(env, "ROLES_CLAIM") || "roles";
  if (!CLAIM_NAME.test(rolesClaim)) return bad("AUTH_JWT_ROLES_CLAIM must be a claim name");
  const map = parseRoleMap(read(env, "ROLE_MAP"));
  if (typeof map === "string") return bad(map);
  const allowAdmin = choice(env, "ALLOW_ADMIN_ROLES", ["off", "on"] as const);
  if (!allowAdmin) return bad('AUTH_JWT_ALLOW_ADMIN_ROLES must be "off" or "on"');
  const formOrigins = origins(read(env, "FORM_ORIGINS"));
  if (typeof formOrigins === "string") return bad(formOrigins);
  const link = linkUrl(read(env, "LINK_URL"));
  if (link === null) return bad("AUTH_JWT_LINK_URL must be an https:// URL without credentials");
  return {
    kind: "enabled",
    setup: {
      method: JWT_PREFIX + issuer,
      issuer,
      audience,
      algorithms: algs,
      source,
      maxAgeSeconds,
      identityClaim,
      emailVerified: verified === "required",
      unknown,
      rolesClaim,
      roleMap: map,
      allowAdminRoles: allowAdmin === "on",
      formOrigins,
      linkUrl: link,
    },
  };
}
