// The ACP feed push (GO-164): a standalone Worker on a Cron Trigger that
// carries the feed the ssg build published to the agent platform.

import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import worker, { logLine, pushFeed, type FeedEnv } from "../acp-feed-push/src/index.ts";

const TOKEN = "feed-token-secret-93ad";
const env: FeedEnv = {
  ACP_FEED_SOURCE_URL: "https://example.com/acp/products.jsonl?v=1",
  ACP_FEED_TARGET_URL: "https://agent.example.com/feeds/upload?sig=private",
  ACP_FEED_TOKEN: TOKEN,
};
const FEED = '{"item_id":"EBOOK-1"}\n{"item_id":"EBOOK-2"}\n';
const realFetch = globalThis.fetch;

/** A site that serves `feed` and a platform that answers `status`. */
function world(opts: { feed?: string | null; feedStatus?: number; status?: number; length?: string; contentType?: string | null; fail?: "source" | "target"; error?: string } = {}) {
  const sent: Array<{ method: string; headers: Headers; body: string }> = [];
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const isSource = url.startsWith("https://example.com/");
    if ((opts.fail === "source" && isSource) || (opts.fail === "target" && !isSource)) {
      const e = new Error("boom");
      e.name = opts.error ?? "TypeError";
      throw e;
    }
    if (isSource) {
      const headers: Record<string, string> = {};
      if (opts.contentType !== null) headers["content-type"] = opts.contentType ?? "application/x-ndjson";
      if (opts.length) headers["content-length"] = opts.length;
      const text = opts.feed === undefined ? FEED : opts.feed;
      return new Response(text === null ? null : new TextEncoder().encode(text), { status: opts.feedStatus ?? 200, headers });
    }
    sent.push({ method: String(init?.method), headers: new Headers(init?.headers), body: new TextDecoder().decode(init?.body as Uint8Array) });
    return new Response("ok", { status: opts.status ?? 202 });
  }) as typeof fetch;
  return { sent, fetcher };
}

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("pushing the feed", () => {
  it("fetches the published feed and sends it with the credential", async () => {
    const w = world();
    const result = await pushFeed(env, w.fetcher);
    assert.equal(result.outcome, "pushed");
    assert.equal(result.status, 202);
    assert.equal(result.bytes, FEED.length);
    assert.equal(w.sent[0]!.method, "POST");
    assert.equal(w.sent[0]!.headers.get("authorization"), `Bearer ${TOKEN}`);
    assert.equal(w.sent[0]!.headers.get("content-type"), "application/x-ndjson");
    assert.equal(w.sent[0]!.body, FEED);
  });

  it("uses the configured method and falls back to NDJSON without a content type", async () => {
    const w = world({ contentType: null });
    await pushFeed({ ...env, ACP_FEED_METHOD: "put" }, w.fetcher);
    assert.equal(w.sent[0]!.method, "PUT");
    assert.equal(w.sent[0]!.headers.get("content-type"), "application/x-ndjson");
  });

  it("does nothing without its configuration", async () => {
    for (const broken of [
      { ACP_FEED_TOKEN: undefined },
      { ACP_FEED_SOURCE_URL: "http://example.com/feed" },
      { ACP_FEED_TARGET_URL: "not a url" },
      { ACP_FEED_SOURCE_URL: undefined },
      { ACP_FEED_METHOD: "DELETE" },
    ]) {
      const w = world();
      assert.equal((await pushFeed({ ...env, ...broken }, w.fetcher)).outcome, "not_configured");
      assert.equal(w.sent.length, 0);
    }
  });

  it("reports a failed fetch, a refused push and timeouts", async () => {
    assert.deepEqual(
      [(await pushFeed(env, world({ feedStatus: 404 }).fetcher)).outcome, (await pushFeed(env, world({ feedStatus: 404 }).fetcher)).status],
      ["fetch_failed", 404],
    );
    assert.equal((await pushFeed(env, world({ status: 401 }).fetcher)).outcome, "push_failed");
    assert.equal((await pushFeed(env, world({ fail: "source" }).fetcher)).outcome, "fetch_failed");
    assert.equal((await pushFeed(env, world({ fail: "source", error: "TimeoutError" }).fetcher)).outcome, "timeout");
    assert.equal((await pushFeed(env, world({ fail: "target" }).fetcher)).outcome, "push_failed");
    assert.equal((await pushFeed(env, world({ fail: "target", error: "AbortError" }).fetcher)).outcome, "timeout");
  });

  it("gives up on a real request that outlives the timeout", async () => {
    const slow = (async (_: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => init?.signal?.addEventListener("abort", () => reject(init.signal!.reason)))) as typeof fetch;
    const result = await pushFeed({ ...env, ACP_FEED_TIMEOUT_MS: "20" }, slow);
    assert.equal(result.outcome, "timeout");
  });

  it("never sends a feed over the cap, declared or read, nor an empty one", async () => {
    const small = { ...env, ACP_FEED_MAX_BYTES: "10" };
    const declared = world({ length: "999999" });
    assert.equal((await pushFeed(small, declared.fetcher)).outcome, "too_large");
    const read = world();
    assert.equal((await pushFeed(small, read.fetcher)).outcome, "too_large");
    assert.equal(read.sent.length, 0);
    assert.equal((await pushFeed(env, world({ feed: "" }).fetcher)).outcome, "empty_feed");
    assert.equal((await pushFeed(env, world({ feed: null }).fetcher)).outcome, "empty_feed");
    // Nonsense or oversized limits fall back to the defaults or the ceiling.
    assert.equal((await pushFeed({ ...env, ACP_FEED_MAX_BYTES: "lots", ACP_FEED_TIMEOUT_MS: "-1" }, world().fetcher)).outcome, "pushed");
    assert.equal((await pushFeed({ ...env, ACP_FEED_MAX_BYTES: String(10 ** 12) }, world().fetcher)).outcome, "pushed");
  });

  it("logs hosts and numbers, never the credential, paths or query strings", async () => {
    const line = logLine(env, await pushFeed(env, world().fetcher));
    const parsed = JSON.parse(line);
    assert.deepEqual([parsed.event, parsed.outcome, parsed.source, parsed.target], ["acp_feed_push", "pushed", "example.com", "agent.example.com"]);
    for (const secret of [TOKEN, "sig=private", "/feeds/upload"]) assert.ok(!line.includes(secret));
    assert.equal(JSON.parse(logLine({}, { outcome: "not_configured", ms: 0 })).source, null);
  });
});

describe("the scheduled handler", () => {
  it("logs one line and succeeds when the push went through", async () => {
    globalThis.fetch = world().fetcher;
    const lines: string[] = [];
    const log = console.log;
    console.log = (s: string) => lines.push(s);
    try {
      await worker.scheduled({}, env);
    } finally {
      console.log = log;
    }
    assert.equal(lines.length, 1);
    assert.ok(!lines[0]!.includes(TOKEN));
  });

  it("throws, so the Cron Trigger shows a failure, when it did not", async () => {
    globalThis.fetch = world({ status: 500 }).fetcher;
    const log = console.log;
    console.log = () => {};
    try {
      await assert.rejects(worker.scheduled({}, env), /acp feed push: push_failed/);
    } finally {
      console.log = log;
    }
  });
});
