// Shared helpers for the republish-trigger worker. A leading underscore keeps
// this file out of the Pages route table — it is imported, never served.
//
// The worker turns one authenticated request into a CI dispatch on GitHub,
// GitLab or Gitea, so a webhook, a cron, or a button can rebuild the site.

export interface Env {
  // Caller auth: the shared secret sent TO this endpoint (never the CI token).
  REPUBLISH_KEY?: string;

  // Provider + credential.
  REPUBLISH_PROVIDER?: string; // "github" | "gitlab" | "gitea"
  REPUBLISH_TOKEN?: string; // GitHub/Gitea API token, or a GitLab pipeline trigger token

  // Targets (which of these matters depends on the provider).
  REPUBLISH_REPO?: string; // github/gitea: "owner/repo"
  REPUBLISH_PROJECT_ID?: string; // gitlab: numeric id or URL-encoded "group/project"
  REPUBLISH_REF?: string; // branch/tag to build; default "main"
  REPUBLISH_WORKFLOW?: string; // github/gitea: workflow file or id → workflow_dispatch
  REPUBLISH_EVENT_TYPE?: string; // github (no workflow): repository_dispatch type; default "republish"
  REPUBLISH_API_BASE?: string; // override host: GH Enterprise, self-hosted GitLab/Gitea (required for gitea)

  // Guardrails.
  REPUBLISH_ALLOW_GET?: string; // "true" lets a GET trigger (for webhooks that can't POST)
  REPUBLISH_MIN_INTERVAL_SEC?: string; // debounce window; needs REPUBLISH_KV to take effect
  REPUBLISH_KV?: KVNamespace; // optional: stores the last-trigger time for the debounce
}

export function json(data: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store", ...headers },
  });
}

// Constant-time compare so the key check does not leak length or content by timing.
export function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

// The key the caller presented: Authorization: Bearer …, X-Republish-Key, or
// ?key= (last resort for webhooks that can only send a URL — it leaks into logs,
// so a header is preferred).
export function presentedKey(request: Request): string {
  const auth = request.headers.get("authorization") || "";
  if (auth.startsWith("Bearer ")) return auth.slice(7).trim();
  const header = request.headers.get("x-republish-key");
  if (header) return header.trim();
  const q = new URL(request.url).searchParams.get("key");
  return q ? q.trim() : "";
}

// requireKey gates every trigger. 503 when the worker is not configured (so a
// half-set-up deploy hook fails loud), 401 on a missing/wrong key.
export function requireKey(request: Request, env: Env): Response | null {
  if (!env.REPUBLISH_KEY || !env.REPUBLISH_TOKEN || !env.REPUBLISH_PROVIDER) {
    return json({ error: "republish not configured" }, 503);
  }
  const got = presentedKey(request);
  if (got && timingSafeEqual(got, env.REPUBLISH_KEY)) return null;
  return json({ error: "unauthorized" }, 401);
}

export function allowGet(env: Env): boolean {
  return (env.REPUBLISH_ALLOW_GET || "").toLowerCase() === "true";
}

// debounce collapses a burst of triggers into one CI run. A no-op unless both a
// KV namespace is bound and REPUBLISH_MIN_INTERVAL_SEC is set. It writes the
// timestamp optimistically before dispatch, so two near-simultaneous calls
// don't both fire.
export async function debounce(env: Env): Promise<Response | null> {
  const kv = env.REPUBLISH_KV;
  const secs = parseInt(env.REPUBLISH_MIN_INTERVAL_SEC || "0", 10);
  if (!kv || !Number.isFinite(secs) || secs <= 0) return null;

  const now = Date.now();
  const prev = await kv.get("republish:last");
  if (prev) {
    const last = parseInt(prev, 10);
    if (Number.isFinite(last) && now - last < secs * 1000) {
      const retry = Math.ceil((secs * 1000 - (now - last)) / 1000);
      return json({ error: "too soon", retry_after_seconds: retry }, 429, { "retry-after": String(retry) });
    }
  }
  await kv.put("republish:last", String(now), { expirationTtl: Math.max(secs, 60) });
  return null;
}

export interface DispatchResult {
  ok: boolean;
  status: number;
  provider: string;
  detail?: string;
}

// A short, safe slice of an upstream error body for the caller to debug with —
// GitHub/GitLab/Gitea error payloads describe the problem and never echo our
// token. Bounded so a hostile/huge body can't be reflected wholesale.
async function errorDetail(res: Response): Promise<string> {
  try {
    return (await res.text()).slice(0, 300);
  } catch {
    return "";
  }
}

