// Campaign statistics (GO-129) on a seeded database, against numbers worked
// out by hand; the unsubscribe attribution window at its edges; the shape of
// the click table.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { BOUNCES } from "../src/stats-bounces.ts";
import { addCampaign, engineEnv } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";
import { fakeCtx, req } from "./fakes.ts";
import { api } from "./subscriber-seed.ts";

// person inserts subscriber `id`, unsubscribed at `left` (or still confirmed).
function person(env: EngineEnv, id: string, left: string | null = null) {
  env.NEWSLETTER_DB.sqlite
    .prepare(
      `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at, unsubscribed_at)
       VALUES (?, ?, ?, 'en', '', 'yes', '2026-01-01T00:00:00.000Z', ?, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', ?)`,
    )
    .run(id, `${id}@example.com`, left ? "unsubscribed" : "confirmed", `tok-${id}`.padEnd(43, "x"), left);
}

function send(env: EngineEnv, campaign: string, subscriber: string, status: string, sentAt: string | null = null) {
  env.NEWSLETTER_DB.sqlite
    .prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at, sent_at) VALUES (?, ?, ?, 1, '2026-03-01T10:00:00.000Z', ?)")
    .run(campaign, subscriber, status, sentAt);
}

const link = (env: EngineEnv, campaign: string, id: string, url: string, clicks: number) =>
  env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO click_links (campaign_id, link_id, url, clicks, created_at) VALUES (?, ?, ?, ?, '2026-03-01')").run(campaign, id, url, clicks);

// The world, worked out by hand (window 14 days unless said otherwise):
//   A sent 2026-03-01 10:00–10:30, B started 2026-03-10 and still sending,
//   D sent 2026-02-01, C a draft.
//   s1  A 03-01, B 03-10, left 03-11            → B (the later send wins)
//   s2  A 03-01, B 03-10 (after leaving 03-05)  → A (B came after the unsubscribe)
//   s3  A 03-01 10:03, left 19 days later       → nobody (outside 14; A within 30)
//   s9  A 03-01 10:04, left exactly 14 days on  → A (the edge is inclusive)
//   s10 A 03-01 10:05, left 14 days + 1 ms      → nobody (A within 30)
//   s11 A 03-01 10:06, left 03-01 09:00         → nobody (left before the send)
//   s8  never sent, left 03-02                  → nobody
async function world() {
  const env = await engineEnv(0);
  const db = env.NEWSLETTER_DB;
  const A = addCampaign(db, { id: "A", subject: "March", status: "sent", started_at: "2026-03-01T10:00:00.000Z", finished_at: "2026-03-01T10:30:00.000Z" });
  addCampaign(db, { id: "B", subject: "Mid-March", status: "sending", started_at: "2026-03-10T10:00:00.000Z" });
  addCampaign(db, { id: "C", subject: "Draft", status: "draft", scheduled_at: null });
  addCampaign(db, { id: "D", subject: "February", status: "sent", started_at: "2026-02-01T10:00:00.000Z", finished_at: "2026-02-01T10:00:59.600Z" });
  db.sqlite.prepare("UPDATE campaigns SET total_count = CASE id WHEN 'A' THEN 9 WHEN 'B' THEN 3 WHEN 'D' THEN 1 ELSE 0 END").run();
  person(env, "s1", "2026-03-11T00:00:00.000Z");
  person(env, "s2", "2026-03-05T00:00:00.000Z");
  person(env, "s3", "2026-03-20T10:03:00.000Z");
  for (const id of ["s4", "s5", "s6", "s7"]) person(env, id);
  person(env, "s8", "2026-03-02T00:00:00.000Z");
  person(env, "s9", "2026-03-15T10:04:00.000Z");
  person(env, "s10", "2026-03-15T10:05:00.001Z");
  person(env, "s11", "2026-03-01T09:00:00.000Z");
  send(env, A, "s1", "sent", "2026-03-01T10:01:00.000Z");
  send(env, A, "s2", "sent", "2026-03-01T10:02:00.000Z");
  send(env, A, "s3", "sent", "2026-03-01T10:03:00.000Z");
  send(env, A, "s4", "failed");
  send(env, A, "s5", "skipped");
  send(env, A, "s6", "retry");
  send(env, A, "s9", "sent", "2026-03-01T10:04:00.000Z");
  send(env, A, "s10", "sent", "2026-03-01T10:05:00.000Z");
  send(env, A, "s11", "sent", "2026-03-01T10:06:00.000Z");
  send(env, "B", "s1", "sent", "2026-03-10T10:05:00.000Z");
  send(env, "B", "s2", "sent", "2026-03-10T10:06:00.000Z");
  send(env, "B", "s7", "claimed");
  send(env, "D", "s4", "sent", "2026-02-01T10:00:30.000Z");
  link(env, A, "1111111111111111", "https://example.com/b", 2);
  link(env, A, "2222222222222222", "https://example.com/a", 5);
  link(env, "D", "3333333333333333", "https://example.com/d", 1);
  return env;
}

