// The retention purge per status (#465), against real SQLite. Run from the
// repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { purgeExpired, retentionPolicy, specificDays } from "../functions/api/newsletter/_purge.ts";
import { fakeDB } from "./fakes.ts";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";
import type { SqliteD1 } from "./sqlite-d1.ts";

const ago = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString();

interface Row {
  status: string;
  created: number; // days ago
  updated: number;
  consent?: number;
  sent?: number | null;
  unsub?: number | null;
}

// seed inserts one row per name; ages are in days, null leaves the column NULL.
function seed(rows: Record<string, Row>): SqliteD1 {
  const d1 = withSchemaV1(sqliteD1());
  const stmt = d1.sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, consent_text, consent_at, token, created_at, updated_at,
       confirm_sent_at, unsubscribed_at) VALUES (?, ?, ?, 'I agree', ?, ?, ?, ?, ?, ?)`,
  );
  const opt = (d: number | null | undefined) => (d == null ? null : ago(d));
  for (const [name, r] of Object.entries(rows)) {
    stmt.run(name, `${name}@example.com`, r.status, ago(r.consent ?? r.created), `tok-${name}`, ago(r.created), ago(r.updated), opt(r.sent), opt(r.unsub));
  }
  return d1;
}

const left = (d1: SqliteD1) =>
  d1.sqlite.prepare("SELECT id FROM subscribers ORDER BY id").all().map((r) => String(r.id));

const purge = (d1: SqliteD1, vars: Record<string, string>) => purgeExpired({ NEWSLETTER_DB: d1, ...vars } as never, true);

test("specificDays: blank falls back, a whole number counts, anything else keeps", () => {
  assert.equal(specificDays(undefined), null);
  assert.equal(specificDays(""), null);
  assert.equal(specificDays("  "), null);
  assert.equal(specificDays(" 14 "), 14);
  for (const bad of ["0", "-3", "x", "1.5", "7d"]) assert.equal(specificDays(bad), 0, bad);
});

test("retentionPolicy: neither set is the old rule for both statuses", () => {
  const p = retentionPolicy({ NEWSLETTER_RETENTION_DAYS: "30" } as never);
  assert.deepEqual(p.pending, { days: 30, clock: "updated_at" });
  assert.deepEqual(p.unsubscribed, { days: 30, clock: "updated_at" });
  const q = retentionPolicy({ NEWSLETTER_RETENTION_DAYS: "30", NEWSLETTER_PENDING_RETENTION_DAYS: "0", NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: "1095" } as never);
  assert.equal(q.pending.days, 0);
  assert.equal(q.unsubscribed.days, 1095);
  assert.match(q.unsubscribed.clock, /unsubscribed_at/);
});

test("neither variable set: the single statement of before, counted from updated_at", async () => {
  const db = fakeDB({ changes: 2 });
  assert.equal(await purgeExpired({ NEWSLETTER_DB: db, NEWSLETTER_RETENTION_DAYS: "30" } as never, true), 2);
  assert.equal(db.calls.length, 1);
  assert.equal(db.calls[0].sql, "DELETE FROM subscribers WHERE status IN ('pending', 'unsubscribed') AND updated_at < ?");
});

test("pending: counted from the latest of sign-up, consent and confirmation mail", async () => {
  const d1 = seed({
    "p-old": { status: "pending", created: 20, updated: 20, sent: 20 },
    "p-never-mailed": { status: "pending", created: 20, updated: 20, sent: null },
    "p-mailed-lately": { status: "pending", created: 20, updated: 20, sent: 3 },
    // re-subscribed seconds ago, before markConfirmSent: the old mail must not count
    "p-resubscribed": { status: "pending", created: 400, updated: 0, consent: 0, sent: 300 },
    "c-old": { status: "confirmed", created: 900, updated: 900 },
    "u-old": { status: "unsubscribed", created: 900, updated: 900, unsub: 900 },
  });
  assert.equal(await purge(d1, { NEWSLETTER_PENDING_RETENTION_DAYS: "14" }), 2);
  assert.deepEqual(left(d1), ["c-old", "p-mailed-lately", "p-resubscribed", "u-old"]);
});

test("unsubscribed: counted from unsubscribed_at, not from an import's updated_at", async () => {
  const d1 = seed({
    "u-imported": { status: "unsubscribed", created: 2, updated: 2, unsub: 3000 },
    "u-recent": { status: "unsubscribed", created: 2000, updated: 10, unsub: 10 },
    "u-no-date": { status: "unsubscribed", created: 2000, updated: 1200, unsub: null },
    "p-old": { status: "pending", created: 2000, updated: 2000 },
    "c-old": { status: "confirmed", created: 3000, updated: 3000 },
  });
  assert.equal(await purge(d1, { NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: "1095" }), 2);
  assert.deepEqual(left(d1), ["c-old", "p-old", "u-recent"]);
});

test("one status on its own period, the other falls back to NEWSLETTER_RETENTION_DAYS", async () => {
  const rows: Record<string, Row> = {
    "p-old": { status: "pending", created: 40, updated: 40 },
    "u-imported": { status: "unsubscribed", created: 1, updated: 1, unsub: 3000 },
    "u-stale": { status: "unsubscribed", created: 400, updated: 400, unsub: 400 },
  };
  let d1 = seed(rows);
  assert.equal(await purge(d1, { NEWSLETTER_RETENTION_DAYS: "365", NEWSLETTER_PENDING_RETENTION_DAYS: "14" }), 2);
  assert.deepEqual(left(d1), ["u-imported"]); // the fallback still counts from updated_at

  d1 = seed(rows);
  assert.equal(await purge(d1, { NEWSLETTER_RETENTION_DAYS: "30", NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: "1095" }), 2);
  assert.deepEqual(left(d1), ["u-stale"]);
});

test("0 or a bad value keeps that status, even with NEWSLETTER_RETENTION_DAYS set", async () => {
  const rows: Record<string, Row> = {
    "p-old": { status: "pending", created: 900, updated: 900 },
    "u-old": { status: "unsubscribed", created: 900, updated: 900, unsub: 900 },
  };
  for (const keep of ["0", "-1", "forever"]) {
    let d1 = seed(rows);
    assert.equal(await purge(d1, { NEWSLETTER_RETENTION_DAYS: "30", NEWSLETTER_PENDING_RETENTION_DAYS: keep }), 1);
    assert.deepEqual(left(d1), ["p-old"]);
    d1 = seed(rows);
    assert.equal(await purge(d1, { NEWSLETTER_RETENTION_DAYS: "30", NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: keep }), 1);
    assert.deepEqual(left(d1), ["u-old"]);
    d1 = seed(rows);
    assert.equal(await purge(d1, { NEWSLETTER_PENDING_RETENTION_DAYS: keep, NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: keep }), 0);
    assert.equal(left(d1).length, 2);
  }
});

test("an absurd period deletes nothing and does not throw", async () => {
  const d1 = seed({ "u-old": { status: "unsubscribed", created: 900, updated: 900, unsub: 900 } });
  assert.equal(await purge(d1, { NEWSLETTER_UNSUBSCRIBED_RETENTION_DAYS: "99999999999" }), 0);
  assert.equal(left(d1).length, 1);
});

test("the per-status DELETEs stay on a status index, never a full table scan", () => {
  const d1 = withSchemaV1(sqliteD1());
  for (const where of [
    "status = 'pending' AND MAX(created_at, consent_at, COALESCE(confirm_sent_at, created_at)) < ?",
    "status = 'unsubscribed' AND COALESCE(unsubscribed_at, updated_at) < ?",
  ]) {
    const plan = d1.sqlite.prepare(`EXPLAIN QUERY PLAN DELETE FROM subscribers WHERE ${where}`).all().map((r) => String(r.detail)).join("\n");
    assert.match(plan, /USING (COVERING )?INDEX idx_subscribers_/, plan);
  }
});
