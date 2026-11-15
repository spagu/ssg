// Stand-ins for the shop's bindings, shared by every test file.
//
// The D1 fake runs real SQL on node:sqlite (SQLite, the engine D1 is built
// on) — the same approach as the auth and newsletter-ui templates — so the
// tests exercise the statements the shop actually sends. R2 and KV are small
// in-memory maps with the slice of each API the shop uses: R2's ranged get
// (the download route hands it the request's headers) and KV's expiry and
// prefix listing.
import { DatabaseSync } from "node:sqlite";

type Arg = string | number | bigint | null | Uint8Array;

/** fakeD1 is a D1Database over an in-memory SQLite database. `state.failOn`
 *  makes any statement matching it throw, for the paths that handle a failing
 *  database. */
export function fakeD1(sqlite = new DatabaseSync(":memory:")) {
  const calls: string[] = [];
  const state = { failOn: null as RegExp | null };
  const toArgs = (args: unknown[]): Arg[] =>
    args.map((v) => {
      // D1 refuses undefined rather than storing NULL; so does this fake, so a
      // test cannot pass on a binding the real database would reject.
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
      return {
        results: [] as Record<string, unknown>[],
        meta: { changes: Number(r.changes), last_row_id: Number(r.lastInsertRowid) },
      };
    }
    const results = stmt.all(...bound).map((row) => ({ ...row }));
    return { results, meta: { changes: results.length, last_row_id: 0 } };
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
    // A batch is one transaction in D1: all of it or none of it.
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

interface Stored {
  bytes: Uint8Array;
  httpMetadata: { contentType?: string };
  customMetadata: Record<string, string>;
}

/** The range R2 resolves from a Range header: one `bytes=a-b`, `bytes=a-` or
 *  `bytes=-n` window, clamped to the object. Anything else is the whole
 *  object, as R2 answers an unsatisfiable range. */
function parseRange(header: string | null, size: number): { offset: number; length: number } | null {
  const m = /^bytes=(\d*)-(\d*)$/.exec(header ?? "");
  if (!m || (m[1] === "" && m[2] === "")) return null;
  if (m[1] === "") {
    const length = Math.min(Number(m[2]), size);
    return { offset: size - length, length };
  }
  const offset = Number(m[1]);
  if (offset >= size) return null;
  const end = m[2] === "" ? size - 1 : Math.min(Number(m[2]), size - 1);
  return { offset, length: end - offset + 1 };
}

/** fakeR2 is an R2Bucket in memory: put, get (with `range` as the request's
 *  Headers or as {offset, length}), head and delete. */
export function fakeR2() {
  const objects = new Map<string, Stored>();
  const describe = (key: string, o: Stored, range?: { offset: number; length: number }) => ({
    key,
    size: o.bytes.byteLength,
    httpMetadata: o.httpMetadata,
    customMetadata: o.customMetadata,
    range,
    writeHttpMetadata(headers: Headers) {
      if (o.httpMetadata.contentType) headers.set("content-type", o.httpMetadata.contentType);
    },
  });
  return {
    objects,
    async put(
      key: string,
      value: Uint8Array | ArrayBuffer | string,
      opts: { httpMetadata?: Stored["httpMetadata"]; customMetadata?: Record<string, string> } = {},
    ) {
      const bytes =
        typeof value === "string" ? new TextEncoder().encode(value) : new Uint8Array(value as ArrayBuffer).slice();
      const stored = { bytes, httpMetadata: opts.httpMetadata ?? {}, customMetadata: opts.customMetadata ?? {} };
      objects.set(key, stored);
      return describe(key, stored);
    },
    async head(key: string) {
      const o = objects.get(key);
      return o ? describe(key, o) : null;
    },
    async get(key: string, opts: { range?: Headers | { offset: number; length?: number } } = {}) {
      const o = objects.get(key);
      if (!o) return null;
      const size = o.bytes.byteLength;
      let range: { offset: number; length: number } | null = null;
      if (opts.range instanceof Headers) range = parseRange(opts.range.get("range"), size);
      else if (opts.range) range = { offset: opts.range.offset, length: opts.range.length ?? size - opts.range.offset };
      const slice = range ? o.bytes.slice(range.offset, range.offset + range.length) : o.bytes;
      return {
        ...describe(key, o, range ?? undefined),
        body: new Blob([slice]).stream(),
        arrayBuffer: async () => slice.buffer.slice(slice.byteOffset, slice.byteOffset + slice.byteLength),
        text: async () => new TextDecoder().decode(slice),
      };
    },
    async delete(keys: string | string[]) {
      for (const k of Array.isArray(keys) ? keys : [keys]) objects.delete(k);
    },
  };
}
export type FakeR2 = ReturnType<typeof fakeR2>;

/** fakeKV is a KVNamespace in memory. Expiry follows `clock.now`, so a test
 *  can move time past an `expirationTtl` without waiting. */
export function fakeKV(clock: { now: () => number } = { now: () => Date.now() }) {
  const store = new Map<string, { value: string; expires: number | null }>();
  const live = (key: string) => {
    const e = store.get(key);
    if (!e) return null;
    if (e.expires !== null && e.expires <= clock.now()) {
      store.delete(key);
      return null;
    }
    return e;
  };
  return {
    store,
    async get(key: string) {
      return live(key)?.value ?? null;
    },
    async put(key: string, value: string, opts: { expirationTtl?: number } = {}) {
      store.set(key, { value, expires: opts.expirationTtl ? clock.now() + opts.expirationTtl * 1000 : null });
    },
    async delete(key: string) {
      store.delete(key);
    },
    async list(opts: { prefix?: string } = {}) {
      const keys = [...store.keys()].filter((k) => k.startsWith(opts.prefix ?? "") && live(k)).map((name) => ({ name }));
      return { keys, list_complete: true, cursor: "" };
    },
  };
}
export type FakeKV = ReturnType<typeof fakeKV>;

/** sparseD1 wraps a fake D1 so that statements matching `match` answer in the
 *  thinnest shape D1's types allow: `all()` with no `results` (or with `rows`),
 *  `run()` with no `meta`, `first()` with no row. The shop guards each of those
 *  with a fallback (`results ?? []`, `meta?.changes ?? 0`, `row?.n ?? 0`); this
 *  is how a test reaches the fallback without changing the code. Every other
 *  statement still runs real SQL.
 *
 *  `first` narrows which statements lose their row (default: `match`; null:
 *  none), and `run: false` leaves `run()` untouched. */
export function sparseD1(
  db: FakeD1,
  match: RegExp,
  opts: { rows?: Record<string, unknown>[]; first?: RegExp | null; run?: boolean } = {},
): FakeD1 {
  type Stmt = ReturnType<FakeD1["prepare"]>;
  const firstMatch = opts.first === undefined ? match : opts.first;
  const wrap = (stmt: Stmt): Stmt => ({
    ...stmt,
    bind: (...a: unknown[]) => wrap(stmt.bind(...a)),
    all: async () => (match.test(stmt.sql) ? ({ success: true, results: opts.rows } as never) : stmt.all()),
    run: async () => (opts.run !== false && match.test(stmt.sql) ? ({ success: true } as never) : stmt.run()),
    first: async (column?: string) => (firstMatch?.test(stmt.sql) ? null : stmt.first(column)),
  });
  return { ...db, prepare: (sql: string) => wrap(db.prepare(sql)) };
}