const get = async (env: EngineEnv, path: string) => {
  const res = await api(env, "GET", path);
  return { status: res.status, body: await res.json() };
};

test("one campaign: the numbers worked out by hand", async () => {
  const env = await world();
  const { status, body } = await get(env, "/api/campaigns/A/stats");
  assert.equal(status, 200);
  assert.deepEqual(body, {
    id: "A",
    subject: "March",
    status: "sent",
    recipients: { total: 9, sent: 6, failed: 1, skipped: 1, pending: 1, uncertain: 0 },
    bounces: BOUNCES,
    unsubscribes: { count: 2, windowDays: 14, attribution: "time-window" },
    clicks: {
      tracking: false,
      total: 7,
      links: [
        { linkId: "2222222222222222", url: "https://example.com/a", clicks: 5 },
        { linkId: "1111111111111111", url: "https://example.com/b", clicks: 2 },
      ],
    },
    timing: { scheduledAt: "2026-01-01T00:00:00.000Z", startedAt: "2026-03-01T10:00:00.000Z", finishedAt: "2026-03-01T10:30:00.000Z", durationSeconds: 1800 },
  });
  assert.doesNotMatch(JSON.stringify(body), /@example|"s\d+"|tok-/, "no person in the answer");
  assert.equal(body.bounces.hard, null, "not credited to a campaign without the data: null, with the reason");

  const b = (await get(env, "/api/campaigns/B/stats")).body;
  assert.deepEqual(b.recipients, { total: 3, sent: 2, failed: 0, skipped: 0, pending: 1, uncertain: 0 });
  assert.equal(b.unsubscribes.count, 1, "s1: the later of two campaigns gets it");
  assert.deepEqual(b.clicks, { tracking: false, total: 0, links: [] });
  assert.equal(b.timing.durationSeconds, null, "still sending");

  const draft = (await get(env, "/api/campaigns/C/stats")).body;
  assert.deepEqual(draft.recipients, { total: 0, sent: 0, failed: 0, skipped: 0, pending: 0, uncertain: 0 });
  assert.equal(draft.unsubscribes.count, 0);
  assert.equal(draft.timing.startedAt, null);

  assert.equal((await get(env, "/api/campaigns/nope/stats")).status, 404);
});

test("unsubscribe window: the variable moves the edge; tracking shows as on", async () => {
  const env = await world();
  env.NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS = "30";
  env.NEWSLETTER_TRACK_CLICKS = "on";
  const a = (await get(env, "/api/campaigns/A/stats")).body;
  assert.deepEqual(a.unsubscribes, { count: 4, windowDays: 30, attribution: "time-window" }, "s2, s9 + s3 and s10");
  assert.equal(a.clicks.tracking, true);
  env.NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS = "1";
  assert.equal((await get(env, "/api/campaigns/A/stats")).body.unsubscribes.count, 0, "s2 left 4 days after A");
  assert.equal((await get(env, "/api/campaigns/B/stats")).body.unsubscribes.count, 1, "s1 left 14 hours after B");
  env.NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS = "nonsense";
  assert.equal((await get(env, "/api/campaigns/A/stats")).body.unsubscribes.windowDays, 14, "default");
});

