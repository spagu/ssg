// CORE_DB's tables (GO-149): schema.sql describes exactly what the worker
// creates, creation is idempotent and runs once per database, a failure is
// retried by the next request, and every name stays in core's own namespace.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { BASE_VERSION, COMPONENT, TABLES, ensureSchema } from "../functions/api/core/_schema.ts";
import { fakeD1 } from "./d1.ts";
import type { FakeD1 } from "./d1.ts";

const SQL = readFileSync(new URL("../schema.sql", import.meta.url), "utf8");
const shape = (d1: FakeD1) =>
  d1
    .rows("SELECT type, name, tbl_name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name")
    .map((o) => ({ ...o, columns: o.type === "table" ? d1.rows(`PRAGMA table_info(${o.name})`) : [] }));

test("schema.sql matches the runtime schema, version row included", async () => {
  const fromCode = fakeD1();
  await ensureSchema(fromCode as never);
  const fromFile = fakeD1();
  fromFile.sqlite.exec(SQL);
  assert.deepEqual(shape(fromFile), shape(fromCode));
  assert.deepEqual(fromFile.rows("SELECT component, version FROM schema_meta"), [{ component: COMPONENT, version: BASE_VERSION }]);
  assert.deepEqual(fromCode.rows("SELECT component, version FROM schema_meta"), [{ component: COMPONENT, version: BASE_VERSION }]);
});

test("every table and index is core's own (core_ prefix), except the shared schema_meta", () => {
  const names = [...TABLES.join("\n").matchAll(/(?:TABLE|INDEX) IF NOT EXISTS (\w+)/g)].map((m) => m[1]);
  assert.ok(names.length >= 6);
  for (const n of names) assert.ok(n === "schema_meta" || /^(core_|idx_core_)/.test(n), n);
  assert.doesNotMatch(SQL.replace(/--.*$/gm, ""), /\bDROP\b|\bALTER\b/i);
});

test("ensureSchema runs once per database, tolerates a database another template made, and retries after a failure", async () => {
  const d1 = fakeD1();
  // A database the auth worker already uses: its schema_meta has the same shape.
  d1.sqlite.exec("CREATE TABLE schema_meta (component TEXT PRIMARY KEY, version INTEGER NOT NULL, updated_at TEXT NOT NULL); INSERT INTO schema_meta VALUES ('auth', 3, 'x')");
  await ensureSchema(d1 as never);
  const after = d1.calls.length;
  await ensureSchema(d1 as never);
  assert.equal(d1.calls.length, after, "the second call sends nothing");
  assert.equal(d1.rows("SELECT COUNT(*) AS n FROM schema_meta")[0].n, 2);

  const failing = fakeD1();
  failing.state.failOn = /core_lockout/;
  await assert.rejects(ensureSchema(failing as never), /forced failure/);
  assert.equal(failing.rows("SELECT name FROM sqlite_master WHERE name = 'core_module_settings'").length, 0, "the batch rolled back");
  failing.state.failOn = null;
  await ensureSchema(failing as never);
  assert.equal(failing.rows("SELECT name FROM sqlite_master WHERE name = 'core_lockout'").length, 1, "the next call tried again");
});
