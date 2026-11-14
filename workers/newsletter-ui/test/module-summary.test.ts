// GET {base}/api/module/summary (GO-148): behind the same gates as every /api/
// route; the dashboard's numbers, warnings and configuration problems as a
// module summary — aggregates only, at most three statements.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";
import { LINKS, SERIES_DAYS, configProblems, newsletterUiSummary } from "../src/module-summary.ts";
import { counted, summaryOf } from "../src/summary-kit.ts";
import * as texts from "../src/summary-texts.ts";
import { readSummary } from "../../module-manifest.ts";
import { parse } from "../public/js/core/router.js";
import { PASSWORD, fakeCtx, fakeD1, makeEnv, req } from "./fakes.ts";
import { CONFIGURED, EXPECTED, PII, seededEnv } from "./summary-fixtures.ts";

const B = "/newsletter-admin";
const schema = JSON.parse(readFileSync(new URL("../../module-summary.schema.json", import.meta.url), "utf8"));
const run = (r: Request, env: Record<string, unknown>) => handleFetch(r, env as never, fakeCtx() as never);
const LANGS = ["en", "pl", "hi", "de", "fr"];

test("summary: with admin credentials, a valid summary with the panel's security headers", async () => {
  const env = await seededEnv();
  const res = await run(req(B + "/api/module/summary", { auth: PASSWORD }), env);
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(res.headers.get("x-robots-tag"), "noindex, nofollow");
  const r = readSummary(await res.json(), schema);
  assert.ok(r.ok, r.ok ? "" : r.errors.join("\n"));
  const s = r.summary;
  assert.deepEqual(s.metrics.map((m) => [m.key, m.value]), [
    ["confirmed", EXPECTED.confirmed],
    ["pending", EXPECTED.pending],
    ["campaignsInProgress", EXPECTED.inProgress],
    ["suppressed", EXPECTED.suppressed],
  ]);
  assert.deepEqual(s.problems, [], "a configured worker has no problems");
  assert.equal(s.series?.points.length, SERIES_DAYS);
  const values = s.series!.points.map((p) => p.value);
  assert.deepEqual(values.slice(-3), [1, 1, 3], "sign-ups per day, oldest first, ending today");
  assert.equal(s.series!.points.at(-1)!.day, new Date().toISOString().slice(0, 10));
});

