// Backward-compatibility suite, part 2 (this template's side): the table an
// install deployed before 1.9.8 has is the table this code creates and uses.
// ensureSchema on a v1 database changes nothing, and on an empty one creates
// exactly v1; schema.sql (for hand-applied setups) defines the same columns
// and indexes. Its own file: ensureSchema caches per process.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";
import { ensureSchema } from "../functions/api/newsletter/_schema.ts";
import { sqliteD1, subscribersMaster, withSchemaV1 } from "./sqlite-d1.ts";

const columns = (db: DatabaseSync) => db.prepare("PRAGMA table_info(subscribers)").all().map((c) => ({ ...c }));
const indexes = (db: DatabaseSync) =>
  db.prepare("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'subscribers' AND sql IS NOT NULL ORDER BY name").all().map((r) => r.name);

test("compat: ensureSchema creates schema v1 exactly (table and index SQL text included)", async () => {
  const empty = sqliteD1();
  await ensureSchema({ NEWSLETTER_DB: empty } as never);
  assert.deepEqual(subscribersMaster(empty), subscribersMaster(withSchemaV1()));
});

test("compat: schema.sql defines the same columns and indexes as schema v1", () => {
  const byFile = new DatabaseSync(":memory:");
  byFile.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  const v1 = withSchemaV1().sqlite;
  assert.deepEqual(columns(byFile), columns(v1));
  assert.deepEqual(indexes(byFile), indexes(v1));
  assert.deepEqual(indexes(v1), ["idx_subscribers_status", "idx_subscribers_updated"]);
});
