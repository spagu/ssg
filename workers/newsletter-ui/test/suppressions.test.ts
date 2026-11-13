// GO-127: the suppression list admin API — list, add, lift, stats (the CSV
// import is in suppression-import.test.ts) —
// and the auth rules every one of its routes is under.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { sha256hex } from "../src/lib.ts";
import { findSuppression, suppress } from "../src/suppress.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { B, api, readyEnv, row, seed } from "./subscriber-seed.ts";

const H = (email: string) => sha256hex(email);

test("every /api/ route of the feature refuses without auth, and state changes without the CSRF header", async () => {
  const env = await readyEnv();
  const hash = await H("a@example.com");
  const routes: [string, string][] = [
    ["GET", "/api/suppressions"],
    ["POST", "/api/suppressions"],
    ["DELETE", `/api/suppressions/${hash}`],
    ["POST", "/api/suppressions/import/preview"],
    ["POST", "/api/suppressions/import"],
    ["GET", "/api/stats/suppressions"],
  ];
  for (const [method, path] of routes) {
    const anon = await handleFetch(req(B + path, { method }), env as never, fakeCtx() as never);
    assert.equal(anon.status, 401, `${method} ${path} without credentials`);
    const wrong = await handleFetch(req(B + path, { method, auth: "wrong" }), env as never, fakeCtx() as never);
    assert.equal(wrong.status, 401, `${method} ${path} with a wrong password`);
    if (method !== "GET") {
      const noCsrf = await handleFetch(req(B + path, { method, auth: PASSWORD, csrf: false }), env as never, fakeCtx() as never);
      assert.equal(noCsrf.status, 403, `${method} ${path} without the CSRF header`);
    }
  }
  assert.equal(Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM suppressions").get() as { n: number }).n), 0);
});

test("list: newest change first, filter by reason, q matches kept addresses only, cursor", async () => {
  const env = await readyEnv();
  const d1 = env.NEWSLETTER_DB;
  const put = (email: string, reason: string, at: string, keep = true) =>
    d1.sqlite
      .prepare("INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at) VALUES (?, ?, ?, 1, 'test', ?, ?)")
      .run(email.replace(/@.*/, "").padEnd(64, "0"), keep ? email : null, reason, at, at);
  put("ann@example.com", "hard", "2026-01-01T00:00:00.000Z");
  put("bob@example.com", "complaint", "2026-01-02T00:00:00.000Z");
  put("cat@example.com", "manual", "2026-01-03T00:00:00.000Z", false);
  put("dan_x@example.com", "soft", "2026-01-03T00:00:00.000Z");

  const all = await (await api(env, "GET", "/api/suppressions")).json();
  assert.deepEqual(all.items.map((i: { email: string | null }) => i.email), ["dan_x@example.com", null, "bob@example.com", "ann@example.com"]);
  assert.equal(all.nextCursor, null);
  assert.deepEqual(Object.keys(all.items[0]).sort(), ["count", "createdAt", "email", "emailHash", "reason", "source", "updatedAt"]);

  const hard = await (await api(env, "GET", "/api/suppressions?reason=complaint")).json();
  assert.deepEqual(hard.items.map((i: { email: string }) => i.email), ["bob@example.com"]);
  const q = await (await api(env, "GET", "/api/suppressions?q=CAT")).json();
  assert.deepEqual(q.items, [], "an erased address cannot be found by searching");
  const like = await (await api(env, "GET", "/api/suppressions?q=n_x")).json();
  assert.deepEqual(like.items.map((i: { email: string }) => i.email), ["dan_x@example.com"]);

  const p1 = await (await api(env, "GET", "/api/suppressions?limit=3")).json();
  assert.equal(p1.items.length, 3);
  const p2 = await (await api(env, "GET", `/api/suppressions?limit=3&cursor=${p1.nextCursor}`)).json();
  assert.deepEqual(p2.items.map((i: { email: string }) => i.email), ["ann@example.com"]);
  assert.equal(p2.nextCursor, null);

  for (const [qs, error] of [["reason=bogus", "invalid_reason"], ["limit=0", "invalid_limit"], ["limit=x", "invalid_limit"], ["cursor=%%%", "invalid_cursor"]]) {
    const res = await api(env, "GET", `/api/suppressions?${qs}`);
    assert.equal(res.status, 400, qs);
    assert.equal((await res.json()).error, error);
  }
  assert.equal((await (await api(env, "GET", "/api/suppressions?limit=999")).json()).items.length, 4);
});

test("add by hand: manual by default, hard, complaint (which unsubscribes); stronger reason kept", async () => {
  const env = await readyEnv();
  const [ann] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com" }]);
  const res = await api(env, "POST", "/api/suppressions", { email: " Ann@Example.com " });
  assert.equal(res.status, 201);
  const body = await res.json();
  assert.deepEqual([body.suppression.reason, body.suppression.source, body.suppression.email], ["manual", "admin", "ann@example.com"]);
  assert.equal(row(env.NEWSLETTER_DB, ann)?.status, "confirmed", "a manual block does not unsubscribe");

  const complaint = await api(env, "POST", "/api/suppressions", { email: "ann@example.com", reason: "complaint" });
  assert.equal(complaint.status, 200);
  assert.equal((await complaint.json()).suppression.reason, "complaint");
  assert.equal(row(env.NEWSLETTER_DB, ann)?.status, "unsubscribed");
  const again = await api(env, "POST", "/api/suppressions", { email: "ann@example.com", reason: "manual" });
  assert.equal((await again.json()).suppression.reason, "complaint", "a weaker reason never replaces a complaint");
  assert.equal((await api(env, "POST", "/api/suppressions", { email: "hard@example.com", reason: "hard" })).status, 201);

  for (const [b, error] of [[{}, "invalid_email"], [{ email: "nope" }, "invalid_email"], [{ email: "x@example.com", reason: "soft" }, "invalid_reason"], [{ email: "x@example.com", reason: 1 }, "invalid_reason"]] as const) {
    const r = await api(env, "POST", "/api/suppressions", b);
    assert.equal(r.status, 400);
    assert.equal((await r.json()).error, error);
  }
  assert.equal((await api(env, "POST", "/api/suppressions", "not json", { "content-type": "application/json" })).status, 400);
});

test("lift: 204; a complaint needs {confirm: true} (409 otherwise); unknown → 404", async () => {
  const env = await readyEnv();
  await suppress(env as never, { email: "hard@example.com", reason: "hard", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "spam@example.com", reason: "complaint", source: "resend", keepEmail: true });
  env.NEWSLETTER_DB.sqlite
    .prepare("INSERT INTO soft_bounces (email_hash, email, count, source, updated_at) VALUES (?, 'hard@example.com', 2, 'smtp', 'x')")
    .run(await H("hard@example.com"));

  const hard = await H("hard@example.com");
  assert.equal((await api(env, "DELETE", `/api/suppressions/${hard.toUpperCase()}`)).status, 204);
  assert.equal(await findSuppression(env as never, "hard@example.com"), null);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT 1 FROM soft_bounces").get(), undefined, "the soft count goes too");

  const spam = await H("spam@example.com");
  for (const body of [undefined, {}, { confirm: "yes" }]) {
    const res = await api(env, "DELETE", `/api/suppressions/${spam}`, body);
    assert.equal(res.status, 409);
    assert.equal((await res.json()).error, "confirm_required");
  }
  assert.notEqual(await findSuppression(env as never, "spam@example.com"), null);
  assert.equal((await api(env, "DELETE", `/api/suppressions/${spam}`, { confirm: true })).status, 204);
  assert.equal(await findSuppression(env as never, "spam@example.com"), null);

  assert.equal((await api(env, "DELETE", `/api/suppressions/${spam}`)).status, 404);
  assert.equal((await api(env, "DELETE", "/api/suppressions/not-a-hash")).status, 404);
});

test("stats: counts by reason, soft below the limit, a per-day series by reason", async () => {
  const env = await readyEnv();
  const today = new Date().toISOString();
  const d1 = env.NEWSLETTER_DB;
  const put = (n: number, reason: string, at: string) =>
    d1.sqlite
      .prepare("INSERT INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at) VALUES (?, NULL, ?, 1, 't', ?, ?)")
      .run(String(n).padEnd(64, "0"), reason, at, at);
  put(1, "hard", today);
  put(2, "hard", today);
  put(3, "complaint", today);
  put(4, "manual", "2020-01-01T00:00:00.000Z");
  put(5, "legacy", today);
  d1.sqlite.prepare("INSERT INTO soft_bounces (email_hash, email, count, source, updated_at) VALUES ('s', 's@example.com', 1, 'smtp', 'x')").run();

  const res = await api(env, "GET", "/api/stats/suppressions");
  const body = await res.json();
  assert.equal(body.days, 30);
  assert.equal(body.total, 5);
  assert.deepEqual(body.byReason, { hard: 2, soft: 0, complaint: 1, manual: 1, legacy: 1 });
  assert.equal(body.softCounting, 1);
  assert.equal(body.series.length, 30);
  const last = body.series[29];
  assert.deepEqual(last, { day: today.slice(0, 10), hard: 2, soft: 0, complaint: 1, manual: 0, legacy: 1 });
  assert.deepEqual(body.series[0], { day: body.series[0].day, hard: 0, soft: 0, complaint: 0, manual: 0 });

  assert.equal((await (await api(env, "GET", "/api/stats/suppressions?days=365")).json()).series.length, 365);
  const bad = await api(env, "GET", "/api/stats/suppressions?days=7");
  assert.equal(bad.status, 400);
  assert.equal((await bad.json()).error, "invalid_days");
});