test("campaign list: newest start first, headline numbers, cursor, drafts left out", async () => {
  const env = await world();
  const all = (await get(env, "/api/stats/campaigns")).body;
  assert.equal(all.windowDays, 14);
  assert.equal(all.nextCursor, null);
  assert.deepEqual(
    all.items.map((i: Record<string, unknown>) => [i.id, i.total, i.sent, i.failed, i.skipped, i.pending, i.unsubscribes, i.clicks, i.durationSeconds]),
    [
      ["B", 3, 2, 0, 0, 1, 1, 0, null],
      ["A", 9, 6, 1, 1, 1, 2, 7, 1800],
      ["D", 1, 1, 0, 0, 0, 0, 1, 60],
    ],
  );
  assert.deepEqual(Object.keys(all.items[0]).sort(), [
    "clicks", "durationSeconds", "failed", "finishedAt", "id", "pending", "sent", "skipped", "startedAt", "status", "subject", "total", "uncertain", "unsubscribes",
  ]);
  const p1 = (await get(env, "/api/stats/campaigns?limit=2")).body;
  assert.deepEqual(p1.items.map((i: { id: string }) => i.id), ["B", "A"]);
  assert.equal(p1.nextCursor, "2026-03-01T10:00:00.000Z~A");
  const p2 = (await get(env, "/api/stats/campaigns?limit=2&cursor=" + encodeURIComponent(p1.nextCursor))).body;
  assert.deepEqual(p2.items.map((i: { id: string }) => i.id), ["D"]);
  assert.equal(p2.nextCursor, null);
  for (const bad of ["0", "51", "x", "1.5", ""]) {
    const r = await get(env, "/api/stats/campaigns?limit=" + bad);
    assert.equal(r.status, 400, bad);
    assert.equal(r.body.error, "invalid_limit");
  }
  for (const bad of ["x", "~A", "2026-03-01T10:00:00.000Z~", "not-a-date~A"]) {
    const r = await get(env, "/api/stats/campaigns?cursor=" + encodeURIComponent(bad));
    assert.equal(r.status, 400, bad);
    assert.equal(r.body.error, "invalid_cursor");
  }
  const empty = (await get(await engineEnv(0), "/api/stats/campaigns")).body;
  assert.deepEqual(empty, { items: [], nextCursor: null, windowDays: 14 });
});

test("the stats routes need admin auth; the click redirect does not", async () => {
  const env = await world();
  for (const path of ["/api/campaigns/A/stats", "/api/stats/campaigns"]) {
    const none = await handleFetch(req("/newsletter-admin" + path), env as never, fakeCtx() as never);
    assert.equal(none.status, 401, path);
    const wrong = await handleFetch(req("/newsletter-admin" + path, { auth: "wrong", ip: "198.51.100.9" }), env as never, fakeCtx() as never);
    assert.equal(wrong.status, 401, path);
  }
  const click = await handleFetch(req("/newsletter-admin/hooks/c/A/2222222222222222"), env as never, fakeCtx() as never);
  assert.equal(click.status, 302);
  assert.equal(click.headers.get("location"), "https://example.com/a");
});

test("schema: no column of the click table can point at a person", async () => {
  const env = await engineEnv(0);
  const columns = env.NEWSLETTER_DB.sqlite.prepare("PRAGMA table_info(click_links)").all().map((c) => String(c.name));
  assert.deepEqual(columns, ["campaign_id", "link_id", "url", "clicks", "created_at"]);
  const person = /subscriber|email|mail|token|(^|_)ip(_|$)|(^|_)ua(_|$)|agent|address|person|user|hash/;
  for (const c of columns) assert.doesNotMatch(c, person, c);
  const tables = env.NEWSLETTER_DB.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'click%'").all();
  assert.deepEqual(tables.map((t) => t.name), ["click_links"], "the only click table");
});
