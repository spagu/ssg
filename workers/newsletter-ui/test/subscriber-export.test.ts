// Per-person GDPR export (JSON) and the list export (CSV).
import { test } from "node:test";
import assert from "node:assert/strict";
import { CSV_COLUMNS } from "../src/subscriber-export.ts";
import { suppress } from "../src/suppress.ts";
import { api, readyEnv, seed } from "./subscriber-seed.ts";

test("person export: a JSON download with everything but hashes and token", async () => {
  const env = await readyEnv();
  const [id] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com", tags: "news" }]);
  env.NEWSLETTER_DB.sqlite.exec(`INSERT INTO campaign_sends (campaign_id, subscriber_id, status, claimed_at) VALUES ('c', '${id}', 'sent', 'x')`);
  const res = await api(env, "GET", `/api/subscribers/${id}/export`);
  assert.equal(res.status, 200);
  assert.match(res.headers.get("content-disposition") as string, new RegExp(`^attachment; filename="subscriber-${id}-\\d{4}-\\d{2}-\\d{2}\\.json"$`));
  assert.equal(res.headers.get("cache-control"), "no-store");
  const body = await res.json();
  assert.equal(body.subscriber.email, "ann@example.com");
  assert.equal(body.subscriber.consentText, "I agree to the newsletter");
  assert.equal(body.subscriber.sends.length, 1);
  assert.match(body.exportedAt, /^\d{4}-/);
  assert.doesNotMatch(JSON.stringify(body), /token|hash|secret|tok\d/i);
  assert.equal((await api(env, "GET", "/api/subscribers/nope/export")).status, 404);
});

test("CSV export: the template's columns and safety, minus suppressed addresses", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [
    { email: "ann@example.com", tags: "=cmd", created_at: "2026-01-01T00:00:00.000Z" },
    { email: "bounced@example.com", created_at: "2026-01-02T00:00:00.000Z" },
    { email: "erased@example.com", created_at: "2026-01-03T00:00:00.000Z" },
    { email: "pend@example.com", status: "pending", created_at: "2026-01-04T00:00:00.000Z" },
  ]);
  await suppress(env as never, { email: "bounced@example.com", reason: "hard", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "erased@example.com", reason: "manual", source: "erasure", keepEmail: false });

  const res = await api(env, "GET", "/api/subscribers.csv");
  assert.equal(res.headers.get("content-type"), "text/csv; charset=utf-8");
  assert.match(res.headers.get("content-disposition") as string, /^attachment; filename="newsletter-confirmed-\d{4}-\d{2}-\d{2}\.csv"$/);
  assert.equal(res.headers.get("cache-control"), "no-store");
  // Response.text() strips a byte-order mark, so read the bytes.
  const bytes = new Uint8Array(await res.arrayBuffer());
  assert.deepEqual([...bytes.slice(0, 3)], [0xef, 0xbb, 0xbf]);
  const csv = new TextDecoder("utf-8", { ignoreBOM: true }).decode(bytes).slice(1);
  assert.ok(csv.startsWith(CSV_COLUMNS.join(",") + "\r\n"));
  const lines = csv.trimEnd().split("\r\n");
  assert.equal(lines.length, 2);
  assert.match(lines[1], /^ann@example\.com,confirmed,en,'=cmd,/);
  assert.match(lines[1], /,https:\/\/example\.com\/api\/newsletter\/unsubscribe\?token=tok\d+x+$/);

  const all = (await (await api(env, "GET", "/api/subscribers.csv?status=all")).text()).trimEnd().split("\r\n");
  assert.deepEqual(all.slice(1).map((l) => l.split(",")[0]), ["ann@example.com", "pend@example.com"]);
  const pending = await (await api(env, "GET", "/api/subscribers.csv?status=pending")).text();
  assert.match(pending, /pend@example\.com/);
  assert.doesNotMatch(pending, /ann@example/);
  // GO-127: the export follows the engine's fresh-consent rule. Someone who
  // signed up again after their erasure (consent_at newer than the hash-only
  // suppression) is mailable, so listed; a hard bounce stays out whatever the
  // consent.
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE suppressions SET updated_at = '2026-01-01T00:00:00.000Z' WHERE email IS NULL").run();
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET consent_at = '2026-02-01T00:00:00.000Z' WHERE email IN ('erased@example.com', 'bounced@example.com')").run();
  const again = (await (await api(env, "GET", "/api/subscribers.csv")).text()).trimEnd().split("\r\n");
  assert.deepEqual(again.slice(1).map((l) => l.split(",")[0]), ["ann@example.com", "erased@example.com"]);

  const bad = await api(env, "GET", "/api/subscribers.csv?status=nope");
  assert.equal(bad.status, 400);
  assert.equal((await bad.json()).error, "invalid_status");
});
