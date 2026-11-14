// A D1 stand-in on node:sqlite (SQLite, the engine D1 is built on), so tests
// run the statements the worker really sends — the approach of the auth and
// newsletter-ui templates. `calls` records every statement; `failOn` makes
// matching statements throw, to exercise the paths that survive a database
// failure. batch() is a transaction, as D1's is.
import { DatabaseSync } from "node:sqlite";

type Arg = string | number | bigint | null | Uint8Array;

export function fakeD1(sqlite = new DatabaseSync(":memory:")) {
  const calls: string[] = [];
  const state = { failOn: null as RegExp | null };
  const exec = (sql: string, args: unknown[]) => {
    calls.push(sql);
    if (state.failOn?.test(sql)) throw new Error("D1_ERROR: forced failure");
    const stmt = sqlite.prepare(sql);
    const bound = args.map((v) => {
      if (v === undefined) throw new Error("D1_TYPE_ERROR: Type 'undefined' not supported");
      return typeof v === "boolean" ? (v ? 1 : 0) : (v as Arg);
    });
    if (stmt.columns().length === 0) {
      stmt.run(...bound);
      return [] as Record<string, unknown>[];
    }
    return stmt.all(...bound).map((row) => ({ ...row }));
  };
  const statement = (sql: string, args: unknown[] = []) => ({
    sql,
    args,
    bind: (...a: unknown[]) => statement(sql, a),
    async first() {
      return exec(sql, args)[0] ?? null;
    },
    async all() {
      return { results: exec(sql, args) };
    },
    async run() {
      return { results: exec(sql, args) };
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
        const out = list.map((s) => ({ results: exec(s.sql, s.args) }));
        sqlite.exec("COMMIT");
        return out;
      } catch (e) {
        sqlite.exec("ROLLBACK");
        throw e;
      }
    },
    rows: (sql: string) => sqlite.prepare(sql).all().map((r) => ({ ...r })),
  };
}
export type FakeD1 = ReturnType<typeof fakeD1>;
