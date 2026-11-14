// Stand-ins for the bindings and the browser, shared by every test file.
//
// The D1 fake runs real SQL on node:sqlite (SQLite, the engine D1 is built
// on) — the same approach as newsletter-ui's — so tests exercise the
// statements the worker actually sends.
import { DatabaseSync } from "node:sqlite";
import { clock } from "../functions/api/auth/_lib.ts";
import { kdf } from "../functions/api/auth/_password.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

type Arg = string | number | bigint | null | Uint8Array;

export function fakeD1(sqlite = new DatabaseSync(":memory:")) {
  const calls: string[] = [];
  const state = { failOn: null as RegExp | null };
  const toArgs = (args: unknown[]): Arg[] =>
    args.map((v) => {
      if (v === undefined) throw new Error("D1_TYPE_ERROR: Type 'undefined' not supported");
      return typeof v === "boolean" ? (v ? 1 : 0) : (v as Arg);
    });
  const exec = (sql: string, args: unknown[]) => {
    calls.push(sql);
    if (state.failOn?.test(sql)) throw new Error("D1_ERROR: forced failure for " + sql.slice(0, 40));
    const stmt = sqlite.prepare(sql);
    const bound = toArgs(args);
    if (stmt.columns().length === 0) {
      const r = stmt.run(...bound);
      return { results: [] as Record<string, unknown>[], meta: { changes: Number(r.changes) } };
    }
    const results = stmt.all(...bound).map((row) => ({ ...row }));
    return { results, meta: { changes: results.length } };
  };
  const statement = (sql: string, args: unknown[] = []) => ({
    sql,
    args,
    bind: (...a: unknown[]) => statement(sql, a),
    async first(column?: string) {
      const row = exec(sql, args).results[0];
      if (!row) return null;
      return column ? row[column] : row;
    },
    async all() {
      const r = exec(sql, args);
      return { success: true, results: r.results, meta: r.meta };
    },
    async run() {
      const r = exec(sql, args);
      return { success: true, results: r.results, meta: r.meta };
    },
  });
  type Statement = ReturnType<typeof statement>;
  return {
    sqlite,
    calls,
    state,
    prepare: (sql: string) => statement(sql),
    async batch(list: Statement[]) {
      sqlite.exec("BEGIN");
      try {
        const out = list.map((s) => ({ success: true, ...exec(s.sql, s.args) }));
        sqlite.exec("COMMIT");
        return out;
      } catch (e) {
        sqlite.exec("ROLLBACK");
        throw e;
      }
    },
  };
}
export type FakeD1 = ReturnType<typeof fakeD1>;

export const ORIGIN = "https://example.com";
export const T0 = Date.parse("2026-11-13T10:00:00.000Z");

// useClock pins the worker's clock; returns a setter.
export function useClock(start = T0) {
  let now = start;
  clock.now = () => now;
  return {
    get now() {
      return now;
    },
    set: (ms: number) => (now = ms),
    advance: (ms: number) => (now += ms),
  };
}

// newKeyPair returns a fresh P-256 private JWK (as AUTH_SIGNING_KEY expects)
// and its public JWK, both as JSON strings.
export async function newKeyPair(): Promise<{ priv: string; pub: string; d: string }> {
  const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
  const { kty, crv, x, y, d } = (await crypto.subtle.exportKey("jwk", pair.privateKey)) as JsonWebKey;
  return { priv: JSON.stringify({ kty, crv, x, y, d }), pub: JSON.stringify({ kty, crv, x, y }), d: d as string };
}

const DEFAULT_KEYS = await newKeyPair();
export const KEYS = DEFAULT_KEYS;

// makeEnv: a configured worker with the cheapest accepted hash cost.
export function makeEnv(over: Partial<Env> = {}): Env & { AUTH_DB: FakeD1 } {
  return {
    AUTH_DB: fakeD1() as never,
    AUTH_SIGNING_KEY: KEYS.priv,
    AUTH_PBKDF2_ITERATIONS: "10000",
    ...over,
  } as Env & { AUTH_DB: FakeD1 };
}

// Browser: a cookie jar that applies Set-Cookie (Max-Age=0 deletes) and sends
// what a same-origin page would.
export class Browser {
  jar = new Map<string, string>();
  ip: string;
  constructor(ip = "203.0.113.7") {
    this.ip = ip;
  }
  take(res: Response): Response {
    for (const c of res.headers.getSetCookie()) {
      const [pair] = c.split(";");
      const at = pair.indexOf("=");
      const name = pair.slice(0, at);
      if (/Max-Age=0\b/.test(c)) this.jar.delete(name);
      else this.jar.set(name, pair.slice(at + 1));
    }
    return res;
  }
  cookieHeader(): string {
    return [...this.jar].map(([k, v]) => `${k}=${v}`).join("; ");
  }
}

export interface ReqOptions {
  method?: string;
  json?: unknown;
  form?: Record<string, string>;
  headers?: Record<string, string>;
  browser?: Browser;
  csrf?: boolean; // default true: x-ssg-auth: 1 on JSON, Origin on everything
  params?: Record<string, string>; // the route's [param] values, as Pages passes them
}

export function req(path: string, o: ReqOptions = {}): Request {
  const headers = new Headers(o.headers);
  const csrf = o.csrf ?? true;
  if (csrf && !headers.has("origin")) headers.set("origin", ORIGIN);
  if (o.browser) {
    headers.set("cf-connecting-ip", o.browser.ip);
    if (o.browser.jar.size) headers.set("cookie", o.browser.cookieHeader());
  }
  let body: string | undefined;
  if (o.form) {
    headers.set("content-type", "application/x-www-form-urlencoded");
    body = new URLSearchParams(o.form).toString();
  } else if (o.json !== undefined) {
    headers.set("content-type", "application/json");
    body = JSON.stringify(o.json);
    if (csrf && !headers.has("x-ssg-auth")) headers.set("x-ssg-auth", "1");
  } else if (o.method && o.method !== "GET" && csrf && !headers.has("x-ssg-auth")) {
    headers.set("x-ssg-auth", "1");
  }
  return new Request(ORIGIN + path, { method: o.method ?? (body !== undefined ? "POST" : "GET"), headers, body });
}

type Handler = (ctx: { request: Request; env: Env; params: Record<string, string> }) => Response | Promise<Response>;

// call runs a Pages handler; a Browser in the options keeps the cookies.
export async function call(handler: unknown, env: Env, path: string, o: ReqOptions = {}): Promise<Response> {
  const res = await (handler as Handler)({ request: req(path, o), env, params: o.params ?? {} });
  return o.browser ? o.browser.take(res) : res;
}

export const PASSWORD = "correct horse battery";

// counting wraps kdf.derive, recording the iteration count of every call;
// `refuse` makes the "runtime" reject counts above it, like Cloudflare's cap.
export function counting(refuse = Infinity) {
  const real = kdf.derive;
  const calls: number[] = [];
  kdf.derive = async (p, s, n) => {
    calls.push(n);
    if (n > refuse) throw new Error(`Pbkdf2 failed: iteration counts above ${refuse} are not supported (requested ${n}).`);
    return real(p, s, n);
  };
  return { calls, restore: () => (kdf.derive = real) };
}

