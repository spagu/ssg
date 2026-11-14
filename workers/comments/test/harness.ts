// Test machinery for the comments worker: a D1 stand-in on node:sqlite (real
// SQL, so the pinned answers are what a real table gives), a `call` that runs
// a Pages handler the way Pages does, and a Cloudflare Access issuer with a
// real RSA key so the Access mode is exercised end to end.
import { DatabaseSync } from "node:sqlite";
import { readFileSync } from "node:fs";

type Arg = string | number | bigint | null | Uint8Array;

// sqliteD1 wraps an in-memory database in the D1 surface the template uses,
// with the template's own schema.sql applied (ensureSchema caches per isolate,
// so a fresh database per test must already have its table).
export function sqliteD1() {
  const sqlite = new DatabaseSync(":memory:");
  sqlite.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  const exec = (sql: string, args: unknown[]) => {
    const stmt = sqlite.prepare(sql);
    if (stmt.columns().length === 0) {
      const r = stmt.run(...(args as Arg[]));
      return { results: [] as Record<string, unknown>[], meta: { changes: Number(r.changes) } };
    }
    return { results: stmt.all(...(args as Arg[])).map((row) => ({ ...row })), meta: { changes: 0 } };
  };
  const statement = (sql: string, args: unknown[] = []) => ({
    sql,
    args,
    bind: (...a: unknown[]) => statement(sql, a),
    async first() {
      return exec(sql, args).results[0] ?? null;
    },
    async all() {
      return { success: true, ...exec(sql, args) };
    },
    async run() {
      return { success: true, ...exec(sql, args) };
    },
  });
  return {
    sqlite,
    prepare: (sql: string) => statement(sql),
    async batch(list: ReturnType<typeof statement>[]) {
      return list.map((s) => ({ success: true, ...exec(s.sql, s.args) }));
    },
    rows: () => sqlite.prepare("SELECT id, url, author, status FROM comments ORDER BY id").all().map((r) => ({ ...r })),
  };
}
export type SqliteD1 = ReturnType<typeof sqliteD1>;

// seed inserts comments as the public endpoint would have stored them.
export function seed(db: SqliteD1, rows: { id: string; status: string; created_at: string; url?: string }[]) {
  const stmt = db.sqlite.prepare(
    "INSERT INTO comments (id, url, author, email, body, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
  );
  for (const r of rows) stmt.run(r.id, r.url ?? "/blog/post/", `Author ${r.id}`, `${r.id}@example.com`, `Body ${r.id}`, r.status, r.created_at);
}

export const PASSWORD = "moderator-pass";
export const basic = (password: string) => "Basic " + btoa(`:${password}`);

type Handler = (ctx: { request: Request; env: unknown }) => Promise<Response>;

// Observed is what a client sees: status, every header, the body text.
export interface Observed {
  status: number;
  headers: [string, string][];
  body: string;
}

export async function call(
  handler: Handler,
  init: { method?: string; path: string; headers?: Record<string, string>; body?: string; env: Record<string, unknown> },
): Promise<Observed> {
  const request = new Request(`https://example.com${init.path}`, { method: init.method ?? "GET", headers: init.headers, body: init.body });
  const res = await handler({ request, env: init.env });
  return { status: res.status, headers: [...res.headers.entries()].sort(), body: await res.text() };
}

export const JSON_HEADERS: [string, string][] = [["cache-control", "no-store"], ["content-type", "application/json"]];

// Access issues tokens for one Access application and answers the JWKS
// request the worker makes; install() swaps it in as the global fetch.
export const TEAM = "example-team";
export const AUD = "aud-comments";
const b64url = (b: Uint8Array | string) =>
  Buffer.from(typeof b === "string" ? b : b).toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");

export async function accessIssuer() {
  const pair = (await crypto.subtle.generateKey(
    { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const certs = { keys: [{ kid: "k1", kty: "RSA", n: jwk.n, e: jwk.e }] };
  return {
    async token(claims: Record<string, unknown> = {}): Promise<string> {
      const head = b64url(JSON.stringify({ alg: "RS256", kid: "k1" }));
      const body = b64url(
        JSON.stringify({ aud: [AUD], iss: `https://${TEAM}.cloudflareaccess.com`, exp: Math.floor(Date.now() / 1000) + 600, ...claims }),
      );
      const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, new TextEncoder().encode(`${head}.${body}`));
      return `${head}.${body}.${b64url(new Uint8Array(sig))}`;
    },
    install(): () => void {
      const real = globalThis.fetch;
      globalThis.fetch = (async (input: string | URL) => {
        if (String(input) === `https://${TEAM}.cloudflareaccess.com/cdn-cgi/access/certs`) return Response.json(certs);
        throw new Error(`unexpected fetch ${input}`);
      }) as typeof fetch;
      return () => void (globalThis.fetch = real);
    },
  };
}
