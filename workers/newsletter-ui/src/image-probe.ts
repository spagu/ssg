// Whether a mail image answers a mail client's image proxy (#446): the probe
// half of the pre-send image check (src/image-check.ts). It only reads
// response headers — never the body, which is cancelled unread — and stores
// nothing about the attempt.
//
// The request. Webmail does not load images from the reader's device: an image
// proxy fetches them, identified by a User-Agent token — "GoogleImageProxy",
// "YahooMailProxy". Bot protection that answers those with 403 (or a challenge
// page) is the usual reason a mail shows empty boxes. The probe sends one GET
// per token in PROBE_AGENTS, with a User-Agent that says what it is
// (ssg-newsletter-image-check) and carries the token, so a rule keyed on the
// token is exercised. It does not pretend to be a full browser string, and it
// cannot prove the real proxies get through: a rule keyed on their IP ranges
// or verified-bot status sees a Cloudflare Worker, not Google or Yahoo.
//
// Bounds — a Worker on Workers Free has 50 subrequests per invocation:
//   - PROBE_AGENTS: 2 probes per image, the second only when the first passed;
//   - MAX_REDIRECTS: 1, followed by hand, https: only (proxies follow
//     redirects, so one hop is allowed; a chain is reported, not walked);
//   - PROBE_TIMEOUT_MS per fetch;
//   - the check probes at most MAX_PROBED_IMAGES distinct images, PROBE_POOL
//     at a time: at most 8 × 2 × 2 = 32 fetches, the rest of the budget left
//     for D1 and R2.

export const PROBE_AGENTS = [
  { token: "GoogleImageProxy", userAgent: "ssg-newsletter-image-check/1.0 (mail image proxy test; GoogleImageProxy)" },
  { token: "YahooMailProxy", userAgent: "ssg-newsletter-image-check/1.0 (mail image proxy test; YahooMailProxy)" },
] as const;
export const MAX_REDIRECTS = 1;
export const PROBE_TIMEOUT_MS = 5000;
const ACCEPT = "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8";

export type ProbeError = "timeout" | "fetch_failed" | "not_https" | "too_many_redirects" | "not_image";

// Probe: what one image answered. `status` is the last HTTP status seen (null
// without an answer); `agent` names the token whose probe failed.
export interface Probe {
  via: "fetch" | "store";
  ok: boolean;
  status: number | null;
  contentType: string | null;
  error: ProbeError | null;
  agent: string | null;
}

export type Fetcher = (url: string, init: RequestInit) => Promise<Response>;

const isImage = (type: string | null): boolean => /^image\//i.test((type ?? "").trim());

// probeOnce GETs `url` as one agent, following at most MAX_REDIRECTS.
async function probeOnce(url: string, agent: (typeof PROBE_AGENTS)[number], fetcher: Fetcher, timeoutMs: number): Promise<Probe> {
  const result = (status: number | null, contentType: string | null, error: ProbeError | null): Probe => {
    const ok = error === null && status === 200 && isImage(contentType);
    return { via: "fetch", ok, status, contentType, error: error ?? (status === 200 && !ok ? "not_image" : null), agent: ok ? null : agent.token };
  };
  let at = new URL(url);
  for (let hop = 0; ; hop++) {
    // The sanitizer keeps https: images only; asserted again before any fetch.
    if (at.protocol !== "https:") return result(null, null, "not_https");
    let res: Response;
    try {
      res = await fetcher(at.href, {
        method: "GET",
        redirect: "manual",
        headers: { "user-agent": agent.userAgent, accept: ACCEPT },
        signal: AbortSignal.timeout(timeoutMs),
      });
    } catch (e) {
      const timedOut = e instanceof Error && (e.name === "TimeoutError" || e.name === "AbortError");
      return result(null, null, timedOut ? "timeout" : "fetch_failed");
    }
    await res.body?.cancel().catch(() => {}); // headers are the answer; never read the image
    const location = res.headers.get("location");
    if (res.status < 300 || res.status >= 400 || !location) return result(res.status, res.headers.get("content-type"), null);
    if (hop >= MAX_REDIRECTS) return result(res.status, null, "too_many_redirects");
    try {
      at = new URL(location, at);
    } catch {
      return result(res.status, null, "fetch_failed");
    }
  }
}

// probeFetch probes `url` as each agent in turn, stopping at the first failure.
export async function probeFetch(url: string, fetcher: Fetcher = fetch, timeoutMs = PROBE_TIMEOUT_MS): Promise<Probe> {
  let last: Probe | null = null;
  for (const agent of PROBE_AGENTS) {
    last = await probeOnce(url, agent, fetcher, timeoutMs);
    if (!last.ok) return last;
  }
  return last as Probe;
}

// probeStore judges an image this worker serves (/hooks/a/:key) from the R2
// object instead of fetching the worker's own route: it proves the bytes are
// there with an image type, not that the zone's bot rules let a proxy through.
export async function probeStore(bucket: R2Bucket, key: string): Promise<Probe> {
  try {
    const obj = await bucket.head(key);
    const type = obj?.httpMetadata?.contentType ?? null;
    const ok = !!obj && isImage(type);
    return { via: "store", ok, status: obj ? 200 : 404, contentType: type, error: obj && !ok ? "not_image" : null, agent: null };
  } catch {
    return { via: "store", ok: false, status: null, contentType: null, error: "fetch_failed", agent: null };
  }
}

// pool runs `work` over `items`, at most `size` at a time, keeping order.
export async function pool<T, R>(items: T[], size: number, work: (item: T) => Promise<R>): Promise<R[]> {
  const out: R[] = new Array(items.length);
  let next = 0;
  const lane = async () => {
    while (next < items.length) {
      const i = next++;
      out[i] = await work(items[i]);
    }
  };
  await Promise.all(Array.from({ length: Math.min(size, items.length) }, lane));
  return out;
}
