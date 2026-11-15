// The three admin modes for the tests (GO-155), without importing workers/auth
// (the template is scaffolded alone):
//   - auth: a P-256 key pair made here and tokens signed exactly as the `auth`
//     worker signs them (ES256 compact JWS, kid = RFC 7638 thumbprint, the
//     contract's claims), and SHOP_AUTH_* trusting the public key;
//   - Cloudflare Access: an RSA key pair standing in for the team's, served
//     from a replaced fetch;
//   - the shared password.
// Each test gets a fresh database: the schema cache is reset with it.
import { onRequest as gate } from "../functions/api/shop/admin/_middleware";
import type { AdminData } from "../functions/api/shop/admin/_middleware";
import { resetAccessCache } from "../functions/api/shop/_access";
import type { Env } from "../functions/api/shop/_env";
import { resetSchemaCache } from "../functions/api/shop/_schema";
import { invalidateSettings } from "../functions/api/shop/_settings";
import { fakeD1 } from "./fakes";

export const ORIGIN = "https://shop.example.com";
export const ISSUER = `${ORIGIN}/api/auth`;
export const LOGIN = "/auth/login";
export const COOKIE = "__Host-ssg-auth";
export const PASSWORD = "a shared secret, long enough";
export const TEAM = "myteam";
export const AUD = "aud-tag";

const enc = new TextEncoder();
const b64u = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const part = (v: unknown) => b64u(enc.encode(JSON.stringify(v)));

const ec = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const ecJwk = await crypto.subtle.exportKey("jwk", ec.publicKey);
const canonical = { crv: "P-256", kty: "EC", x: ecJwk.x as string, y: ecJwk.y as string };
export const PUBLIC_JWK = JSON.stringify({ ...canonical, alg: "ES256", use: "sig" });
const KID = b64u(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(canonical)))));

/** A complete SHOP_AUTH_* set; the audience defaults to the request's host. */
export const AUTH_VARS = { SHOP_AUTH_ISSUER: ISSUER, SHOP_AUTH_PUBLIC_KEY: PUBLIC_JWK, SHOP_AUTH_LOGIN_URL: LOGIN };
export const ACCESS_VARS = { SHOP_ACCESS_TEAM: TEAM, SHOP_ACCESS_AUD: AUD };

/** An `auth` access token for `perms` (claims overridable). */
export async function authToken(perms: string[], over: Record<string, unknown> = {}): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: "shop.example.com", sub: "u-ada", sid: "s-1", email: "ada@example.com", iat: now, exp: now + 600, ver: 1, perms, ...over };
  const head = `${part({ alg: "ES256", typ: "JWT", kid: KID })}.${part(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, ec.privateKey, enc.encode(head)));
  return `${head}.${b64u(sig)}`;
}

const rsa = (await crypto.subtle.generateKey(
  { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
  true,
  ["sign", "verify"],
)) as CryptoKeyPair;
const ACCESS_JWKS = JSON.stringify({ keys: [{ ...(await crypto.subtle.exportKey("jwk", rsa.publicKey)), kid: "kid-1" }] });

/** A Cloudflare Access token for `email`. */
export async function accessToken(email = "Owner@Example.com"): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const claims = { aud: [AUD], email, sub: "access-sub", iss: `https://${TEAM}.cloudflareaccess.com`, exp: now + 600, iat: now };
  const head = `${part({ alg: "RS256", typ: "JWT", kid: "kid-1" })}.${part(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign("RSASSA-PKCS1-v1_5", rsa.privateKey, enc.encode(head)));
  return `${head}.${b64u(sig)}`;
}

/** Answers Access's key endpoint; anything else is a test bug. */
export function serveAccessKeys(): () => void {
  const real = globalThis.fetch;
  resetAccessCache();
  globalThis.fetch = (async (url: unknown) => {
    if (String(url).includes("/cdn-cgi/access/certs")) return new Response(ACCESS_JWKS);
    throw new Error(`unexpected fetch to ${String(url)}`);
  }) as typeof fetch;
  return () => {
    globalThis.fetch = real;
  };
}

/** A shop with a fresh database and the given admin configuration. */
export function shopEnv(vars: Record<string, unknown> = {}): Env & { SHOP_DB: ReturnType<typeof fakeD1> & D1Database } {
  resetSchemaCache();
  invalidateSettings();
  return { SHOP_DB: fakeD1(), SHOP_TEST: "1", SHOP_IP_SALT: "test-salt", ...vars } as never;
}

export interface Ask {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  /** An `auth` session for these permissions (the cookie, as the browser sends it). */
  perms?: string[];
  /** The shared password, as HTTP Basic. */
  password?: string;
  /** A Cloudflare Access token in its header. */
  access?: boolean;
  /** The panel's CSRF header and this origin, as its fetch sends them. */
  panel?: boolean;
}

/** A request to `path` on the shop, built as a browser or the panel would. */
export async function ask(path: string, a: Ask = {}): Promise<Request> {
  const headers = new Headers(a.headers);
  if (a.perms) headers.set("cookie", `${COOKIE}=${await authToken(a.perms)}`);
  if (a.password !== undefined) headers.set("authorization", `Basic ${Buffer.from(`:${a.password}`).toString("base64")}`);
  if (a.access) headers.set("cf-access-jwt-assertion", await accessToken());
  if (a.panel) {
    headers.set("x-ssg-auth", "1");
    headers.set("origin", ORIGIN);
  }
  return new Request(`${ORIGIN}${path}`, { method: a.method ?? "GET", headers, body: a.body });
}

/** Runs the admin middleware; `handler` stands in for the route behind it. */
export async function throughGate(
  request: Request,
  env: Env,
  handler: (ctx: { request: Request; env: Env; data: AdminData; params: Record<string, string> }) => Response | Promise<Response> = () =>
    Response.json({ ok: true }),
  params: Record<string, string> = {},
): Promise<{ res: Response; data: AdminData }> {
  const data = {} as AdminData;
  const pending: Promise<unknown>[] = [];
  const res = (await gate({
    request,
    env,
    data,
    params,
    functionPath: new URL(request.url).pathname,
    waitUntil: (p: Promise<unknown>) => void pending.push(p),
    passThroughOnException: () => {},
    next: async () => handler({ request, env, data, params }),
  } as never)) as Response;
  await Promise.allSettled(pending);
  return { res, data };
}
