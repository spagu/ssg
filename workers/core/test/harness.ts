// Test machinery for the core worker: `call` runs a Pages handler as Pages
// does, a fake fetch records every outgoing request and answers per path, a
// Cloudflare Access issuer with a real RSA key, and manifest builders.
import { setImmediate as tick } from "node:timers/promises";

export const ORIGIN = "https://example.com";
export const PASSWORD = "core-pass";
export const basic = (password: string) => "Basic " + btoa(`admin:${password}`);
export const PW_ENV = { CORE_ADMIN_PASSWORD: PASSWORD };

type Handler = (ctx: { request: Request; env: unknown }) => Promise<Response>;

export interface Observed {
  status: number;
  headers: Record<string, string>;
  body: string;
  json: () => any;
}

let ip = 0;
// freshIP: each test gets its own address, so the per-isolate lockout of one
// test never touches another.
export const freshIP = () => `192.0.2.${++ip % 250}`;

export async function call(
  handler: Handler,
  init: { method?: string; path: string; headers?: Record<string, string>; body?: string; env: Record<string, unknown> },
): Promise<Observed> {
  const request = new Request(`${ORIGIN}${init.path}`, { method: init.method ?? "GET", headers: init.headers, body: init.body });
  const res = await handler({ request, env: init.env });
  const body = await res.text();
  return { status: res.status, headers: Object.fromEntries(res.headers.entries()), body, json: () => JSON.parse(body) };
}

// settle lets every pending promise and I/O callback run (mocked timers do not
// affect setImmediate).
export async function settle(rounds = 30): Promise<void> {
  for (let i = 0; i < rounds; i++) await tick();
}

// settleUntil keeps settling until `ready()` holds. Use it before moving a
// mocked clock: core hashes the caller's credential with WebCrypto before it
// sets its timeouts, and WebCrypto runs off the main thread — under load that
// can take longer than settle()'s fixed rounds. A test that ticked the clock
// then moved it before the timeout existed, the timeout never fired, and the
// test hung. Waiting for the request to go out (it goes out right after the
// timeout is set) makes the order certain. Gives up after real `limitMs`.
export async function settleUntil(ready: () => boolean, limitMs = 10_000): Promise<void> {
  const until = performance.now() + limitMs;
  while (!ready()) {
    if (performance.now() > until) throw new Error("settleUntil: the condition never held");
    await tick();
  }
  await settle();
}

export interface Sent {
  url: string;
  method: string;
  headers: Record<string, string>;
  redirect: string | undefined;
}
type Answer = Response | (() => Response) | ((signal: AbortSignal) => Promise<Response>) | "network-error";

// fakeFetch answers by full URL; anything else is a network error. Every
// request is recorded with all its headers.
export function fakeFetch(routes: Record<string, Answer>) {
  const sent: Sent[] = [];
  const real = globalThis.fetch;
  globalThis.fetch = (async (input: string | URL, init: RequestInit = {}) => {
    const url = String(input);
    sent.push({ url, method: init.method ?? "GET", headers: Object.fromEntries(new Headers(init.headers).entries()), redirect: init.redirect });
    const answer = routes[url];
    if (answer === undefined || answer === "network-error") throw new TypeError("fetch failed");
    // A fresh Response each time: a clone() shares a tee with the original,
    // and cancelling one branch of a tee waits for the other.
    if (answer instanceof Response) return new Response(await answer.clone().arrayBuffer(), answer);
    return (answer as (s: AbortSignal) => Response | Promise<Response>)(init.signal as AbortSignal);
  }) as typeof fetch;
  return { sent, restore: () => void (globalThis.fetch = real) };
}

// slow answers after ms (of the mocked clock), or fails when aborted first.
export const slow = (ms: number, res: () => Response) => (signal: AbortSignal) =>
  new Promise<Response>((resolve, reject) => {
    const t = setTimeout(() => resolve(res()), ms);
    signal.addEventListener("abort", () => {
      clearTimeout(t);
      reject(new DOMException("aborted", "AbortError"));
    });
  });

export const text = (s: string) => ({ en: s, pl: `${s} (pl)`, hi: `${s} (hi)`, de: `${s} (de)`, fr: `${s} (fr)` });

