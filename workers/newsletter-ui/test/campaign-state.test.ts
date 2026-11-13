// The state machine: every allowed and every forbidden pair, generated from
// the table, against a real database; and a race only one caller wins.
import { test } from "node:test";
import assert from "node:assert/strict";
import { STATUSES, TRANSITIONS, canTransition, isStatus, transition } from "../src/campaign-state.ts";
import { ensureSchema } from "../src/schema.ts";
import { fakeD1 } from "./fakes.ts";

async function envWith(status: string) {
  const d1 = fakeD1();
  const env = { NEWSLETTER_DB: d1 } as never;
  await ensureSchema(env);
  d1.sqlite
    .prepare("INSERT INTO campaigns (id, subject, status, started_at, created_at, updated_at) VALUES ('c1', 's', ?, ?, '2026-01-01', '2026-01-01')")
    .run(status, status === "paused" ? "2026-01-02T00:00:00.000Z" : null);
  const row = () => d1.sqlite.prepare("SELECT * FROM campaigns WHERE id = 'c1'").get() as Record<string, string | null>;
  return { env, row };
}

test("the table: final states have no way out, nothing moves to itself", () => {
  assert.deepEqual(TRANSITIONS.sent, []);
  assert.deepEqual(TRANSITIONS.cancelled, []);
  for (const s of STATUSES) assert.ok(!TRANSITIONS[s].includes(s), s);
  assert.ok(isStatus("draft") && !isStatus("bogus") && !isStatus(1));
  assert.equal(canTransition("bogus", "draft"), false);
  assert.equal(canTransition("draft", "bogus"), false);
});

for (const from of STATUSES) {
  for (const to of STATUSES) {
    const allowed = TRANSITIONS[from].includes(to);
    test(`${from} → ${to}: ${allowed ? "allowed" : "refused"}`, async () => {
      const { env, row } = await envWith(from);
      assert.equal(canTransition(from, to), allowed);
      assert.equal(await transition(env, "c1", from, to, "2030-01-01T00:00:00.000Z"), allowed);
      const r = row();
      assert.equal(r.status, allowed ? to : from);
      if (!allowed) return;
      if (to === "scheduled") assert.equal(r.scheduled_at, "2030-01-01T00:00:00.000Z");
      if (to === "draft") assert.equal(r.scheduled_at, null);
      if (to === "sending") assert.ok(r.started_at, "started_at stamped");
      if (to === "sending" && from === "paused") assert.equal(r.started_at, "2026-01-02T00:00:00.000Z", "first start kept");
      if (to === "sent" || to === "cancelled") assert.ok(r.finished_at);
      assert.notEqual(r.updated_at, "2026-01-01");
    });
  }
}

test("a stale `from` loses; scheduled_at defaults to now", async () => {
  const { env, row } = await envWith("draft");
  assert.equal(await transition(env, "c1", "scheduled", "sending"), false);
  assert.equal(row().status, "draft");
  assert.equal(await transition(env, "c1", "draft", "scheduled"), true);
  assert.ok(Date.now() - Date.parse(row().scheduled_at as string) < 5000);
  assert.equal(await transition(env, "missing", "scheduled", "draft"), false);
});

test("concurrent transitions: exactly one wins", async () => {
  const { env, row } = await envWith("scheduled");
  const results = await Promise.all(Array.from({ length: 8 }, () => transition(env, "c1", "scheduled", "sending")));
  assert.equal(results.filter(Boolean).length, 1);
  assert.equal(row().status, "sending");
  // Two different moves from the same state: still one winner.
  const { env: env2, row: row2 } = await envWith("sending");
  const [pause, cancel] = await Promise.all([transition(env2, "c1", "sending", "paused"), transition(env2, "c1", "sending", "cancelled")]);
  assert.equal(Number(pause) + Number(cancel), 1);
  assert.equal(row2().status, pause ? "paused" : "cancelled");
});
