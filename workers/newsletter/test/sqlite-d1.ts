// A D1 stand-in that runs real SQL on node:sqlite (SQLite, the engine D1 is
// built on). The backward-compatibility suite needs it: it pins what the
// endpoints answer and what they leave in the table, and only a real database
// shows that — a recording fake would only repeat the statements back.
import { DatabaseSync } from "node:sqlite";

type Arg = string | number | bigint | null | Uint8Array;

// SCHEMA_V1 is the `subscribers` table exactly as schema v1 (`_schema.ts` on
// the 1.9.7 release) creates it, statement for statement. Frozen on purpose:
// an install deployed before this round has precisely this table, and the
// suite must keep proving the current code works on it.
export const SCHEMA_V1 = [
  `CREATE TABLE IF NOT EXISTS subscribers (
     id              TEXT PRIMARY KEY,
     email           TEXT NOT NULL UNIQUE,
     status          TEXT NOT NULL,
     language        TEXT,
     source_page     TEXT,
     tags            TEXT NOT NULL DEFAULT '',
     consent_text    TEXT NOT NULL,
     consent_at      TEXT NOT NULL,
     token           TEXT NOT NULL UNIQUE,
     created_at      TEXT NOT NULL,
     updated_at      TEXT NOT NULL,
     confirmed_at    TEXT,
     unsubscribed_at TEXT,
     confirm_sent_at TEXT,
     ip_hash         TEXT,
     ua_hash         TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_status ON subscribers (status, language)`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_updated ON subscribers (status, updated_at)`,
];

// sqliteD1 wraps an in-memory database in the D1 surface the template uses:
// prepare/bind/first/all/run and batch (one transaction).
export function sqliteD1(sqlite = new DatabaseSync(":memory:")) {
  const exec = (sql: string, args: unknown[]) => {
    const stmt = sqlite.prepare(sql);
    const bound = args.map((v) => {
      if (v === undefined) throw new Error("D1_TYPE_ERROR: Type 'undefined' not supported");
      return v as Arg;
    });
    if (stmt.columns().length === 0) {
      const r = stmt.run(...bound);
      return { results: [] as Record<string, unknown>[], meta: { changes: Number(r.changes) } };
    }
    return { results: stmt.all(...bound).map((row) => ({ ...row })), meta: { changes: 0 } };
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
export type SqliteD1 = ReturnType<typeof sqliteD1>;

// withSchemaV1 creates the v1 table and indexes, as a deployed install has them.
export function withSchemaV1(d1: SqliteD1 = sqliteD1()): SqliteD1 {
  for (const s of SCHEMA_V1) d1.sqlite.exec(s);
  return d1;
}

// subscribersMaster is everything SQLite knows about the table: the table and
// its indexes, with their SQL text, in a stable order.
export function subscribersMaster(d1: SqliteD1): unknown[] {
  return d1.sqlite
    .prepare("SELECT type, name, tbl_name, sql FROM sqlite_master WHERE tbl_name = 'subscribers' ORDER BY type, name")
    .all()
    .map((r) => ({ ...r }));
}
