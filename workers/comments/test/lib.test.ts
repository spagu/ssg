// The shared helpers of _lib.ts and _schema.ts that the endpoint suites do not
// reach: Turnstile, the spam check, the URL rule, the auto-close window, the
// throttled purge, and the schema's retry after a failed first run (GO-136
// brought these files to full coverage).
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  closeWindowMs, heuristicSpam, isClosed, isSpam, normaliseURL, purgeExpired, sha256hex, turnstileSecret, verifyTurnstile,
} from "../functions/api/comments/_lib.ts";
import { ensureSchema } from "../functions/api/comments/_schema.ts";
import { seed, sqliteD1 } from "./harness.ts";

type Env = Parameters<typeof isSpam>[0];
const env = (e: Record<string, unknown>) => e as unknown as Env;

// withFetch swaps the global fetch for one test body and records what was asked.
async function withFetch(answer: (url: string, init: RequestInit) => Response | Promise<Response>, body: (seen: { url: string; init: RequestInit }[]) => Promise<void>) {
  const real = globalThis.fetch;
  const seen: { url: string; init: RequestInit }[] = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => (seen.push({ url: String(url), init }), answer(String(url), init))) as typeof fetch;
  try {
    await body(seen);
  } finally {
    globalThis.fetch = real;
  }
}

test("schema: a failed first run is not cached — the next request retries", async () => {
  const broken = { prepare: (sql: string) => ({ sql }), batch: async () => Promise.reject(new Error("D1 down")) };
  await assert.rejects(ensureSchema(env({ COMMENTS_DB: broken })), /D1 down/);
  // A database from before threaded replies and GO-136: the retry adds parent_id and comment_events.
  const db = sqliteD1();
  db.sqlite.exec("DROP TABLE comment_events; DROP TABLE comments; CREATE TABLE comments (id TEXT PRIMARY KEY, url TEXT NOT NULL, author TEXT NOT NULL, email TEXT, body TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', created_at TEXT NOT NULL, ip_hash TEXT, user_agent TEXT, avatar_hash TEXT)");
  await ensureSchema(env({ COMMENTS_DB: db }));
  assert.ok(db.sqlite.prepare("SELECT name FROM sqlite_master WHERE name = 'comment_events'").get());
  assert.ok(db.sqlite.prepare("SELECT parent_id FROM comments").all());
});

test("turnstile: the prefixed secret wins over the shared one", () => {
  assert.equal(turnstileSecret(env({ COMMENTS_TURNSTILE_SECRET: "own", TURNSTILE_SECRET: "shared" })), "own");
  assert.equal(turnstileSecret(env({ TURNSTILE_SECRET: "shared" })), "shared");
  assert.equal(turnstileSecret(env({})), undefined);
});

test("turnstile: siteverify gets the secret, the token and the IP when known", async () => {
  await withFetch(
    (_url, init) => Response.json({ success: (init.body as FormData).get("response") === "good" }),
    async (seen) => {
      assert.equal(await verifyTurnstile("s", "good", "203.0.113.1"), true);
      assert.equal(await verifyTurnstile("s", "bad", null), false);
      assert.equal(seen[0].url, "https://challenges.cloudflare.com/turnstile/v0/siteverify");
      assert.equal((seen[0].init.body as FormData).get("remoteip"), "203.0.113.1");
      assert.equal((seen[1].init.body as FormData).get("remoteip"), null);
    },
  );
});

test("spam: the heuristic scores links, keywords, BBCode and very short bodies", () => {
  assert.equal(heuristicSpam("Ada", "A thoughtful comment."), 0);
  assert.equal(heuristicSpam("Ada", "http://a http://b"), 0.5);
  assert.equal(heuristicSpam("Ada", "http://a http://b http://c http://d"), 0.8);
  assert.equal(heuristicSpam("casino", "Nice post"), 0.6);
  assert.equal(heuristicSpam("Ada", "[url=x]hi[/url] and more"), 0.5);
  assert.equal(heuristicSpam("Ada", "ok"), 0.3);
  assert.equal(heuristicSpam("viagra", "[url=x] http://a http://b"), 1);
});

