// Backward-compatibility suite, newsletter-ui side: this worker runs on a
// database the `newsletter` template created before 1.9.8 and never changes
// the `subscribers` table's definition. The table below is the 1.9.7
// template's `_schema.ts`, statement for statement. Every code path that
// touches the database runs against it — the schema setup, every route in
// ROUTES at least once, the cron, a whole campaign through the queue, an
// import, a webhook, an erasure — and after each one the table, its indexes
// and its columns must be exactly what they were. Rows may change (that is the
// worker's job); the definition may not, and no statement may ALTER, CREATE,
// DROP or index it.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { handleFetch } from "../src/app.ts";
import { handleScheduled } from "../src/jobs.ts";
import { ROUTES } from "../src/routes.ts";
import type { Route } from "../src/router.ts";
import { ensureSchema } from "../src/schema.ts";
import { addCampaign, fakeMailer, pump } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";
import { PASSWORD, fakeCtx, fakeD1, makeEnv, req } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";
import { RESEND_SECRET, resendEvent, svixHeaders } from "./hook-fixtures.ts";

// MAIN_V1: workers/newsletter/functions/api/newsletter/_schema.ts as released
// in 1.9.7 (main c867868), verbatim — including its whitespace, which SQLite
// keeps in sqlite_master.
const MAIN_V1 = [
  `CREATE TABLE IF NOT EXISTS subscribers (
     id              TEXT PRIMARY KEY,
     email           TEXT NOT NULL UNIQUE,
     status          TEXT NOT NULL,
     language        TEXT,
     source_page     TEXT,
     tags            TEXT NOT NULL DEFAULT '',
     consent_text    TEXT NOT NULL,
     consent_at      TEXT NOT NULL,
     token           TEXT NOT NULL UNIQUE,
     created_at      TEXT NOT NULL,
     updated_at      TEXT NOT NULL,
     confirmed_at    TEXT,
     unsubscribed_at TEXT,
     confirm_sent_at TEXT,
     ip_hash         TEXT,
     ua_hash         TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_status ON subscribers (status, language)`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_updated ON subscribers (status, updated_at)`,
];

const B = "/newsletter-admin";
const definition = (d1: FakeD1) =>
  JSON.stringify({
    master: d1.sqlite.prepare("SELECT type, name, tbl_name, sql FROM sqlite_master WHERE tbl_name = 'subscribers' ORDER BY type, name").all(),
    columns: d1.sqlite.prepare("PRAGMA table_info(subscribers)").all(),
  });
const DDL_ON_SUBSCRIBERS = /\b(ALTER|CREATE|DROP|REINDEX)\b[\s\S]*\bsubscribers\b/i;

function v1Env(): EngineEnv {
  const d1 = fakeD1();
  for (const s of MAIN_V1) d1.sqlite.exec(s);
  const ins = d1.sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, language, source_page, tags, consent_text, consent_at, token, created_at, updated_at, confirmed_at)
     VALUES (?, ?, ?, ?, '/blog/', ?, 'Send me the newsletter', '2026-01-01T00:00:00.000Z', ?, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', ?)`,
  );
  const rows: [string, string, string | null, string][] = [
    ["s1", "confirmed", "en", "news"], ["s2", "confirmed", "pl", "offers"], ["s3", "pending", "en", ""], ["s4", "unsubscribed", null, "news"],
  ];
  for (const [id, status, lang, tags] of rows) ins.run(id, `${id}@example.com`, status, lang, tags, `tok-${id}`.padEnd(43, "x"), status === "confirmed" ? "2026-01-01T00:00:00.000Z" : null);
  return makeEnv({
    NEWSLETTER_DB: d1,
    NEWSLETTER_SITE_URL: "https://example.com",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    NEWSLETTER_RESEND_API_KEY: "re_test",
    NEWSLETTER_RESEND_WEBHOOK_SECRET: RESEND_SECRET,
    NEWSLETTER_FEED_URL: "/feed.xml",
  }) as EngineEnv;
}

const FEED = `<?xml version="1.0"?><rss version="2.0"><channel><title>Blog</title>
<item><title>Post</title><link>https://example.com/blog/post/</link><guid>p1</guid><pubDate>Fri, 13 Nov 2026 10:00:00 GMT</pubDate><description>Hello</description></item>
</channel></rss>`;

