// Cloudflare Pages Function: republish trigger.
//   POST /api/republish   → authenticate, then dispatch a CI build
//   GET  /api/republish   → same, only when REPUBLISH_ALLOW_GET=true
//
// One authenticated request becomes a build on GitHub, GitLab or Gitea, so a
// CMS webhook, a scheduled cron, or a "Republish" button can rebuild the site
// without anyone touching the repo. The caller proves itself with REPUBLISH_KEY;
// the provider token stays server-side and is never exposed.

import { Env, json, requireKey, allowGet, debounce, dispatch } from "./_lib";

async function handle(request: Request, env: Env): Promise<Response> {
  const denied = requireKey(request, env);
  if (denied) return denied;

  const throttled = await debounce(env);
  if (throttled) return throttled;

  const result = await dispatch(env);
  if (!result.ok) {
    // 503 from a validation branch is a config problem; anything else is the
    // provider rejecting the call. Surface the status either way.
    const status = result.status === 503 ? 503 : 502;
    return json({ error: "dispatch failed", provider: result.provider, upstream_status: result.status, detail: result.detail }, status);
  }
  return json({ ok: true, provider: result.provider, ref: env.REPUBLISH_REF || "main" }, 202);
}

export const onRequestPost: PagesFunction<Env> = ({ request, env }) => handle(request, env);

export const onRequestGet: PagesFunction<Env> = ({ request, env }) => {
  if (!allowGet(env)) {
    return json({ error: "use POST (set REPUBLISH_ALLOW_GET=true to allow GET triggering)" }, 405);
  }
  return handle(request, env);
};
