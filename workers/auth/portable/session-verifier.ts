// ssg auth — portable session verifier, contract v1 (workers/auth/docs/CONTRACT.md). SOURCE: workers/auth/portable/
// session-verifier.ts; templates carry byte-identical copies guarded by a test (portable/copy-guard.ts) — edit
// the source, copy again. No imports, WebCrypto + fetch only (Pages Functions or a Worker, unchanged). Checks
// `auth`'s token as auth's verifyToken (functions/api/auth/_jwt.ts) does: locally, never asking `auth`.

// CONTRACT_VERSION: the only `ver` known (others → unsupported_contract). LEEWAY_SECONDS: clock skew on exp/iat.
// MAX_TOKEN: a cookie holds 4096 bytes. KEY_TTL_MS: a fetched JWKS is trusted this long (= /api/auth/jwks
// max-age); REFETCH_COOLDOWN_MS: at most one JWKS fetch per URL per minute, whatever kids arrive.
export const CONTRACT_VERSION = 1, ACCESS_COOKIE = "__Host-ssg-auth", LEEWAY_SECONDS = 30, MAX_TOKEN = 8192;
export const KEY_TTL_MS = 300_000, REFETCH_COOLDOWN_MS = 60_000, FETCH_TIMEOUT_MS = 3_000, MAX_JWKS_BYTES = 16_384;
// AuthConfig: plain data (configFrom builds it); "" = unset, audience "" = the request's host. publicKey: a public
// JWK or JWKS as JSON (recommended); jwksUrl: same-origin https URL or a path. configProblem set → 503.
export interface AuthConfig { issuer: string; audience: string; publicKey: string; jwksUrl: string; loginUrl: string; configProblem: string | null }
export type Reason =
  | "no_session" | "malformed" | "bad_signature" | "expired" | "not_yet_valid" | "wrong_issuer"
  | "wrong_audience" | "unknown_key" | "unsupported_contract" | "wrong_kind" | "keys_unavailable" | "not_configured";
export type Verdict =
  | { ok: true; user: { id: string; email: string }; perms: string[]; sessionId: string; expiresAt: string; kind: "user" | "service" }
  | { ok: false; reason: Reason };
// Deps: injected by tests; now() is in milliseconds.
export interface Deps { fetch?: (url: string, init: RequestInit) => Promise<Response>; now?: () => number }
type Keys = Map<string, CryptoKey>;

// ---- begin: the code of workers/auth/functions/api/auth/_can.ts, byte for byte (a test compares) ----
const PERMISSION = /^(?:\*|[a-z][a-z0-9-]*:(?:\*|[a-z][a-z0-9-]*))$/;

// validPermission: a well-formed name or wildcard of at most 64 characters
// (the manifest format's limit for a name).
export function validPermission(s: unknown): s is string {
  return typeof s === "string" && s.length <= 64 && PERMISSION.test(s);
}

// can tells whether a holder of `perms` may do `required`. `required` is one
// concrete permission (never a wildcard — asking "may they do everything?"
// is a different question, and answers false here). Only three entries can
// allow it: the exact name, its worker's `worker:*`, or `*`; every other
// string, however close, compares unequal.
export function can(perms: readonly string[], required: string): boolean {
  if (!Array.isArray(perms) || !validPermission(required) || required.endsWith("*")) return false;
  const all = required.slice(0, required.indexOf(":") + 1) + "*";
  return perms.some((p) => p === required || p === all || p === "*");
}
// ---- end: _can.ts ----

