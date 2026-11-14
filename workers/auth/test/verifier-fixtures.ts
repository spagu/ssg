// Shared fixtures for the portable verifier's tests (GO-135): tokens signed by auth's own
// signToken with the test key, a matching consumer config, and requests carrying a token.
import { loadKeys } from "../functions/api/auth/_keys.ts";
import type { KeySet } from "../functions/api/auth/_keys.ts";
import { CONTRACT_VERSION, signToken } from "../functions/api/auth/_jwt.ts";
import type { AccessClaims } from "../functions/api/auth/_jwt.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import type { AuthConfig } from "../portable/session-verifier.ts";
import { KEYS } from "./fakes.ts";

export const NOW = 1_800_000_000; // seconds
export const NOW_MS = NOW * 1000;
export const ISSUER = "https://example.com/api/auth";
export const PAGE = "https://example.com/admin/";

export const keySet = async (priv = KEYS.priv): Promise<KeySet> => (await loadKeys({ AUTH_SIGNING_KEY: priv } as Env)) as KeySet;
export const KEYSET = await keySet();

export const claims = (over: Record<string, unknown> = {}): AccessClaims =>
  ({ iss: ISSUER, aud: "example.com", sub: "u1", sid: "s1", email: "ada@example.com", iat: NOW, exp: NOW + 600, ver: CONTRACT_VERSION, perms: ["demo:read"], ...over }) as AccessClaims;

export const sign = (over: Record<string, unknown> = {}, keys: KeySet = KEYSET): Promise<string> => signToken(keys.signing, claims(over));
export const part = (v: unknown): string => b64urlEncode(new TextEncoder().encode(JSON.stringify(v)));

export const config = (over: Partial<AuthConfig> = {}): AuthConfig => ({
  issuer: ISSUER,
  audience: "",
  publicKey: KEYS.pub,
  jwksUrl: "",
  loginUrl: "/auth/login",
  configProblem: null,
  ...over,
});

export const withCookie = (token: string, url = PAGE, headers: Record<string, string> = {}): Request =>
  new Request(url, { headers: { cookie: `theme=dark; __Host-ssg-auth=${token}; other=1`, ...headers } });
export const withBearer = (token: string, url = PAGE): Request => new Request(url, { headers: { authorization: `Bearer ${token}` } });
export const deps = { now: () => NOW_MS };