// dispatch fires the provider's build. Each branch validates its own required
// config and returns a 503-style result (ok:false, status:503) when something
// is missing, so a misconfiguration is reported, not silently swallowed.
export async function dispatch(env: Env): Promise<DispatchResult> {
  const provider = (env.REPUBLISH_PROVIDER || "").toLowerCase();
  const token = env.REPUBLISH_TOKEN as string;
  const ref = env.REPUBLISH_REF || "main";

  if (provider === "github") return dispatchGitHub(env, token, ref);
  if (provider === "gitlab") return dispatchGitLab(env, token, ref);
  if (provider === "gitea") return dispatchGitea(env, token, ref);
  return { ok: false, status: 503, provider, detail: `unknown provider "${provider}" (use github, gitlab or gitea)` };
}

async function dispatchGitHub(env: Env, token: string, ref: string): Promise<DispatchResult> {
  const repo = env.REPUBLISH_REPO;
  if (!repo) return { ok: false, status: 503, provider: "github", detail: "REPUBLISH_REPO (owner/repo) is required" };
  const api = env.REPUBLISH_API_BASE || "https://api.github.com";
  const headers: Record<string, string> = {
    authorization: `Bearer ${token}`,
    accept: "application/vnd.github+json",
    "x-github-api-version": "2022-11-28",
    "user-agent": "ssg-republish-trigger",
    "content-type": "application/json",
  };

  let url: string;
  let body: string;
  if (env.REPUBLISH_WORKFLOW) {
    // workflow_dispatch: run one named workflow. Succeeds with 204.
    url = `${api}/repos/${repo}/actions/workflows/${encodeURIComponent(env.REPUBLISH_WORKFLOW)}/dispatches`;
    body = JSON.stringify({ ref });
  } else {
    // repository_dispatch: fire a custom event any workflow can key off. 204.
    url = `${api}/repos/${repo}/dispatches`;
    body = JSON.stringify({ event_type: env.REPUBLISH_EVENT_TYPE || "republish", client_payload: { source: "republish-trigger" } });
  }
  const res = await fetch(url, { method: "POST", headers, body });
  return { ok: res.status === 204, status: res.status, provider: "github", detail: res.status === 204 ? undefined : await errorDetail(res) };
}

async function dispatchGitLab(env: Env, token: string, ref: string): Promise<DispatchResult> {
  const project = env.REPUBLISH_PROJECT_ID;
  if (!project) return { ok: false, status: 503, provider: "gitlab", detail: "REPUBLISH_PROJECT_ID is required" };
  const api = env.REPUBLISH_API_BASE || "https://gitlab.com/api/v4";
  // The GitLab pipeline trigger endpoint takes the trigger token in the body,
  // not an Authorization header. Succeeds with 201.
  const form = new URLSearchParams({ token, ref });
  const url = `${api}/projects/${encodeURIComponent(project)}/trigger/pipeline`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body: form,
  });
  return { ok: res.status === 201, status: res.status, provider: "gitlab", detail: res.status === 201 ? undefined : await errorDetail(res) };
}

async function dispatchGitea(env: Env, token: string, ref: string): Promise<DispatchResult> {
  const repo = env.REPUBLISH_REPO;
  const api = env.REPUBLISH_API_BASE; // Gitea is self-hosted, so there is no default host.
  if (!repo) return { ok: false, status: 503, provider: "gitea", detail: "REPUBLISH_REPO (owner/repo) is required" };
  if (!api) return { ok: false, status: 503, provider: "gitea", detail: "REPUBLISH_API_BASE (e.g. https://git.example.com/api/v1) is required" };
  if (!env.REPUBLISH_WORKFLOW) return { ok: false, status: 503, provider: "gitea", detail: "REPUBLISH_WORKFLOW (workflow file) is required" };
  const url = `${api}/repos/${repo}/actions/workflows/${encodeURIComponent(env.REPUBLISH_WORKFLOW)}/dispatches`;
  const res = await fetch(url, {
    method: "POST",
    headers: {
      authorization: `token ${token}`,
      "content-type": "application/json",
      "user-agent": "ssg-republish-trigger",
    },
    body: JSON.stringify({ ref }),
  });
  return { ok: res.status === 204, status: res.status, provider: "gitea", detail: res.status === 204 ? undefined : await errorDetail(res) };
}