const enc = new TextEncoder();
const isNum = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);
const b64e = (b: Uint8Array) => btoa(String.fromCharCode(...b)).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
// b64d accepts unpadded base64url only and returns null for anything else (token parts are hostile).
function b64d(raw: string) {
  if (!/^[A-Za-z0-9_-]*$/.test(raw) || raw.length % 4 === 1) return null;
  return Uint8Array.from(atob(raw.replaceAll("-", "+").replaceAll("_", "/") + "=".repeat((4 - (raw.length % 4)) % 4)), (c) => c.charCodeAt(0));
}
// parse: strict UTF-8 JSON, or null. `any` because every caller narrows the shape itself.
const parse = (b: Uint8Array): any => { try { return JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(b)); } catch { return null; } };
function decodeJSON(s: string): Record<string, unknown> | null {
  const bytes = b64d(s);
  const v = bytes && parse(bytes);
  return v && typeof v === "object" && !Array.isArray(v) ? v : null;
}
// importKeys reads a JWK or a JWKS; the kid is the RFC 7638 thumbprint, computed as `auth` does.
// Skipped: anything but an ES256 P-256 signing key, a key off the curve, and a PRIVATE key (`d`):
// a consumer must never hold the signing key.
async function importKeys(raw: string): Promise<Keys | null> {
  const keys: Keys = new Map();
  const v = parse(enc.encode(raw));
  for (const item of Array.isArray(v?.keys) ? v.keys : [v]) {
    const k = (item && typeof item === "object" ? item : {}) as Record<string, unknown>;
    const usable = k.kty === "EC" && k.crv === "P-256" && (k.use ?? "sig") === "sig" && (k.alg ?? "ES256") === "ES256";
    if (!usable || typeof k.x !== "string" || typeof k.y !== "string" || "d" in k) continue;
    const jwk = { crv: "P-256", kty: "EC", x: k.x, y: k.y };
    try {
      const key = await crypto.subtle.importKey("jwk", jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
      keys.set(b64e(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(jwk))))), key);
    } catch { /* off the curve: skipped */ }
  }
  return keys.size ? keys : null;
}
// fetchKeys: one bounded request — timeout, size cap, redirects not followed — or null.
async function fetchKeys(url: string, d: Required<Deps>): Promise<Keys | null> {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), FETCH_TIMEOUT_MS);
  const read = async (): Promise<Keys | null> => {
    const res = await d.fetch(url, { signal: ctl.signal, redirect: "manual", headers: { accept: "application/json" } });
    if (res.status !== 200 || !res.body) return null;
    const [reader, dec] = [res.body.getReader(), new TextDecoder()];
    let [text, size] = ["", 0];
    for (let r = await reader.read(); !r.done; r = await reader.read()) {
      if ((size += r.value.byteLength) > MAX_JWKS_BYTES) return null; // ctl.abort() below drops the rest
      text += dec.decode(r.value, { stream: true });
    }
    return importKeys(text);
  };
  const keys = await read().catch(() => null);
  clearTimeout(timer);
  ctl.abort(); // releases the connection and any body left unread; a no-op after a complete read
  return keys;
}
// Per-isolate caches: the inline key set by its exact text, a fetched JWKS by its URL.
let inline: { raw: string; keys: Promise<Keys | null> } | null = null;
const remote = new Map<string, { keys: Keys | null; at: number; tried: number; pending: Promise<unknown> | null }>();
// remoteKeys serves a fetched JWKS for KEY_TTL_MS. A cold or expired cache or an unknown kid refetches at most once
// per REFETCH_COOLDOWN_MS (concurrent requests share it); a failed fetch keeps old keys until their TTL, then denies.
async function remoteKeys(url: string, kid: string, d: Required<Deps>): Promise<Keys | null> {
  const e = remote.get(url) ?? { keys: null, at: 0, tried: -Infinity, pending: null };
  remote.set(url, e);
  if (e.pending) await e.pending;
  const live = () => (e.keys && d.now() - e.at < KEY_TTL_MS ? e.keys : null);
  if (live()?.has(kid) || d.now() - e.tried < REFETCH_COOLDOWN_MS) return live();
  e.tried = d.now();
  await (e.pending = fetchKeys(url, d).then((k) => k && Object.assign(e, { keys: k, at: d.now() })));
  e.pending = null;
  return live();
}
async function keysFor(request: Request, c: AuthConfig, kid: string, d: Required<Deps>): Promise<Keys | Reason> {
  if (c.publicKey) {
    if (inline?.raw !== c.publicKey) inline = { raw: c.publicKey, keys: importKeys(c.publicKey) };
    return (await inline.keys) ?? "not_configured";
  }
  const [origin, url] = [new URL(request.url).origin, new URL(c.jwksUrl, request.url)];
  if (url.protocol !== "https:" || url.origin !== origin) return "not_configured";
  return (await remoteKeys(url.href, kid, d)) ?? "keys_unavailable";
}
// tokenOf: `Authorization: Bearer` (machines) wins over the cookie (browsers) and is never
// combined with it; any other scheme (Basic, a consumer's password mode) is ignored here.
function tokenOf(request: Request): { token: string; bearer: boolean } {
  const header = request.headers.get("authorization") ?? "";
  if (/^bearer(?:\s|$)/i.test(header)) return { token: header.slice(6).trim(), bearer: true };
  const m = new RegExp(`(?:^|;)\\s*${ACCESS_COOKIE}\\s*=([^;]*)`).exec(request.headers.get("cookie") ?? ""); // the first, as auth reads it
  return { token: m ? m[1].trim() : "", bearer: false };
}
// verifySession checks in verifyToken's order: shape, algorithm (ES256 from this code, never the header), key by kid,
// signature, ver, iss, aud, exp/iat, claim types; then kind vs channel: cookie → `user` only, bearer → `service` only.
export async function verifySession(request: Request, c: AuthConfig, deps: Deps = {}): Promise<Verdict> {
  const d: Required<Deps> = { fetch: deps.fetch ?? ((u, i) => fetch(u, i)), now: deps.now ?? (() => Date.now()) };
  const no = (reason: Reason): Verdict => ({ ok: false, reason });
  if (c.configProblem || !c.issuer || !c.loginUrl || !c.publicKey === !c.jwksUrl) return no("not_configured");
  const { token, bearer } = tokenOf(request);
  if (!token) return no("no_session");
  const parts = token.split(".");
  if (token.length > MAX_TOKEN || parts.length !== 3) return no("malformed");
  const [header, p, sig] = [decodeJSON(parts[0]), decodeJSON(parts[1]), b64d(parts[2])];
  if (!header || !p || !sig) return no("malformed");
  if (header.alg !== "ES256" || "crit" in header) return no("bad_signature");
  if (typeof header.kid !== "string") return no("unknown_key");
  const keys = await keysFor(request, c, header.kid, d);
  if (typeof keys === "string") return no(keys);
  const key = keys.get(header.kid);
  if (!key) return no("unknown_key");
  const signed = enc.encode(`${parts[0]}.${parts[1]}`);
  if (sig.length !== 64 || !(await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, sig, signed))) return no("bad_signature");
  const now = Math.floor(d.now() / 1000);
  if (p.ver !== CONTRACT_VERSION) return no("unsupported_contract");
  if (p.iss !== c.issuer) return no("wrong_issuer");
  if (p.aud !== (c.audience || new URL(request.url).host)) return no("wrong_audience");
  if (!isNum(p.exp) || !isNum(p.iat)) return no("malformed");
  if (p.exp <= now - LEEWAY_SECONDS) return no("expired");
  if (p.iat > now + LEEWAY_SECONDS) return no("not_yet_valid");
  if (typeof p.sub !== "string" || typeof p.sid !== "string" || typeof p.email !== "string") return no("malformed");
  if (!Array.isArray(p.perms) || !p.perms.every((x) => typeof x === "string")) return no("malformed");
  const kind = p.kind ?? "user";
  if (kind !== "user" && kind !== "service") return no("unsupported_contract");
  if ((kind === "service") !== bearer) return no("wrong_kind");
  const expiresAt = new Date(Math.min(p.exp * 1000, 8.64e15)).toISOString();
  return { ok: true, user: { id: p.sub, email: p.email }, perms: p.perms, sessionId: p.sid, expiresAt, kind };
}
// configFrom reads <PREFIX>_AUTH_{ISSUER,AUDIENCE,PUBLIC_KEY,JWKS_URL,LOGIN_URL}. None set → null (mode off). Some but
// not all → configProblem says what is wrong and every request is not_configured: a loud 503, never a weaker mode.
export function configFrom(env: object, prefix: string): AuthConfig | null {
  const v = (n: string) => String((env as Record<string, unknown>)[`${prefix}_AUTH_${n}`] ?? "").trim();
  if (["ISSUER", "AUDIENCE", "PUBLIC_KEY", "JWKS_URL", "LOGIN_URL"].every((n) => !v(n))) return null;
  const c = { issuer: v("ISSUER"), audience: v("AUDIENCE"), publicKey: v("PUBLIC_KEY"), jwksUrl: v("JWKS_URL"), loginUrl: v("LOGIN_URL") };
  const problems = ["ISSUER", "LOGIN_URL"].filter((n) => !v(n)).map((n) => `${prefix}_AUTH_${n} is not set`);
  if (!c.publicKey === !c.jwksUrl) problems.push(`set exactly one of ${prefix}_AUTH_PUBLIC_KEY and ${prefix}_AUTH_JWKS_URL`);
  if (c.jwksUrl && !/^(?:https:\/\/|\/(?![/\\]))/.test(c.jwksUrl)) problems.push(`${prefix}_AUTH_JWKS_URL must be https:// or a path`);
  return { ...c, configProblem: problems.length ? problems.join("; ") : null };
}
const answer = (status: number, body: unknown, headers: Record<string, string> = {}): Response =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", ...headers } });
// wantsPage: a browser navigation (GET/HEAD asking for HTML) rather than an API call.
export const wantsPage = (request: Request): boolean =>
  ["GET", "HEAD"].includes(request.method) && (request.headers.get("accept") ?? "").includes("text/html");
