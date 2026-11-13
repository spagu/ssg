// GO-124 routes: auth on every one, the list (filters, pagination, LIKE
// escaping), the detail view, and that secrets never leave the database.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { decodeCursor, encodeCursor } from "../src/subscribers.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";
import { B, api, readyEnv, seed } from "./subscriber-seed.ts";

const OURS: [string, string][] = [
  ["GET", "/api/subscribers"],
  ["GET", "/api/subscribers.csv"],
  ["GET", "/api/subscribers/x"],
  ["PATCH", "/api/subscribers/x"],
  ["DELETE", "/api/subscribers/x"],
  ["POST", "/api/subscribers/x/unsubscribe"],
  ["GET", "/api/subscribers/x/export"],
  ["POST", "/api/import/preview"],
  ["POST", "/api/import"],
  ["GET", "/api/stats/subscribers"],
];

test("every GO-124 route refuses without credentials, without config, and without CSRF", async () => {
  for (const [method, path] of OURS) {
    const run = (r: Request, env = makeEnv()) => handleFetch(r, env as never, fakeCtx() as never);
    assert.equal((await run(req(B + path, { method }))).status, 401, `${method} ${path} no credentials`);
    assert.equal((await run(req(B + path, { method, auth: "wrong" }))).status, 401, `${method} ${path} wrong password`);
    const unconfigured = makeEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined });
    assert.equal((await run(req(B + path, { method, auth: PASSWORD }), unconfigured)).status, 503, `${method} ${path} no config`);
    if (method !== "GET") {
      assert.equal((await run(req(B + path, { method, auth: PASSWORD, csrf: false }))).status, 403, `${method} ${path} no CSRF`);
    }
    // With credentials the route exists (never a 404 for the path itself).
    const ok = await run(req(B + path, { method, auth: PASSWORD }));
    assert.notEqual(ok.status, 405, `${method} ${path}`);
  }
});

test("cursor encoding round-trips and refuses junk", () => {
  const c = encodeCursor("2026-01-01T00:00:00.000Z", "id/+?");
  assert.deepEqual(decodeCursor(c), ["2026-01-01T00:00:00.000Z", "id/+?"]);
  assert.equal(decodeCursor("!!"), null);
  assert.equal(decodeCursor(btoa("[1,2]")), null);
  assert.equal(decodeCursor(btoa('["a"]')), null);
  assert.equal(decodeCursor(btoa("{}")), null);
});

test("list: newest first, stable cursor pages over equal timestamps, no secrets", async () => {
  const env = await readyEnv();
  // 5 rows share one created_at (an import), 2 are newer.
  seed(env.NEWSLETTER_DB, [
    ...["a", "b", "c", "d", "e"].map((x) => ({ id: "same-" + x, email: x + "@example.com" })),
    { id: "new-1", email: "n1@example.com", created_at: "2026-02-01T00:00:00.000Z" },
    { id: "new-2", email: "n2@example.com", created_at: "2026-03-01T00:00:00.000Z" },
  ]);
  const ids: string[] = [];
  let cursor: string | null = "";
  let pages = 0;
  while (cursor !== null) {
    const res = await api(env, "GET", `/api/subscribers?limit=2${cursor ? "&cursor=" + cursor : ""}`);
    assert.equal(res.status, 200);
    const body = await res.json();
    ids.push(...body.items.map((i: { id: string }) => i.id));
    cursor = body.nextCursor;
    pages++;
    const raw = JSON.stringify(body);
    assert.doesNotMatch(raw, /token|ip_hash|ua_hash|iphash-secret|uahash-secret|tok\d/);
  }
  assert.equal(pages, 4);
  assert.deepEqual(ids, ["new-2", "new-1", "same-e", "same-d", "same-c", "same-b", "same-a"]);

  const first = await (await api(env, "GET", "/api/subscribers")).json();
  assert.equal(first.items.length, 7);
  assert.equal(first.nextCursor, null);
  assert.deepEqual(Object.keys(first.items[0]).sort(), [
    "confirmedAt", "createdAt", "email", "id", "language", "sourcePage", "status", "tags", "unsubscribedAt", "updatedAt",
  ]);
  // An exact page: no phantom next cursor.
  assert.equal((await (await api(env, "GET", "/api/subscribers?limit=7")).json()).nextCursor, null);
});

