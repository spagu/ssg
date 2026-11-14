// Scripted fakes of the five login providers (GO-151), built from their
// published protocols: the authorization step (fake.authorize reads the URL
// the worker sent the browser to and answers like the provider's consent
// screen would), the token endpoints (code single-use, redirect URI, client
// credentials — Apple's signed client secret — and PKCE all checked), the
// JWKS, and GitHub's and Facebook's APIs. They replace fetch; every request
// the worker makes is recorded. `tweak` scripts misbehaviour per test.
import { clock } from "../functions/api/auth/_lib.ts";
import { appleKey, rsaKey, s256, signJwt, verifyEs256 } from "./providers-jwt.ts";
import type { Forge, RsaKey } from "./providers-jwt.ts";

export const MS_TENANT = "11111111-2222-3333-4444-555555555555";
const KEYS = { google: await rsaKey("g1"), microsoft: await rsaKey("m1"), apple: await rsaKey("a1") };
export const OTHER_KEY = await rsaKey("g1"); // same kid, different key
const APPLE = await appleKey();

export const PROVIDER_ENV = {
  AUTH_GOOGLE_CLIENT_ID: "google-client.apps.example.com",
  AUTH_GOOGLE_CLIENT_SECRET: "GOCSPX-google-secret-0001",
  AUTH_MICROSOFT_CLIENT_ID: "ms-client-0001",
  AUTH_MICROSOFT_CLIENT_SECRET: "ms~secret~0001",
  AUTH_APPLE_CLIENT_ID: "com.example.web",
  AUTH_APPLE_TEAM_ID: "TEAM123456",
  AUTH_APPLE_KEY_ID: "KEY1234567",
  AUTH_APPLE_PRIVATE_KEY: APPLE.pem,
  AUTH_GITHUB_CLIENT_ID: "Iv1.github0001",
  AUTH_GITHUB_CLIENT_SECRET: "github-secret-0001",
  AUTH_FACEBOOK_CLIENT_ID: "1234567890",
  AUTH_FACEBOOK_CLIENT_SECRET: "facebook-secret-0001",
};
export const SECRETS = [PROVIDER_ENV.AUTH_GOOGLE_CLIENT_SECRET, PROVIDER_ENV.AUTH_MICROSOFT_CLIENT_SECRET, PROVIDER_ENV.AUTH_GITHUB_CLIENT_SECRET, PROVIDER_ENV.AUTH_FACEBOOK_CLIENT_SECRET, "PRIVATE KEY"];
export const OIDC = ["google", "microsoft", "apple"] as const;
export const ALL = ["google", "microsoft", "apple", "github", "facebook"] as const;
export type Pid = (typeof ALL)[number];

export interface Person {
  sub: string;
  email?: string;
  verified?: boolean;
  name?: string;
}

interface Code {
  provider: Pid;
  clientId: string;
  redirectUri: string;
  challenge: string | null;
  nonce: string;
  person: Person;
  used: boolean;
}

export interface Tweak {
  claims?: (c: Record<string, unknown>) => void; // edit the ID token claims
  header?: Record<string, unknown>;
  forge?: Forge;
  signWith?: RsaKey;
  jwks?: unknown; // replace the JWKS answer
  redirect?: boolean; // every provider endpoint answers 302 to another host
  oversize?: "declared" | "streamed";
  slow?: boolean;
  tokenAnswer?: { status: number; body: unknown };
  emails?: unknown; // GitHub /user/emails answer ("broken": not JSON)
  api?: Record<string, unknown>; // GitHub /user or Facebook /me answer
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
const ISS: Record<string, string> = { google: "https://accounts.google.com", apple: "https://appleid.apple.com", microsoft: `https://login.microsoftonline.com/${MS_TENANT}/v2.0` };
const CLIENT: Record<Pid, string> = { google: PROVIDER_ENV.AUTH_GOOGLE_CLIENT_ID, microsoft: PROVIDER_ENV.AUTH_MICROSOFT_CLIENT_ID, apple: PROVIDER_ENV.AUTH_APPLE_CLIENT_ID, github: PROVIDER_ENV.AUTH_GITHUB_CLIENT_ID, facebook: PROVIDER_ENV.AUTH_FACEBOOK_CLIENT_ID };
const SECRET: Record<string, string> = { google: PROVIDER_ENV.AUTH_GOOGLE_CLIENT_SECRET, microsoft: PROVIDER_ENV.AUTH_MICROSOFT_CLIENT_SECRET, github: PROVIDER_ENV.AUTH_GITHUB_CLIENT_SECRET, facebook: PROVIDER_ENV.AUTH_FACEBOOK_CLIENT_SECRET };

export class FakeProviders {
  calls: { url: string; method: string; headers: Record<string, string>; body: string }[] = [];
  codes = new Map<string, Code>();
  tokens = new Map<string, { provider: Pid; person: Person }>();
  idTokens: string[] = [];
  tweak: Tweak = {};
  private real = globalThis.fetch;

