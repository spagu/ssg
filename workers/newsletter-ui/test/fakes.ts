// Stand-ins for the bindings the worker uses, shared by every test file.
//
// The D1 fake runs real SQL on node:sqlite (SQLite, the engine D1 is built
// on), so a test exercises the statements the worker actually sends instead of
// matching query strings by hand. Later features write a lot of SQL; give them
// a database, not a mock.
import { DatabaseSync } from "node:sqlite";

type Arg = string | number | bigint | null | Uint8Array;

// fakeD1 wraps an in-memory SQLite database in the D1 API surface the worker
// uses: prepare/bind/first/all/run/raw, batch (one transaction) and exec.
// `failOn` makes any statement matching it throw, to test error paths; `calls`
// records every statement in order.
export function fakeD1(sqlite = new DatabaseSync(":memory:")) {
  const calls: string[] = [];
  const state = { failOn: null as RegExp | null };

  const toArgs = (args: unknown[]): Arg[] =>
    args.map((v) => {
      if (v === undefined) throw new Error("D1_TYPE_ERROR: Type 'undefined' not supported");
      return typeof v === "boolean" ? (v ? 1 : 0) : (v as Arg);
    });

  // exec runs one statement synchronously; batch needs that to stay atomic.
  const exec = (sql: string, args: unknown[]) => {
    calls.push(sql);
    if (state.failOn?.test(sql)) throw new Error("D1_ERROR: forced failure for " + sql.slice(0, 40));
    const stmt = sqlite.prepare(sql);
    const bound = toArgs(args);
    if (stmt.columns().length === 0) {
      const r = stmt.run(...bound);
      return { results: [] as Record<string, unknown>[], meta: { changes: Number(r.changes), last_row_id: Number(r.lastInsertRowid) } };
    }
    const results = stmt.all(...bound).map((row) => ({ ...row }));
    return { results, meta: { changes: 0, last_row_id: 0 } };
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
    async raw() {
      return exec(sql, args).results.map((row) => Object.values(row));
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
    async exec(sql: string) {
      calls.push(sql);
      sqlite.exec(sql);
      return { count: 1, duration: 0 };
    },
  };
}
export type FakeD1 = ReturnType<typeof fakeD1>;

// SUBSCRIBERS_V1 is the `newsletter` template's table exactly as schema v1
// created it — frozen here on purpose: newsletter-ui must keep working on a
// database that template filled, whatever later versions of it do.
export const SUBSCRIBERS_V1 = `
CREATE TABLE IF NOT EXISTS subscribers (
  id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, status TEXT NOT NULL, language TEXT,
  source_page TEXT, tags TEXT NOT NULL DEFAULT '', consent_text TEXT NOT NULL, consent_at TEXT NOT NULL,
  token TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, confirmed_at TEXT,
  unsubscribed_at TEXT, confirm_sent_at TEXT, ip_hash TEXT, ua_hash TEXT
);
CREATE INDEX IF NOT EXISTS idx_subscribers_status ON subscribers (status, language);
CREATE INDEX IF NOT EXISTS idx_subscribers_updated ON subscribers (status, updated_at);`;

// withSubscribers installs the v1 table, as a deployed `newsletter` would have.
export function withSubscribers(d1: FakeD1): FakeD1 {
  d1.sqlite.exec(SUBSCRIBERS_V1);
  return d1;
}

// fakeQueue records what a producer sends.
export function fakeQueue() {
  const sent: { body: unknown; options?: unknown }[] = [];
  return {
    sent,
    async send(body: unknown, options?: unknown) {
      sent.push({ body, options });
    },
    async sendBatch(messages: { body: unknown; delaySeconds?: number }[]) {
      for (const m of messages) sent.push({ body: m.body, options: { delaySeconds: m.delaySeconds } });
    },
  };
}

// fakeRateLimiter allows `max` calls per key, then refuses, like the binding.
export function fakeRateLimiter(max = Infinity) {
  const counts = new Map<string, number>();
  return {
    counts,
    async limit({ key }: { key: string }) {
      const n = (counts.get(key) ?? 0) + 1;
      counts.set(key, n);
      return { success: n <= max };
    },
  };
}

// fakeAssets serves `files` by path ("/" → "/index.html"), 404 otherwise, and
// records the URLs it was asked for.
export function fakeAssets(files: Record<string, string> = { "/index.html": "<!doctype html><title>panel</title>" }) {
  const requested: string[] = [];
  return {
    requested,
    async fetch(request: Request) {
      const path = new URL(request.url).pathname;
      requested.push(path);
      const body = files[path === "/" ? "/index.html" : path];
      if (body === undefined) return new Response("not found", { status: 404 });
      return new Response(body, { headers: { "content-type": "text/html; charset=utf-8", "cache-control": "public, max-age=3600" } });
    },
  };
}

// fakeCtx collects waitUntil promises so a test can await them.
export function fakeCtx() {
  const pending: Promise<unknown>[] = [];
  return { pending, waitUntil: (p: Promise<unknown>) => void pending.push(p), passThroughOnException() {} };
}

// fakeBatch builds a queue MessageBatch whose messages record ack/retry.
export function fakeBatch(bodies: unknown[], queue = "ssg-newsletter-send") {
  const messages = bodies.map((body, i) => ({
    id: String(i),
    timestamp: new Date(0),
    body,
    attempts: 1,
    acked: false,
    retried: null as null | { delaySeconds?: number },
    ack() {
      this.acked = true;
    },
    retry(options: { delaySeconds?: number } = {}) {
      this.retried = options;
    },
  }));
  return { queue, messages, ackAll() {}, retryAll() {} };
}

// makeEnv is a Basic-auth environment on a fresh database with every binding;
// override or delete what a test is about.
export const PASSWORD = "correct horse battery staple";
export function makeEnv(overrides: Record<string, unknown> = {}) {
  return {
    NEWSLETTER_DB: withSubscribers(fakeD1()),
    NEWSLETTER_QUEUE: fakeQueue(),
    ASSETS: fakeAssets(),
    NEWSLETTER_ADMIN_PASSWORD: PASSWORD,
    ...overrides,
  } as Record<string, unknown> & { NEWSLETTER_DB: FakeD1 };
}

export const ORIGIN = "https://example.com";
export const basic = (password: string, user = "admin") => "Basic " + btoa(`${user}:${password}`);

// req builds a request to ORIGIN + path; `auth` adds Basic credentials and a
// state-changing method gets the CSRF header unless `csrf: false`.
export function req(path: string, init: RequestInit & { auth?: string; csrf?: boolean; ip?: string } = {}) {
  const { auth, csrf, ip, ...rest } = init;
  const headers = new Headers(rest.headers);
  if (auth !== undefined) headers.set("authorization", basic(auth));
  if (csrf !== false && rest.method && !["GET", "HEAD"].includes(rest.method)) headers.set("x-newsletter-admin", "1");
  headers.set("cf-connecting-ip", ip ?? "203.0.113.7");
  return new Request(ORIGIN + path, { ...rest, headers });
}
