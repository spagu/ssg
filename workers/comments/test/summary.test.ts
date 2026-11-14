// GET /api/comments/module/summary (GO-148): behind the moderation gate,
// counts by status, the queue as something to do, configuration problems —
// aggregates only, from one statement however large the table.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as admin from "../functions/api/comments/admin.ts";
import * as route from "../functions/api/comments/module/summary.ts";
import { PANEL, STALE_DAYS, commentsSummary, configProblems } from "../functions/api/comments/_summary.ts";
import { SUMMARY_LIMITS, counted, summaryOf } from "../functions/api/comments/_summary-kit.ts";
import { readSummary } from "../../module-manifest.ts";
import { AUD, JSON_HEADERS, PASSWORD, TEAM, accessIssuer, basic, call, seed, sqliteD1 } from "./harness.ts";

const S = "/api/comments/module/summary";
const schema = JSON.parse(readFileSync(new URL("../../module-summary.schema.json", import.meta.url), "utf8"));
const auth = { authorization: basic(PASSWORD) };
const DAY = 86_400_000;
const ago = (days: number) => new Date(Date.now() - days * DAY).toISOString();
const env = (extra: Record<string, unknown> = {}) => ({ COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD, COMMENTS_TURNSTILE_SECRET: "ts", ...extra });

// counting wraps a database so every statement it is asked to prepare is counted.
function counting(db: ReturnType<typeof sqliteD1>) {
  const seen: string[] = [];
  return { seen, db: { ...db, prepare: (sql: string) => (seen.push(sql), db.prepare(sql)) } };
}

test("summary: with the moderation password it answers a valid summary", async () => {
  const e = env();
  seed(e.COMMENTS_DB, [{ id: "p1", status: "pending", created_at: ago(1) }]);
  const got = await call(route.onRequestGet, { path: S, headers: auth, env: e });
  assert.equal(got.status, 200);
  assert.deepEqual(got.headers, JSON_HEADERS);
  const r = readSummary(JSON.parse(got.body), schema);
  assert.ok(r.ok, r.ok ? "" : r.errors.join("\n"));
  assert.deepEqual(r.summary.metrics.map((m) => [m.key, m.value]), [["pending", 1], ["approved", 0], ["spam", 0]]);
});

test("summary: Basic mode refusals are the admin endpoint's, byte for byte", async () => {
  const e = env();
  const cases: [Record<string, unknown>, Record<string, string>][] = [
    [{ COMMENTS_DB: sqliteD1() }, auth],
    [e, {}],
    [e, { authorization: basic("nope") }],
    [e, { authorization: "Basic %%%" }],
    [e, { authorization: `Bearer ${PASSWORD}` }],
  ];
  for (const [en, headers] of cases) {
    const want = await call(admin.onRequestGet, { path: "/api/comments/admin", headers, env: en });
    assert.notEqual(want.status, 200);
    assert.deepEqual(await call(route.onRequestGet, { path: S, headers, env: en }), want);
  }
});

test("summary: Access mode — a valid token gets it, others are refused as by the admin endpoint", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const e = env({ COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD });
    const ok = await call(route.onRequestGet, { path: S, headers: { "cf-access-jwt-assertion": await issuer.token() }, env: e });
    assert.equal(ok.status, 200);
    for (const headers of [{}, auth, { "cf-access-jwt-assertion": await issuer.token({ aud: ["other"] }) }] as Record<string, string>[]) {
      const want = await call(admin.onRequestGet, { path: "/api/comments/admin", headers, env: e });
      assert.deepEqual(await call(route.onRequestGet, { path: S, headers, env: e }), want);
    }
  } finally {
    restore();
  }
});

test("summary: severity — a queue is an action, a queue older than STALE_DAYS also a warning", async () => {
  const fresh = env();
  seed(fresh.COMMENTS_DB, [
    { id: "p1", status: "pending", created_at: ago(STALE_DAYS - 1) },
    { id: "p2", status: "pending", created_at: ago(0) },
    { id: "a1", status: "approved", created_at: ago(30) },
    { id: "s1", status: "spam", created_at: ago(30) },
  ]);
  const a = await commentsSummary(fresh as never);
  assert.deepEqual(a.attention.map((x) => [x.severity, x.href, x.text.en]), [["action", PANEL, "2 comments await moderation."]]);
  assert.equal(a.attention[0].text.pl, "2 komentarze czekają na moderację.");

  const stale = env();
  seed(stale.COMMENTS_DB, [{ id: "p1", status: "pending", created_at: ago(STALE_DAYS + 3) }]);
  const b = await commentsSummary(stale as never);
  assert.deepEqual(b.attention.map((x) => [x.severity, x.text.en]), [
    ["action", "1 comment awaits moderation."],
    ["warning", `The oldest comment in the queue has been waiting for ${STALE_DAYS + 3} days.`],
  ]);
  assert.equal(b.attention[1].text.pl, `Najstarszy komentarz w kolejce czeka od ${STALE_DAYS + 3} dni.`);

  const quiet = env();
  seed(quiet.COMMENTS_DB, [{ id: "a1", status: "approved", created_at: ago(100) }]);
  assert.deepEqual((await commentsSummary(quiet as never)).attention, []);
  const odd = env();
  seed(odd.COMMENTS_DB, [{ id: "p1", status: "pending", created_at: "not a date" }]);
  assert.deepEqual((await commentsSummary(odd as never)).attention.map((x) => x.severity), ["action"], "an unreadable date is no warning");
});

