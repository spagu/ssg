// acp-feed-push: on a Cron Trigger, fetch the site's ACP product feed and send
// it to the agent platform (GO-164).
//
// The feed is built by ssg (`feeds:` with `format: acp`) and published with
// the site. This Worker only carries it across: it never reads the shop's
// database and holds one secret, the platform credential (ACP_FEED_TOKEN).
//
// Guard rails, because it runs unattended:
//   - both URLs must be https;
//   - the feed is read up to ACP_FEED_MAX_BYTES and refused past it — the
//     declared Content-Length first, then the bytes actually read;
//   - an empty feed is never sent: to a platform that replaces the catalogue
//     with each upload, an empty file is "delete everything";
//   - each request is abandoned after ACP_FEED_TIMEOUT_MS;
//   - one JSON log line per run, naming hosts but never paths, query strings
//     or the credential; a run that did not push throws, so the Cron Trigger
//     shows it as failed in the dashboard.

export interface FeedEnv {
  ACP_FEED_SOURCE_URL?: string;
  ACP_FEED_TARGET_URL?: string;
  ACP_FEED_TOKEN?: string;
  ACP_FEED_METHOD?: string;
  ACP_FEED_MAX_BYTES?: string;
  ACP_FEED_TIMEOUT_MS?: string;
}

export type Outcome =
  | "pushed"
  | "not_configured"
  | "fetch_failed"
  | "too_large"
  | "empty_feed"
  | "push_failed"
  | "timeout";

export interface PushResult {
  outcome: Outcome;
  /** HTTP status of the step that decided the outcome, when there was one. */
  status?: number;
  bytes?: number;
  ms: number;
}

const DEFAULT_MAX_BYTES = 20 * 1024 * 1024;
const LIMIT_MAX_BYTES = 100 * 1024 * 1024;
const DEFAULT_TIMEOUT_MS = 30_000;
const METHODS = ["POST", "PUT", "PATCH"];

/** A positive whole number from a var, clamped; the default otherwise. */
function bounded(value: string | undefined, fallback: number, max: number): number {
  const n = Number.parseInt(value ?? "", 10);
  return Number.isFinite(n) && n > 0 ? Math.min(n, max) : fallback;
}

function httpsUrl(value: string | undefined): URL | null {
  try {
    const url = new URL(value ?? "");
    return url.protocol === "https:" ? url : null;
  } catch {
    return null;
  }
}

/** The body, or null when it is larger than `max`. */
async function readCapped(res: Response, max: number): Promise<Uint8Array | null> {
  if (Number(res.headers.get("content-length") ?? "0") > max) return null;
  const chunks: Uint8Array[] = [];
  let total = 0;
  if (res.body) {
    const reader = res.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > max) {
        await reader.cancel();
        return null;
      }
      chunks.push(value);
    }
  }
  const out = new Uint8Array(total);
  let at = 0;
  for (const chunk of chunks) {
    out.set(chunk, at);
    at += chunk.byteLength;
  }
  return out;
}

const timedOut = (e: unknown): boolean => e instanceof Error && (e.name === "TimeoutError" || e.name === "AbortError");

/** One run: fetch, check, send. Never throws; the outcome says what happened. */
export async function pushFeed(env: FeedEnv, fetcher: typeof fetch = fetch): Promise<PushResult> {
  const started = Date.now();
  const done = (outcome: Outcome, extra: { status?: number; bytes?: number } = {}): PushResult => ({
    outcome,
    ...extra,
    ms: Date.now() - started,
  });

  const source = httpsUrl(env.ACP_FEED_SOURCE_URL);
  const target = httpsUrl(env.ACP_FEED_TARGET_URL);
  const method = (env.ACP_FEED_METHOD || "POST").toUpperCase();
  if (!source || !target || !env.ACP_FEED_TOKEN || !METHODS.includes(method)) return done("not_configured");
  const max = bounded(env.ACP_FEED_MAX_BYTES, DEFAULT_MAX_BYTES, LIMIT_MAX_BYTES);
  const timeout = bounded(env.ACP_FEED_TIMEOUT_MS, DEFAULT_TIMEOUT_MS, 15 * 60_000);

  let feed: Uint8Array | null;
  let contentType: string;
  try {
    const res = await fetcher(source.toString(), { signal: AbortSignal.timeout(timeout), headers: { accept: "*/*" } });
    if (!res.ok) return done("fetch_failed", { status: res.status });
    contentType = res.headers.get("content-type") || "application/x-ndjson";
    feed = await readCapped(res, max);
  } catch (e) {
    return done(timedOut(e) ? "timeout" : "fetch_failed");
  }
  if (!feed) return done("too_large");
  if (feed.byteLength === 0) return done("empty_feed");

  try {
    const res = await fetcher(target.toString(), {
      method,
      headers: { authorization: `Bearer ${env.ACP_FEED_TOKEN}`, "content-type": contentType },
      body: feed,
      signal: AbortSignal.timeout(timeout),
    });
    return done(res.ok ? "pushed" : "push_failed", { status: res.status, bytes: feed.byteLength });
  } catch (e) {
    return done(timedOut(e) ? "timeout" : "push_failed", { bytes: feed.byteLength });
  }
}

/** The log line: outcome and numbers, the hosts involved, nothing secret. */
export function logLine(env: FeedEnv, result: PushResult): string {
  return JSON.stringify({
    event: "acp_feed_push",
    ...result,
    source: httpsUrl(env.ACP_FEED_SOURCE_URL)?.host ?? null,
    target: httpsUrl(env.ACP_FEED_TARGET_URL)?.host ?? null,
  });
}

export default {
  async scheduled(_controller: unknown, env: FeedEnv): Promise<void> {
    const result = await pushFeed(env);
    console.log(logLine(env, result));
    if (result.outcome !== "pushed") throw new Error(`acp feed push: ${result.outcome}`);
  },
};
