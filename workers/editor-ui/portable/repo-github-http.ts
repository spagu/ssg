// ssg editor-ui — portable: the one place that talks HTTP to GitHub's REST API. Every request goes to the
// API host fixed by repo-config.ts, never follows a redirect (a 3xx is an error, so no request can reach
// another host with the token), and turns any failure into a RepoError with our own code. The token is
// only ever placed in the Authorization header: no message, detail or log line built here contains it, nor
// any part of a provider response body.
import { RepoError, type RepoErrorCode } from "./repo-provider";
import type { RepoConfig } from "./repo-config";

export type FetchFn = (url: string, init: RequestInit) => Promise<Response>;
export interface HttpOptions {
  fetch?: FetchFn;
  now?: () => number; // milliseconds; rate-limit reset times are reported relative to it
  timeoutMs?: number; // per request; default 15 s
  maxSubrequests?: number; // refuse the request that would exceed it (Workers Free allows 50 per invocation)
}
export interface ApiRequest { method?: "GET" | "POST" | "PATCH"; path: string; body?: unknown; accept?: string }
// Stale: the ref moved under us (a non-fast-forward PATCH). Internal — commit() retries or reports conflict.
export class StaleRef extends Error { constructor() { super("stale_ref"); this.name = "StaleRef"; } }

const JSON_ACCEPT = "application/vnd.github+json";
export const RAW_ACCEPT = "application/vnd.github.raw+json";

// githubHttp creates a client bound to one repository; `used` counts subrequests sent so far.
export function githubHttp(config: RepoConfig, opts: HttpOptions = {}) {
  const doFetch: FetchFn = opts.fetch ?? ((url, init) => fetch(url, init));
  const now = opts.now ?? Date.now;
  const timeoutMs = opts.timeoutMs ?? 15_000;
  const max = opts.maxSubrequests ?? Infinity;
  const repoBase = `${config.apiBase}/repos/${encodeURIComponent(config.owner)}/${encodeURIComponent(config.repo)}`;
  const state = { used: 0 };

  async function send(req: ApiRequest): Promise<Response> {
    if (state.used >= max) throw new RepoError("provider_error", { detail: "subrequest budget exhausted" });
    state.used++;
    const headers: Record<string, string> = {
      accept: req.accept ?? JSON_ACCEPT,
      authorization: `Bearer ${config.token}`,
      "x-github-api-version": "2022-11-28",
      "user-agent": "ssg-editor-ui",
    };
    if (req.body !== undefined) headers["content-type"] = "application/json";
    let res: Response;
    try {
      res = await doFetch(repoBase + req.path, {
        method: req.method ?? "GET", headers, redirect: "manual",
        body: req.body === undefined ? undefined : JSON.stringify(req.body),
        signal: AbortSignal.timeout(timeoutMs),
      });
    } catch {
      // The thrown error may quote the request; it is dropped, only our code leaves.
      throw new RepoError("provider_error", { detail: "request failed" });
    }
    if (res.status >= 200 && res.status < 300) return res;
    throw await toError(res, req, now());
  }

  // json sends a request and parses the JSON answer (any shape; callers check what they use).
  async function json(req: ApiRequest): Promise<any> {
    const res = await send(req);
    try { return await res.json(); } catch { throw new RepoError("provider_error", { detail: "bad response" }); }
  }

  // bytes sends a request and returns the raw body (blob contents).
  async function bytes(req: ApiRequest): Promise<Uint8Array> {
    const res = await send(req);
    return new Uint8Array(await res.arrayBuffer());
  }

  return { json, bytes, send, state };
}

// providerMessage reads the error body's `message` for classification only; it never leaves this file.
async function providerMessage(res: Response): Promise<string> {
  try {
    const b = (await res.json()) as { message?: unknown };
    return typeof b?.message === "string" ? b.message : "";
  } catch { return ""; }
}

// toError maps a non-2xx answer to our codes (GitHub REST docs, verified 2026-11-14 — see the README).
export async function toError(res: Response, req: ApiRequest, nowMs: number): Promise<Error> {
  const s = res.status;
  if (s >= 300 && s < 400) return new RepoError("provider_error", { detail: "redirect refused" });
  const msg = await providerMessage(res);
  const remaining = res.headers.get("x-ratelimit-remaining");
  const retryAfter = res.headers.get("retry-after");
  if (s === 429 || (s === 403 && (remaining === "0" || retryAfter !== null || /rate limit/i.test(msg)))) {
    return new RepoError("rate_limited", { retryAt: retryTime(res, nowMs) });
  }
  if (req.method === "PATCH" && (s === 422 || s === 409) && /fast[ -]?forward/i.test(msg)) return new StaleRef();
  if (s === 422 || s === 409 || s === 403) {
    if (/protected branch|rule violation|pull request|required status/i.test(msg)) return new RepoError("branch_protected");
    if (/too large|too big/i.test(msg)) return new RepoError("too_large");
  }
  const code: RepoErrorCode = s === 401 || s === 403 ? "token_rejected"
    : s === 404 || (s === 409 && /empty/i.test(msg)) ? "not_found"
    : s === 413 ? "too_large"
    : "provider_error";
  return new RepoError(code);
}

// retryTime: retry-after (seconds) wins, then x-ratelimit-reset (epoch seconds), else one minute — the
// waits GitHub's rate-limit documentation prescribes.
function retryTime(res: Response, nowMs: number): string {
  const after = Number(res.headers.get("retry-after"));
  if (res.headers.has("retry-after") && Number.isFinite(after) && after >= 0) return new Date(nowMs + after * 1000).toISOString();
  const reset = Number(res.headers.get("x-ratelimit-reset"));
  if (res.headers.has("x-ratelimit-reset") && Number.isFinite(reset) && reset > 0) return new Date(reset * 1000).toISOString();
  return new Date(nowMs + 60_000).toISOString();
}
