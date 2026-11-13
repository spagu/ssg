// Own tables only: created idempotently, migrated by version, and the
// `newsletter` template's subscribers table left exactly as it was.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE_VERSION, COMPONENT, MIGRATIONS, TABLES, ensureSchema, migrate, schemaVersion, subscribersReady } from "../src/schema.ts";
import { fakeD1, withSubscribers } from "./fakes.ts";

const tablesOf = (d1: ReturnType<typeof fakeD1>) =>
  d1.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").all().map((r) => r.name);

// subscribersSnapshot is everything SQLite knows about the table: its DDL,
// its indexes, its columns, and its rows.
const subscribersSnapshot = (d1: ReturnType<typeof fakeD1>) =>
  JSON.stringify({
    master: d1.sqlite.prepare("SELECT type, name, tbl_name, sql FROM sqlite_master WHERE tbl_name = 'subscribers' ORDER BY name").all(),
    columns: d1.sqlite.prepare("PRAGMA table_info(subscribers)").all(),
    rows: d1.sqlite.prepare("SELECT * FROM subscribers ORDER BY id").all(),
  });

test("ensureSchema creates the own tables and records the latest version", async () => {
  const d1 = fakeD1();
  await ensureSchema({ NEWSLETTER_DB: d1 } as never);
  assert.deepEqual(tablesOf(d1), [
    "auth_attempts", "campaign_sends", "campaigns", "click_links", "hook_events", "schema_meta",
    "send_batches", "send_dead_letters", "send_plans", "send_rate", "settings", "soft_bounces", "suppressions",
  ]);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), MIGRATIONS[MIGRATIONS.length - 1].version);
});

test("a version-1 database gets the send engine's tables from migration 2", async () => {
  const d1 = fakeD1();
  await migrate(d1 as never, []);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), BASE_VERSION);
  d1.sqlite.exec("DROP TABLE send_plans"); // as if created before GO-126
  await migrate(d1 as never, MIGRATIONS);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), MIGRATIONS[MIGRATIONS.length - 1].version);
  assert.ok(tablesOf(d1).includes("send_plans"));
});

test("ensureSchema never creates, alters or indexes subscribers", async () => {
  const d1 = withSubscribers(fakeD1());
  d1.sqlite
    .prepare("INSERT INTO subscribers (id, email, status, consent_text, consent_at, token, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)")
    .run("s1", "a@example.com", "confirmed", "yes", "2026-01-01", "t".repeat(43), "2026-01-01", "2026-01-01");
  const before = subscribersSnapshot(d1);
  await ensureSchema({ NEWSLETTER_DB: d1 } as never);
  await migrate(d1 as never, [{ version: 2, statements: ["ALTER TABLE campaigns ADD COLUMN preheader TEXT"] }]);
  assert.equal(subscribersSnapshot(d1), before);
  assert.ok(!d1.calls.some((sql) => /\bsubscribers\b/i.test(sql) && !/sqlite_master/.test(sql)), "no statement names subscribers");
  assert.ok(TABLES.every((s) => /^CREATE (TABLE|INDEX) IF NOT EXISTS/.test(s)), "only IF NOT EXISTS");
});

test("ensureSchema runs once per database and is idempotent across databases", async () => {
  const d1 = fakeD1();
  const env = { NEWSLETTER_DB: d1 } as never;
  await ensureSchema(env);
  const n = d1.calls.length;
  await ensureSchema(env);
  assert.equal(d1.calls.length, n, "cached");
  // A second run of the full migration on the same database changes nothing.
  const meta = d1.sqlite.prepare("SELECT * FROM schema_meta").all();
  await migrate(d1 as never, []);
  assert.deepEqual(d1.sqlite.prepare("SELECT * FROM schema_meta").all(), meta);
});

test("ensureSchema retries after a failure", async () => {
  const d1 = fakeD1();
  d1.state.failOn = /CREATE TABLE IF NOT EXISTS settings/;
  const env = { NEWSLETTER_DB: d1 } as never;
  await assert.rejects(ensureSchema(env), /forced failure/);
  assert.deepEqual(tablesOf(d1), [], "the batch rolled back");
  d1.state.failOn = null;
  await ensureSchema(env);
  assert.ok(tablesOf(d1).includes("settings"));
});

test("migrate applies newer migrations once, tolerating a duplicate column", async () => {
  const d1 = fakeD1();
  const m2 = { version: 2, statements: ["ALTER TABLE campaigns ADD COLUMN preheader TEXT"] };
  const m3 = { version: 3, statements: ["ALTER TABLE campaigns ADD COLUMN preheader TEXT", "CREATE TABLE IF NOT EXISTS x (a)"] };
  await migrate(d1 as never, [m2]);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), 2);
  const before = d1.calls.length;
  await migrate(d1 as never, [m2]);
  assert.ok(!d1.calls.slice(before).some((s) => s.startsWith("ALTER")), "an applied migration is skipped");
  await migrate(d1 as never, [m2, m3]);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), 3);
  assert.ok(tablesOf(d1).includes("x"));
});

test("migrate surfaces any other migration error", async () => {
  const d1 = fakeD1();
  await assert.rejects(migrate(d1 as never, [{ version: 2, statements: ["ALTER TABLE nope ADD COLUMN a TEXT"] }]), /no such table/);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), 1);
});

test("schemaVersion is 0 before the schema exists for this component", async () => {
  const d1 = fakeD1();
  await ensureSchema({ NEWSLETTER_DB: d1 } as never);
  d1.sqlite.prepare("DELETE FROM schema_meta WHERE component = ?").run(COMPONENT);
  assert.equal(await schemaVersion({ NEWSLETTER_DB: d1 } as never), 0);
});

test("subscribersReady: false until the table exists, then cached", async () => {
  const d1 = fakeD1();
  const env = { NEWSLETTER_DB: d1 } as never;
  assert.equal(await subscribersReady(env), false);
  withSubscribers(d1);
  assert.equal(await subscribersReady(env), true);
  const n = d1.calls.length;
  assert.equal(await subscribersReady(env), true);
  assert.equal(d1.calls.length, n);
});
