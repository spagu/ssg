// The runtime schema: schema.sql describes the same tables, creation is
// idempotent, migrations apply once, a failure is retried.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { BASE_VERSION, COMPONENT, ensureSchema, migrate } from "../functions/api/auth/_schema.ts";
import { fakeD1 } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

const shape = (d1: FakeD1) =>
  d1.sqlite
    .prepare("SELECT type, name, tbl_name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name")
    .all()
    .map((o) => ({ ...o, columns: o.type === "table" ? d1.sqlite.prepare(`PRAGMA table_info(${o.name})`).all().map((c) => ({ ...c })) : [] }));

test("schema.sql matches the runtime schema", async () => {
  const fromCode = fakeD1();
  await ensureSchema({ AUTH_DB: fromCode } as never);
  const fromFile = fakeD1();
  fromFile.sqlite.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  assert.deepEqual(shape(fromFile), shape(fromCode));
  assert.equal((fromFile.sqlite.prepare("SELECT version FROM schema_meta WHERE component = 'auth'").get() as { version: number }).version, BASE_VERSION);
});

test("migrate is idempotent and applies a later migration once, tolerating a raced ALTER", async () => {
  const d1 = fakeD1();
  await migrate(d1 as never, []);
  await migrate(d1 as never, []);
  const m = [{ version: 2, statements: ["ALTER TABLE users ADD COLUMN locale TEXT", "ALTER TABLE users ADD COLUMN locale TEXT"] }];
  await migrate(d1 as never, m);
  const version = () => (d1.sqlite.prepare("SELECT version FROM schema_meta WHERE component = ?").get(COMPONENT) as { version: number }).version;
  assert.equal(version(), 2);
  const before = d1.calls.length;
  await migrate(d1 as never, m);
  assert.ok(!d1.calls.slice(before).some((s) => s.startsWith("ALTER")), "an applied migration is not run again");
  await assert.rejects(migrate(d1 as never, [{ version: 3, statements: ["ALTER TABLE nowhere ADD COLUMN x TEXT"] }]), /no such table/);
  assert.equal(version(), 2);
});

test("ensureSchema runs once per database and retries after a failure", async () => {
  const d1 = fakeD1();
  d1.state.failOn = /CREATE TABLE IF NOT EXISTS users/;
  await assert.rejects(ensureSchema({ AUTH_DB: d1 } as never), /forced failure/);
  d1.state.failOn = null;
  await ensureSchema({ AUTH_DB: d1 } as never);
  const n = d1.calls.length;
  await ensureSchema({ AUTH_DB: d1 } as never);
  assert.equal(d1.calls.length, n, "cached");
});