test("summary: refused exactly as /api/health is — no credentials, wrong ones, no auth configured", async () => {
  const cases: [string, Record<string, unknown>, string | undefined][] = [
    ["no credentials", makeEnv(), undefined],
    ["wrong password", makeEnv(), "wrong"],
    ["no admin auth configured", makeEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined }), PASSWORD],
    ["Access configured, Basic sent", makeEnv({ NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud" }), PASSWORD],
  ];
  for (const [name, env, auth] of cases) {
    const health = await run(req(B + "/api/health", { auth }), env);
    const sum = await run(req(B + "/api/module/summary", { auth }), env);
    assert.notEqual(sum.status, 200, name);
    assert.equal(sum.status, health.status, name);
    assert.deepEqual([...sum.headers].sort(), [...health.headers].sort(), name);
    assert.equal(await sum.text(), await health.text(), name);
  }
  const post = await run(req(B + "/api/module/summary", { method: "POST", auth: PASSWORD }), makeEnv());
  assert.equal(post.status, 405);
});

test("summary: severity of each attention item on a seeded database, links open real panel views", async () => {
  const s = await newsletterUiSummary((await seededEnv()) as never);
  assert.deepEqual(
    s.attention.map((a) => [a.severity, a.href, a.text.en]),
    [
      ["action", LINKS.paused, "1 campaign was paused by the send engine and waits for you."],
      ["action", LINKS.dashboard, "3 messages may or may not have gone out. They are never re-sent without you."],
      ["action", LINKS.pending, "1 pending subscriber has never been sent a confirmation email."],
      ["warning", LINKS.dashboard, "2 send jobs ran out of retries. Their recipients are re-queued automatically after the stale timeout; if the number keeps growing, the transport keeps failing."],
    ],
  );
  assert.equal(s.attention[1].text.pl, "3 wiadomości mogły wyjść albo nie. Bez Twojej decyzji nigdy nie zostaną wysłane ponownie.");
  for (const href of Object.values(LINKS)) {
    const view = parse(href.slice(href.indexOf("#")));
    assert.notEqual(view.name, "notFound", href);
    assert.ok(href.startsWith("/#/"), "the panel is index.html at the base");
  }
  assert.equal(parse(LINKS.paused.slice(1)).query.status, "paused");
  assert.equal(parse(LINKS.pending.slice(1)).query.status, "pending");
  const empty = makeEnv(CONFIGURED);
  await ensureSchema(empty as never);
  const quiet = await newsletterUiSummary(empty as never);
  assert.deepEqual(quiet.attention, [], "nothing waits on an empty database");
});

test("summary: configuration problems come from the settings logic, never a secret", () => {
  const bare = makeEnv({ NEWSLETTER_QUEUE: undefined });
  assert.deepEqual(configProblems(bare as never, true).map((p) => p.code), ["no_transport", "no_site_url", "no_sender", "no_queue"]);
  const smtp = configProblems(makeEnv({ ...CONFIGURED, NEWSLETTER_RESEND_API_KEY: undefined, NEWSLETTER_SMTP_HOST: "smtp.example.com", NEWSLETTER_SMTP_USER: "u", NEWSLETTER_SMTP_PASSWORD: undefined }) as never, true);
  assert.deepEqual(smtp.map((p) => p.code), ["transport_unusable"]);
  assert.equal(smtp[0].text.en, "The configured mail transport can never send: smtp: a user is set but the password is not");
  assert.equal(smtp[0].text.de, "Der konfigurierte Mailversand kann nie senden: smtp: a user is set but the password is not");
  const secret = configProblems(makeEnv({ ...CONFIGURED, NEWSLETTER_SMTP_PASSWORD: "hunter2-secret" }) as never, false);
  assert.deepEqual(secret.map((p) => p.code), ["no_subscribers_table"]);
  assert.ok(!JSON.stringify(secret).includes("hunter2") && !JSON.stringify(secret).includes("re_test_key"));
});

test("summary: without the subscribers table — campaign numbers and the problem, no series", async () => {
  const env = makeEnv({ ...CONFIGURED, NEWSLETTER_DB: fakeD1() });
  const res = await run(req(B + "/api/module/summary", { auth: PASSWORD }), env);
  assert.equal(res.status, 200);
  const r = readSummary(await res.json(), schema);
  assert.ok(r.ok);
  assert.deepEqual(r.summary.metrics.map((m) => m.key), ["campaignsInProgress", "suppressed"]);
  assert.deepEqual(r.summary.problems.map((p) => p.code), ["no_subscribers_table"]);
  assert.equal(r.summary.series, undefined);
});

test("summary: no person's data — addresses, tokens, subjects, engine errors, dead-letter jobs stay out", async () => {
  const env = await seededEnv();
  const body = (await (await run(req(B + "/api/module/summary", { auth: PASSWORD }), env)).text()).toLowerCase();
  for (const marker of PII) assert.ok(!body.includes(marker), `the summary leaks ${marker}`);
});

test("summary: at most three statements on a warm isolate, also over 100 000 subscribers", async () => {
  const env = await seededEnv();
  await newsletterUiSummary(env as never); // the subscribers check is cached after the first answer
  env.NEWSLETTER_DB.sqlite.exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 100000)
    INSERT INTO subscribers (id, email, status, consent_text, consent_at, token, created_at, updated_at)
    SELECT 'bulk' || i, 'x' || i || '@example.com', 'confirmed', 'c', '2026-01-01', 't' || i, '2026-01-01', '2026-01-01' FROM n`);
  env.NEWSLETTER_DB.calls.length = 0;
  const s = await newsletterUiSummary(env as never);
  assert.ok(env.NEWSLETTER_DB.calls.length <= 3, env.NEWSLETTER_DB.calls.join("\n"));
  assert.equal(env.NEWSLETTER_DB.calls.length, 3);
  assert.equal(s.metrics[0].value, EXPECTED.confirmed + 100_000);
});

test("summary texts: the panel's words where the panel says it, five real languages everywhere", async () => {
  const strings = Object.fromEntries(await Promise.all(LANGS.map(async (l) => [l, (await import(`../public/js/strings/${l}.js`)).default])));
  const panel = (key: string) => Object.fromEntries(LANGS.map((l) => [l, strings[l][key]]));
  assert.deepEqual(texts.PANEL_LABELS, { confirmed: panel("tile.confirmed"), pending: panel("tile.pending"), suppressed: panel("tile.suppressed"), signups: panel("series.signups") });
  assert.deepEqual(texts.DEAD_LETTERS, panel("warn.deadLetters"));
  assert.deepEqual(texts.PANEL_PROBLEMS, { no_transport: panel("warn.noTransport"), no_site_url: panel("warn.noSiteUrl"), no_sender: panel("warn.noSender"), no_queue: panel("warn.noQueue") });
  for (const phrase of [texts.PAUSED_BY_ENGINE, texts.UNCERTAIN, texts.NEVER_MAILED]) {
    for (const n of [1, 2, 5, 22, 1000000]) {
      const t = counted(phrase, n);
      for (const l of LANGS) assert.ok(t[l as "en"].includes(new Intl.NumberFormat(l).format(n)) && !t[l as "en"].includes("{"), `${l} ${n}`);
    }
  }
  for (const t of [texts.IN_PROGRESS, ...Object.values(texts.OWN_PROBLEMS)]) for (const l of LANGS) assert.ok(t[l as "en"].trim());
});

test("summary kit (this template's copy): caps and order", () => {
  const t = { en: "t", pl: "t", hi: "t", de: "t", fr: "t" };
  const s = summaryOf({ metrics: Array.from({ length: 5 }, (_, i) => ({ key: `k${i}`, label: t, value: i })), problems: Array.from({ length: 11 }, () => ({ code: "c", text: t })) });
  assert.deepEqual([s.metrics.length, s.problems.length, s.attention.length, "series" in s], [4, 10, 0, false]);
  assert.deepEqual(summaryOf({ metrics: [] }).problems, []);
  assert.equal(counted({ en: { other: "{count}" }, pl: { other: "{count}" }, hi: { other: "{count}" }, de: { other: "{count}" }, fr: { other: "{count}" } }, 3).fr, "3");
});
