// GET /api/newsletter/module/summary (GO-148): behind the admin gate,
// subscribers by status and the configuration problems that refuse sign-ups —
// aggregates only, one statement however large the table.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as admin from "../functions/api/newsletter/admin.ts";
import * as route from "../functions/api/newsletter/module/summary.ts";
import { configProblems, newsletterSummary } from "../functions/api/newsletter/_summary.ts";
import { counted, summaryOf } from "../functions/api/newsletter/_summary-kit.ts";
import { readSummary } from "../../module-manifest.ts";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";
import { PII, seedPeople } from "./summary-seed.ts";

type Fn = (ctx: { request: Request; env: unknown }) => Promise<Response>;
const S = "/api/newsletter/module/summary";
const schema = JSON.parse(readFileSync(new URL("../../module-summary.schema.json", import.meta.url), "utf8"));
const PASSWORD = "admin-pass";
const basic = (p: string) => "Basic " + btoa(`admin:${p}`);
const env = (extra: Record<string, unknown> = {}) => ({ NEWSLETTER_DB: withSchemaV1(sqliteD1()), NEWSLETTER_ADMIN_PASSWORD: PASSWORD, NEWSLETTER_TURNSTILE_SECRET: "ts", ...extra });

async function call(fn: Fn, path: string, authorization: string | null, e: Record<string, unknown>) {
  const request = new Request(`https://example.com${path}`, { headers: authorization ? { authorization } : {} });
  const res = await fn({ request, env: e });
  return { status: res.status, headers: [...res.headers.entries()].sort(), body: await res.text() };
}

test("summary: with admin credentials it answers the counts by status, valid against the schema", async () => {
  const e = env();
  seedPeople(e.NEWSLETTER_DB.sqlite, "confirmed", 3);
  seedPeople(e.NEWSLETTER_DB.sqlite, "pending", 2);
  seedPeople(e.NEWSLETTER_DB.sqlite, "unsubscribed", 1);
  const got = await call(route.onRequestGet as unknown as Fn, S, basic(PASSWORD), e);
  assert.equal(got.status, 200);
  assert.deepEqual(got.headers, [["cache-control", "no-store"], ["content-type", "application/json"]]);
  const r = readSummary(JSON.parse(got.body), schema);
  assert.ok(r.ok, r.ok ? "" : r.errors.join("\n"));
  assert.deepEqual(r.summary.metrics.map((m) => [m.key, m.value]), [["confirmed", 3], ["pending", 2], ["unsubscribed", 1]]);
  assert.deepEqual([r.summary.attention, r.summary.problems], [[], []], "no panel, nothing to link to; nothing wrong");
  const counts = JSON.parse((await call(admin.onRequestGet as unknown as Fn, "/api/newsletter/admin", basic(PASSWORD), e)).body);
  assert.deepEqual(r.summary.metrics.map((m) => m.value), [counts.byStatus.confirmed, counts.byStatus.pending, counts.byStatus.unsubscribed], "the admin endpoint's numbers");
});

test("summary: every refusal is the admin endpoint's, byte for byte", async () => {
  const e = env();
  const cases: [Record<string, unknown>, string | null][] = [
    [{ NEWSLETTER_DB: e.NEWSLETTER_DB }, basic(PASSWORD)],
    [e, null],
    [e, basic("nope")],
    [e, "Basic %%%"],
    [{ ...e, NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud" }, basic(PASSWORD)],
  ];
  for (const [en, authorization] of cases) {
    const want = await call(admin.onRequestGet as unknown as Fn, "/api/newsletter/admin", authorization, en);
    assert.notEqual(want.status, 200);
    assert.deepEqual(await call(route.onRequestGet as unknown as Fn, S, authorization, en), want);
  }
});

test("summary: configuration problems — no database, no Turnstile secret, unusable mail, half of Access", async () => {
  const bare = await newsletterSummary({ NEWSLETTER_ACCESS_AUD: "aud-value" } as never);
  assert.deepEqual(bare.problems.map((p) => p.code), ["no_database", "no_turnstile_secret", "access_incomplete"]);
  assert.deepEqual(bare.metrics, []);
  assert.ok(!JSON.stringify(bare).includes("aud-value"));
  const smtp = configProblems(env({ NEWSLETTER_MAIL_FROM: "news@example.com", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25", NEWSLETTER_SMTP_PASSWORD: "hunter2-secret" }) as never);
  assert.deepEqual(smtp.map((p) => p.code), ["mail_unusable"]);
  assert.match(smtp[0].text.en, /^Double opt-in is configured, but the mail transport can never send \(smtp: port 25 is blocked/);
  assert.match(smtp[0].text.pl, /\(smtp: port 25/);
  assert.ok(!JSON.stringify(smtp).includes("hunter2-secret"));
  assert.deepEqual(configProblems(env({ NEWSLETTER_TURNSTILE_SECRET: undefined, TURNSTILE_SECRET: "shared", NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_PORT: "25" }) as never), [], "no sender: single opt-in, nothing refused");
  assert.ok(readSummary(bare, schema).ok);
});

test("summary: no person's data — addresses, tokens, consent texts, hashes stay out", async () => {
  const e = env();
  for (const s of ["confirmed", "pending", "unsubscribed"]) seedPeople(e.NEWSLETTER_DB.sqlite, s, 2);
  const body = (await call(route.onRequestGet as unknown as Fn, S, basic(PASSWORD), e)).body.toLowerCase();
  for (const marker of PII) assert.ok(!body.includes(marker.toLowerCase()), `the summary leaks ${marker}`);
});

test("summary: one statement on a warm isolate, also over 100 000 subscribers", async () => {
  const e = env();
  await newsletterSummary(e as never); // the isolate's schema check runs once
  e.NEWSLETTER_DB.sqlite.exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 100000)
    INSERT INTO subscribers (id, email, status, consent_text, consent_at, token, created_at, updated_at)
    SELECT 's' || i, 'x' || i || '@example.com', CASE i % 4 WHEN 0 THEN 'pending' ELSE 'confirmed' END, 'c', '2026-01-01', 't' || i, '2026-01-01', '2026-01-01' FROM n`);
  const seen: string[] = [];
  const db = { ...e.NEWSLETTER_DB, prepare: (sql: string) => (seen.push(sql), e.NEWSLETTER_DB.prepare(sql)) };
  const s = await newsletterSummary({ ...e, NEWSLETTER_DB: db } as never);
  assert.equal(seen.length, 1);
  assert.deepEqual(s.metrics.map((m) => m.value), [75000, 25000, 0]);
});

test("summary kit (this template's copy): plural forms, severity order, caps, series", () => {
  const forms = { en: { one: "{count} x", other: "{count} xs" }, pl: { one: "{count} a", few: "{count} b", other: "{count} c" }, hi: { other: "{count} h" }, de: { other: "{count} d" }, fr: { other: "{count} f" } };
  assert.deepEqual(counted(forms, 1), { en: "1 x", pl: "1 a", hi: "1 h", de: "1 d", fr: "1 f" });
  const t = { en: "t", pl: "t", hi: "t", de: "t", fr: "t" };
  const s = summaryOf({
    metrics: [],
    attention: [{ severity: "info", text: t, href: "/a" }, { severity: "action", text: t, href: "/b" }],
    problems: [],
    series: { label: t, points: Array.from({ length: 91 }, (_, i) => ({ day: `d${i}`, value: i })) },
  });
  assert.deepEqual(s.attention.map((a) => a.href), ["/b", "/a"]);
  assert.equal(s.series?.points.length, 90);
  assert.deepEqual(summaryOf({ metrics: [] }).problems, []);
});