test("list: default 50, cap 200, bad limit / cursor are 400", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, Array.from({ length: 205 }, (_, i) => ({ email: `p${i}@example.com` })));
  assert.equal((await (await api(env, "GET", "/api/subscribers")).json()).items.length, 50);
  assert.equal((await (await api(env, "GET", "/api/subscribers?limit=1000")).json()).items.length, 200);
  for (const q of ["limit=0", "limit=-1", "limit=x", "limit=1.5", "cursor=bogus"]) {
    const res = await api(env, "GET", "/api/subscribers?" + q);
    assert.equal(res.status, 400, q);
  }
});

test("list: filters by status, language, tag, and q with LIKE wildcards escaped", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [
    { id: "1", email: "ann_lee@example.com", status: "confirmed", language: "pl", tags: "news,a_b" },
    { id: "2", email: "annxlee@example.com", status: "pending", language: "en", tags: "axb" },
    { id: "3", email: "100%sure@example.com", status: "unsubscribed", language: null, tags: "news" },
    { id: "4", email: "back\\slash@example.com", status: "confirmed", language: "pt-BR", tags: "" },
  ]);
  const ids = async (q: string) => (await (await api(env, "GET", "/api/subscribers?" + q)).json()).items.map((i: { id: string }) => i.id).sort();
  assert.deepEqual(await ids("status=pending"), ["2"]);
  assert.deepEqual(await ids("language=PL"), ["1"]);
  assert.deepEqual(await ids("language=pt-BR"), ["4"]);
  assert.deepEqual(await ids("tag=news"), ["1", "3"]);
  assert.deepEqual(await ids("tag=a_b"), ["1"], "_ in a tag is literal");
  assert.deepEqual(await ids("q=ANN_"), ["1"], "_ in q is literal");
  assert.deepEqual(await ids("q=ann"), ["1", "2"]);
  assert.deepEqual(await ids("q=%25"), ["3"], "% in q is literal");
  assert.deepEqual(await ids("q=%5C"), ["4"], "backslash in q is literal");
  assert.deepEqual(await ids("q=%20%20"), ["1", "2", "3", "4"], "blank q is no filter");
  assert.deepEqual(await ids("status=confirmed&tag=news&q=lee"), ["1"]);
  for (const q of ["status=active", "language=english", "tag=bad%20tag"]) {
    const res = await api(env, "GET", "/api/subscribers?" + q);
    assert.equal(res.status, 400, q);
    assert.match((await res.json()).error, /^invalid_/);
  }
});

test("detail: consent, sends with the campaign subject, suppression; 404", async () => {
  const env = await readyEnv();
  const d1 = env.NEWSLETTER_DB;
  const [id] = seed(d1, [{ email: "ann@example.com", tags: "news" }]);
  d1.sqlite.exec(`INSERT INTO campaigns (id, subject, created_at, updated_at) VALUES ('c1', 'Spring issue', 'x', 'x')`);
  d1.sqlite.exec(`INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at, sent_at)
                  VALUES ('c1', '${id}', 'sent', 1, '2026-04-01', '2026-04-01'), ('gone', '${id}', 'failed', 3, '2026-03-01', NULL)`);
  const body = await (await api(env, "GET", "/api/subscribers/" + id)).json();
  assert.equal(body.consentText, "I agree to the newsletter");
  assert.equal(body.consentAt, "2026-01-01T00:00:00.000Z");
  assert.deepEqual(body.tags, ["news"]);
  assert.deepEqual(body.sends.map((s: { subject: string | null }) => s.subject), ["Spring issue", null]);
  assert.equal(body.suppression, null);
  assert.doesNotMatch(JSON.stringify(body), /token|hash|secret/i);

  d1.sqlite.exec(`INSERT INTO suppressions (email_hash, email, reason, source, created_at, updated_at)
                  VALUES ('${await (await import("../src/suppress.ts")).suppressionKey("ann@example.com")}', 'ann@example.com', 'hard', 'smtp', 'x', 'x')`);
  const sup = (await (await api(env, "GET", "/api/subscribers/" + id)).json()).suppression;
  assert.deepEqual(sup, { reason: "hard", source: "smtp", count: 1, createdAt: "x", updatedAt: "x" });

  const missing = await api(env, "GET", "/api/subscribers/nope");
  assert.equal(missing.status, 404);
  assert.deepEqual(await missing.json(), { error: "not_found" });
});