test("summary: configuration problems are named, never a value", async () => {
  assert.deepEqual(configProblems(env() as never), []);
  const all = await commentsSummary({ COMMENTS_ACCESS_TEAM: "secret-team-value" } as never);
  assert.deepEqual(all.problems.map((p) => p.code), ["no_database", "no_turnstile_secret", "access_incomplete"]);
  assert.deepEqual(all.metrics, []);
  assert.ok(!JSON.stringify(all).includes("secret-team-value"));
  assert.deepEqual(configProblems(env({ COMMENTS_TURNSTILE_SECRET: undefined, TURNSTILE_SECRET: "shared", COMMENTS_ACCESS_AUD: "a" }) as never).map((p) => p.code), ["access_incomplete"]);
  assert.ok(readSummary(all, schema).ok);
});

test("summary: no person's data — authors, addresses, bodies, IP hashes, user agents stay out", async () => {
  const e = env();
  seed(e.COMMENTS_DB, [{ id: "p1", status: "pending", created_at: ago(20) }]);
  e.COMMENTS_DB.sqlite
    .prepare("INSERT INTO comments (id, url, author, email, body, status, created_at, ip_hash, user_agent, avatar_hash, parent_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
    .run("pii-id-1", "/pii-page/", "Alice Piimarker", "pii-alice@example.com", "PII-BODY-MARKER", "pending", ago(30), "PII-IPHASH", "PII-UA-MARKER", "PII-AVATAR", "pii-parent");
  const body = (await call(route.onRequestGet, { path: S, headers: auth, env: e })).body;
  for (const marker of ["pii", "Piimarker", "PII-", "@", "/blog/post/", "Author p1", "Body p1", "p1@example.com"]) {
    assert.ok(!body.toLowerCase().includes(marker.toLowerCase()), `the summary leaks ${marker}`);
  }
});

test("summary: one statement on a warm isolate, also over 100 000 comments", async () => {
  const e = env();
  await commentsSummary(e as never); // the isolate's schema check runs once
  e.COMMENTS_DB.sqlite.exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 100000)
    INSERT INTO comments (id, url, author, body, status, created_at)
    SELECT 'c' || i, '/p/', 'a', 'b', CASE i % 10 WHEN 0 THEN 'pending' WHEN 1 THEN 'spam' ELSE 'approved' END, '2026-01-01T00:00:00.000Z' FROM n`);
  const { seen, db } = counting(e.COMMENTS_DB);
  const s = await commentsSummary({ ...e, COMMENTS_DB: db } as never);
  assert.ok(seen.length <= 3, `${seen.length} statements`);
  assert.equal(seen.length, 1);
  assert.deepEqual(s.metrics.map((m) => m.value), [10000, 80000, 10000]);
  assert.equal(s.attention[1].severity, "warning");
});

test("summary kit (this template's copy): plural forms, severity order, caps, series", () => {
  const forms = { en: { one: "{count} x", other: "{count} xs" }, pl: { one: "{count} a", few: "{count} b", other: "{count} c" }, hi: { other: "{count} h" }, de: { other: "{count} d" }, fr: { other: "{count} f" } };
  assert.deepEqual(counted(forms, 5), { en: "5 xs", pl: "5 c", hi: "5 h", de: "5 d", fr: "5 f" }, "pl many falls back to other");
  assert.equal(counted(forms, 22).pl, "22 b");
  assert.equal(counted(forms, 12345).de, "12.345 d");
  const t = { en: "t", pl: "t", hi: "t", de: "t", fr: "t" };
  const item = (severity: "action" | "warning" | "info", n: number) => ({ severity, text: { ...t, en: String(n) }, href: "/" });
  const s = summaryOf(
    {
      metrics: Array.from({ length: 6 }, (_, i) => ({ key: `m${i}`, label: t, value: i })),
      attention: [item("info", 1), item("warning", 2), item("action", 3), item("info", 4), ...Array.from({ length: 10 }, (_, i) => item("info", 10 + i))],
      problems: Array.from({ length: 12 }, (_, i) => ({ code: `p${i}`, text: t })),
      series: { label: t, points: Array.from({ length: 100 }, (_, i) => ({ day: `d${i}`, value: i })) },
    },
    new Date("2026-11-13T10:00:00.000Z"),
  );
  assert.equal(s.generatedAt, "2026-11-13T10:00:00.000Z");
  assert.equal(s.metrics.length, SUMMARY_LIMITS.metrics);
  assert.deepEqual(s.attention.slice(0, 4).map((a) => a.text.en), ["3", "2", "1", "4"]);
  assert.equal(s.attention.length, SUMMARY_LIMITS.attention);
  assert.equal(s.problems.length, SUMMARY_LIMITS.problems);
  assert.equal(s.series?.points.length, SUMMARY_LIMITS.points);
  assert.equal(s.series?.points[0].day, "d10", "the newest days are kept");
  assert.deepEqual(summaryOf({ metrics: [] }).problems, []);
});
