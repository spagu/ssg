// The ACP modules one at a time (GO-163): idempotency key states that need a
// clock, webhook delivery failures, and the corners the routes cannot reach.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { claimKey, settleKey } from "../functions/api/shop/_acp-idempotency";
import { canonicalJson } from "../functions/api/shop/_acp-http";
import { priceSession } from "../functions/api/shop/_acp-pricing";
import { deliverAcpEvent, enqueueOrderEvent, permalink } from "../functions/api/shop/_acp-webhook";
import { drainOutbox } from "../functions/api/shop/_outbox";
import { readToken } from "../functions/api/shop/_acp-payment";
import { freshShop } from "./_helpers";
import { ACP_WEBHOOK_SECRET, enableAcp, fakeReceiver, lossyD1, RECEIVER } from "./acp-helpers";

const realFetch = globalThis.fetch;
const old = "2000-01-01T00:00:00.000Z";

beforeEach(async () => {
  await freshShop();
  enableAcp();
});
afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("idempotency keys", () => {
  it("refuses an over-long key", async () => {
    const claim = await claimKey(env, "POST /x", "k".repeat(256), "{}");
    assert.equal(claim.proceed, false);
    if (!claim.proceed) assert.equal(((await claim.response.json()) as { code: string }).code, "invalid_idempotency_key");
  });

  it("answers 409 while the first request runs, and takes over a dead one", async () => {
    assert.equal((await claimKey(env, "POST /x", "k1", "{}")).proceed, true);
    const busy = await claimKey(env, "POST /x", "k1", "{}");
    assert.equal(busy.proceed, false);
    if (!busy.proceed) {
      assert.equal(busy.response.status, 409);
      assert.equal(busy.response.headers.get("retry-after"), "1");
    }
    await env.SHOP_DB.prepare(`UPDATE acp_idempotency SET created_at = ? WHERE key = 'k1'`).bind(new Date(Date.now() - 120_000).toISOString()).run();
    assert.equal((await claimKey(env, "POST /x", "k1", "{}")).proceed, true);
  });

  it("treats a key past retention as new, and purges very old rows", async () => {
    await claimKey(env, "POST /x", "k2", `{"a":1}`);
    await settleKey(env, "POST /x", "k2", { status: 201, body: { ok: true } });
    await env.SHOP_DB.prepare(`UPDATE acp_idempotency SET created_at = ? WHERE key = 'k2'`).bind(old).run();
    // A different body is fine once the key has expired.
    assert.equal((await claimKey(env, "POST /x", "k2", `{"a":2}`)).proceed, true);
    await env.SHOP_DB.prepare(`UPDATE acp_idempotency SET created_at = ? WHERE key = 'k2'`).bind(old).run();
    await claimKey(env, "POST /x", "k3", "{}");
    const left = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM acp_idempotency WHERE key = 'k2'`).first<{ n: number }>();
    assert.equal(left?.n, 0);
  });

  it("answers 409 to a takeover that lost the race", async () => {
    await claimKey(env, "POST /x", "k4", "{}");
    await env.SHOP_DB.prepare(`UPDATE acp_idempotency SET created_at = ? WHERE key = 'k4'`).bind(old).run();
    const lost = await claimKey({ ...env, SHOP_DB: lossyD1(env.SHOP_DB, /^UPDATE acp_idempotency SET body_hash/) } as never, "POST /x", "k4", "{}");
    assert.equal(lost.proceed ? 0 : lost.response.status, 409);
  });

  it("hashes a body that is not JSON by its bytes, and keys per endpoint", async () => {
    assert.equal((await claimKey(env, "POST /x", "k5", "not json")).proceed, true);
    assert.equal((await claimKey(env, "POST /y", "k5", "not json")).proceed, true);
    await settleKey(env, "POST /x", "k5", { status: 400, body: { type: "invalid_request" } });
    const replay = await claimKey(env, "POST /x", "k5", "not json");
    assert.equal(replay.proceed, false);
    if (!replay.proceed) assert.equal(replay.response.status, 400);
  });

  it("replays a stored row with no status as 200", async () => {
    await claimKey(env, "POST /x", "k6", "{}");
    await env.SHOP_DB.prepare(`UPDATE acp_idempotency SET state = 'done', status = NULL, response_json = '{}' WHERE key = 'k6'`).run();
    const replay = await claimKey(env, "POST /x", "k6", "{}");
    assert.equal(replay.proceed ? 0 : replay.response.status, 200);
  });
});

describe("canonical JSON", () => {
  it("sorts keys at every depth and keeps array order and null", () => {
    assert.equal(canonicalJson({ b: [2, { d: 1, c: null }], a: 1.0 }), `{"a":1,"b":[2,{"c":null,"d":1}]}`);
  });
});

describe("pricing", () => {
  it("names the basket when it is empty", async () => {
    const { state } = await priceSession(env, { items: [], currency: "EUR", country: "DE", origin: "https://shop.example.com" });
    assert.equal(state.messages[0]!.param, "$.line_items");
  });
});

describe("payment data", () => {
  it("accepts a credential without a handler id", () => {
    assert.equal(readToken({ instrument: { type: "card", credential: { type: "spt", token: "spt_1" } } }), "spt_1");
  });
});

describe("order events for the platform", () => {
  const event = { type: "order_create", data: { type: "order", id: "o1" } };

  it("builds a permalink with or without the order key", () => {
    assert.equal(permalink("https://shop.example.com", "o1", null), "https://shop.example.com/shop/thanks/?order=o1");
    assert.equal(permalink("https://shop.example.com", "o1", "a b"), "https://shop.example.com/shop/thanks/?order=o1&k=a%20b");
  });

  it("signs the event so the receiver verifies it", async () => {
    const receiver = fakeReceiver();
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => receiver.handle(new Request(String(input), init))) as typeof fetch;
    await deliverAcpEvent(env, RECEIVER, event);
    assert.equal(receiver.received[0]!.verified, true);
    assert.deepEqual(receiver.received[0]!.body, event);
  });

  it("throws, so the outbox retries, on every failure", async () => {
    const receiver = fakeReceiver(500);
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => receiver.handle(new Request(String(input), init))) as typeof fetch;
    await assert.rejects(deliverAcpEvent(env, RECEIVER, event), /ACP webhook 500/);
    await assert.rejects(deliverAcpEvent(env, "not a url", event), /invalid ACP webhook url/);
    await assert.rejects(deliverAcpEvent(env, "http://agent.example.com/x", event), /must be https/);
    await assert.rejects(deliverAcpEvent({ ...env, SHOP_ACP_WEBHOOK_SECRET: undefined }, RECEIVER, event), /SHOP_ACP_WEBHOOK_SECRET is not set/);
  });

  it("is retried by the outbox with the reason recorded, secret-free", async () => {
    globalThis.fetch = (async () => new Response("nope", { status: 503 })) as typeof fetch;
    assert.equal(await enqueueOrderEvent(env, "order_update", event.data), true);
    assert.equal(await drainOutbox(env, 5), 0);
    const row = await env.SHOP_DB.prepare(`SELECT attempts, last_error, payload_json FROM outbox WHERE kind = 'acp'`).first<Record<string, unknown>>();
    assert.equal(row?.attempts, 1);
    assert.equal(row?.last_error, "ACP webhook 503");
    assert.ok(!String(row?.payload_json).includes(ACP_WEBHOOK_SECRET));
  });
});