// loginRedirect: a page gets a 302 to the login URL with next = this request's path (never a
// host); an API call gets 401 JSON with a coarse reason: no_session, expired or invalid_session.
export function loginRedirect(request: Request, c: AuthConfig, reason = "no_session"): Response {
  if (!wantsPage(request)) return answer(401, { error: "unauthorized", reason }, { "www-authenticate": 'Bearer realm="ssg-auth"' });
  const here = new URL(request.url);
  const next = "/" + (here.pathname + here.search).replace(/^\/+/, "");
  const to = new URL(c.loginUrl, here);
  to.searchParams.set("next", next.length <= 512 ? next : "/");
  return new Response(null, { status: 302, headers: { location: to.href, "cache-control": "no-store" } });
}
// denied turns a failed verdict into the one answer every consumer gives (CONTRACT.md).
export function denied(request: Request, c: AuthConfig, reason: Reason): Response {
  if (reason === "not_configured") return answer(503, { error: "not_configured", detail: c.configProblem ?? "the auth key or issuer is unusable" });
  if (reason === "keys_unavailable") return answer(503, { error: "auth_unavailable" }, { "retry-after": "60" });
  return loginRedirect(request, c, reason === "no_session" || reason === "expired" ? reason : "invalid_session");
}
// forbidden: signed in, but without `permission`.
export const forbidden = (permission: string): Response => answer(403, { error: "forbidden", permission });