  install(): this {
    globalThis.fetch = ((url: string, init: RequestInit = {}) => this.handle(String(url), init)) as typeof fetch;
    return this;
  }
  restore(): void {
    globalThis.fetch = this.real;
  }

  // authorize plays the provider's consent screen for the URL the worker
  // redirected to; returns the callback the provider would send the browser
  // to (Apple: the form it would POST).
  async authorize(location: string, person: Person): Promise<{ path: string; form?: Record<string, string>; state: string }> {
    const u = new URL(location);
    const provider = (u.host.includes("google") ? "google" : u.host.includes("microsoft") ? "microsoft" : u.host.includes("apple") ? "apple" : u.host.includes("github") ? "github" : "facebook") as Pid;
    const q = u.searchParams;
    const code = `code-${crypto.randomUUID()}`;
    this.codes.set(code, { provider, clientId: q.get("client_id") ?? "", redirectUri: q.get("redirect_uri") ?? "", challenge: q.get("code_challenge"), nonce: q.get("nonce") ?? "", person, used: false });
    const state = q.get("state") ?? "";
    const back = new URL(q.get("redirect_uri") ?? "");
    if (q.get("response_mode") === "form_post") return { path: back.pathname, form: { code, state, user: JSON.stringify({ name: { firstName: "Ann", lastName: "Apple" }, email: person.email }) }, state };
    return { path: `${back.pathname}?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`, state };
  }

  private async handle(url: string, init: RequestInit): Promise<Response> {
    const headers = Object.fromEntries(new Headers(init.headers as HeadersInit).entries());
    const body = typeof init.body === "string" ? init.body : "";
    this.calls.push({ url, method: init.method ?? "GET", headers, body });
    const t = this.tweak;
    if (t.redirect) {
      if (init.redirect === "error") throw new TypeError("fetch failed: redirect mode is set to error");
      return new Response(null, { status: 302, headers: { location: "https://evil.example.com/" } });
    }
    if (t.slow) {
      return new Promise((_, reject) => init.signal?.addEventListener("abort", () => reject(init.signal?.reason)));
    }
    if (t.oversize === "declared") return json({ pad: "x".repeat(10) }, 200, { "content-length": String(10 * 1024 * 1024) });
    if (t.oversize === "streamed") return new Response(new Blob(["[", "0,".repeat(200_000), "0]"]).stream(), { status: 200 });
    const u = new URL(url);
    if (u.pathname.endsWith("/certs") || u.pathname.endsWith("/keys")) {
      const k = u.host.includes("google") ? KEYS.google : u.host.includes("microsoft") ? KEYS.microsoft : KEYS.apple;
      return json(t.jwks ?? { keys: [k.jwk] });
    }
    if (u.pathname.endsWith("/token") || u.pathname.endsWith("/access_token")) return this.token(u, new URLSearchParams(body));
    if (u.host === "api.github.com") return this.github(u, headers);
    if (u.host === "graph.facebook.com") return this.facebook(u, headers);
    return json({ error: "unexpected" }, 500);
  }

