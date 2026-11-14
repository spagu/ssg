// A scripted Cloudflare Access team for the GO-139 tests: its certs endpoint
// (https://myteam.cloudflareaccess.com/cdn-cgi/access/certs), answered from
// test keys, and tokens minted like Access's application tokens — or forged.
// It replaces fetch while installed; any other URL is a test failure.
import { onRequestGet as accessGet } from "../functions/auth/access/index.ts";
import { accessKeyCache } from "../functions/api/auth/_access-keys.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { Browser, T0, call, makeEnv } from "./fakes.ts";
import type { ReqOptions } from "./fakes.ts";
import { rsaKey, signJwt } from "./providers-jwt.ts";
import type { Forge, RsaKey } from "./providers-jwt.ts";

export const TEAM = "myteam";
export const HOST = "myteam.cloudflareaccess.com";
export const ISS = `https://${HOST}`;
export const AUD = "4714c1358e65fe4b408ad6d432a5f878f08194bdb4752441fd56faefa9b2b6f2";
export const OTHER_AUD = "9c2b8f1e0d3a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9012345678";
export const ACCESS_ENV = { AUTH_ACCESS_TEAM: TEAM, AUTH_ACCESS_AUD: AUD };

export const KEY = await rsaKey("access-key-1");
export const OTHER_KEY = await rsaKey("access-key-2");
export const STRANGER_KEY = await rsaKey("access-key-1"); // same kid, another team's key

export const accessEnv = (over: Partial<Env> = {}) => makeEnv({ ...ACCESS_ENV, ...over } as Partial<Env>);

export class FakeAccess {
  requests: string[] = [];
  keys: RsaKey[] = [KEY];
  mode: "ok" | "500" | "throw" | "empty" | "odd" | "nokeys" = "ok";
  private real = globalThis.fetch;
  install(): this {
    accessKeyCache.clear();
    globalThis.fetch = (async (url: string) => {
      const u = String(url);
      this.requests.push(u);
      if (u !== `${ISS}/cdn-cgi/access/certs`) throw new Error(`unexpected fetch ${u}`);
      if (this.mode === "throw") throw new TypeError("fetch failed");
      if (this.mode === "500") return new Response("{}", { status: 500 });
      if (this.mode === "nokeys") return new Response(JSON.stringify({ public_certs: [] }), { headers: { "content-type": "application/json" } });
      const keys =
        this.mode === "empty"
          ? []
          : this.mode === "odd"
            ? [null, "x", { kty: "EC", kid: "ec" }, { kty: "RSA", kid: "", n: "a", e: "b" }, { kty: "RSA", kid: "hs", n: "a", e: "b", alg: "HS256" }, { kty: "RSA", kid: "junk", n: "REFUSED", e: "AQAB" }, { kty: "RSA", kid: 5, n: "a", e: "b" }]
            : this.keys.map((k) => k.jwk);
      return new Response(JSON.stringify({ keys, public_cert: { kid: "x", cert: "-----BEGIN CERTIFICATE-----" } }), { headers: { "content-type": "application/json" } });
    }) as typeof fetch;
    return this;
  }
  restore(): void {
    globalThis.fetch = this.real;
    accessKeyCache.clear();
  }
}

export interface Person {
  sub: string;
  email: string;
  groups?: unknown;
}

export const PERSON: Person = { sub: "7335d417-61da-459d-899c-0a01c76a2f94", email: "ada@example.com" };

// mint: an Access application token for `p`, valid an hour from `now`.
export async function mint(p: Person = PERSON, claims: Record<string, unknown> = {}, o: { key?: RsaKey; forge?: Forge; header?: Record<string, unknown>; now?: number } = {}): Promise<string> {
  const iat = Math.floor((o.now ?? T0) / 1000);
  const base: Record<string, unknown> = { aud: [AUD], email: p.email, exp: iat + 3600, iat, nbf: iat, iss: ISS, type: "app", identity_nonce: "n0nce", sub: p.sub, country: "PL" };
  if (p.groups !== undefined) base.groups = p.groups;
  return signJwt(o.header ?? {}, { ...base, ...claims }, o.key ?? KEY, o.forge);
}

// enter: the browser arrives at /auth/access with the token in the header
// Access adds (default) or in the CF_Authorization cookie.
export async function enter(env: Env, token: string | null, o: { via?: "header" | "cookie"; next?: string; browser?: Browser; lang?: string } = {}): Promise<Response> {
  const browser = o.browser ?? new Browser();
  const headers: Record<string, string> = {};
  if (token && (o.via ?? "header") === "header") headers["cf-access-jwt-assertion"] = token;
  if (token && o.via === "cookie") browser.jar.set("CF_Authorization", token);
  const q = new URLSearchParams();
  if (o.next !== undefined) q.set("next", o.next);
  if (o.lang) q.set("lang", o.lang);
  const opts: ReqOptions = { browser, headers, csrf: false };
  return call(accessGet, env, `/auth/access${q.size ? `?${q}` : ""}`, opts);
}

export const where = (res: Response): string => res.headers.get("location") ?? "";
export const errorCode = (res: Response): string | null => new URL(where(res) || "/", "https://example.com").searchParams.get("error");