test("spam: Akismet decides when configured; a broken answer or call falls back to the heuristic", async () => {
  const akismet = env({ COMMENTS_AKISMET_KEY: "k", COMMENTS_AKISMET_URL: "https://akismet.example.com/check" });
  const verdicts = ["true", "false", "invalid", "THROW"];
  let i = 0;
  await withFetch(
    () => {
      const v = verdicts[i++];
      if (v === "THROW") throw new Error("offline");
      return new Response(` ${v}\n`);
    },
    async (seen) => {
      assert.equal(await isSpam(akismet, "Ada", "ada@example.com", "Fine", "203.0.113.1", "UA"), true);
      assert.equal(await isSpam(akismet, "casino", "", "viagra http://a http://b", null, "UA"), false);
      assert.equal(await isSpam(akismet, "casino", "", "viagra http://a http://b", null, "UA"), true, "invalid → heuristic");
      assert.equal(await isSpam(akismet, "Ada", "", "A fine comment", null, "UA"), false, "throws → heuristic");
      const form = new URLSearchParams(String(seen[0].init.body));
      assert.equal(form.get("user_ip"), "203.0.113.1");
      assert.equal(new URLSearchParams(String(seen[1].init.body)).get("user_ip"), "");
    },
  );
  assert.equal(await isSpam(env({}), "Ada", "", "A fine comment", null, "UA"), false);
});

test("urls: only same-origin paths, query and fragment cut", () => {
  assert.equal(normaliseURL("/blog/post/?utm=x#c1"), "/blog/post/");
  for (const bad of [42, "https://example.com/", "//evil.example.com", "/\\evil.example.com", "blog/"]) assert.equal(normaliseURL(bad), null);
});

test("auto-close: the window, and which date anchors it", () => {
  assert.equal(closeWindowMs(env({})), 0);
  assert.equal(closeWindowMs(env({ COMMENTS_CLOSE_AFTER_DAYS: "x" })), 0);
  assert.equal(closeWindowMs(env({ COMMENTS_CLOSE_AFTER_DAYS: "2" })), 2 * 86400000);
  const day = 86400000;
  const ago = (d: number) => new Date(Date.now() - d * day).toISOString();
  assert.equal(isClosed(0, ago(100), null), false);
  assert.equal(isClosed(day, ago(2), null), true);
  assert.equal(isClosed(day, ago(0.5), ago(100)), false, "the last comment governs");
  assert.equal(isClosed(day, null, ago(2)), true, "an empty thread: the publish date");
  assert.equal(isClosed(day, null, null), false);
  assert.equal(isClosed(day, "not a date", null), false);
});

test("purge: off without retention; lazily at most hourly per isolate; forced on demand", async () => {
  const db = sqliteD1();
  seed(db, [
    { id: "s1", status: "spam", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "s2", status: "spam", created_at: "2000-01-01T00:00:00.000Z" },
  ]);
  assert.equal(await purgeExpired(env({ COMMENTS_DB: db })), 0);
  assert.equal(await purgeExpired(env({ COMMENTS_DB: db, COMMENTS_RETENTION_DAYS: "x" })), 0);
  const kept = env({ COMMENTS_DB: db, COMMENTS_RETENTION_DAYS: "30" });
  assert.equal(await purgeExpired(kept), 2);
  seed(db, [{ id: "s3", status: "spam", created_at: "2000-01-01T00:00:00.000Z" }]);
  assert.equal(await purgeExpired(kept), 0, "throttled");
  assert.equal(await purgeExpired(kept, true), 1);
  const silent = { prepare: () => ({ bind: () => ({ run: async () => ({ meta: {} }) }) }) };
  assert.equal(await purgeExpired(env({ COMMENTS_DB: silent, COMMENTS_RETENTION_DAYS: "30" }), true), 0, "no change count → 0");
});

test("sha256hex: lower-case hex", async () => {
  assert.equal(await sha256hex("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
});
