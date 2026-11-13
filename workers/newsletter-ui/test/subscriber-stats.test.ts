// Aggregates on a seeded database: totals and the per-day series.
import { test } from "node:test";
import assert from "node:assert/strict";
import { api, readyEnv, seed } from "./subscriber-seed.ts";

const dayAgo = (n: number, hour = 12) => {
  const d = new Date();
  d.setUTCHours(hour, 0, 0, 0);
  return new Date(d.getTime() - n * 86_400_000).toISOString();
};

test("stats: totals by status and language, per-day sign-ups and unsubscribes", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [
    { email: "a@example.com", created_at: dayAgo(0, 1), language: "en" },
    { email: "b@example.com", created_at: dayAgo(0, 2), language: "pl", status: "pending" },
    { email: "c@example.com", created_at: dayAgo(5), language: null, status: "unsubscribed", unsubscribed_at: dayAgo(0, 3) },
    { email: "d@example.com", created_at: dayAgo(29), language: "en", status: "unsubscribed", unsubscribed_at: dayAgo(5) },
    { email: "e@example.com", created_at: dayAgo(30), language: "en" }, // outside 30 days, inside 90
    { email: "f@example.com", created_at: dayAgo(400), language: "en" }, // outside every window
  ]);
  const res = await api(env, "GET", "/api/stats/subscribers");
  assert.equal(res.status, 200);
  const s = await res.json();
  assert.equal(s.days, 30);
  assert.equal(s.total, 6);
  assert.deepEqual(s.byStatus, { pending: 1, confirmed: 3, unsubscribed: 2 });
  assert.deepEqual(s.byLanguage, {
    en: { pending: 0, confirmed: 3, unsubscribed: 1 },
    pl: { pending: 1, confirmed: 0, unsubscribed: 0 },
    unknown: { pending: 0, confirmed: 0, unsubscribed: 1 },
  });
  assert.equal(s.series.length, 30);
  assert.equal(s.series[29].day, dayAgo(0).slice(0, 10), "ends today");
  assert.equal(s.series[0].day, dayAgo(29).slice(0, 10), "starts days-1 ago");
  assert.deepEqual(s.series[29], { day: dayAgo(0).slice(0, 10), signups: 2, unsubscribes: 1 });
  assert.deepEqual(s.series[24], { day: dayAgo(5).slice(0, 10), signups: 1, unsubscribes: 1 });
  assert.deepEqual(s.series[0], { day: dayAgo(29).slice(0, 10), signups: 1, unsubscribes: 0 });
  const sum = (k: string) => s.series.reduce((n: number, d: Record<string, number>) => n + d[k], 0);
  assert.equal(sum("signups"), 4);
  assert.equal(sum("unsubscribes"), 2);
  assert.doesNotMatch(JSON.stringify(s), /@example/, "no person-level data");

  const ninety = await (await api(env, "GET", "/api/stats/subscribers?days=90")).json();
  assert.equal(ninety.series.length, 90);
  assert.equal(ninety.series.reduce((n: number, d: { signups: number }) => n + d.signups, 0), 5);
  assert.equal((await (await api(env, "GET", "/api/stats/subscribers?days=365")).json()).series.length, 365);
  for (const d of ["7", "x", "30.5", ""]) {
    const bad = await api(env, "GET", "/api/stats/subscribers?days=" + d);
    assert.equal(bad.status, 400, d);
    assert.equal((await bad.json()).error, "invalid_days");
  }
});

test("stats: an empty table and a status the template may add later", async () => {
  const env = await readyEnv();
  const empty = await (await api(env, "GET", "/api/stats/subscribers")).json();
  assert.equal(empty.total, 0);
  assert.deepEqual(empty.byLanguage, {});
  assert.ok(empty.series.every((d: { signups: number }) => d.signups === 0));
  seed(env.NEWSLETTER_DB, [{ email: "x@example.com", status: "bounced" }]);
  const odd = await (await api(env, "GET", "/api/stats/subscribers")).json();
  assert.equal(odd.byStatus.bounced, 1);
  assert.equal(odd.byLanguage.en.bounced, 1);
});