  private async token(u: URL, f: URLSearchParams): Promise<Response> {
    if (this.tweak.tokenAnswer) return json(this.tweak.tokenAnswer.body, this.tweak.tokenAnswer.status);
    const c = this.codes.get(f.get("code") ?? "");
    const isGithub = u.host === "github.com";
    const refuse = (error: string) => (isGithub ? json({ error }) : json({ error }, 400));
    if (!c || c.used) return refuse("invalid_grant");
    c.used = true;
    if (f.get("grant_type") !== "authorization_code" || f.get("redirect_uri") !== c.redirectUri || f.get("client_id") !== CLIENT[c.provider]) return refuse("invalid_grant");
    if (c.provider === "apple") {
      const jwt = await verifyEs256(f.get("client_secret") ?? "", APPLE.publicKey);
      const cl = jwt?.claims;
      if (!jwt || jwt.header.kid !== PROVIDER_ENV.AUTH_APPLE_KEY_ID || cl?.iss !== PROVIDER_ENV.AUTH_APPLE_TEAM_ID || cl?.sub !== CLIENT.apple || cl?.aud !== "https://appleid.apple.com") return refuse("invalid_client");
    } else if (f.get("client_secret") !== SECRET[c.provider]) return refuse("invalid_client");
    if (c.challenge && (await s256(f.get("code_verifier") ?? "")) !== c.challenge) return refuse("invalid_grant");
    const access = `at-${crypto.randomUUID()}`;
    this.tokens.set(access, { provider: c.provider, person: c.person });
    if (c.provider === "github" || c.provider === "facebook") return json({ access_token: access, token_type: "bearer" });
    const idToken = await this.idToken(c);
    this.idTokens.push(idToken);
    return json({ access_token: access, token_type: "Bearer", id_token: idToken });
  }

  private async idToken(c: Code): Promise<string> {
    const now = Math.floor(clock.now() / 1000);
    const p = c.person;
    const claims: Record<string, unknown> = { iss: ISS[c.provider], aud: c.clientId, sub: p.sub, nonce: c.nonce, iat: now, exp: now + 3600, name: p.name };
    if (p.email !== undefined) claims.email = p.email;
    if (c.provider === "google") claims.email_verified = p.verified ?? false;
    if (c.provider === "apple") claims.email_verified = p.verified ? "true" : "false";
    if (c.provider === "microsoft") Object.assign(claims, { tid: MS_TENANT, ...(p.verified ? { xms_edov: true } : {}) });
    this.tweak.claims?.(claims);
    const key = this.tweak.signWith ?? KEYS[c.provider as "google"];
    return signJwt(this.tweak.header ?? {}, claims, key, this.tweak.forge);
  }

  private github(u: URL, h: Record<string, string>): Response {
    const who = this.tokens.get((h.authorization ?? "").replace(/^Bearer /, ""));
    if (!who || !h["user-agent"]) return json({ message: "Bad credentials" }, 401);
    const p = who.person;
    if (u.pathname === "/user") return json(this.tweak.api ?? { id: Number(p.sub), login: `user${p.sub}`, name: p.name ?? null, email: null });
    if (this.tweak.emails === "broken") return new Response("<html>", { status: 502 });
    return json(this.tweak.emails ?? (p.email ? [{ email: "old@example.com", primary: false, verified: true }, { email: p.email, primary: true, verified: p.verified ?? false }] : []));
  }

  private async facebook(u: URL, h: Record<string, string>): Promise<Response> {
    const token = (h.authorization ?? "").replace(/^Bearer /, "");
    const who = this.tokens.get(token);
    const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(SECRET.facebook), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
    const proof = [...new Uint8Array(await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(token)))].map((b) => b.toString(16).padStart(2, "0")).join("");
    if (!who || u.searchParams.get("appsecret_proof") !== proof) return json({ error: { message: "Invalid OAuth access token" } }, 400);
    return json(this.tweak.api ?? { id: who.person.sub, name: who.person.name, ...(who.person.email ? { email: who.person.email } : {}) });
  }
}