test("compat: newsletter-ui on a database made by the 1.9.7 newsletter template never changes the subscribers table", async () => {
  const realFetch = globalThis.fetch;
  globalThis.fetch = (async (url: string | URL) =>
    String(url).includes("feed") ? new Response(FEED, { headers: { "content-type": "application/rss+xml" } }) : Response.json({ id: "msg" })) as typeof fetch;
  const quiet = console.error;
  console.error = () => {};
  try {
    const env = v1Env();
    const d1 = env.NEWSLETTER_DB;
    const before = definition(d1);
    const unchanged = (after: string) => assert.equal(definition(d1), before, `subscribers definition changed after: ${after}`);

    await ensureSchema(env as never);
    unchanged("ensureSchema");

    // Every route, through the whole pipeline. Handlers are wrapped to count
    // calls, so a route the walk misses fails the test.
    const hit = new Map<Route, number>();
    const routes = ROUTES.map((r) => ({ ...r, handler: (c: Parameters<Route["handler"]>[0]) => (hit.set(r, (hit.get(r) ?? 0) + 1), r.handler(c)) }));
    const statuses: string[] = [];
    const send = async (method: string, path: string, body?: string, headers: Record<string, string> = {}) => {
      const init: RequestInit & { auth?: string } = { method, auth: PASSWORD, headers: { "content-type": "application/json", ...headers } };
      if (body !== undefined) init.body = body;
      const res = await handleFetch(req(B + path, init), env as never, fakeCtx() as never, routes);
      unchanged(`${method} ${path}`);
      statuses.push(`${res.status} ${method} ${path}`);
      return res;
    };
    const created = await (await send("POST", "/api/campaigns", JSON.stringify({ subject: "Hello", body: "Hi {{unsubscribe_url}}" }))).json();
    const campaign = created.id as string;
    assert.ok(campaign, "a campaign was created");
    const ids: Record<string, string> = { ":id": campaign, ":campaign": campaign, ":link": "0", ":hash": "0".repeat(64) };
    const bodies: Record<string, string> = {
      "/api/import/preview": JSON.stringify({ csv: "email,language\nnew@example.com,en\n", confirmed: true }),
      "/api/import": JSON.stringify({ csv: "email,language,status\nnew@example.com,en,confirmed\ns2@example.com,pl,unsubscribed\n", confirmed: true, consentSource: "Sign-up form, 2025" }),
      "/api/suppressions/import/preview": JSON.stringify({ csv: "email,reason\nbad@example.com,hard\n" }),
      "/api/suppressions/import": JSON.stringify({ csv: "email,reason\nbad@example.com,hard\n" }),
      "/api/suppressions": JSON.stringify({ email: "s3@example.com", reason: "manual" }),
      "/api/subscribers/:id": JSON.stringify({ language: "de", tags: ["vip"] }),
      "/api/campaigns/:id/test": JSON.stringify({ to: "editor@example.com" }),
      "/api/campaigns/from-post": JSON.stringify({ guid: "p1" }),
    };
    // Destructive routes last, so the others find something to work on.
    const order = [...routes].sort((a, b) => Number(a.method === "DELETE") - Number(b.method === "DELETE"));
    for (const r of order) {
      if (r.path.startsWith("/hooks/")) continue;
      const subscriberRoute = r.path.startsWith("/api/subscribers/:id");
      const path = r.path.replace(/:[a-z]+/g, (p) => (subscriberRoute && p === ":id" ? "s1" : ids[p]));
      const body = r.method === "GET" || r.method === "HEAD" || r.method === "DELETE" ? undefined : (bodies[r.path] ?? "{}");
      await send(r.method, path, body);
    }

    // The public routes: a click redirect and two webhooks, one with a valid
    // signature (a complaint unsubscribes the person: a row change, nothing more).
    for (const method of ["GET", "HEAD"]) {
      await handleFetch(new Request(`https://example.com${B}/hooks/c/${campaign}/0`, { method }), env as never, fakeCtx() as never, routes);
    }
    const complaint = resendEvent("email.complained", "s2@example.com");
    await handleFetch(new Request(`https://example.com${B}/hooks/resend`, { method: "POST", body: complaint, headers: await svixHeaders(complaint) }), env as never, fakeCtx() as never, routes);
    await handleFetch(new Request(`https://example.com${B}/hooks/mailchannels`, { method: "POST", body: "{}" }), env as never, fakeCtx() as never, routes);
    unchanged("hooks");
    // Each route did its work, or refused for a reason it gives; none crashed.
    assert.deepEqual(statuses.filter((s) => s.startsWith("5")), [], "no route answered 5xx");
    assert.ok(statuses.includes("200 POST /api/import"), "the import ran");
    assert.deepEqual(ROUTES.filter((r) => !hit.get(r)).map((r) => `${r.method} ${r.path}`), [], "every route ran");

    // The cron and a whole campaign through the queue.
    await handleScheduled({ cron: "* * * * *", scheduledTime: Date.now() } as never, env as never);
    unchanged("cron");
    // The walk above erased or unsubscribed most of the list; sign one more up
    // the way the template does, so the campaign has somebody to reach.
    d1.sqlite.exec(`INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at, confirmed_at)
      VALUES ('s5', 's5@example.com', 'confirmed', 'en', '', 'yes', '2026-01-02T00:00:00.000Z', '${"tok-s5".padEnd(43, "x")}', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z')`);
    const due = addCampaign(d1);
    const mailer = fakeMailer();
    await pump(env, { open: mailer.open });
    unchanged("campaign through pump");
    assert.equal((d1.sqlite.prepare("SELECT status FROM campaigns WHERE id = ?").get(due) as { status: string }).status, "sent");
    assert.ok(mailer.sent.length > 0, "the campaign reached somebody");

    assert.ok(!d1.calls.some((sql) => DDL_ON_SUBSCRIBERS.test(sql)), "no statement creates, alters, drops or indexes subscribers");
    unchanged("everything");
  } finally {
    globalThis.fetch = realFetch;
    console.error = quiet;
  }
});

// When the template sits next to this directory (the ssg repository, a site
// that scaffolded both), its current schema must still be the frozen v1.
const template = new URL("../../newsletter/functions/api/newsletter/_schema.ts", import.meta.url);
test("compat: the newsletter template next to this one still creates exactly the frozen v1 table", { skip: existsSync(template) ? false : "the newsletter template is not next to this directory" }, async () => {
  const { ensureSchema: templateSchema } = await import(template.href);
  const byTemplate = fakeD1();
  await templateSchema({ NEWSLETTER_DB: byTemplate });
  const frozen = fakeD1();
  for (const s of MAIN_V1) frozen.sqlite.exec(s);
  assert.equal(definition(byTemplate), definition(frozen));
});
