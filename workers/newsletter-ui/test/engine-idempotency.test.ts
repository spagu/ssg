// Resend's 409 answers to a keyed send, through the real transport: a key
// reused with a different payload means the message under it was accepted
// earlier (the row is `sent`, with a note); a request still in progress is a
// temporary failure (retried, then sent). Nobody is mailed twice.
import { test } from "node:test";
import assert from "node:assert/strict";
import { DUPLICATE_DETAIL } from "../src/engine-deliver.ts";
import { campaignRow, pump, sendRows, started, subId, truth } from "./engine-fixtures.ts";

const conflict = (name: string) => new Response(JSON.stringify({ statusCode: 409, name, message: "conflict" }), { status: 409 });

test("Resend 409: reused key = sent with a note, in progress = retried, the campaign finishes", async () => {
  const { env, id, jobs } = await started(3);
  const keys: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (_url: string, init: RequestInit) => {
    const key = new Headers(init.headers).get("idempotency-key") as string;
    keys.push(key);
    if (key.endsWith(subId(2))) return conflict("invalid_idempotent_request");
    if (key.endsWith(subId(3)) && keys.filter((k) => k === key).length === 1) return conflict("concurrent_idempotent_requests");
    return new Response("{}", { status: 200 });
  }) as typeof fetch;
  try {
    env.NEWSLETTER_QUEUE.sent.push(...jobs.map((body) => ({ body })));
    await pump(env);
  } finally {
    globalThis.fetch = original;
  }
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 3 });
  const rows = sendRows(env.NEWSLETTER_DB, id);
  assert.deepEqual(rows.map((r) => [r.subscriber_id, r.error, r.attempts]), [
    [subId(1), null, 1],
    [subId(2), DUPLICATE_DETAIL, 1],
    [subId(3), null, 2],
  ]);
  assert.ok(rows.every((r) => typeof r.sent_at === "string"), "a duplicate is sent at the time it was recorded");
  assert.deepEqual(keys.map((k) => k.slice(k.indexOf(":") + 1)), [subId(1), subId(2), subId(3), subId(3)], "no key is tried after it was settled");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});
