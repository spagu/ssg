// PATCH (tags + language only), manual unsubscribe, GDPR erasure.
import { test } from "node:test";
import assert from "node:assert/strict";
import { isSuppressed } from "../src/suppress.ts";
import { api, readyEnv, row, seed } from "./subscriber-seed.ts";

test("PATCH changes tags and language and nothing else", async () => {
  const env = await readyEnv();
  const [id] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com", status: "pending", tags: "old" }]);
  const res = await api(env, "PATCH", "/api/subscribers/" + id, { tags: ["News", "offers"], language: "PT-br" });
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.deepEqual(body.tags, ["news", "offers"]);
  assert.equal(body.language, "pt-br");
  const r = row(env.NEWSLETTER_DB, id) as Record<string, unknown>;
  assert.equal(r.tags, "news,offers");
  assert.equal(r.status, "pending");
  assert.notEqual(r.updated_at, r.created_at);

  await api(env, "PATCH", "/api/subscribers/" + id, { language: null, tags: "" });
  assert.equal(row(env.NEWSLETTER_DB, id)?.language, null);
  assert.equal(row(env.NEWSLETTER_DB, id)?.tags, "");
  await api(env, "PATCH", "/api/subscribers/" + id, { language: "" });
  assert.equal(row(env.NEWSLETTER_DB, id)?.language, null);
});

test("PATCH refuses status, email, consent and anything unknown — admins never confirm", async () => {
  const env = await readyEnv();
  const [id] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com", status: "pending" }]);
  const cases: [unknown, string][] = [
    [{ status: "confirmed" }, "field_not_editable"],
    [{ tags: "a", status: "confirmed" }, "field_not_editable"],
    [{ email: "bob@example.com" }, "field_not_editable"],
    [{ consent_text: "x" }, "field_not_editable"],
    [{ consentAt: "x" }, "field_not_editable"],
    [{ confirmed_at: "x" }, "field_not_editable"],
    [{}, "invalid_body"],
    [[1], "invalid_body"],
    [{ tags: "bad tag" }, "invalid_tags"],
    [{ tags: 5 }, "invalid_tags"],
    [{ language: "english" }, "invalid_language"],
    [{ language: 5 }, "invalid_language"],
  ];
  for (const [body, error] of cases) {
    const res = await api(env, "PATCH", "/api/subscribers/" + id, body);
    assert.equal(res.status, 400, JSON.stringify(body));
    assert.equal((await res.json()).error, error, JSON.stringify(body));
  }
  const notJSON = await api(env, "PATCH", "/api/subscribers/" + id, "{", { "content-type": "application/json" });
  assert.equal(notJSON.status, 400);
  const r = row(env.NEWSLETTER_DB, id) as Record<string, unknown>;
  assert.equal(r.status, "pending");
  assert.equal(r.email, "ann@example.com");
  assert.equal((await api(env, "PATCH", "/api/subscribers/nope", { tags: "a" })).status, 404);
});

test("manual unsubscribe: same fields as the public link, idempotent, 404", async () => {
  const env = await readyEnv();
  const [id] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com" }]);
  const res = await api(env, "POST", `/api/subscribers/${id}/unsubscribe`);
  const body = await res.json();
  assert.equal(body.changed, true);
  assert.equal(body.subscriber.status, "unsubscribed");
  const first = row(env.NEWSLETTER_DB, id) as Record<string, unknown>;
  assert.equal(first.unsubscribed_at, first.updated_at);
  const again = await (await api(env, "POST", `/api/subscribers/${id}/unsubscribe`)).json();
  assert.equal(again.changed, false);
  assert.equal(row(env.NEWSLETTER_DB, id)?.unsubscribed_at, first.unsubscribed_at, "original time kept");
  assert.equal((await api(env, "POST", "/api/subscribers/nope/unsubscribe")).status, 404);
});

test("erasure: row and sends gone, hash-only suppression, address nowhere in clear text", async () => {
  const env = await readyEnv();
  const d1 = env.NEWSLETTER_DB;
  const email = "erase.me@example.com";
  const [id, keep] = seed(d1, [{ email }, { email: "keep@example.com" }]);
  d1.sqlite.exec(`INSERT INTO campaigns (id, subject, created_at, updated_at) VALUES ('c1', 'Hello', 'x', 'x')`);
  d1.sqlite.exec(`INSERT INTO campaign_sends (campaign_id, subscriber_id, status, error, claimed_at)
                  VALUES ('c1', '${id}', 'failed', '550 ${email} unknown', 'x'), ('c1', '${keep}', 'sent', NULL, 'x')`);
  // A bounce had kept the address in clear text.
  d1.sqlite.exec(`INSERT INTO suppressions (email_hash, email, reason, source, created_at, updated_at)
                  VALUES ('${await (await import("../src/suppress.ts")).suppressionKey(email)}', '${email}', 'soft', 'smtp', 'x', 'x')`);

  const res = await api(env, "DELETE", "/api/subscribers/" + id);
  assert.deepEqual(await res.json(), { ok: true, erased: id });
  assert.equal(row(d1, id), undefined);
  assert.equal(d1.sqlite.prepare("SELECT COUNT(*) AS n FROM campaign_sends WHERE subscriber_id = ?").get(id)?.n, 0);
  assert.equal(d1.sqlite.prepare("SELECT COUNT(*) AS n FROM campaign_sends").get()?.n, 1, "others' sends stay");
  assert.equal(await isSuppressed(env as never, email), true);
  const sup = d1.sqlite.prepare("SELECT email, reason, source FROM suppressions").get();
  assert.deepEqual({ ...sup }, { email: null, reason: "manual", source: "erasure" });

  // Scan every column of every table for the address.
  const tables = d1.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all().map((t) => t.name as string);
  assert.ok(tables.includes("subscribers") && tables.includes("suppressions"));
  for (const t of tables) {
    for (const r of d1.sqlite.prepare(`SELECT * FROM ${t}`).all()) {
      assert.doesNotMatch(JSON.stringify(r).toLowerCase(), /erase\.me@example\.com/, `found in ${t}`);
    }
  }
  assert.equal((await api(env, "DELETE", "/api/subscribers/" + id)).status, 404);
});