// manifest builds a valid module.json for id; nav is a list of [path, permission].
export function manifest(id: string, opts: { nav?: [string, string][]; perms?: string[]; panel?: string | null; endpoints?: Record<string, string>; kind?: string; version?: string } = {}) {
  const nav = opts.nav ?? [];
  const perms = opts.perms ?? [...new Set(nav.map(([, p]) => p))];
  return {
    id,
    contract: 1,
    version: opts.version ?? "1.2.3",
    kind: opts.kind ?? "pages-function",
    title: text(`${id} title`),
    description: text(`${id} description`),
    panel: opts.panel !== undefined ? opts.panel : nav.length ? nav[0][0] : null,
    nav: nav.map(([path, permission]) => ({ label: text(`${id} ${path}`), path, permission })),
    permissions: perms.map((name) => ({ name, description: text(name) })),
    endpoints: opts.endpoints ?? { manifest: `/api/${id}/module` },
    disable: text("Remove it."),
  };
}

export function entry(id: string, m: unknown, base = "/", extra: Record<string, unknown> = {}) {
  const v = m as { kind?: string; version?: string } | null;
  return { id, kind: v?.kind ?? null, base, bundled: base === "/", contract: m ? 1 : null, version: v?.version ?? null, manifest: m, ...extra };
}
export const registry = (...modules: unknown[]) => ({ format: 1, modules, available: [], theme: null });

// Access issues tokens for one Access application and answers the JWKS
// request; install() chains it in front of whatever fetch is installed.
export const TEAM = "example-team";
export const AUD = "aud-core";
export const ACCESS_ENV = { CORE_ACCESS_TEAM: TEAM, CORE_ACCESS_AUD: AUD };
const b64url = (b: Uint8Array | string) => Buffer.from(b).toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");

export async function accessIssuer(extraKeys: Record<string, unknown>[] = []) {
  const pair = (await crypto.subtle.generateKey(
    { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const certs = { keys: [...extraKeys, { kid: "k1", kty: "RSA", n: jwk.n, e: jwk.e }] };
  let certsCalls = 0;
  return {
    certsUrl: `https://${TEAM}.cloudflareaccess.com/cdn-cgi/access/certs`,
    certsCalls: () => certsCalls,
    async token(claims: Record<string, unknown> = {}, header: Record<string, unknown> = { alg: "RS256", kid: "k1" }): Promise<string> {
      const head = b64url(JSON.stringify(header));
      const body = b64url(
        JSON.stringify({ aud: [AUD], iss: `https://${TEAM}.cloudflareaccess.com`, exp: Math.floor(Date.now() / 1000) + 600, email: "ada@example.com", ...claims }),
      );
      const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, new TextEncoder().encode(`${head}.${body}`));
      return `${head}.${body}.${b64url(new Uint8Array(sig))}`;
    },
    install(): () => void {
      const inner = globalThis.fetch;
      globalThis.fetch = (async (input: string | URL, init?: RequestInit) => {
        if (String(input) === this.certsUrl) {
          certsCalls++;
          return Response.json(certs);
        }
        return inner(input, init);
      }) as typeof fetch;
      return () => void (globalThis.fetch = inner);
    },
  };
}

// An `auth` worker stand-in: a P-256 key, the CORE_AUTH_* variables of a
// consumer, and session cookies signed as `auth` signs them (contract v1).
export const AUTH_ISSUER = "https://example.com/api/auth";
const authPair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const authJwk = await crypto.subtle.exportKey("jwk", authPair.publicKey);
const publicJwk = { crv: "P-256", kty: "EC", x: authJwk.x, y: authJwk.y };
const authKid = b64url(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(JSON.stringify(publicJwk)))));
export const AUTH_ENV = { CORE_AUTH_ISSUER: AUTH_ISSUER, CORE_AUTH_PUBLIC_KEY: JSON.stringify(publicJwk), CORE_AUTH_LOGIN_URL: "/auth/login" };

export async function session(perms: string[], over: Record<string, unknown> = {}): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const head = b64url(JSON.stringify({ alg: "ES256", typ: "JWT", kid: authKid }));
  const body = b64url(JSON.stringify({ iss: AUTH_ISSUER, aud: "example.com", sub: "u1", sid: "s1", email: "ada@example.com", iat: now, exp: now + 600, ver: 1, perms, ...over }));
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, authPair.privateKey, new TextEncoder().encode(`${head}.${body}`));
  return `__Host-ssg-auth=${head}.${body}.${b64url(new Uint8Array(sig))}`;
}
